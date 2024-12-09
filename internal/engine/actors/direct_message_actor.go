package actors

import (
	stdctx "context" // Alias for standard context to avoid confusion with actor.Context
	"gator-swamp/internal/database"
	"gator-swamp/internal/models"
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
)

// DirectMessageActor manages direct messaging operations
type DirectMessageActor struct {
	messages     map[uuid.UUID]*models.DirectMessage
	userMessages map[uuid.UUID]map[uuid.UUID][]*models.DirectMessage
	mongodb      *database.MongoDB
}

func NewDirectMessageActor(mongodb *database.MongoDB) actor.Actor {
	return &DirectMessageActor{
		messages:     make(map[uuid.UUID]*models.DirectMessage),
		userMessages: make(map[uuid.UUID]map[uuid.UUID][]*models.DirectMessage),
		mongodb:      mongodb,
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

	// Save to MongoDB in the background
	go func() {
		ctx := stdctx.Background()
		if err := a.mongodb.SaveMessage(ctx, newMessage); err != nil {
			log.Printf("Failed to save message to MongoDB: %v", err)
		}
	}()

	log.Printf("New message sent from %s to %s", msg.FromID, msg.ToID)
	context.Respond(newMessage)
}

func (a *DirectMessageActor) handleGetUserMessages(context actor.Context, msg *GetUserMessagesMsg) {
	// Get messages from MongoDB in the background
	go func() {
		ctx := stdctx.Background()
		messages, err := a.mongodb.GetMessagesByUser(ctx, msg.UserID)
		if err != nil {
			log.Printf("Failed to get messages from MongoDB: %v", err)
		}
		// Update in-memory cache with MongoDB data
		for _, message := range messages {
			a.messages[message.ID] = message
		}
	}()

	if conversations, exists := a.userMessages[msg.UserID]; exists {
		var allMessages []*models.DirectMessage
		for _, messages := range conversations {
			for _, message := range messages {
				if !message.IsDeleted {
					allMessages = append(allMessages, message)
				}
			}
		}
		context.Respond(allMessages)
	} else {
		context.Respond([]*models.DirectMessage{})
	}
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
		if message.ToID == msg.UserID {
			message.IsRead = true

			// Update MongoDB in the background
			go func() {
				ctx := stdctx.Background()
				isRead := true
				if err := a.mongodb.UpdateMessageStatus(ctx, msg.MessageID, &isRead, nil); err != nil {
					log.Printf("Failed to update message read status in MongoDB: %v", err)
				}
			}()

			context.Respond(true)
			return
		}
	}
	context.Respond(false)
}

func (a *DirectMessageActor) handleDeleteMessage(context actor.Context, msg *DeleteMessageMsg) {
	if message, exists := a.messages[msg.MessageID]; exists {
		if message.FromID == msg.UserID || message.ToID == msg.UserID {
			message.IsDeleted = true

			// Update MongoDB in the background
			go func() {
				ctx := stdctx.Background()
				isDeleted := true
				if err := a.mongodb.UpdateMessageStatus(ctx, msg.MessageID, nil, &isDeleted); err != nil {
					log.Printf("Failed to update message deleted status in MongoDB: %v", err)
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
