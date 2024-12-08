package database

import (
	"context"
	"fmt"
	"gator-swamp/internal/models"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SubredditDB represents the MongoDB document structure for subreddits
type SubredditDB struct {
	ID          string    `bson:"_id"`
	Name        string    `bson:"name"`
	Description string    `bson:"description"`
	CreatorID   string    `bson:"creatorId"`
	Members     int       `bson:"members"`
	CreatedAt   time.Time `bson:"createdAt"`
}

// CreateSubreddit creates a new subreddit in MongoDB
func (m *MongoDB) CreateSubreddit(ctx context.Context, subreddit *models.Subreddit) error {
	subredditDB := SubredditDB{
		ID:          subreddit.ID.String(),
		Name:        subreddit.Name,
		Description: subreddit.Description,
		CreatorID:   subreddit.CreatorID.String(),
		Members:     subreddit.Members,
		CreatedAt:   subreddit.CreatedAt,
	}

	_, err := m.Subreddits.InsertOne(ctx, subredditDB)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("subreddit with name %s already exists", subreddit.Name)
		}
		return fmt.Errorf("failed to create subreddit: %v", err)
	}

	return nil
}

// GetSubredditByID retrieves a subreddit by its ID
func (m *MongoDB) GetSubredditByID(ctx context.Context, id uuid.UUID) (*models.Subreddit, error) {
	var subredditDB SubredditDB
	err := m.Subreddits.FindOne(ctx, bson.M{"_id": id.String()}).Decode(&subredditDB)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get subreddit: %v", err)
	}

	creatorID, err := uuid.Parse(subredditDB.CreatorID)
	if err != nil {
		return nil, fmt.Errorf("invalid creator ID in database: %v", err)
	}

	return &models.Subreddit{
		ID:          id,
		Name:        subredditDB.Name,
		Description: subredditDB.Description,
		CreatorID:   creatorID,
		Members:     subredditDB.Members,
		CreatedAt:   subredditDB.CreatedAt,
	}, nil
}

// GetSubredditByName retrieves a subreddit by its name
func (m *MongoDB) GetSubredditByName(ctx context.Context, name string) (*models.Subreddit, error) {
	var subredditDB SubredditDB
	err := m.Subreddits.FindOne(ctx, bson.M{"name": name}).Decode(&subredditDB)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get subreddit: %v", err)
	}

	id, err := uuid.Parse(subredditDB.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid ID in database: %v", err)
	}

	creatorID, err := uuid.Parse(subredditDB.CreatorID)
	if err != nil {
		return nil, fmt.Errorf("invalid creator ID in database: %v", err)
	}

	return &models.Subreddit{
		ID:          id,
		Name:        subredditDB.Name,
		Description: subredditDB.Description,
		CreatorID:   creatorID,
		Members:     subredditDB.Members,
		CreatedAt:   subredditDB.CreatedAt,
	}, nil
}

// ListSubreddits retrieves all subreddits
func (m *MongoDB) ListSubreddits(ctx context.Context) ([]*models.Subreddit, error) {
	cursor, err := m.Subreddits.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to list subreddits: %v", err)
	}
	defer cursor.Close(ctx)

	var subreddits []*models.Subreddit
	for cursor.Next(ctx) {
		var subredditDB SubredditDB
		if err := cursor.Decode(&subredditDB); err != nil {
			return nil, fmt.Errorf("failed to decode subreddit: %v", err)
		}

		id, err := uuid.Parse(subredditDB.ID)
		if err != nil {
			return nil, fmt.Errorf("invalid ID in database: %v", err)
		}

		creatorID, err := uuid.Parse(subredditDB.CreatorID)
		if err != nil {
			return nil, fmt.Errorf("invalid creator ID in database: %v", err)
		}

		subreddits = append(subreddits, &models.Subreddit{
			ID:          id,
			Name:        subredditDB.Name,
			Description: subredditDB.Description,
			CreatorID:   creatorID,
			Members:     subredditDB.Members,
			CreatedAt:   subredditDB.CreatedAt,
		})
	}

	return subreddits, nil
}

// UpdateSubredditMembers updates the member count
func (m *MongoDB) UpdateSubredditMembers(ctx context.Context, id uuid.UUID, delta int) error {
	result, err := m.Subreddits.UpdateOne(
		ctx,
		bson.M{"_id": id.String()},
		bson.M{"$inc": bson.M{"members": delta}},
	)

	if err != nil {
		return fmt.Errorf("failed to update member count: %v", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("subreddit not found")
	}

	return nil
}

// EnsureSubredditIndexes creates required indexes
func (m *MongoDB) EnsureSubredditIndexes(ctx context.Context) error {
	_, err := m.Subreddits.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "name", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	if err != nil {
		return fmt.Errorf("failed to create name index: %v", err)
	}

	return nil
}
