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

// SubredditDB represents the MongoDB document structure for subreddits
type SubredditDB struct {
	ID          string    `bson:"_id"`
	Name        string    `bson:"name"`
	Description string    `bson:"description"`
	CreatorID   string    `bson:"creatorId"`
	Members     int       `bson:"members"`
	CreatedAt   time.Time `bson:"createdAt"`
	Posts       []string  `bson:"posts"`
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
		Posts:       make([]string, 0), // Initialize empty posts array
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

	// Convert post IDs from strings to UUIDs
	posts := make([]uuid.UUID, 0, len(subredditDB.Posts))
	for _, postIDStr := range subredditDB.Posts {
		postID, err := uuid.Parse(postIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid post ID in database: %v", err)
		}
		posts = append(posts, postID)
	}

	return &models.Subreddit{
		ID:          id,
		Name:        subredditDB.Name,
		Description: subredditDB.Description,
		CreatorID:   creatorID,
		Members:     subredditDB.Members,
		CreatedAt:   subredditDB.CreatedAt,
		Posts:       posts,
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
	posts := make([]uuid.UUID, 0, len(subredditDB.Posts))
	for _, postIDStr := range subredditDB.Posts {
		postID, err := uuid.Parse(postIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid post ID in database: %v", err)
		}
		posts = append(posts, postID)
	}

	return &models.Subreddit{
		ID:          id,
		Name:        subredditDB.Name,
		Description: subredditDB.Description,
		CreatorID:   creatorID,
		Members:     subredditDB.Members,
		CreatedAt:   subredditDB.CreatedAt,
		Posts:       posts,
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

func (m *MongoDB) UpdateSubredditPosts(ctx context.Context, subredditID uuid.UUID, postID uuid.UUID, isAdding bool) error {
	filter := bson.M{"_id": subredditID.String()}
	var update bson.M

	if isAdding {
		update = bson.M{"$addToSet": bson.M{"posts": postID.String()}}
	} else {
		update = bson.M{"$pull": bson.M{"posts": postID.String()}}
	}

	result, err := m.Subreddits.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update subreddit posts: %v", err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("subreddit not found")
	}

	return nil
}

func (m *MongoDB) GetSubredditMembers(ctx context.Context, subredditID uuid.UUID) ([]string, error) {
	// Find all users who have this subreddit in their subreddits array
	filter := bson.M{"subreddits": subredditID.String()}

	cursor, err := m.Users.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get subreddit members: %v", err)
	}
	defer cursor.Close(ctx)

	var memberIDs []string
	for cursor.Next(ctx) {
		var user struct {
			ID string `bson:"_id"`
		}
		if err := cursor.Decode(&user); err != nil {
			return nil, fmt.Errorf("failed to decode user: %v", err)
		}
		memberIDs = append(memberIDs, user.ID)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %v", err)
	}

	return memberIDs, nil
}

func (m *MongoDB) VerifyAndGetSubreddit(ctx context.Context, subredditID uuid.UUID) error {
	var subredditDB SubredditDB
	err := m.Subreddits.FindOne(ctx, bson.M{"_id": subredditID.String()}).Decode(&subredditDB)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil)
		}
		return utils.NewAppError(utils.ErrDatabase, "failed to verify subreddit", err)
	}
	return nil
}
