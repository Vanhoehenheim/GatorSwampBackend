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

// CommentDocument represents comment data in MongoDB
type CommentDocument struct {
	ID          string    `bson:"_id"`
	Content     string    `bson:"content"`
	AuthorID    string    `bson:"authorId"`
	PostID      string    `bson:"postId"`
	SubredditID string    `bson:"subredditId"`
	ParentID    *string   `bson:"parentId,omitempty"`
	Children    []string  `bson:"children"`
	CreatedAt   time.Time `bson:"createdAt"`
	UpdatedAt   time.Time `bson:"updatedAt"`
	IsDeleted   bool      `bson:"isDeleted"`
	Upvotes     int       `bson:"upvotes"`
	Downvotes   int       `bson:"downvotes"`
	Karma       int       `bson:"karma"`
}

type VoteDocument struct {
	ID        string    `bson:"_id"`
	UserID    string    `bson:"userId"`
	CommentID string    `bson:"commentId"`
	IsUpvote  bool      `bson:"isUpvote"`
	CreatedAt time.Time `bson:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt"`
}

// SaveComment creates or updates a comment in MongoDB
func (m *MongoDB) SaveComment(ctx context.Context, comment *models.Comment) error {
	log.Printf("Saving comment with ID: %s", comment.ID.String())

	doc := CommentDocument{
		ID:          comment.ID.String(),
		Content:     comment.Content,
		AuthorID:    comment.AuthorID.String(),
		PostID:      comment.PostID.String(),
		Children:    make([]string, len(comment.Children)),
		CreatedAt:   comment.CreatedAt,
		UpdatedAt:   comment.UpdatedAt,
		IsDeleted:   comment.IsDeleted,
		Upvotes:     comment.Upvotes,
		Downvotes:   comment.Downvotes,
		Karma:       comment.Karma,
		SubredditID: comment.SubredditID.String(),
	}

	// Convert Children UUIDs to strings
	for i, childID := range comment.Children {
		doc.Children[i] = childID.String()
	}

	// Handle optional ParentID
	if comment.ParentID != nil {
		parentIDStr := comment.ParentID.String()
		doc.ParentID = &parentIDStr
	}

	opts := options.Update().SetUpsert(true)
	filter := bson.M{"_id": doc.ID}
	update := bson.M{"$set": doc}

	result, err := m.Comments.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		log.Printf("Error saving comment %s: %v", comment.ID.String(), err)
		return fmt.Errorf("failed to save comment: %v", err)
	}

	log.Printf("Successfully saved comment %s. Matched: %d, Modified: %d, Upserted: %d",
		comment.ID.String(), result.MatchedCount, result.ModifiedCount, result.UpsertedCount)

	return nil
}

// GetComment retrieves a comment by ID
func (m *MongoDB) GetComment(ctx context.Context, id uuid.UUID) (*models.Comment, error) {
	var doc CommentDocument

	// Add logging
	log.Printf("Attempting to find comment with ID: %s", id.String())

	err := m.Comments.FindOne(ctx, bson.M{"_id": id.String()}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		log.Printf("No comment found with ID: %s", id.String())
		return nil, utils.NewAppError(utils.ErrNotFound, "Comment not found", err)
	}
	if err != nil {
		log.Printf("Error finding comment with ID %s: %v", id.String(), err)
		return nil, fmt.Errorf("failed to get comment: %v", err)
	}

	log.Printf("Successfully found comment with ID: %s", id.String())
	return convertCommentDocumentToModel(&doc)
}

// GetPostComments retrieves all comments for a post
func (m *MongoDB) GetPostComments(ctx context.Context, postID uuid.UUID) ([]*models.Comment, error) {
	cursor, err := m.Comments.Find(ctx, bson.M{"postId": postID.String()})
	if err != nil {
		return nil, fmt.Errorf("failed to get post comments: %v", err)
	}
	defer cursor.Close(ctx)

	var comments []*models.Comment
	for cursor.Next(ctx) {
		var doc CommentDocument
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("failed to decode comment: %v", err)
		}

		comment, err := convertCommentDocumentToModel(&doc)
		if err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}

	return comments, nil
}

// UpdateCommentVotes updates the vote counts and karma for a comment
func (m *MongoDB) UpdateCommentVotes(ctx context.Context, commentID uuid.UUID, upvotes, downvotes int) error {
	filter := bson.M{"_id": commentID.String()}
	update := bson.M{
		"$set": bson.M{
			"upvotes":   upvotes,
			"downvotes": downvotes,
			"karma":     upvotes - downvotes,
			"updatedAt": time.Now(),
		},
	}

	result, err := m.Comments.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update comment votes: %v", err)
	}

	if result.MatchedCount == 0 {
		return utils.NewAppError(utils.ErrNotFound, "Comment not found", nil)
	}

	return nil
}

