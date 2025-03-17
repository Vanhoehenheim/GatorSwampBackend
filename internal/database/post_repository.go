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

// PostDocument represents the MongoDB schema for a post.
type PostDocument struct {
	ID             string          `bson:"_id"`
	Title          string          `bson:"title"`
	Content        string          `bson:"content"`
	AuthorID       string          `bson:"authorid"`
	AuthorUsername string          `bson:"authorusername"`
	SubredditID    string          `bson:"subredditid"`
	SubredditName  string          `bson:"subredditname"`
	CreatedAt      time.Time       `bson:"createdat"`
	Upvotes        int             `bson:"upvotes"`
	Downvotes      int             `bson:"downvotes"`
	Karma          int             `bson:"karma"`
	UserVotes      map[string]bool `bson:"uservotes,omitempty"` // Map of userID to vote type
	CommentCount   int             `bson:"commentcount"`        // Number of comments on the post
}

// ModelToDocument converts a Post model to a MongoDB document.
func (m *MongoDB) ModelToDocument(post *models.Post) *PostDocument {
	return &PostDocument{
		ID:             post.ID.String(),
		Title:          post.Title,
		Content:        post.Content,
		AuthorID:       post.AuthorID.String(),
		AuthorUsername: post.AuthorUsername,
		SubredditID:    post.SubredditID.String(),
		SubredditName:  post.SubredditName,
		CreatedAt:      post.CreatedAt,
		Upvotes:        post.Upvotes,
		Downvotes:      post.Downvotes,
		Karma:          post.Karma,
		UserVotes:      post.UserVotes,
		CommentCount:   post.CommentCount,
	}
}

// DocumentToModel converts a MongoDB document to a Post model.
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
		ID:             id,
		Title:          doc.Title,
		Content:        doc.Content,
		AuthorID:       authorID,
		AuthorUsername: doc.AuthorUsername,
		SubredditID:    subredditID,
		SubredditName:  doc.SubredditName,
		CreatedAt:      doc.CreatedAt,
		Upvotes:        doc.Upvotes,
		Downvotes:      doc.Downvotes,
		Karma:          doc.Karma,
		UserVotes:      doc.UserVotes,
		CommentCount:   doc.CommentCount,
	}, nil
}

// SavePost creates or updates a post in MongoDB.
func (m *MongoDB) SavePost(ctx context.Context, post *models.Post) error {
	doc := m.ModelToDocument(post)

	opts := options.Update().SetUpsert(true)
	filter := bson.M{"_id": post.ID.String()}
	update := bson.M{"$set": doc}

	_, err := m.Posts.UpdateOne(ctx, filter, update, opts)
	return err
}

// GetPost retrieves a post by its ID.
func (m *MongoDB) GetPost(ctx context.Context, id uuid.UUID) (*models.Post, error) {
	var doc PostDocument

	// Find the post by its ID.
	err := m.Posts.FindOne(ctx, bson.M{"_id": id.String()}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, utils.NewAppError(utils.ErrNotFound, "Post not found", err)
	}
	if err != nil {
		return nil, err
	}

	return m.DocumentToModel(&doc)
}

// GetSubredditPosts retrieves all posts for a given subreddit ID.
func (m *MongoDB) GetSubredditPosts(ctx context.Context, subredditID uuid.UUID) ([]*models.Post, error) {
	log.Printf("Querying MongoDB for posts in subreddit: %s", subredditID.String())

	cursor, err := m.Posts.Find(ctx, bson.M{"subredditid": subredditID.String()})
	if err != nil {
		return nil, fmt.Errorf("database query failed: %v", err)
	}
	defer cursor.Close(ctx)

	var posts []*models.Post
	for cursor.Next(ctx) {
		var doc PostDocument
		if err := cursor.Decode(&doc); err != nil {
			log.Printf("Error decoding post document: %v", err)
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
		return nil, fmt.Errorf("cursor iteration failed: %v", err)
	}

	log.Printf("Found %d posts in subreddit %s", len(posts), subredditID)
	return posts, nil
}

// UpdatePostVotes modifies the vote counts and karma for a post.
func (m *MongoDB) UpdatePostVotes(ctx context.Context, postID uuid.UUID, upvoteDelta, downvoteDelta int) error {
	filter := bson.M{"_id": postID.String()}
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

// UpdatePostCommentCount updates the comment count for a post
func (m *MongoDB) UpdatePostCommentCount(ctx context.Context, postID uuid.UUID, commentCount int) error {
	filter := bson.M{"_id": postID.String()}
	update := bson.M{
		"$set": bson.M{
			"commentcount": commentCount,
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

// GetUserFeedPosts retrieves a user's feed posts, sorted by karma and creation date.
func (m *MongoDB) GetUserFeedPosts(ctx context.Context, userID uuid.UUID, limit int) ([]*models.Post, error) {
	// Fetch the user's subscribed subreddits.
	user, err := m.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user subscriptions: %v", err)
	}

	// Prepare subreddit ID strings for the query.
	subredditIDStrings := make([]string, len(user.Subreddits))
	for i, id := range user.Subreddits {
		subredditIDStrings[i] = id.String()
	}

	// Define aggregation pipeline to retrieve feed posts.
	pipeline := []bson.M{
		{"$match": bson.M{"subredditid": bson.M{"$in": subredditIDStrings}}},
		{"$sort": bson.D{
			{Key: "karma", Value: -1},
			{Key: "createdat", Value: -1},
		}},
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
