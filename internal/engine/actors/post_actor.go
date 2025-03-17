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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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
		PostID uuid.UUID
	}

	GetSubredditPostsMsg struct {
		SubredditID uuid.UUID
	}

	VotePostMsg struct {
		PostID     uuid.UUID
		UserID     uuid.UUID
		IsUpvote   bool
		RemoveVote bool // New field to support vote toggling
	}

	GetUserFeedMsg struct {
		UserID uuid.UUID
		Limit  int
	}

	DeletePostMsg struct {
		PostID uuid.UUID
		UserID uuid.UUID
	}

	// Internal messages for actor initialization and metrics
	GetCountsMsg           struct{}
	initializePostActorMsg struct{}
	loadPostsFromDBMsg     struct{}

	// Internal struct for tracking votes
	voteStatus struct {
		IsUpvote bool
		VotedAt  time.Time
	}

	GetRecentPostsMsg struct {
		Limit int
	}
)

// PostActor handles post-related operations
type PostActor struct {
	postsByID       map[uuid.UUID]*models.Post             // Cache for posts by their ID
	subredditPosts  map[uuid.UUID][]uuid.UUID              // Mapping of subreddit IDs to their posts
	postVotes       map[uuid.UUID]map[uuid.UUID]voteStatus // Tracking user votes for posts
	metrics         *utils.MetricsCollector                // Metrics for performance tracking
	enginePID       *actor.PID                             // Reference to the Engine actor
	mongodb         *database.MongoDB                      // MongoDB client
	commentActorPID *actor.PID                             // Reference to the Comment actor
}

// NewPostActor creates a new PostActor instance
func NewPostActor(metrics *utils.MetricsCollector, enginePID *actor.PID, mongodb *database.MongoDB, commentActorPID *actor.PID) actor.Actor {
	return &PostActor{
		postsByID:       make(map[uuid.UUID]*models.Post),
		subredditPosts:  make(map[uuid.UUID][]uuid.UUID),
		postVotes:       make(map[uuid.UUID]map[uuid.UUID]voteStatus),
		metrics:         metrics,
		enginePID:       enginePID,
		mongodb:         mongodb,
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

// Handles loading all posts from MongoDB into memory during initialization
func (a *PostActor) handleLoadPosts(context actor.Context) {
	ctx := stdctx.Background()

	cursor, err := a.mongodb.Posts.Find(ctx, bson.M{})
	if err != nil {
		log.Printf("Error loading posts from MongoDB: %v", err)
		return
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var doc database.PostDocument
		if err := cursor.Decode(&doc); err != nil {
			log.Printf("Error decoding post document: %v", err)
			continue
		}

		post, err := a.mongodb.DocumentToModel(&doc)
		if err != nil {
			log.Printf("Error converting document to model: %v", err)
			continue
		}

		a.postsByID[post.ID] = post
		a.postVotes[post.ID] = make(map[uuid.UUID]voteStatus)
		a.subredditPosts[post.SubredditID] = append(a.subredditPosts[post.SubredditID], post.ID)
	}

	log.Printf("Loaded %d posts from MongoDB", len(a.postsByID))
}

// Handles creating a new post
func (a *PostActor) handleCreatePost(context actor.Context, msg *CreatePostMsg) {
	startTime := time.Now()
	ctx := stdctx.Background()

	// Fetch the user to get their username
	user, err := a.mongodb.GetUser(ctx, msg.AuthorID)
	if err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch author details", err))
		return
	}

	// Fetch the subreddit to get its name
	subreddit, err := a.mongodb.GetSubredditByID(ctx, msg.SubredditID)
	if err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch subreddit details", err))
		return
	}

	newPost := &models.Post{
		ID:             uuid.New(),
		Title:          msg.Title,
		Content:        msg.Content,
		AuthorID:       msg.AuthorID,
		AuthorUsername: user.Username,
		SubredditID:    msg.SubredditID,
		SubredditName:  subreddit.Name,
		CreatedAt:      time.Now(),
		Upvotes:        0,
		Downvotes:      0,
		Karma:          0,
		UserVotes:      make(map[string]bool), // Initialize UserVotes map
	}

	postDoc := a.mongodb.ModelToDocument(newPost)
	if _, err := a.mongodb.Posts.InsertOne(ctx, postDoc); err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to save post", err))
		return
	}

	// Update local caches and respond as before
	a.postsByID[newPost.ID] = newPost
	a.postVotes[newPost.ID] = make(map[uuid.UUID]voteStatus)
	a.subredditPosts[msg.SubredditID] = append(a.subredditPosts[msg.SubredditID], newPost.ID)

	a.metrics.AddOperationLatency("create_post", time.Since(startTime))
	context.Respond(newPost)
}

