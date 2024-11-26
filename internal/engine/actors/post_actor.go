package actors

import (
	"gator-swamp/internal/models"
	"gator-swamp/internal/utils"
	"sort"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/google/uuid"
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

	GetCountsMsg struct{}
)

// PostActor handles post-related operations
type PostActor struct {
	postsByID      map[uuid.UUID]*models.Post
	subredditPosts map[uuid.UUID][]uuid.UUID
	postVotes      map[uuid.UUID]map[uuid.UUID]voteStatus
	metrics        *utils.MetricsCollector
}

// NewPostActor creates a new PostActor instance
func NewPostActor(metrics *utils.MetricsCollector) actor.Actor {
	return &PostActor{
		postsByID:      make(map[uuid.UUID]*models.Post),
		subredditPosts: make(map[uuid.UUID][]uuid.UUID),
		postVotes:      make(map[uuid.UUID]map[uuid.UUID]voteStatus),
		metrics:        metrics,
	}
}

// Receive handles incoming messages
func (a *PostActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
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
	case *GetCountsMsg:
		context.Respond(len(a.postsByID))
	}
}

func (a *PostActor) handleCreatePost(context actor.Context, msg *CreatePostMsg) {
	startTime := time.Now()

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

	a.postsByID[newPost.ID] = newPost
	a.postVotes[newPost.ID] = make(map[uuid.UUID]voteStatus)
	a.subredditPosts[msg.SubredditID] = append(a.subredditPosts[msg.SubredditID], newPost.ID)

	a.metrics.AddOperationLatency("create_post", time.Since(startTime))
	context.Respond(newPost)
}

func (a *PostActor) handleGetPost(context actor.Context, msg *GetPostMsg) {
	if post, exists := a.postsByID[msg.PostID]; exists {
		context.Respond(post)
	} else {
		context.Respond(utils.NewAppError(utils.ErrNotFound, "post not found", nil))
	}
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
	post, exists := a.postsByID[msg.PostID]
	if !exists {
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
	context.Send(context.Parent(), &UpdateKarmaMsg{
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

func (a *PostActor) handleGetUserFeed(context actor.Context, msg *GetUserFeedMsg) {
	startTime := time.Now()

	// Get user's subscribed subreddits
	future := context.RequestFuture(
		context.Parent(),
		&GetUserProfileMsg{UserID: msg.UserID},
		5*time.Second,
	)

	result, err := future.Result()
	if err != nil {
		context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to get user profile", err))
		return
	}

	userProfile, ok := result.(*UserState)
	if !ok || userProfile == nil {
		context.Respond(utils.NewAppError(utils.ErrNotFound, "User not found", nil))
		return
	}

	// Collect posts from subscribed subreddits
	var feedPosts []*models.Post
	for _, subredditID := range userProfile.Subscriptions {
		if postIDs, exists := a.subredditPosts[subredditID]; exists {
			for _, postID := range postIDs {
				if post := a.postsByID[postID]; post != nil {
					feedPosts = append(feedPosts, post)
				}
			}
		}
	}

	// Sort posts by score (karma and time)
	sort.Slice(feedPosts, func(i, j int) bool {
		timeI := time.Since(feedPosts[i].CreatedAt).Hours()
		timeJ := time.Since(feedPosts[j].CreatedAt).Hours()
		scoreI := float64(feedPosts[i].Karma) / (timeI + 2.0)
		scoreJ := float64(feedPosts[j].Karma) / (timeJ + 2.0)
		return scoreI > scoreJ
	})

	if msg.Limit > 0 && len(feedPosts) > msg.Limit {
		feedPosts = feedPosts[:msg.Limit]
	}

	a.metrics.AddOperationLatency("get_feed", time.Since(startTime))
	context.Respond(feedPosts)
}
