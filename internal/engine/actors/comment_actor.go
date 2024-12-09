package actors

import (
	stdctx "context"
	"gator-swamp/internal/database"
	"gator-swamp/internal/models"
	"gator-swamp/internal/utils"
	"log"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// Message types for CommentActor
type (
	CreateCommentMsg struct {
		Content     string     `json:"content"`
		AuthorID    uuid.UUID  `json:"authorId"`
		PostID      uuid.UUID  `json:"postId"`
		SubredditID uuid.UUID  `json:"subredditId"`
		ParentID    *uuid.UUID `json:"parentId,omitempty"`
	}

	EditCommentMsg struct {
		CommentID uuid.UUID `json:"commentId"`
		AuthorID  uuid.UUID `json:"authorId"`
		Content   string    `json:"content"`
	}

	DeleteCommentMsg struct {
		CommentID uuid.UUID `json:"commentId"`
		AuthorID  uuid.UUID `json:"authorId"`
	}

	GetCommentMsg struct {
		CommentID uuid.UUID `json:"commentId"`
	}

	GetCommentsForPostMsg struct {
		PostID uuid.UUID `json:"postId"`
	}

	VoteCommentMsg struct {
		CommentID uuid.UUID `json:"commentId"`
		UserID    uuid.UUID `json:"userId"`
		IsUpvote  bool      `json:"isUpvote"`
	}

	loadCommentsFromDBMsg struct{}
)

// CommentActor manages comment operations
type CommentActor struct {
	comments     map[uuid.UUID]*models.Comment
	postComments map[uuid.UUID][]uuid.UUID
	commentVotes map[uuid.UUID]map[uuid.UUID]bool
	enginePID    *actor.PID
	mongodb      *database.MongoDB
}

func NewCommentActor(enginePID *actor.PID, mongodb *database.MongoDB) actor.Actor {
	return &CommentActor{
		comments:     make(map[uuid.UUID]*models.Comment),
		postComments: make(map[uuid.UUID][]uuid.UUID),
		commentVotes: make(map[uuid.UUID]map[uuid.UUID]bool),
		enginePID:    enginePID,
		mongodb:      mongodb,
	}
}

func (a *CommentActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		log.Printf("CommentActor started")
		context.Send(context.Self(), &loadCommentsFromDBMsg{})

	case *loadCommentsFromDBMsg:
		a.handleLoadComments(context)

	case *CreateCommentMsg:
		a.handleCreateComment(context, msg)

	case *EditCommentMsg:
		a.handleEditComment(context, msg)

	case *DeleteCommentMsg:
		a.handleDeleteComment(context, msg)

	case *GetCommentMsg:
		a.handleGetComment(context, msg)

	case *GetCommentsForPostMsg:
		a.handleGetPostComments(context, msg)

	case *VoteCommentMsg:
		a.handleVoteComment(context, msg)
	}
}

func (a *CommentActor) handleLoadComments(context actor.Context) {
	ctx := stdctx.Background()
	// Find all comments
	cursor, err := a.mongodb.Comments.Find(ctx, bson.M{})
	if err != nil {
		log.Printf("Error loading comments from MongoDB: %v", err)
		return
	}
	defer cursor.Close(ctx)

	// Iterate through cursor results
	for cursor.Next(ctx) {
		var doc database.CommentDocument
		if err := cursor.Decode(&doc); err != nil {
			log.Printf("Error decoding comment: %v", err)
			continue
		}

		// Get the parsed IDs
		id, _ := uuid.Parse(doc.ID)
		authorID, _ := uuid.Parse(doc.AuthorID)
		postID, _ := uuid.Parse(doc.PostID)

		// Parse parent ID if it exists
		var parentID *uuid.UUID
		if doc.ParentID != nil {
			parsed, _ := uuid.Parse(*doc.ParentID)
			parentID = &parsed
		}

		// Convert children string IDs to UUID
		children := make([]uuid.UUID, 0)
		for _, childStr := range doc.Children {
			if childID, err := uuid.Parse(childStr); err == nil {
				children = append(children, childID)
			}
		}

		// Create the comment
		comment := &models.Comment{
			ID:        id,
			Content:   doc.Content,
			AuthorID:  authorID,
			PostID:    postID,
			ParentID:  parentID,
			Children:  children,
			CreatedAt: doc.CreatedAt,
			UpdatedAt: doc.UpdatedAt,
			IsDeleted: doc.IsDeleted,
			Upvotes:   doc.Upvotes,
			Downvotes: doc.Downvotes,
			Karma:     doc.Karma,
		}

		// Update local caches
		a.comments[comment.ID] = comment
		a.postComments[comment.PostID] = append(a.postComments[comment.PostID], comment.ID)
		a.commentVotes[comment.ID] = make(map[uuid.UUID]bool)
	}

	log.Printf("Loaded %d comments from MongoDB", len(a.comments))
}

func (a *CommentActor) handleCreateComment(context actor.Context, msg *CreateCommentMsg) {
	now := time.Now()
	commentID := uuid.New()

	newComment := &models.Comment{
		ID:          commentID,
		Content:     msg.Content,
		AuthorID:    msg.AuthorID,
		PostID:      msg.PostID,
		SubredditID: msg.SubredditID,
		ParentID:    msg.ParentID,
		Children:    make([]uuid.UUID, 0),
		CreatedAt:   now,
		UpdatedAt:   now,
		IsDeleted:   false,
		Upvotes:     0,
		Downvotes:   0,
		Karma:       0,
	}

	// Save to MongoDB first
	ctx := stdctx.Background()
	if err := a.mongodb.SaveComment(ctx, newComment); err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to save comment", err))
		return
	}

	// Update local state after successful MongoDB save
	a.comments[commentID] = newComment
	a.postComments[msg.PostID] = append(a.postComments[msg.PostID], commentID)
	a.commentVotes[commentID] = make(map[uuid.UUID]bool)

	context.Respond(newComment)
}

