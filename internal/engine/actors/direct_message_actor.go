package actors

import (
	stdctx "context" // Alias for standard context to avoid confusion with actor.Context
	"encoding/json"  // Add for marshalling
	"gator-swamp/internal/database"
	"gator-swamp/internal/models"
	"gator-swamp/internal/websocket" // Import websocket package
	"log"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/google/uuid"
)

// Message types for DirectMessageActor
type (
	SendDirectMessageMsg struct {
		FromID  uuid.UUID `json:"fromId"`
		ToID    uuid.UUID `json:"toId"`
		Content string    `json:"content"`
	}

	GetUserMessagesMsg struct {
		UserID uuid.UUID `json:"userId"`
	}

	GetConversationMsg struct {
		UserID1 uuid.UUID `json:"userId1"`
		UserID2 uuid.UUID `json:"userId2"`
	}

	MarkMessageReadMsg struct {
		MessageID uuid.UUID `json:"messageId"`
		UserID    uuid.UUID `json:"userId"`
	}

	DeleteMessageMsg struct {
		MessageID uuid.UUID `json:"messageId"`
		UserID    uuid.UUID `json:"userId"`
	}

	// MessageStatusUpdate is sent via WebSocket when a message's read status changes
	MessageStatusUpdate struct {
		Type      string    `json:"type"` // e.g., "messageRead"
		MessageID uuid.UUID `json:"messageId"`
		ReadAt    time.Time `json:"readAt"`
	}
)

// DirectMessageActor manages direct messaging operations
type DirectMessageActor struct {
	messages     map[uuid.UUID]*models.DirectMessage
	userMessages map[uuid.UUID]map[uuid.UUID][]*models.DirectMessage
	db           database.DBAdapter
	hub          *websocket.Hub
}

func NewDirectMessageActor(db database.DBAdapter, hub *websocket.Hub) actor.Actor {
	return &DirectMessageActor{
		messages:     make(map[uuid.UUID]*models.DirectMessage),
		userMessages: make(map[uuid.UUID]map[uuid.UUID][]*models.DirectMessage),
		db:           db,
		hub:          hub,
	}
}

func (a *DirectMessageActor) handleSendMessage(context actor.Context, msg *SendDirectMessageMsg) {
	newMessage := &models.DirectMessage{
		ID:        uuid.New(),
		FromID:    msg.FromID,
		ToID:      msg.ToID,
		Content:   msg.Content,
		CreatedAt: time.Now(),
		IsRead:    false,
		IsDeleted: false,
	}

	// Store in messages map
	a.messages[newMessage.ID] = newMessage

	// Initialize user message maps if they don't exist
	if _, exists := a.userMessages[msg.FromID]; !exists {
		a.userMessages[msg.FromID] = make(map[uuid.UUID][]*models.DirectMessage)
	}
	if _, exists := a.userMessages[msg.ToID]; !exists {
		a.userMessages[msg.ToID] = make(map[uuid.UUID][]*models.DirectMessage)
	}

	// Store in both users' message lists
	a.userMessages[msg.FromID][msg.ToID] = append(a.userMessages[msg.FromID][msg.ToID], newMessage)
	a.userMessages[msg.ToID][msg.FromID] = append(a.userMessages[msg.ToID][msg.FromID], newMessage)

	// Save to DB in the background
	go func() {
		ctx := stdctx.Background()
		if err := a.db.SaveMessage(ctx, newMessage); err != nil {
			log.Printf("Failed to save message to DB: %v", err)
		}
	}()

	// Respond to the original HTTP request immediately
	context.Respond(newMessage)

	// Push message via WebSocket Hub to recipient
	go func() {
		payload, err := json.Marshal(newMessage)
		if err != nil {
			log.Printf("Failed to marshal message for WebSocket push: %v", err)
			return
		}
		a.hub.SendDirectMessage(newMessage.ToID, payload)
		log.Printf("Message %s pushed to Hub for recipient %s", newMessage.ID, newMessage.ToID)
	}()

	log.Printf("New message %s processed (sent from %s to %s)", newMessage.ID, msg.FromID, msg.ToID)
}

