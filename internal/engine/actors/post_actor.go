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
	// "go.mongodb.org/mongo-driver/mongo" // No longer needed for this file's logic
)

// Message types for Post operations
type (
	CreatePostMsg struct {
		Title       string
		Content     string
		AuthorID    uuid.UUID
		SubredditID uuid.UUID
	}

	GetPostMsg struct {
		PostID           uuid.UUID
		RequestingUserID uuid.UUID
	}

	GetSubredditPostsMsg struct {
		SubredditID uuid.UUID
	}

	VotePostMsg struct {
		PostID     uuid.UUID
		UserID     uuid.UUID
		IsUpvote   bool
		RemoveVote bool // If true, vote is removed regardless of IsUpvote
	}

	GetUserFeedMsg struct {
		UserID           uuid.UUID `json:"userId"` // User whose feed is being requested
		Limit            int       `json:"limit"`
		Offset           int       `json:"offset"`
		RequestingUserID uuid.UUID `json:"requestingUserId"` // User making the request (for vote status)
	}

	DeletePostMsg struct {
		PostID uuid.UUID
		UserID uuid.UUID
	}

	// Internal messages for actor initialization and metrics
	GetCountsMsg           struct{}
	initializePostActorMsg struct{}
	loadPostsFromDBMsg     struct{}

	GetRecentPostsMsg struct {
		Limit            int       `json:"limit"`
		Offset           int       `json:"offset"`
		RequestingUserID uuid.UUID `json:"requestingUserId"`
	}
)

// PostActor manages posts and related operations.
type PostActor struct {
	postsByID       map[uuid.UUID]*models.Post // Cache for posts by their ID
	subredditPosts  map[uuid.UUID][]uuid.UUID  // Mapping of subreddit IDs to their posts
	metrics         *utils.MetricsCollector    // Metrics for performance tracking
	enginePID       *actor.PID                 // Reference to the Engine actor
	db              database.DBAdapter         // Database adapter interface
	commentActorPID *actor.PID                 // PID of the CommentActor for interaction
}

// NewPostActor creates a new PostActor instance
func NewPostActor(metrics *utils.MetricsCollector, enginePID *actor.PID, db database.DBAdapter, commentActorPID *actor.PID) actor.Actor {
	return &PostActor{
		postsByID:       make(map[uuid.UUID]*models.Post),
		subredditPosts:  make(map[uuid.UUID][]uuid.UUID),
		metrics:         metrics,
		enginePID:       enginePID,
		db:              db,
		commentActorPID: commentActorPID,
	}
}

// Receive handles incoming messages for the PostActor
func (a *PostActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		log.Printf("PostActor started")
		context.Send(context.Self(), &initializePostActorMsg{}) // Start initialization

	case *initializePostActorMsg:
		context.Send(context.Self(), &loadPostsFromDBMsg{}) // Trigger loading posts from DB

	case *loadPostsFromDBMsg:
		a.handleLoadPosts(context)

	case *CreatePostMsg:
		a.handleCreatePost(context, msg)

	case *GetPostMsg:
		a.handleGetPost(context, msg)

	case *GetSubredditPostsMsg:
		a.handleGetSubredditPosts(context, msg)

	case *VotePostMsg:
		a.handleVote(context, msg)

	case *GetUserFeedMsg:
		a.handleGetUserFeed(context, msg)
	case *GetRecentPostsMsg:
		a.handleGetRecentPosts(context, msg)

	default:
		log.Printf("PostActor: Unknown message type: %T", msg)
	}
}

// Handles loading all posts from DB into memory during initialization
func (a *PostActor) handleLoadPosts(context actor.Context) {
	log.Println("PostActor: Loading initial posts from database...")
	ctx := stdctx.Background()

	posts, err := a.db.GetAllPosts(ctx)
	if err != nil {
		log.Printf("PostActor: CRITICAL - Failed to load initial posts: %v", err)
		// Consider how to handle this - retry? panic? For now, log and continue with empty cache.
		return
	}

	loadedCount := 0
	for _, post := range posts {
		// Populate derived fields (essential for cache consistency if used directly)
		// We need the actor context for getCommentCount
		if err := a.populatePostDetails(ctx, context, post); err != nil {
			log.Printf("PostActor: Warning - Failed to populate details for post %s during initial load: %v", post.ID, err)
			// Continue caching the post even if details are incomplete
		}

		a.postsByID[post.ID] = post
		if _, ok := a.subredditPosts[post.SubredditID]; !ok {
			a.subredditPosts[post.SubredditID] = make([]uuid.UUID, 0)
		}
		a.subredditPosts[post.SubredditID] = append(a.subredditPosts[post.SubredditID], post.ID)
		loadedCount++
	}

	log.Printf("PostActor: Finished loading %d posts into cache.", loadedCount)
}

