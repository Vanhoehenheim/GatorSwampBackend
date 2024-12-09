// internal/database/post_repository.go
package database

import (
	"context"
	"fmt"
	"gator-swamp/internal/models"
	"gator-swamp/internal/utils"
	"log"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// PostDocument represents post data in MongoDB
type PostDocument struct {
	ID          string    `bson:"_id"` // Using _id to match MongoDB's default
	Title       string    `bson:"title"`
	Content     string    `bson:"content"`
	AuthorID    string    `bson:"authorid"`
	SubredditID string    `bson:"subredditid"`
	CreatedAt   time.Time `bson:"createdat"`
	Upvotes     int       `bson:"upvotes"`
	Downvotes   int       `bson:"downvotes"`
	Karma       int       `bson:"karma"`
}

func (m *MongoDB) ModelToDocument(post *models.Post) *PostDocument {
	return &PostDocument{
		ID:          post.ID.String(),
		Title:       post.Title,
		Content:     post.Content,
		AuthorID:    post.AuthorID.String(),
		SubredditID: post.SubredditID.String(),
		CreatedAt:   post.CreatedAt,
		Upvotes:     post.Upvotes,
		Downvotes:   post.Downvotes,
		Karma:       post.Karma,
	}
}

func (m *MongoDB) DocumentToModel(doc *PostDocument) (*models.Post, error) {
	id, err := uuid.Parse(doc.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid post ID: %v", err)
	}

	authorID, err := uuid.Parse(doc.AuthorID)
	if err != nil {
		return nil, fmt.Errorf("invalid author ID: %v", err)
	}

	subredditID, err := uuid.Parse(doc.SubredditID)
	if err != nil {
		return nil, fmt.Errorf("invalid subreddit ID: %v", err)
	}

	return &models.Post{
		ID:          id,
		Title:       doc.Title,
		Content:     doc.Content,
		AuthorID:    authorID,
		SubredditID: subredditID,
		CreatedAt:   doc.CreatedAt,
		Upvotes:     doc.Upvotes,
		Downvotes:   doc.Downvotes,
		Karma:       doc.Karma,
	}, nil
}

// SavePost creates or updates a post in MongoDB
func (m *MongoDB) SavePost(ctx context.Context, post *models.Post) error {
	doc := PostDocument{
		ID:          post.ID.String(),
		Title:       post.Title,
		Content:     post.Content,
		AuthorID:    post.AuthorID.String(),
		SubredditID: post.SubredditID.String(),
		CreatedAt:   post.CreatedAt,
		Upvotes:     post.Upvotes,
		Downvotes:   post.Downvotes,
		Karma:       post.Karma,
	}

	opts := options.Update().SetUpsert(true)
	filter := bson.M{"_id": post.ID}
	update := bson.M{"$set": doc}

	_, err := m.Posts.UpdateOne(ctx, filter, update, opts)
	return err
}

// GetPost retrieves a post by ID
func (m *MongoDB) GetPost(ctx context.Context, id uuid.UUID) (*models.Post, error) {
	var doc PostDocument
	err := m.Posts.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, utils.NewAppError(utils.ErrNotFound, "Post not found", err)
	}
	if err != nil {
		return nil, err
	}
	// convert doc.id to uuid
	id_uuid, _ := uuid.Parse(doc.ID)
	authorid_uuid, _ := uuid.Parse(doc.AuthorID)
	subredditid_uuid, _ := uuid.Parse(doc.SubredditID)
	return &models.Post{
		ID:          id_uuid,
		Title:       doc.Title,
		Content:     doc.Content,
		AuthorID:    authorid_uuid,
		SubredditID: subredditid_uuid,
		CreatedAt:   doc.CreatedAt,
		Upvotes:     doc.Upvotes,
		Downvotes:   doc.Downvotes,
		Karma:       doc.Karma,
	}, nil
}

// GetSubredditPosts retrieves all posts for a subreddit
func (m *MongoDB) GetSubredditPosts(ctx context.Context, subredditID uuid.UUID) ([]*models.Post, error) {
	cursor, err := m.Posts.Find(ctx, bson.M{"subredditId": subredditID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var posts []*models.Post
	for cursor.Next(ctx) {
		var doc PostDocument
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		id_uuid, _ := uuid.Parse(doc.ID)
		authorid_uuid, _ := uuid.Parse(doc.AuthorID)
		subredditid_uuid, _ := uuid.Parse(doc.SubredditID)
		posts = append(posts, &models.Post{
			ID:          id_uuid,
			Title:       doc.Title,
			Content:     doc.Content,
			AuthorID:    authorid_uuid,
			SubredditID: subredditid_uuid,
			CreatedAt:   doc.CreatedAt,
			Upvotes:     doc.Upvotes,
			Downvotes:   doc.Downvotes,
			Karma:       doc.Karma,
		})
	}
	return posts, nil
}

// UpdatePostVotes updates the vote counts and karma for a post
func (m *MongoDB) UpdatePostVotes(ctx context.Context, postID uuid.UUID, upvoteDelta, downvoteDelta int) error {
	filter := bson.M{"_id": postID}
	update := bson.M{
		"$inc": bson.M{
			"upvotes":   upvoteDelta,
			"downvotes": downvoteDelta,
			"karma":     upvoteDelta - downvoteDelta,
		},
	}

	result, err := m.Posts.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return utils.NewAppError(utils.ErrNotFound, "Post not found", nil)
	}
	return nil
}

// In post_repository.go
func (m *MongoDB) GetUserFeedPosts(ctx context.Context, userID uuid.UUID, limit int) ([]*models.Post, error) {
	user, err := m.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user subscriptions: %v", err)
	}

	// Convert UUID array to string array for query
	subredditIDStrings := make([]string, len(user.Subreddits))
	for i, id := range user.Subreddits {
		subredditIDStrings[i] = id.String()
	}

	pipeline := []bson.M{
		{
			"$match": bson.M{
				"subredditid": bson.M{"$in": subredditIDStrings},
			},
		},
		{
			"$sort": bson.D{
				{Key: "karma", Value: -1},
				{Key: "createdat", Value: -1},
			},
		},
	}

	if limit > 0 {
		pipeline = append(pipeline, bson.M{"$limit": limit})
	}

	cursor, err := m.Posts.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch feed: %v", err)
	}
	defer cursor.Close(ctx)

	var posts []*models.Post
	for cursor.Next(ctx) {
		var doc PostDocument
		if err := cursor.Decode(&doc); err != nil {
			log.Printf("Error decoding post: %v", err)
			continue
		}

		post, err := m.DocumentToModel(&doc)
		if err != nil {
			log.Printf("Error converting document to model: %v", err)
			continue
		}
		posts = append(posts, post)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("error reading feed posts: %v", err)
	}

	return posts, nil
}
