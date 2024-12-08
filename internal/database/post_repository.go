// internal/database/post_repository.go
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

// PostDocument represents post data in MongoDB
type PostDocument struct {
	ID          uuid.UUID `bson:"_id"`
	Title       string    `bson:"title"`
	Content     string    `bson:"content"`
	AuthorID    uuid.UUID `bson:"authorId"`
	SubredditID uuid.UUID `bson:"subredditId"`
	CreatedAt   time.Time `bson:"createdAt"`
	Upvotes     int       `bson:"upvotes"`
	Downvotes   int       `bson:"downvotes"`
	Karma       int       `bson:"karma"`
}

// SavePost creates or updates a post in MongoDB
func (m *MongoDB) SavePost(ctx context.Context, post *models.Post) error {
	doc := PostDocument{
		ID:          post.ID,
		Title:       post.Title,
		Content:     post.Content,
		AuthorID:    post.AuthorID,
		SubredditID: post.SubredditID,
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

	return &models.Post{
		ID:          doc.ID,
		Title:       doc.Title,
		Content:     doc.Content,
		AuthorID:    doc.AuthorID,
		SubredditID: doc.SubredditID,
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
		posts = append(posts, &models.Post{
			ID:          doc.ID,
			Title:       doc.Title,
			Content:     doc.Content,
			AuthorID:    doc.AuthorID,
			SubredditID: doc.SubredditID,
			CreatedAt:   doc.CreatedAt,
			Upvotes:     doc.Upvotes,
			Downvotes:   doc.Downvotes,
			Karma:       doc.Karma,
		})
	}
	return posts, nil
}

// GetUserFeed retrieves posts from multiple subreddits
func (m *MongoDB) GetUserFeed(ctx context.Context, subredditIDs []uuid.UUID, limit int) ([]*models.Post, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "karma", Value: -1}, {Key: "createdAt", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := m.Posts.Find(ctx,
		bson.M{"subredditId": bson.M{"$in": subredditIDs}},
		opts,
	)
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
		posts = append(posts, &models.Post{
			ID:          doc.ID,
			Title:       doc.Title,
			Content:     doc.Content,
			AuthorID:    doc.AuthorID,
			SubredditID: doc.SubredditID,
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
