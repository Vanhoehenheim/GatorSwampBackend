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
		PostID           uuid.UUID `json:"postId"`
		RequestingUserID uuid.UUID `json:"requestingUserId,omitempty"`
	}

	VoteCommentMsg struct {
		CommentID  uuid.UUID `json:"commentId"`
		UserID     uuid.UUID `json:"userId"`
		IsUpvote   bool      `json:"isUpvote"`
		RemoveVote bool      `json:"removeVote"`
	}

	GetCommentCountMsg struct {
		PostID uuid.UUID `json:"postId"`
	}

	loadCommentsFromDBMsg struct{}
)

// CommentActor manages comment operations
type CommentActor struct {
	comments     map[uuid.UUID]*models.Comment
	postComments map[uuid.UUID][]uuid.UUID
	enginePID    *actor.PID
	db           database.DBAdapter
	userCache    map[uuid.UUID]string // Simple cache for usernames
}

func NewCommentActor(enginePID *actor.PID, db database.DBAdapter) actor.Actor {
	return &CommentActor{
		comments:     make(map[uuid.UUID]*models.Comment),
		postComments: make(map[uuid.UUID][]uuid.UUID),
		enginePID:    enginePID,
		db:           db,
		userCache:    make(map[uuid.UUID]string), // Initialize user cache
	}
}

func (a *CommentActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		log.Printf("CommentActor started with PID: %v", context.Self())
		context.Send(context.Self(), &loadCommentsFromDBMsg{})

	case *loadCommentsFromDBMsg:
		log.Printf("Loading comments from database")
		a.handleLoadComments(context)

	case *CreateCommentMsg:
		log.Printf("Received CreateCommentMsg: %+v", msg)
		a.handleCreateComment(context, msg)
		log.Printf("Finished handling CreateCommentMsg")

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

	case *GetCommentCountMsg:
		a.handleGetCommentCount(context, msg)

	default:
		log.Printf("CommentActor: Unknown message type %T", msg)
	}
}

// Helper function to get username, using cache first
func (a *CommentActor) getUsername(ctx stdctx.Context, userID uuid.UUID) string {
	if username, ok := a.userCache[userID]; ok {
		return username
	}

	user, err := a.db.GetUser(ctx, userID)
	if err != nil {
		log.Printf("Error fetching user %s for username: %v", userID, err)
		return "[unknown]" // Return placeholder on error
	}

	// Cache the username
	a.userCache[userID] = user.Username
	return user.Username
}

// Helper function to populate usernames for a slice of comments
func (a *CommentActor) populateUsernames(ctx stdctx.Context, comments []*models.Comment) {
	for _, comment := range comments {
		if comment.AuthorUsername == "" { // Populate only if missing
			comment.AuthorUsername = a.getUsername(ctx, comment.AuthorID)
		}
	}
}

func (a *CommentActor) handleLoadComments(context actor.Context) {
	log.Println("CommentActor: Loading initial comments from database...")
	ctx := stdctx.Background()

	comments, err := a.db.GetAllComments(ctx)
	if err != nil {
		log.Printf("CommentActor: CRITICAL - Failed to load initial comments: %v", err)
		return
	}

	// Populate usernames after loading
	a.populateUsernames(ctx, comments)

	loadedCount := 0
	for _, comment := range comments {
		// Username should now be populated by populateUsernames
		a.comments[comment.ID] = comment
		if _, ok := a.postComments[comment.PostID]; !ok {
			a.postComments[comment.PostID] = make([]uuid.UUID, 0)
		}
		// Avoid duplicates in postComments list if loaded multiple times?
		found := false
		for _, existingID := range a.postComments[comment.PostID] {
			if existingID == comment.ID {
				found = true
				break
			}
		}
		if !found {
			a.postComments[comment.PostID] = append(a.postComments[comment.PostID], comment.ID)
		}
		loadedCount++
	}

	log.Printf("CommentActor: Finished loading %d comments into cache.", loadedCount)
}