// Handles creating a new post
func (a *PostActor) handleCreatePost(context actor.Context, msg *CreatePostMsg) {
	startTime := time.Now()
	ctx := stdctx.Background()

	// Fetch the user to get their username
	user, err := a.db.GetUser(ctx, msg.AuthorID)
	if err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch author details", err))
		return
	}

	// Fetch the subreddit to get its name
	subreddit, err := a.db.GetSubredditByID(ctx, msg.SubredditID)
	if err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch subreddit details", err))
		return
	}

	newPost := &models.Post{
		ID:             uuid.New(),
		Title:          msg.Title,
		Content:        msg.Content,
		AuthorID:       msg.AuthorID,
		AuthorUsername: user.Username, // Populated from fetched user
		SubredditID:    msg.SubredditID,
		SubredditName:  subreddit.Name, // Populated from fetched subreddit
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(), // Initialize UpdatedAt
		Karma:          1,          // Start with 1 karma (initial upvote from author?)
		CommentCount:   0,
		// UserVotes field removed
	}

	if err := a.db.SavePost(ctx, newPost); err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to save post", err))
		return
	}

	// TODO: Consider if author should automatically upvote their own post via RecordVote?
	// For now, just save the post with karma 1.

	// Update local caches
	a.postsByID[newPost.ID] = newPost
	// a.postVotes[newPost.ID] = make(map[uuid.UUID]voteStatus) // REMOVED
	a.subredditPosts[msg.SubredditID] = append(a.subredditPosts[msg.SubredditID], newPost.ID)

	a.metrics.AddOperationLatency("create_post", time.Since(startTime))
	context.Respond(newPost)
}

// Handles retrieving a specific post by ID
func (a *PostActor) handleGetPost(context actor.Context, msg *GetPostMsg) {
	// Prefer cache, but fallback to DB
	// NOTE: Cache does not currently store user-specific vote status.
	// If cache hits, the CurrentUserVote will be nil. A DB refetch is needed for this.
	// Consider invalidating cache more aggressively or enhancing cache structure.
	if post, exists := a.postsByID[msg.PostID]; exists {
		// Temporarily, we will still fetch from DB if requesting user is provided
		// to get their vote status, even if the post is cached.
		// A better approach would be to store vote status separately or enhance the post cache.
		if msg.RequestingUserID != uuid.Nil {
			// Fall through to DB fetch to get user-specific vote status
		} else {
			// Populate derived fields for cached post (without user vote)
			if err := a.populatePostDetails(stdctx.Background(), context, post); err != nil {
				log.Printf("Error populating cached post %s details: %v", msg.PostID, err)
			}
			context.Respond(post) // Respond with cached post (no user vote info)
			return
		}
	}

	ctx := stdctx.Background()
	// Modified DB call to include requesting user ID
	post, err := a.db.GetPost(ctx, msg.PostID, msg.RequestingUserID)
	if err != nil {
		if appErr, ok := err.(*utils.AppError); ok && appErr.Code == utils.ErrNotFound {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "Post not found", nil))
		} else {
			context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch post", err))
		}
		return
	}

	// Populate derived fields for DB-fetched post
	if err := a.populatePostDetails(ctx, context, post); err != nil {
		log.Printf("Error populating fetched post %s details: %v", msg.PostID, err)
		// Respond with post data anyway, but maybe log error
	}

	// Cache the fetched post
	a.postsByID[post.ID] = post
	// Initialize subreddit post list if needed
	if _, ok := a.subredditPosts[post.SubredditID]; !ok {
		a.subredditPosts[post.SubredditID] = make([]uuid.UUID, 0)
	}
	// Avoid adding duplicate if already present (edge case)
	found := false
	for _, existingID := range a.subredditPosts[post.SubredditID] {
		if existingID == post.ID {
			found = true
			break
		}
	}
	if !found {
		a.subredditPosts[post.SubredditID] = append(a.subredditPosts[post.SubredditID], post.ID)
	}

	context.Respond(post)
}