// Handles retrieving a specific post by ID
func (a *PostActor) handleGetPost(context actor.Context, msg *GetPostMsg) {
	if post, exists := a.postsByID[msg.PostID]; exists {
		// Ensure UserVotes is initialized
		if post.UserVotes == nil {
			post.UserVotes = make(map[string]bool)
		}

		// Count comments for this post
		commentCount, err := a.getCommentCount(msg.PostID)
		if err != nil {
			log.Printf("Error counting comments for post %s: %v", msg.PostID, err)
		} else {
			post.CommentCount = commentCount
		}

		context.Respond(post)
		return
	}

	ctx := stdctx.Background()
	post, err := a.mongodb.GetPost(ctx, msg.PostID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "Post not found", nil))
		} else {
			context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch post", err))
		}
		return
	}

	// Ensure UserVotes is initialized
	if post.UserVotes == nil {
		post.UserVotes = make(map[string]bool)
	}

	// Count comments for this post
	commentCount, err := a.getCommentCount(msg.PostID)
	if err != nil {
		log.Printf("Error counting comments for post %s: %v", msg.PostID, err)
	} else {
		post.CommentCount = commentCount
	}

	a.postsByID[post.ID] = post
	a.postVotes[post.ID] = make(map[uuid.UUID]voteStatus)
	a.subredditPosts[post.SubredditID] = append(a.subredditPosts[post.SubredditID], post.ID)

	context.Respond(post)
}

// Handles retrieving all posts for a subreddit
func (a *PostActor) handleGetSubredditPosts(context actor.Context, msg *GetSubredditPostsMsg) {
	log.Printf("Fetching posts for subreddit: %s", msg.SubredditID)

	// Query MongoDB directly for the latest data
	ctx := stdctx.Background()
	posts, err := a.mongodb.GetSubredditPosts(ctx, msg.SubredditID)
	if err != nil {
		log.Printf("Error fetching subreddit posts: %v", err)
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch subreddit posts", err))
		return
	}

	if len(posts) == 0 {
		log.Printf("No posts found for subreddit: %s", msg.SubredditID)
		context.Respond([]*models.Post{}) // Return empty array instead of error
		return
	}

	// Update local cache with fetched posts
	for _, post := range posts {
		a.postsByID[post.ID] = post
		if _, exists := a.postVotes[post.ID]; !exists {
			a.postVotes[post.ID] = make(map[uuid.UUID]voteStatus)
		}

		// Count comments for each post
		commentCount, err := a.getCommentCount(post.ID)
		if err != nil {
			log.Printf("Error counting comments for post %s: %v", post.ID, err)
		} else {
			post.CommentCount = commentCount
		}
	}

	log.Printf("Found %d posts for subreddit: %s", len(posts), msg.SubredditID)
	context.Respond(posts)
}