func (a *CommentActor) handleCreateComment(context actor.Context, msg *CreateCommentMsg) {
	// Add initial logging
	log.Printf("Creating new comment for post %s by user %s", msg.PostID, msg.AuthorID)

	// First, fetch the post to get its subredditID
	ctx := stdctx.Background()
	// Pass uuid.Nil as requestingUserID, as we only need subredditID here
	post, err := a.db.GetPost(ctx, msg.PostID, uuid.Nil)
	if err != nil {
		log.Printf("Error fetching post: %v", err)
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch parent post", err))
		return
	}

	// Fetch the user to get their username
	user, err := a.db.GetUser(ctx, msg.AuthorID)
	if err != nil {
		log.Printf("Error fetching user: %v", err)
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch author details", err))
		return
	}

	now := time.Now()
	commentID := uuid.New()
	log.Printf("Generated new comment ID: %s", commentID)

	newComment := &models.Comment{
		ID:             commentID,
		Content:        msg.Content,
		AuthorID:       msg.AuthorID,
		AuthorUsername: user.Username,
		PostID:         msg.PostID,
		SubredditID:    post.SubredditID,
		ParentID:       msg.ParentID,
		Children:       make([]uuid.UUID, 0),
		CreatedAt:      now,
		UpdatedAt:      now,
		IsDeleted:      false,
		Karma:          1, // Start with 1 karma (author's implicit upvote?)
	}
	if msg.ParentID != nil {
		log.Printf("This is a reply to comment ID: %s", msg.ParentID.String())

		parentComment, err := a.db.GetComment(ctx, *msg.ParentID)
		if err != nil {
			log.Printf("Error fetching parent comment: %v", err)
			if utils.IsErrorCode(err, utils.ErrNotFound) {
				context.Respond(utils.NewAppError(utils.ErrNotFound, "Parent comment not found", nil))
			} else {
				context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch parent comment", err))
			}
			return
		}

		// Update parent's children array
		parentComment.Children = append(parentComment.Children, commentID)
		parentComment.UpdatedAt = now

		// Temporarily comment out saving the parent to isolate the issue
		/*
			// Save updated parent comment
			if err := a.db.SaveComment(ctx, parentComment); err != nil {
				log.Printf("Error updating parent comment: %v", err)
				context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to update parent comment", err))
				return
			}
		*/

		// Update cache
		a.comments[parentComment.ID] = parentComment
	}

	// Add log right before saving the NEW comment
	log.Printf("Before saving NEW comment %s, ParentID is: %v", newComment.ID, newComment.ParentID)
	// Save the new comment
	if err := a.db.SaveComment(ctx, newComment); err != nil {
		log.Printf("Error saving comment to database: %v", err)
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to save comment", err))
		return
	}

	// Update local cache for the new comment
	a.comments[commentID] = newComment
	a.postComments[msg.PostID] = append(a.postComments[msg.PostID], commentID)

	// Create response
	response := struct {
		ID             string    `json:"id"`
		Content        string    `json:"content"`
		AuthorID       string    `json:"authorId"`
		AuthorUsername string    `json:"authorUsername"`
		PostID         string    `json:"postId"`
		SubredditID    string    `json:"subredditId"`
		ParentID       *string   `json:"parentId,omitempty"`
		Children       []string  `json:"children"`
		CreatedAt      time.Time `json:"createdAt"`
		UpdatedAt      time.Time `json:"updatedAt"`
		IsDeleted      bool      `json:"isDeleted"`
		Karma          int       `json:"karma"`
	}{
		ID:             newComment.ID.String(),
		Content:        newComment.Content,
		AuthorID:       newComment.AuthorID.String(),
		AuthorUsername: newComment.AuthorUsername,
		PostID:         newComment.PostID.String(),
		SubredditID:    newComment.SubredditID.String(),
		Children:       make([]string, 0),
		CreatedAt:      newComment.CreatedAt,
		UpdatedAt:      newComment.UpdatedAt,
		IsDeleted:      newComment.IsDeleted,
		Karma:          newComment.Karma,
	}

	if newComment.ParentID != nil {
		parentIDStr := newComment.ParentID.String()
		response.ParentID = &parentIDStr
	}

	log.Printf("Successfully created comment with ID: %s", commentID)
	// Log the response struct right before sending
	log.Printf("Responding to handler with: %+v", response)
	context.Respond(response)
}

// If this is a reply to another comment, update the parent comment's children array

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

	// Update in database
	ctx := stdctx.Background()
	if err := a.db.SaveComment(ctx, comment); err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to update comment", err))
		return
	}

	context.Respond(comment)
}

