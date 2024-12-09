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
		PostID   uuid.UUID
		UserID   uuid.UUID
		IsUpvote bool
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
)

// PostActor handles post-related operations
type PostActor struct {
	postsByID      map[uuid.UUID]*models.Post             // Cache for posts by their ID
	subredditPosts map[uuid.UUID][]uuid.UUID              // Mapping of subreddit IDs to their posts
	postVotes      map[uuid.UUID]map[uuid.UUID]voteStatus // Tracking user votes for posts
	metrics        *utils.MetricsCollector                // Metrics for performance tracking
	enginePID      *actor.PID                             // Reference to the Engine actor
	mongodb        *database.MongoDB                      // MongoDB client
}

// NewPostActor creates a new PostActor instance
func NewPostActor(metrics *utils.MetricsCollector, enginePID *actor.PID, mongodb *database.MongoDB) actor.Actor {
	return &PostActor{
		postsByID:      make(map[uuid.UUID]*models.Post),
		subredditPosts: make(map[uuid.UUID][]uuid.UUID),
		postVotes:      make(map[uuid.UUID]map[uuid.UUID]voteStatus),
		metrics:        metrics,
		enginePID:      enginePID,
		mongodb:        mongodb,
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

	newPost := &models.Post{
		ID:          uuid.New(),
		Title:       msg.Title,
		Content:     msg.Content,
		AuthorID:    msg.AuthorID,
		SubredditID: msg.SubredditID,
		CreatedAt:   time.Now(),
		Upvotes:     0,
		Downvotes:   0,
		Karma:       0,
	}

	postDoc := a.mongodb.ModelToDocument(newPost)

	if _, err := a.mongodb.Posts.InsertOne(ctx, postDoc); err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to save post", err))
		return
	}

	err := a.mongodb.UpdateSubredditPosts(ctx, msg.SubredditID, newPost.ID, true)
	if err != nil {
		if _, deleteErr := a.mongodb.Posts.DeleteOne(ctx, bson.M{"_id": postDoc.ID}); deleteErr != nil {
			log.Printf("Failed to delete post after subreddit update failure: %v", deleteErr)
		}
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to update subreddit posts", err))
		return
	}

	a.postsByID[newPost.ID] = newPost
	a.postVotes[newPost.ID] = make(map[uuid.UUID]voteStatus)
	a.subredditPosts[msg.SubredditID] = append(a.subredditPosts[msg.SubredditID], newPost.ID)

	a.metrics.AddOperationLatency("create_post", time.Since(startTime))
	context.Respond(newPost)
}

// Handles retrieving a specific post by ID
func (a *PostActor) handleGetPost(context actor.Context, msg *GetPostMsg) {
	if post, exists := a.postsByID[msg.PostID]; exists {
		context.Respond(post)
		return
	}

	ctx := stdctx.Background()
	var post models.Post
	err := a.mongodb.Posts.FindOne(ctx, bson.M{"_id": msg.PostID}).Decode(&post)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "Post not found", nil))
		} else {
			context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch post", err))
		}
		return
	}

	a.postsByID[post.ID] = &post
	a.postVotes[post.ID] = make(map[uuid.UUID]voteStatus)
	a.subredditPosts[post.SubredditID] = append(a.subredditPosts[post.SubredditID], post.ID)

	context.Respond(&post)
}

// Handles retrieving all posts for a subreddit
func (a *PostActor) handleGetSubredditPosts(context actor.Context, msg *GetSubredditPostsMsg) {
	if postIDs, exists := a.subredditPosts[msg.SubredditID]; exists {
		posts := make([]*models.Post, 0, len(postIDs))
		for _, postID := range postIDs {
			if post := a.postsByID[postID]; post != nil {
				posts = append(posts, post)
			}
		}
		context.Respond(posts)
	} else {
		context.Respond(utils.NewAppError(utils.ErrNotFound, "No posts found", nil))
	}
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

	if hasVoted {
		if previousVote.IsUpvote == msg.IsUpvote {
			context.Respond(utils.NewAppError(utils.ErrDuplicate, "Already voted", nil))
			return
		}
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
	} else {
		if msg.IsUpvote {
			upvoteDelta = 1
			post.Upvotes++
		} else {
			downvoteDelta = 1
			post.Downvotes++
		}
	}

	// Update vote status in memory
	a.postVotes[msg.PostID][msg.UserID] = voteStatus{
		IsUpvote: msg.IsUpvote,
		VotedAt:  time.Now(),
	}
	post.Karma = post.Upvotes - post.Downvotes

	// Update MongoDB
	// In handleVote function, replace the MongoDB update section with:
	ctx := stdctx.Background()
	err := a.mongodb.UpdatePostVotes(ctx, post.ID, upvoteDelta, downvoteDelta)
	if err != nil {
		log.Printf("Failed to update post votes in MongoDB: %v", err)
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to persist vote", err))
		return
	}

	// Update user karma
	context.Send(a.enginePID, &UpdateKarmaMsg{
		UserID: post.AuthorID,
		Delta: func() int {
			if msg.IsUpvote {
				return 1
			}
			return -1
		}(),
	})

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

	a.metrics.AddOperationLatency("get_feed", time.Since(startTime))
	context.Respond(feedPosts)
}
