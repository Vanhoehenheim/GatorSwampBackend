// internal/database/user_repository.go
package database

import (
	"context"
	"fmt"
	"gator-swamp/internal/models"
	"gator-swamp/internal/utils"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UserDocument represents the MongoDB schema for a user
type UserDocument struct {
	ID             string    `bson:"_id"`            // MongoDB primary key
	Username       string    `bson:"username"`       // Username
	Email          string    `bson:"email"`          // Email address
	HashedPassword string    `bson:"hashedPassword"` // Hashed password
	Karma          int       `bson:"karma"`          // User's karma points
	CreatedAt      time.Time `bson:"createdAt"`      // Account creation timestamp
	LastActive     time.Time `bson:"lastActive"`     // Last active timestamp
	IsConnected    bool      `bson:"isConnected"`    // Connection status
	Subreddits     []string  `bson:"subreddits"`     // List of subscribed subreddit IDs
}

// SaveUser creates or updates a user in MongoDB
func (m *MongoDB) SaveUser(ctx context.Context, user *models.User) error {
	// Convert User model to MongoDB document
	doc := UserDocument{
		ID:             user.ID.String(),
		Username:       user.Username,
		Email:          user.Email,
		HashedPassword: user.HashedPassword,
		Karma:          user.Karma,
		CreatedAt:      user.CreatedAt,
		LastActive:     user.LastActive,
		IsConnected:    user.IsConnected,
		Subreddits:     make([]string, len(user.Subreddits)),
	}

	// Convert subreddit UUIDs to strings
	for i, subredditID := range user.Subreddits {
		doc.Subreddits[i] = subredditID.String()
	}

	opts := options.Update().SetUpsert(true)
	filter := bson.M{"_id": user.ID.String()}
	update := bson.M{"$set": doc}

	_, err := m.Users.UpdateOne(ctx, filter, update, opts)
	return err
}

// GetUser retrieves a user from MongoDB by their ID
func (m *MongoDB) GetUser(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var doc UserDocument

	// Query the user document by ID
	err := m.Users.FindOne(ctx, bson.M{"_id": id.String()}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, utils.NewAppError(utils.ErrUserNotFound, "User not found", err)
	}
	if err != nil {
		return nil, err
	}

	// Convert string ID to UUID
	userID, err := uuid.Parse(doc.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID in database: %v", err)
	}

	// Convert subreddit string IDs to UUIDs
	subreddits := make([]uuid.UUID, len(doc.Subreddits))
	for i, idStr := range doc.Subreddits {
		subredditID, err := uuid.Parse(idStr)
		if err != nil {
			return nil, fmt.Errorf("invalid subreddit ID in database: %v", err)
		}
		subreddits[i] = subredditID
	}

	return &models.User{
		ID:             userID,
		Username:       doc.Username,
		Email:          doc.Email,
		HashedPassword: doc.HashedPassword,
		Karma:          doc.Karma,
		CreatedAt:      doc.CreatedAt,
		LastActive:     doc.LastActive,
		IsConnected:    doc.IsConnected,
		Subreddits:     subreddits,
	}, nil
}

// GetUserByEmail retrieves a user from MongoDB by their email address
func (m *MongoDB) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var doc UserDocument

	// Query the user document by email
	err := m.Users.FindOne(ctx, bson.M{"email": email}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, utils.NewAppError(utils.ErrUserNotFound, "User not found", err)
	}
	if err != nil {
		return nil, err
	}

	// Convert the document to a User model
	userID, err := uuid.Parse(doc.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID in database: %v", err)
	}

	subreddits := make([]uuid.UUID, len(doc.Subreddits))
	for i, idStr := range doc.Subreddits {
		subredditID, err := uuid.Parse(idStr)
		if err != nil {
			return nil, fmt.Errorf("invalid subreddit ID in database: %v", err)
		}
		subreddits[i] = subredditID
	}

	return &models.User{
		ID:             userID,
		Username:       doc.Username,
		Email:          doc.Email,
		HashedPassword: doc.HashedPassword,
		Karma:          doc.Karma,
		CreatedAt:      doc.CreatedAt,
		LastActive:     doc.LastActive,
		IsConnected:    doc.IsConnected,
		Subreddits:     subreddits,
	}, nil
}

// UpdateUserKarma increments a user's karma score
func (m *MongoDB) UpdateUserKarma(ctx context.Context, userID uuid.UUID, delta int) error {
	filter := bson.M{"_id": userID.String()}
	update := bson.M{"$inc": bson.M{"karma": delta}}

	result, err := m.Users.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return utils.NewAppError(utils.ErrUserNotFound, "User not found", nil)
	}
	return nil
}

// UpdateUserActivity updates a user's last active time and connection status
func (m *MongoDB) UpdateUserActivity(ctx context.Context, userID uuid.UUID, isConnected bool) error {
	filter := bson.M{"_id": userID.String()}
	update := bson.M{"$set": bson.M{
		"lastActive":  time.Now(),
		"isConnected": isConnected,
	}}

	result, err := m.Users.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return utils.NewAppError(utils.ErrUserNotFound, "User not found", nil)
	}
	return nil
}

// GetUserSubreddits retrieves the subreddits a user is subscribed to
func (m *MongoDB) GetUserSubreddits(ctx context.Context, userID uuid.UUID) ([]SubredditTitles, error) {
	var user models.User

	// Query user to get subscribed subreddit IDs
	err := m.Users.FindOne(ctx, bson.M{"_id": userID.String()}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, utils.NewAppError(utils.ErrUserNotFound, "User not found", err)
	}
	if err != nil {
		return nil, err
	}

	if len(user.Subreddits) == 0 {
		return []SubredditTitles{}, nil
	}

	// Fetch subreddit titles for subscribed subreddit IDs
	cursor, err := m.Subreddits.Find(ctx,
		bson.M{"_id": bson.M{"$in": user.Subreddits}},
		options.Find().SetProjection(bson.M{"_id": 1, "name": 1}),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var subreddits []SubredditTitles
	if err = cursor.All(ctx, &subreddits); err != nil {
		return nil, err
	}

	return subreddits, nil
}

// UpdateUserSubreddits adds or removes a subreddit from a user's subscriptions
func (m *MongoDB) UpdateUserSubreddits(ctx context.Context, userID uuid.UUID, subredditID uuid.UUID, isJoining bool) error {
	filter := bson.M{"_id": userID.String()}
	var update bson.M

	if isJoining {
		update = bson.M{"$addToSet": bson.M{"subreddits": subredditID.String()}}
	} else {
		update = bson.M{"$pull": bson.M{"subreddits": subredditID.String()}}
	}

	result, err := m.Users.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return utils.NewAppError(utils.ErrUserNotFound, "User not found", nil)
	}
	return nil
}

// SubredditTitles represents a lightweight structure for subreddit ID and name
type SubredditTitles struct {
	ID   uuid.UUID `bson:"_id" json:"id"`    // Subreddit ID
	Name string    `bson:"name" json:"name"` // Subreddit name
}
