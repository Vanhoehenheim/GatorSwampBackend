// internal/database/user_repository.go
package database

import (
	"context"
	"gator-swamp/internal/models"
	"gator-swamp/internal/utils"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UserDocument represents core user data in MongoDB
type UserDocument struct {
	ID             uuid.UUID `bson:"_id"`
	Username       string    `bson:"username"`
	Email          string    `bson:"email"`
	HashedPassword string    `bson:"hashedPassword"`
	Karma          int       `bson:"karma"`
	CreatedAt      time.Time `bson:"createdAt"`
	LastActive     time.Time `bson:"lastActive"`
	IsConnected    bool      `bson:"isConnected"`
	Subreddits	 []uuid.UUID `bson:"subreddits"`
}

// SaveUser saves or updates a user in MongoDB
func (m *MongoDB) SaveUser(ctx context.Context, user *models.User) error {
	doc := UserDocument{
		ID:             user.ID,
		Username:       user.Username,
		Email:          user.Email,
		HashedPassword: user.HashedPassword,
		Karma:          user.Karma,
		CreatedAt:      user.CreatedAt,
		LastActive:     user.LastActive,
		IsConnected:    user.IsConnected,
		Subreddits:     user.Subreddits,
	}

	opts := options.Update().SetUpsert(true)
	filter := bson.M{"_id": user.ID}
	update := bson.M{"$set": doc}

	_, err := m.Users.UpdateOne(ctx, filter, update, opts)
	return err
}

// GetUser retrieves a user from MongoDB by ID
func (m *MongoDB) GetUser(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var doc UserDocument
	err := m.Users.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, utils.NewAppError(utils.ErrUserNotFound, "User not found", err)
	}
	if err != nil {
		return nil, err
	}

	return &models.User{
		ID:             doc.ID,
		Username:       doc.Username,
		Email:          doc.Email,
		HashedPassword: doc.HashedPassword,
		Karma:          doc.Karma,
		CreatedAt:      doc.CreatedAt,
		LastActive:     doc.LastActive,
		IsConnected:    doc.IsConnected,
		Subreddits:    doc.Subreddits,
	}, nil
}

// GetUserByEmail retrieves a user from MongoDB by email
func (m *MongoDB) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var doc UserDocument
	err := m.Users.FindOne(ctx, bson.M{"email": email}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, utils.NewAppError(utils.ErrUserNotFound, "User not found", err)
	}
	if err != nil {
		return nil, err
	}

	return &models.User{
		ID:             doc.ID,
		Username:       doc.Username,
		Email:          doc.Email,
		HashedPassword: doc.HashedPassword,
		Karma:          doc.Karma,
		CreatedAt:      doc.CreatedAt,
		LastActive:     doc.LastActive,
		IsConnected:    doc.IsConnected,
		Subreddits:    doc.Subreddits,
	}, nil
}

// UpdateUserKarma updates a user's karma in MongoDB
func (m *MongoDB) UpdateUserKarma(ctx context.Context, userID uuid.UUID, delta int) error {
	filter := bson.M{"_id": userID}
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

// UpdateUserActivity updates a user's last active time and connected status
func (m *MongoDB) UpdateUserActivity(ctx context.Context, userID uuid.UUID, isConnected bool) error {
	filter := bson.M{"_id": userID}
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

type SubredditTitles struct {
    ID   uuid.UUID `bson:"_id" json:"id"`
    Name string    `bson:"name" json:"name"`
}

func (m *MongoDB) GetUserSubreddits(ctx context.Context, userID uuid.UUID) ([]SubredditTitles, error) {
    var user models.User
    err := m.Users.FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
    if err != nil {
        if err == mongo.ErrNoDocuments {
            return nil, utils.NewAppError(utils.ErrUserNotFound, "User not found", err)
        }
        return nil, err
    }

    if len(user.Subreddits) == 0 {
        return []SubredditTitles{}, nil
    }

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

func (m *MongoDB) UpdateUserSubreddits(ctx context.Context, userID uuid.UUID, subredditID uuid.UUID, isJoining bool) error {
    filter := bson.M{"_id": userID}
    var update bson.M
    
    if isJoining {
        update = bson.M{"$addToSet": bson.M{"subreddits": subredditID}}
    } else {
        update = bson.M{"$pull": bson.M{"subreddits": subredditID}}
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