// Handles retrieving posts for a specific subreddit
func (a *PostActor) handleGetSubredditPosts(context actor.Context, msg *GetSubredditPostsMsg) {
	log.Printf("Getting posts for subreddit %s", msg.SubredditID)
	ctx := stdctx.Background()

	// Need to define defaults or add pagination to msg
	defaultLimit := 50 // Example limit
	defaultOffset := 0 // Example offset

	posts, err := a.db.GetPostsBySubreddit(ctx, msg.SubredditID, defaultLimit, defaultOffset)
	if err != nil {
		log.Printf("Error fetching posts for subreddit %s from DB: %v", msg.SubredditID, err)
		// Use NewAppError for consistency
		context.Respond(utils.NewAppError(utils.ErrDatabase, "failed to fetch subreddit posts", err))
		return
	}

	// Populate derived fields for each post
	for _, post := range posts {
		if err := a.populatePostDetails(ctx, context, post); err != nil {
			log.Printf("Error populating details for post %s in subreddit %s feed: %v", post.ID, msg.SubredditID, err)
			// Continue with potentially incomplete post data
		}
	}

	context.Respond(posts)
}

// Handles voting on a post using the DBAdapter
func (a *PostActor) handleVote(context actor.Context, msg *VotePostMsg) {
	startTime := time.Now()
	ctx := stdctx.Background()

	var direction models.VoteDirection
	if msg.RemoveVote {
		direction = models.VoteNone
	} else if msg.IsUpvote {
		direction = models.VoteUp
	} else {
		direction = models.VoteDown
	}

	err := a.db.RecordVote(ctx, msg.UserID, msg.PostID, models.PostVote, direction)
	if err != nil {
		log.Printf("Error recording vote for post %s by user %s: %v", msg.PostID, msg.UserID, err)
		// Use NewAppError instead of WrapAppError
		context.Respond(utils.NewAppError(utils.ErrDatabase, "failed to process vote", err))
		return
	}

	// Invalidate or update cache? For now, let's invalidate.
	delete(a.postsByID, msg.PostID)

	a.metrics.AddOperationLatency("vote_post", time.Since(startTime))
	context.Respond(&struct{ Success bool }{Success: true}) // Simple success response
}

// Handles retrieving a personalized feed for a user
func (a *PostActor) handleGetUserFeed(context actor.Context, msg *GetUserFeedMsg) {
	log.Printf("Generating feed for user %s, limit %d, offset %d, requesting user %s", msg.UserID, msg.Limit, msg.Offset, msg.RequestingUserID)
	ctx := stdctx.Background()

	posts, err := a.db.GetUserFeed(ctx, msg.UserID, msg.Limit, msg.Offset, msg.RequestingUserID)
	if err != nil {
		log.Printf("Error fetching user feed for %s: %v", msg.UserID, err)
		context.Respond(utils.NewAppError(utils.ErrDatabase, "failed to fetch user feed", err))
		return
	}

	context.Respond(posts)
}

// Handles retrieving the most recent posts
func (a *PostActor) handleGetRecentPosts(context actor.Context, msg *GetRecentPostsMsg) {
	log.Printf("PostActor: Received GetRecentPostsMsg: Limit=%d, Offset=%d, RequestingUserID=%s", msg.Limit, msg.Offset, msg.RequestingUserID)
	ctx := stdctx.Background()
	posts, err := a.db.GetRecentPosts(ctx, msg.Limit, msg.Offset, msg.RequestingUserID)
	if err != nil {
		log.Printf("PostActor: Error getting recent posts: %v", err)
		context.Respond(utils.NewAppError(utils.ErrDatabase, "failed to fetch recent posts", err))
		return
	}

	context.Respond(posts)
}

// populatePostDetails fetches author username, subreddit name.
// Comment count is now assumed to be up-to-date from the database.
func (a *PostActor) populatePostDetails(ctx stdctx.Context, context actor.Context, post *models.Post) error {
	// Fetch author username
	author, err := a.db.GetUser(ctx, post.AuthorID)
	if err != nil {
		// Log error but don't fail entirely, maybe author was deleted
		log.Printf("Warning: Failed to fetch author %s for post %s: %v", post.AuthorID, post.ID, err)
		post.AuthorUsername = "[deleted]"
	} else {
		post.AuthorUsername = author.Username
	}

	// Fetch subreddit name
	subreddit, err := a.db.GetSubredditByID(ctx, post.SubredditID)
	if err != nil {
		// Log error but don't fail entirely
		log.Printf("Warning: Failed to fetch subreddit %s for post %s: %v", post.SubredditID, post.ID, err)
		post.SubredditName = "[unknown]"
	} else {
		post.SubredditName = subreddit.Name
	}

	// Comment count is now sourced directly from the database query (e.g., in GetPost, GetRecentPosts)
	// and should be up-to-date due to transactional updates in SaveComment and DeleteCommentAndDecrementCount.
	// Thus, no need to call a.getCommentCount(context, post.ID) here anymore.

	// Note: Upvotes/Downvotes are not populated here as they aren't stored directly.
	// Karma is fetched from DB.
	return nil
}