func (a *CommentActor) handleDeleteComment(context actor.Context, msg *DeleteCommentMsg) {
	ctx := stdctx.Background()
	log.Printf("Attempting to delete comment ID: %s by user %s", msg.CommentID, msg.AuthorID)

	// Optional: Fetch the comment to verify authorship before deleting
	comment, err := a.db.GetComment(ctx, msg.CommentID)
	if err != nil {
		if utils.IsErrorCode(err, utils.ErrNotFound) {
			log.Printf("Comment %s not found for deletion.", msg.CommentID)
			context.Respond(utils.NewAppError(utils.ErrNotFound, "Comment not found", nil))
			return
		}
		log.Printf("Error fetching comment %s for deletion: %v", msg.CommentID, err)
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch comment for deletion", err))
		return
	}

	if comment.AuthorID != msg.AuthorID {
		log.Printf("User %s unauthorized to delete comment %s (author is %s)", msg.AuthorID, msg.CommentID, comment.AuthorID)
		context.Respond(utils.NewAppError(utils.ErrUnauthorized, "User not authorized to delete this comment", nil))
		return
	}

	// Perform hard delete using the new database function
	err = a.db.DeleteCommentAndDecrementCount(ctx, msg.CommentID)
	if err != nil {
		// Log the detailed error from the DB layer
		log.Printf("Error during DeleteCommentAndDecrementCount for comment %s: %v", msg.CommentID, err)
		// Respond with the error passed up from the DB layer
		context.Respond(err) // err from DB should already be an AppError or wrapped
		return
	}

	// If successful, update local caches (if any)
	delete(a.comments, msg.CommentID)
	if comment.PostID != uuid.Nil {
		if postCommentIDs, ok := a.postComments[comment.PostID]; ok {
			newPostCommentIDs := make([]uuid.UUID, 0, len(postCommentIDs)-1)
			for _, id := range postCommentIDs {
				if id != msg.CommentID {
					newPostCommentIDs = append(newPostCommentIDs, id)
				}
			}
			a.postComments[comment.PostID] = newPostCommentIDs
		}
	}
	// TODO: Handle recursive deletion of child comments if required.
	// The current `deleteCommentAndChildren` logic would need to be adapted
	// to use `DeleteCommentAndDecrementCount` for each child as well.
	// For now, this commit only handles the direct deletion of the specified comment.

	log.Printf("Successfully deleted comment ID: %s and updated post count.", msg.CommentID)
	context.Respond(&models.StatusResponse{Success: true, Message: "Comment deleted successfully"})
}

// deleteCommentAndChildren recursively sets IsDeleted flag on a comment and its children.
// THIS FUNCTION NEEDS TO BE REVISITED if hard deletes are fully implemented for children.
// Currently, it sets a model field that isn't persisted as 'is_deleted' in the DB.

func (a *CommentActor) handleGetComment(context actor.Context, msg *GetCommentMsg) {
	// Try cache first
	if comment, exists := a.comments[msg.CommentID]; exists {
		context.Respond(comment)
		return
	}

	// If not in cache, try database
	ctx := stdctx.Background()
	comment, err := a.db.GetComment(ctx, msg.CommentID)
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

// handleGetPostComments retrieves comments for a post, fetching from DB if needed.
func (a *CommentActor) handleGetPostComments(context actor.Context, msg *GetCommentsForPostMsg) {
	ctx := stdctx.Background()
	log.Printf("Fetching comments for post %s, requesting user %s", msg.PostID, msg.RequestingUserID)

	// Pass RequestingUserID to the database method
	comments, err := a.db.GetPostComments(ctx, msg.PostID, msg.RequestingUserID)
	if err != nil {
		log.Printf("Error fetching comments for post %s: %v", msg.PostID, err)
		// Check for specific error types if necessary, e.g., utils.IsErrorCode
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch comments", err))
		return
	}

	// Populate usernames for the comments
	a.populateUsernames(ctx, comments)

	// Update cache (optional, consider if this is the source of truth or if DB is always queried)
	// For simplicity, we assume the DB query is the most up-to-date source for this specific request.
	// If caching is implemented for this, ensure it handles user-specific data like CurrentUserVote correctly.

	log.Printf("Fetched %d comments for post %s", len(comments), msg.PostID)
	context.Respond(comments)
}

func (a *CommentActor) handleVoteComment(context actor.Context, msg *VoteCommentMsg) {
	ctx := stdctx.Background()

	var direction models.VoteDirection
	if msg.RemoveVote {
		direction = models.VoteNone
	} else if msg.IsUpvote {
		direction = models.VoteUp
	} else {
		direction = models.VoteDown
	}

	err := a.db.RecordVote(ctx, msg.UserID, msg.CommentID, models.CommentVote, direction)
	if err != nil {
		log.Printf("Error recording vote for comment %s by user %s: %v", msg.CommentID, msg.UserID, err)
		context.Respond(utils.NewAppError(utils.ErrDatabase, "failed to process comment vote", err))
		return
	}

	// Invalidate comment cache entry
	delete(a.comments, msg.CommentID)

	context.Respond(&struct{ Success bool }{Success: true})
}

// handleGetCommentCount handles requests for comment counts (from PostActor)
func (a *CommentActor) handleGetCommentCount(context actor.Context, msg *GetCommentCountMsg) {
	// This can be optimized. Instead of loading all comments, maybe query DB directly.
	// For now, use the cache.
	count := 0
	if ids, ok := a.postComments[msg.PostID]; ok {
		count = len(ids)
		// We might want to filter out deleted comments if the cache holds them
		// filteredCount := 0
		// for _, id := range ids {
		// 	if c, exists := a.comments[id]; exists && !c.IsDeleted {
		// 		filteredCount++
		// 	}
		// }
		// count = filteredCount
	}
	context.Respond(count)
}
