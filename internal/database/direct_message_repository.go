package database

import (
	"context"
	"fmt"
	"gator-swamp/internal/models"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// DirectMessageDocument represents the MongoDB document structure for direct messages
type DirectMessageDocument struct {
	ID        string    `bson:"_id"`
	FromID    string    `bson:"fromId"`
	ToID      string    `bson:"toId"`
	Content   string    `bson:"content"`
	CreatedAt time.Time `bson:"createdAt"`
	IsRead    bool      `bson:"isRead"`
	IsDeleted bool      `bson:"isDeleted"`
}

// SaveMessage saves a new direct message to MongoDB
func (m *MongoDB) SaveMessage(ctx context.Context, message *models.DirectMessage) error {
	doc := DirectMessageDocument{
		ID:        message.ID.String(),
		FromID:    message.FromID.String(),
		ToID:      message.ToID.String(),
		Content:   message.Content,
		CreatedAt: message.CreatedAt,
		IsRead:    message.IsRead,
		IsDeleted: message.IsDeleted,
	}

	_, err := m.Messages.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to save message: %v", err)
	}

	return nil
}

// GetMessagesByUser retrieves all messages for a user (both sent and received)
func (m *MongoDB) GetMessagesByUser(ctx context.Context, userID uuid.UUID) ([]*models.DirectMessage, error) {
	userIDStr := userID.String()

	filter := bson.M{
		"$or": []bson.M{
			{"fromId": userIDStr},
			{"toId": userIDStr},
		},
		"isDeleted": false,
	}

	cursor, err := m.Messages.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get user messages: %v", err)
	}
	defer cursor.Close(ctx)

	var messages []*models.DirectMessage
	for cursor.Next(ctx) {
		var doc DirectMessageDocument
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("failed to decode message: %v", err)
		}

		id, _ := uuid.Parse(doc.ID)
		fromID, _ := uuid.Parse(doc.FromID)
		toID, _ := uuid.Parse(doc.ToID)

		messages = append(messages, &models.DirectMessage{
			ID:        id,
			FromID:    fromID,
			ToID:      toID,
			Content:   doc.Content,
			CreatedAt: doc.CreatedAt,
			IsRead:    doc.IsRead,
			IsDeleted: doc.IsDeleted,
		})
	}

	return messages, nil
}

// UpdateMessageStatus updates IsRead or IsDeleted status of a message
func (m *MongoDB) UpdateMessageStatus(ctx context.Context, messageID uuid.UUID, isRead *bool, isDeleted *bool) error {
	update := bson.M{"$set": bson.M{}}

	if isRead != nil {
		update["$set"].(bson.M)["isRead"] = *isRead
	}
	if isDeleted != nil {
		update["$set"].(bson.M)["isDeleted"] = *isDeleted
	}

	result, err := m.Messages.UpdateOne(ctx, bson.M{"_id": messageID.String()}, update)
	if err != nil {
		return fmt.Errorf("failed to update message status: %v", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("message not found")
	}

	return nil
}
