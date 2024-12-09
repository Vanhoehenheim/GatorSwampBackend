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

	// For internal vote tracking
	voteStatus struct {
		IsUpvote bool
		VotedAt  time.Time
	}

	GetCountsMsg           struct{}
	initializePostActorMsg struct{}
	loadPostsFromDBMsg     struct{}
)

// PostActor handles post-related operations
type PostActor struct {
	postsByID      map[uuid.UUID]*models.Post
	subredditPosts map[uuid.UUID][]uuid.UUID
	postVotes      map[uuid.UUID]map[uuid.UUID]voteStatus
	metrics        *utils.MetricsCollector
	enginePID      *actor.PID
	mongodb        *database.MongoDB
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

func (a *PostActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		log.Printf("PostActor started")
		// Send initialization message when actor starts
		context.Send(context.Self(), &initializePostActorMsg{})

	case *initializePostActorMsg:
		// Start the initialization process
		context.Send(context.Self(), &loadPostsFromDBMsg{})

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

func (a *PostActor) handleGetPost(context actor.Context, msg *GetPostMsg) {
	// First check local cache
	if post, exists := a.postsByID[msg.PostID]; exists {
		context.Respond(post)
		return
	}

	// If not in cache, try MongoDB
	ctx := stdctx.Background()
	var post models.Post
	err := a.mongodb.Posts.FindOne(ctx, bson.M{"_id": msg.PostID}).Decode(&post)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "post not found", nil))
			return
		}
		context.Respond(utils.NewAppError(utils.ErrDatabase, "failed to fetch post", err))
		return
	}

	// Update local cache
	a.postsByID[post.ID] = &post
	// Initialize vote tracking for this post
	a.postVotes[post.ID] = make(map[uuid.UUID]voteStatus)
	// Update subreddit posts mapping
	a.subredditPosts[post.SubredditID] = append(a.subredditPosts[post.SubredditID], post.ID)

	context.Respond(&post)
}

func (a *PostActor) handleLoadPosts(context actor.Context) {
	ctx := stdctx.Background()
	cursor, err := a.mongodb.Posts.Find(ctx, bson.M{})
	if err != nil {
		log.Printf("Error loading posts from MongoDB: %v", err)
		return
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var post models.Post
		if err := cursor.Decode(&post); err != nil {
			log.Printf("Error decoding post: %v", err)
			continue
		}
		a.postsByID[post.ID] = &post
		a.postVotes[post.ID] = make(map[uuid.UUID]voteStatus)
		a.subredditPosts[post.SubredditID] = append(a.subredditPosts[post.SubredditID], post.ID)
	}
	log.Printf("Loaded %d posts from MongoDB", len(a.postsByID))
}

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

	log.Printf("PostActor: Creating new post: %s in subreddit: %s", newPost.ID, newPost.SubredditID)

	// Convert to document using MongoDB helper
	postDoc := a.mongodb.ModelToDocument(newPost)

	// Save the document version to MongoDB
	if _, err := a.mongodb.Posts.InsertOne(ctx, postDoc); err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to save post", err))
		return
	}

	// Update subreddit's posts array using the helper function
	err := a.mongodb.UpdateSubredditPosts(ctx, msg.SubredditID, newPost.ID, true)
	if err != nil {
		// Rollback post creation if subreddit update fails
		if _, deleteErr := a.mongodb.Posts.DeleteOne(ctx, bson.M{"_id": postDoc.ID}); deleteErr != nil {
			log.Printf("Failed to delete post after subreddit update failure: %v", deleteErr)
		}
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to update subreddit posts", err))
		return
	}

	// Update local cache
	a.postsByID[newPost.ID] = newPost
	a.postVotes[newPost.ID] = make(map[uuid.UUID]voteStatus)
	a.subredditPosts[msg.SubredditID] = append(a.subredditPosts[msg.SubredditID], newPost.ID)

	a.metrics.AddOperationLatency("create_post", time.Since(startTime))
	context.Respond(newPost)
}

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
		context.Respond(utils.NewAppError(utils.ErrNotFound, "no posts found", nil))
	}
}

func (a *PostActor) handleVote(context actor.Context, msg *VotePostMsg) {
	startTime := time.Now()

	log.Printf("PostActor: Looking up post %s", msg.PostID)
	post, exists := a.postsByID[msg.PostID]
	if !exists {
		log.Printf("PostActor: Post not found: %s", msg.PostID)
		context.Respond(utils.NewAppError(utils.ErrNotFound, "Post not found", nil))
		return
	}

	// Initialize vote tracking if needed
	if _, exists := a.postVotes[msg.PostID]; !exists {
		a.postVotes[msg.PostID] = make(map[uuid.UUID]voteStatus)
	}

	previousVote, hasVoted := a.postVotes[msg.PostID][msg.UserID]

	if hasVoted {
		if previousVote.IsUpvote == msg.IsUpvote {
			log.Printf("PostActor: User %s already voted on post %s", msg.UserID, msg.PostID)
			context.Respond(utils.NewAppError(utils.ErrDuplicate, "Already voted", nil))
			return
		}
		// Change vote
		if msg.IsUpvote {
			post.Downvotes--
			post.Upvotes++
		} else {
			post.Upvotes--
			post.Downvotes++
		}
	} else {
		// New vote
		if msg.IsUpvote {
			post.Upvotes++
		} else {
			post.Downvotes++
		}
	}

	// Update vote record and recalculate karma
	a.postVotes[msg.PostID][msg.UserID] = voteStatus{
		IsUpvote: msg.IsUpvote,
		VotedAt:  time.Now(),
	}
	post.Karma = post.Upvotes - post.Downvotes

	// Update author's karma
	karmaMsg := &UpdateKarmaMsg{
		UserID: post.AuthorID,
		Delta: func() int {
			if msg.IsUpvote {
				return 1
			}
			return -1
		}(),
	}

	log.Printf("PostActor: Sending karma update to Engine for user %s", post.AuthorID)
	context.Send(a.enginePID, karmaMsg) // Use Send instead of RequestFuture

	a.metrics.AddOperationLatency("vote_post", time.Since(startTime))
	context.Respond(post)
}

func (a *PostActor) handleGetUserFeed(context actor.Context, msg *GetUserFeedMsg) {
	startTime := time.Now()

	ctx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
	defer cancel()

	// Get fresh feed directly from MongoDB
	feedPosts, err := a.mongodb.GetUserFeedPosts(ctx, msg.UserID, msg.Limit)
	if err != nil {
		context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to get feed posts", err))
		return
	}

	a.metrics.AddOperationLatency("get_feed", time.Since(startTime))
	context.Respond(feedPosts)
}