// Helper function to convert CommentDocument to models.Comment
func convertCommentDocumentToModel(doc *CommentDocument) (*models.Comment, error) {
	id, err := uuid.Parse(doc.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid comment ID: %v", err)
	}

	authorID, err := uuid.Parse(doc.AuthorID)
	if err != nil {
		return nil, fmt.Errorf("invalid author ID: %v", err)
	}

	postID, err := uuid.Parse(doc.PostID)
	if err != nil {
		return nil, fmt.Errorf("invalid post ID: %v", err)
	}

	subredditID, err := uuid.Parse(doc.SubredditID)
	if err != nil {
		return nil, fmt.Errorf("invalid subreddit ID: %v", err)
	}

	var parentID *uuid.UUID
	if doc.ParentID != nil {
		parsed, err := uuid.Parse(*doc.ParentID)
		if err != nil {
			return nil, fmt.Errorf("invalid parent ID: %v", err)
		}
		parentID = &parsed
	}

	children := make([]uuid.UUID, len(doc.Children))
	for i, childIDStr := range doc.Children {
		childID, err := uuid.Parse(childIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid child ID: %v", err)
		}
		children[i] = childID
	}

	return &models.Comment{
		ID:          id,
		Content:     doc.Content,
		AuthorID:    authorID,
		PostID:      postID,
		SubredditID: subredditID,
		ParentID:    parentID,
		Children:    children,
		CreatedAt:   doc.CreatedAt,
		UpdatedAt:   doc.UpdatedAt,
		IsDeleted:   doc.IsDeleted,
		Upvotes:     doc.Upvotes,
		Downvotes:   doc.Downvotes,
		Karma:       doc.Karma,
	}, nil
}

// EnsureCommentIndexes creates required indexes for the comments collection
func (m *MongoDB) EnsureCommentIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "postId", Value: 1},
				{Key: "createdAt", Value: -1},
			},
		},
		{
			Keys: bson.D{{Key: "authorId", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "parentId", Value: 1}},
		},
	}

	_, err := m.Comments.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("failed to create comment indexes: %v", err)
	}

	return nil
}

func (m *MongoDB) GetUserVoteOnComment(ctx context.Context, userID, commentID uuid.UUID) (bool, bool, error) {
	var vote VoteDocument
	err := m.Votes.FindOne(ctx, bson.M{
		"userId":    userID.String(),
		"commentId": commentID.String(),
	}).Decode(&vote)

	if err == mongo.ErrNoDocuments {
		return false, false, nil // No vote found
	}
	if err != nil {
		return false, false, err
	}

	return true, vote.IsUpvote, nil // Vote found, return the vote type
}

func (m *MongoDB) SaveCommentVote(ctx context.Context, userID, commentID uuid.UUID, isUpvote bool) error {
	now := time.Now()
	voteID := uuid.New().String()

	vote := VoteDocument{
		ID:        voteID,
		UserID:    userID.String(),
		CommentID: commentID.String(),
		IsUpvote:  isUpvote,
		CreatedAt: now,
		UpdatedAt: now,
	}

	opts := options.Update().SetUpsert(true)
	filter := bson.M{
		"userId":    userID.String(),
		"commentId": commentID.String(),
	}
	update := bson.M{"$set": vote}

	_, err := m.Votes.UpdateOne(ctx, filter, update, opts)
	return err
}

// Add this as a temporary fix method in comment_repository.go
func (m *MongoDB) FixCommentSubreddits(ctx context.Context) error {
	cursor, err := m.Comments.Find(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to fetch comments: %v", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var comment CommentDocument
		if err := cursor.Decode(&comment); err != nil {
			log.Printf("Error decoding comment: %v", err)
			continue
		}

		// Get the parent post to get its subredditID
		var post struct {
			SubredditID string `bson:"subredditid"`
		}
		err := m.Posts.FindOne(ctx, bson.M{"_id": comment.PostID}).Decode(&post)
		if err != nil {
			log.Printf("Error finding post for comment %s: %v", comment.ID, err)
			continue
		}

		// Update the comment with the subredditID
		_, err = m.Comments.UpdateOne(
			ctx,
			bson.M{"_id": comment.ID},
			bson.M{"$set": bson.M{"subredditId": post.SubredditID}},
		)
		if err != nil {
			log.Printf("Error updating comment %s: %v", comment.ID, err)
			continue
		}

		log.Printf("Updated comment %s with subredditID %s", comment.ID, post.SubredditID)
	}

	return nil
}
