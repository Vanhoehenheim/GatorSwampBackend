package actors

import (
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
		UserID    uuid.UUID `json:"userId"` // The user marking the message as read
	}

	DeleteMessageMsg struct {
		MessageID uuid.UUID `json:"messageId"`
		UserID    uuid.UUID `json:"userId"` // The user deleting the message
	}
)

// DirectMessage represents a single message
type DirectMessage struct {
	ID        uuid.UUID `json:"id"`
	FromID    uuid.UUID `json:"fromId"`
	ToID      uuid.UUID `json:"toId"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	IsRead    bool      `json:"isRead"`
	IsDeleted bool      `json:"isDeleted"`
}

// DirectMessageActor manages direct messaging operations
type DirectMessageActor struct {
	messages     map[uuid.UUID]*DirectMessage                 // MessageID -> Message
	userMessages map[uuid.UUID]map[uuid.UUID][]*DirectMessage // UserID -> OtherUserID -> Messages
}

func NewDirectMessageActor() *DirectMessageActor {
	return &DirectMessageActor{
		messages:     make(map[uuid.UUID]*DirectMessage),
		userMessages: make(map[uuid.UUID]map[uuid.UUID][]*DirectMessage),
	}
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

func (a *DirectMessageActor) handleSendMessage(context actor.Context, msg *SendDirectMessageMsg) {
	// Create new message
	newMessage := &DirectMessage{
		ID:        uuid.New(),
		FromID:    msg.FromID,
		ToID:      msg.ToID,
		Content:   msg.Content,
		CreatedAt: time.Now(),
		IsRead:    false,
	}

	// Store in messages map
	a.messages[newMessage.ID] = newMessage

	// Initialize user message maps if they don't exist
	if _, exists := a.userMessages[msg.FromID]; !exists {
		a.userMessages[msg.FromID] = make(map[uuid.UUID][]*DirectMessage)
	}
	if _, exists := a.userMessages[msg.ToID]; !exists {
		a.userMessages[msg.ToID] = make(map[uuid.UUID][]*DirectMessage)
	}

	// Store in both users' message lists
	a.userMessages[msg.FromID][msg.ToID] = append(a.userMessages[msg.FromID][msg.ToID], newMessage)
	a.userMessages[msg.ToID][msg.FromID] = append(a.userMessages[msg.ToID][msg.FromID], newMessage)

	log.Printf("New message sent from %s to %s", msg.FromID, msg.ToID)
	context.Respond(newMessage)
}

func (a *DirectMessageActor) handleGetUserMessages(context actor.Context, msg *GetUserMessagesMsg) {
	if conversations, exists := a.userMessages[msg.UserID]; exists {
		// Collect all messages for the user
		var allMessages []*DirectMessage
		for _, messages := range conversations {
			for _, message := range messages {
				if !message.IsDeleted {
					allMessages = append(allMessages, message)
				}
			}
		}
		context.Respond(allMessages)
	} else {
		context.Respond([]*DirectMessage{})
	}
}

func (a *DirectMessageActor) handleGetConversation(context actor.Context, msg *GetConversationMsg) {
	if messages, exists := a.userMessages[msg.UserID1][msg.UserID2]; exists {
		// Filter out deleted messages
		var activeMessages []*DirectMessage
		for _, message := range messages {
			if !message.IsDeleted {
				activeMessages = append(activeMessages, message)
			}
		}
		context.Respond(activeMessages)
	} else {
		context.Respond([]*DirectMessage{})
	}
}

func (a *DirectMessageActor) handleMarkMessageRead(context actor.Context, msg *MarkMessageReadMsg) {
	if message, exists := a.messages[msg.MessageID]; exists {
		if message.ToID == msg.UserID {
			message.IsRead = true
			context.Respond(true)
			return
		}
	}
	context.Respond(false)
}

func (a *DirectMessageActor) handleDeleteMessage(context actor.Context, msg *DeleteMessageMsg) {
	if message, exists := a.messages[msg.MessageID]; exists {
		// Only allow sender or receiver to delete the message
		if message.FromID == msg.UserID || message.ToID == msg.UserID {
			message.IsDeleted = true
			context.Respond(true)
			return
		}
	}
	context.Respond(false)
}