func (a *DirectMessageActor) handleGetUserMessages(context actor.Context, msg *GetUserMessagesMsg) {
	// Use a foreground DB fetch
	ctx := stdctx.Background()
	messages, err := a.db.GetMessagesByUser(ctx, msg.UserID)
	if err != nil {
		log.Printf("Failed to get messages from DB: %v", err)
		context.Respond([]*models.DirectMessage{})
		return
	}

	// Update in-memory cache with DB data
	for _, message := range messages {
		a.messages[message.ID] = message

		// Initialize user message maps if they don't exist
		if _, exists := a.userMessages[message.FromID]; !exists {
			a.userMessages[message.FromID] = make(map[uuid.UUID][]*models.DirectMessage)
		}
		if _, exists := a.userMessages[message.ToID]; !exists {
			a.userMessages[message.ToID] = make(map[uuid.UUID][]*models.DirectMessage)
		}

		// Update conversation mappings
		if message.FromID == msg.UserID {
			a.userMessages[message.FromID][message.ToID] = append(
				a.userMessages[message.FromID][message.ToID],
				message,
			)
		}
		if message.ToID == msg.UserID {
			a.userMessages[message.ToID][message.FromID] = append(
				a.userMessages[message.ToID][message.FromID],
				message,
			)
		}
	}

	// Filter out deleted messages and return the result
	var activeMessages []*models.DirectMessage
	for _, message := range messages {
		if !message.IsDeleted {
			activeMessages = append(activeMessages, message)
		}
	}

	log.Printf("Found %d active messages for user %s", len(activeMessages), msg.UserID)
	context.Respond(activeMessages)
}

func (a *DirectMessageActor) handleGetConversation(context actor.Context, msg *GetConversationMsg) {
	if messages, exists := a.userMessages[msg.UserID1][msg.UserID2]; exists {
		var activeMessages []*models.DirectMessage
		for _, message := range messages {
			if !message.IsDeleted {
				activeMessages = append(activeMessages, message)
			}
		}
		context.Respond(activeMessages)
	} else {
		context.Respond([]*models.DirectMessage{})
	}
}

func (a *DirectMessageActor) handleMarkMessageRead(context actor.Context, msg *MarkMessageReadMsg) {
	if message, exists := a.messages[msg.MessageID]; exists {
		// Check if the user marking read is the recipient AND the message is not already marked read
		if message.ToID == msg.UserID && !message.IsRead {
			readTime := time.Now()
			message.IsRead = true
			message.ReadAt = &readTime // Update in-memory struct as well

			// Update DB in the background
			go func() {
				ctx := stdctx.Background()
				isRead := true
				// Call DB update with the correct signature (isRead bool pointer)
				if err := a.db.UpdateMessageStatus(ctx, msg.MessageID, &isRead, nil); err != nil {
					log.Printf("Failed to update message read status in DB: %v", err)
					// Potentially revert in-memory change or log for reconciliation
				}
			}()

			// Send WebSocket notification to the original sender
			go func(originalSenderID uuid.UUID, msgID uuid.UUID, rt time.Time) {
				statusUpdatePayload := MessageStatusUpdate{
					Type:      "messageRead",
					MessageID: msgID,
					ReadAt:    rt,
				}
				payloadBytes, err := json.Marshal(statusUpdatePayload)
				if err != nil {
					log.Printf("Failed to marshal read status update for WebSocket push: %v", err)
					return
				}
				a.hub.SendDirectMessage(originalSenderID, payloadBytes)
				log.Printf("Read status update for message %s pushed to Hub for sender %s", msgID, originalSenderID)
			}(message.FromID, message.ID, readTime) // Pass necessary data into the goroutine

			context.Respond(true) // Respond to the original HTTP request
			return
		} else if message.ToID == msg.UserID && message.IsRead {
			// If already marked read (e.g., duplicate request), still respond true
			context.Respond(true)
			return
		}
	}
	// Message not found or user is not the recipient
	context.Respond(false)
}

func (a *DirectMessageActor) handleDeleteMessage(context actor.Context, msg *DeleteMessageMsg) {
	if message, exists := a.messages[msg.MessageID]; exists {
		if message.FromID == msg.UserID || message.ToID == msg.UserID {
			message.IsDeleted = true

			// Update DB in the background
			go func() {
				ctx := stdctx.Background()
				isDeleted := true
				if err := a.db.UpdateMessageStatus(ctx, msg.MessageID, nil, &isDeleted); err != nil {
					log.Printf("Failed to update message deleted status in DB: %v", err)
				}
			}()

			context.Respond(true)
			return
		}
	}
	context.Respond(false)
}

func (a *DirectMessageActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *SendDirectMessageMsg:
		a.handleSendMessage(context, msg)
	case *GetUserMessagesMsg:
		a.handleGetUserMessages(context, msg)
	case *GetConversationMsg:
		a.handleGetConversation(context, msg)
	case *MarkMessageReadMsg:
		a.handleMarkMessageRead(context, msg)
	case *DeleteMessageMsg:
		a.handleDeleteMessage(context, msg)
	}
}