func (a *CommentActor) handleEditComment(context actor.Context, msg *EditCommentMsg) {
	comment, exists := a.comments[msg.CommentID]
	if !exists {
		context.Respond(utils.NewAppError(utils.ErrNotFound, "Comment not found", nil))
		return
	}

	if comment.AuthorID != msg.AuthorID {
		context.Respond(utils.NewAppError(utils.ErrUnauthorized, "Not authorized to edit comment", nil))
		return
	}

	if comment.IsDeleted {
		context.Respond(utils.NewAppError(utils.ErrInvalidInput, "Cannot edit deleted comment", nil))
		return
	}

	comment.Content = msg.Content
	comment.UpdatedAt = time.Now()

	// Update in MongoDB
	ctx := stdctx.Background()
	if err := a.mongodb.SaveComment(ctx, comment); err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to update comment", err))
		return
	}

	context.Respond(comment)
}

func (a *CommentActor) handleDeleteComment(context actor.Context, msg *DeleteCommentMsg) {
	comment, exists := a.comments[msg.CommentID]
	if !exists {
		context.Respond(utils.NewAppError(utils.ErrNotFound, "Comment not found", nil))
		return
	}

	if comment.AuthorID != msg.AuthorID {
		context.Respond(utils.NewAppError(utils.ErrUnauthorized, "Not authorized to delete comment", nil))
		return
	}

	comment.IsDeleted = true
	comment.Content = "[deleted]"
	comment.UpdatedAt = time.Now()

	// Update in MongoDB
	ctx := stdctx.Background()
	if err := a.mongodb.SaveComment(ctx, comment); err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to delete comment", err))
		return
	}

	// Recursively handle child comments if any
	for _, childID := range comment.Children {
		a.deleteCommentAndChildren(context, childID)
	}

	context.Respond(true)
}

func (a *CommentActor) deleteCommentAndChildren(context actor.Context, commentID uuid.UUID) {
	if comment, exists := a.comments[commentID]; exists {
		comment.IsDeleted = true
		comment.Content = "[deleted]"
		comment.UpdatedAt = time.Now()

		// Update in MongoDB
		ctx := stdctx.Background()
		if err := a.mongodb.SaveComment(ctx, comment); err != nil {
			log.Printf("Error deleting child comment %s: %v", commentID, err)
			return
		}

		for _, childID := range comment.Children {
			a.deleteCommentAndChildren(context, childID)
		}
	}
}

func (a *CommentActor) handleGetComment(context actor.Context, msg *GetCommentMsg) {
	// Try cache first
	if comment, exists := a.comments[msg.CommentID]; exists {
		context.Respond(comment)
		return
	}

	// If not in cache, try MongoDB
	ctx := stdctx.Background()
	comment, err := a.mongodb.GetComment(ctx, msg.CommentID)
	if err != nil {
		if utils.IsErrorCode(err, utils.ErrNotFound) {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "Comment not found", nil))
			return
		}
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to get comment", err))
		return
	}

	// Update cache
	a.comments[comment.ID] = comment
	context.Respond(comment)
}

func (a *CommentActor) handleGetPostComments(context actor.Context, msg *GetCommentsForPostMsg) {
	ctx := stdctx.Background()
	comments, err := a.mongodb.GetPostComments(ctx, msg.PostID)
	if err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to get post comments", err))
		return
	}

	// Update cache
	for _, comment := range comments {
		a.comments[comment.ID] = comment
		if _, exists := a.postComments[msg.PostID]; !exists {
			a.postComments[msg.PostID] = make([]uuid.UUID, 0)
		}
		a.postComments[msg.PostID] = append(a.postComments[msg.PostID], comment.ID)
	}

	context.Respond(comments)
}

func (a *CommentActor) handleVoteComment(context actor.Context, msg *VoteCommentMsg) {
	comment, exists := a.comments[msg.CommentID]
	if !exists || comment.IsDeleted {
		context.Respond(utils.NewAppError(utils.ErrNotFound, "Comment not found", nil))
		return
	}

	if _, exists := a.commentVotes[msg.CommentID]; !exists {
		a.commentVotes[msg.CommentID] = make(map[uuid.UUID]bool)
	}

	previousVote, voted := a.commentVotes[msg.CommentID][msg.UserID]
	karmaChange := 0

	if voted {
		if previousVote == msg.IsUpvote {
			context.Respond(utils.NewAppError(utils.ErrDuplicate, "Already voted", nil))
			return
		}

		if msg.IsUpvote {
			comment.Downvotes--
			comment.Upvotes++
			karmaChange = 2
		} else {
			comment.Upvotes--
			comment.Downvotes++
			karmaChange = -2
		}
	} else {
		if msg.IsUpvote {
			comment.Upvotes++
			karmaChange = 1
		} else {
			comment.Downvotes++
			karmaChange = -1
		}
	}

	a.commentVotes[msg.CommentID][msg.UserID] = msg.IsUpvote
	comment.Karma = comment.Upvotes - comment.Downvotes

	// Update in MongoDB
	ctx := stdctx.Background()
	if err := a.mongodb.UpdateCommentVotes(ctx, msg.CommentID, comment.Upvotes, comment.Downvotes); err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to update vote", err))
		return
	}

	// Update author's karma
	if karmaChange != 0 {
		context.Send(a.enginePID, &UpdateKarmaMsg{
			UserID: comment.AuthorID,
			Delta:  karmaChange,
		})
	}

	context.Respond(comment)
}