// Handles voting on a post
func (a *PostActor) handleVote(context actor.Context, msg *VotePostMsg) {
	startTime := time.Now()

	post, exists := a.postsByID[msg.PostID]
	if !exists {
		context.Respond(utils.NewAppError(utils.ErrNotFound, "Post not found", nil))
		return
	}

	if _, exists := a.postVotes[msg.PostID]; !exists {
		a.postVotes[msg.PostID] = make(map[uuid.UUID]voteStatus)
	}

	previousVote, hasVoted := a.postVotes[msg.PostID][msg.UserID]

	// Calculate vote changes
	upvoteDelta := 0
	downvoteDelta := 0

	// Handle the different voting scenarios
	if hasVoted {
		if msg.RemoveVote && previousVote.IsUpvote == msg.IsUpvote {
			// Remove an existing vote if the user is clicking the same button again
			if msg.IsUpvote {
				upvoteDelta = -1
				post.Upvotes--
			} else {
				downvoteDelta = -1
				post.Downvotes--
			}
			// Delete the vote record
			delete(a.postVotes[msg.PostID], msg.UserID)
		} else if previousVote.IsUpvote == msg.IsUpvote {
			// This is a duplicate vote without the RemoveVote flag - keep old behavior
			context.Respond(utils.NewAppError(utils.ErrDuplicate, "Already voted", nil))
			return
		} else {
			// Changing from upvote to downvote or vice versa
			if msg.IsUpvote {
				upvoteDelta = 1
				downvoteDelta = -1
				post.Downvotes--
				post.Upvotes++
			} else {
				upvoteDelta = -1
				downvoteDelta = 1
				post.Upvotes--
				post.Downvotes++
			}
			// Update vote record
			a.postVotes[msg.PostID][msg.UserID] = voteStatus{
				IsUpvote: msg.IsUpvote,
				VotedAt:  time.Now(),
			}
		}
	} else {
		// New vote
		if msg.IsUpvote {
			upvoteDelta = 1
			post.Upvotes++
		} else {
			downvoteDelta = 1
			post.Downvotes++
		}
		// Create new vote record (only if not removing a non-existent vote)
		if !msg.RemoveVote {
			a.postVotes[msg.PostID][msg.UserID] = voteStatus{
				IsUpvote: msg.IsUpvote,
				VotedAt:  time.Now(),
			}
		}
	}

	// Update post karma
	post.Karma = post.Upvotes - post.Downvotes

	// Update MongoDB
	ctx := stdctx.Background()
	err := a.mongodb.UpdatePostVotes(ctx, post.ID, upvoteDelta, downvoteDelta)
	if err != nil {
		log.Printf("Failed to update post votes in MongoDB: %v", err)
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to persist vote", err))
		return
	}

	// Update user karma (only for non-toggle operations or when switching vote types)
	// Skip karma update if we're just removing a vote
	karmaChange := 0
	if upvoteDelta == 1 {
		karmaChange = 1
	} else if downvoteDelta == 1 {
		karmaChange = -1
	}

	if karmaChange != 0 {
		context.Send(a.enginePID, &UpdateKarmaMsg{
			UserID: post.AuthorID,
			Delta:  karmaChange,
		})
	}

	// Add UserVotes field to response to help frontend show correct state
	if post.UserVotes == nil {
		post.UserVotes = make(map[string]bool)
	}

	// Update the UserVotes for the current user
	if hasVote, hasVoted := a.postVotes[msg.PostID][msg.UserID]; hasVoted {
		post.UserVotes[msg.UserID.String()] = hasVote.IsUpvote
	} else {
		// Remove the user's vote if they toggled it off
		delete(post.UserVotes, msg.UserID.String())
	}

	// Count comments for this post
	commentCount, err := a.getCommentCount(msg.PostID)
	if err != nil {
		log.Printf("Error counting comments for post %s: %v", msg.PostID, err)
	} else {
		post.CommentCount = commentCount
	}

	a.metrics.AddOperationLatency("vote_post", time.Since(startTime))
	context.Respond(post)
}

// Handles fetching the user's feed
func (a *PostActor) handleGetUserFeed(context actor.Context, msg *GetUserFeedMsg) {
	startTime := time.Now()
	ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
	defer cancel()

	feedPosts, err := a.mongodb.GetUserFeedPosts(ctx, msg.UserID, msg.Limit)
	if err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to get feed posts", err))
		return
	}

	// Add comment count to each post
	for _, post := range feedPosts {
		commentCount, err := a.getCommentCount(post.ID)
		if err != nil {
			log.Printf("Error counting comments for post %s: %v", post.ID, err)
		} else {
			post.CommentCount = commentCount
		}
	}

	a.metrics.AddOperationLatency("get_feed", time.Since(startTime))
	context.Respond(feedPosts)
}

func (a *PostActor) handleGetRecentPosts(context actor.Context, msg *GetRecentPostsMsg) {
	ctx := stdctx.Background()

	// Set up options for sorting by creation date
	opts := options.Find().
		SetSort(bson.D{{Key: "createdat", Value: -1}}).
		SetLimit(int64(msg.Limit))

	// Query MongoDB for recent posts
	cursor, err := a.mongodb.Posts.Find(ctx, bson.M{}, opts)
	if err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch recent posts", err))
		return
	}
	defer cursor.Close(ctx)

	var posts []*models.Post
	for cursor.Next(ctx) {
		var doc database.PostDocument
		if err := cursor.Decode(&doc); err != nil {
			log.Printf("Error decoding post document: %v", err)
			continue
		}

		post, err := a.mongodb.DocumentToModel(&doc)
		if err != nil {
			log.Printf("Error converting document to model: %v", err)
			continue
		}

		// Count comments for each post
		commentCount, err := a.getCommentCount(post.ID)
		if err != nil {
			log.Printf("Error counting comments for post %s: %v", post.ID, err)
		} else {
			post.CommentCount = commentCount
		}

		posts = append(posts, post)
	}

	if err := cursor.Err(); err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Error reading posts", err))
		return
	}

	context.Respond(posts)
}

// Helper function to get comment count for a post
func (a *PostActor) getCommentCount(postID uuid.UUID) (int, error) {
	ctx := stdctx.Background()
	return a.mongodb.CountPostComments(ctx, postID)
}
