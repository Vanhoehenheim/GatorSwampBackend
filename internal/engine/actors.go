package engine

import (
	"gator-swamp/internal/models"
	"gator-swamp/internal/utils"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/google/uuid"
)

type GetCountsMsg struct{}

// Message types for Subreddit operations
type CreateSubredditMsg struct {
	Name        string
	Description string
	CreatorID   uuid.UUID
}

type JoinSubredditMsg struct {
	SubredditID uuid.UUID
	UserID      uuid.UUID
}

type GetSubredditMsg struct {
	Name string
}

// Message types for Post operations
type CreatePostMsg struct {
	Title       string
	Content     string
	AuthorID    uuid.UUID
	SubredditID uuid.UUID
}

type GetPostMsg struct {
	PostID uuid.UUID
}

type GetSubredditPostsMsg struct {
	SubredditID uuid.UUID
}

// SubredditActor handles all subreddit-related operations
type SubredditActor struct {
	subredditsByName map[string]*models.Subreddit
	subredditMembers map[uuid.UUID]map[uuid.UUID]bool
	metrics          *utils.MetricsCollector
}

func NewSubredditActor(metrics *utils.MetricsCollector) actor.Actor {
	return &SubredditActor{
		subredditsByName: make(map[string]*models.Subreddit),
		subredditMembers: make(map[uuid.UUID]map[uuid.UUID]bool),
		metrics:          metrics,
	}
}

// Receive handles messages sent to the SubredditActor. It processes different types of messages:
// - CreateSubredditMsg: Creates a new subreddit if it doesn't already exist.
// - GetSubredditMsg: Retrieves an existing subreddit by name.
// - JoinSubredditMsg: Adds a user to an existing subreddit by ID.
//
// For CreateSubredditMsg, it checks if the subreddit already exists, creates a new subreddit, stores it,
// and responds with the created subreddit or an error if it already exists.
//
// For GetSubredditMsg, it retrieves the subreddit by name and responds with the subreddit or an error if not found.
//
// For JoinSubredditMsg, it finds the subreddit by ID, adds the user as a member, and responds with a success status or an error if the subreddit is not found.
func (a *SubredditActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *CreateSubredditMsg:
		startTime := time.Now()

		// Check if subreddit already exists
		if _, exists := a.subredditsByName[msg.Name]; exists {
			context.Respond(utils.NewAppError(utils.ErrDuplicate, "subreddit already exists", nil))
			return
		}

		// Create new subreddit
		newSubreddit := &models.Subreddit{
			ID:          uuid.New(),
			Name:        msg.Name,
			Description: msg.Description,
			CreatorID:   msg.CreatorID,
			CreatedAt:   time.Now(),
			Members:     1, // Creator is first member
		}

		// Store subreddit
		a.subredditsByName[msg.Name] = newSubreddit
		a.subredditMembers[newSubreddit.ID] = map[uuid.UUID]bool{
			msg.CreatorID: true,
		}

		a.metrics.AddOperationLatency("create_subreddit", time.Since(startTime))
		context.Respond(newSubreddit)

	case *GetSubredditMsg:
		if subreddit, exists := a.subredditsByName[msg.Name]; exists {
			context.Respond(subreddit)
		} else {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
		}

	case *JoinSubredditMsg:
		startTime := time.Now()

		// Find the subreddit
		var subreddit *models.Subreddit
		for _, s := range a.subredditsByName {
			if s.ID == msg.SubredditID {
				subreddit = s
				break
			}
		}

		if subreddit == nil {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
			return
		}

		// Add member
		if _, exists := a.subredditMembers[msg.SubredditID]; !exists {
			a.subredditMembers[msg.SubredditID] = make(map[uuid.UUID]bool)
		}
		a.subredditMembers[msg.SubredditID][msg.UserID] = true
		subreddit.Members++

		a.metrics.AddOperationLatency("join_subreddit", time.Since(startTime))
		context.Respond(true)
	case *GetCountsMsg:
		count := len(a.subredditsByName)
		context.Respond(count)
	}
}

// PostActor is a struct that manages posts in the system. It maintains mappings
// of posts by their unique identifiers and by subreddit identifiers. It also
// includes a metrics collector for tracking various metrics related to posts.
//
// Fields:
// - postsByID: A map that associates each post's UUID with its corresponding Post object.
// - postsBySubreddit: A map that associates each subreddit's UUID with a slice of Post objects belonging to that subreddit.
// - metrics: A MetricsCollector instance used for collecting and reporting metrics related to posts.
type PostActor struct {
	postsByID        map[uuid.UUID]*models.Post
	postsBySubreddit map[uuid.UUID][]*models.Post
	metrics          *utils.MetricsCollector
}

// NewPostActor creates a new instance of PostActor with initialized maps for posts by ID and posts by subreddit,
// and assigns the provided metrics collector to the PostActor.
//
// Parameters:
//   - metrics: A pointer to a MetricsCollector instance used for collecting metrics.
//
// Returns:
//   - An instance of actor.Actor implemented by PostActor.
func NewPostActor(metrics *utils.MetricsCollector) actor.Actor {
	return &PostActor{
		postsByID:        make(map[uuid.UUID]*models.Post),
		postsBySubreddit: make(map[uuid.UUID][]*models.Post),
		metrics:          metrics,
	}
}

// Receive handles incoming messages for the PostActor.
// It processes different types of messages and performs corresponding actions:
// - CreatePostMsg: Creates a new post, stores it, and responds with the created post.
// - GetPostMsg: Retrieves a post by its ID and responds with the post or an error if not found.
// - GetSubredditPostsMsg: Retrieves all posts for a given subreddit and responds with the list of posts.
func (a *PostActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *CreatePostMsg:
		startTime := time.Now()

		newPost := &models.Post{
			ID:          uuid.New(),
			Title:       msg.Title,
			Content:     msg.Content,
			AuthorID:    msg.AuthorID,
			SubredditID: msg.SubredditID,
			CreatedAt:   time.Now(),
		}

		// Store post
		a.postsByID[newPost.ID] = newPost
		a.postsBySubreddit[msg.SubredditID] = append(
			a.postsBySubreddit[msg.SubredditID],
			newPost,
		)

		a.metrics.AddOperationLatency("create_post", time.Since(startTime))
		context.Respond(newPost)

	case *GetPostMsg:
		if post, exists := a.postsByID[msg.PostID]; exists {
			context.Respond(post)
		} else {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "post not found", nil))
		}

	case *GetSubredditPostsMsg:
		posts := a.postsBySubreddit[msg.SubredditID]
		context.Respond(posts)

	case *GetCountsMsg:
		count := len(a.postsByID)
		context.Respond(count)
	}
}

// Engine coordinates communication between actors
type Engine struct {
	subredditActor *actor.PID
	postActor      *actor.PID
}

func NewEngine(system *actor.ActorSystem, metrics *utils.MetricsCollector) *Engine {
	context := system.Root

	// Spawn subreddit actor
	subredditProps := actor.PropsFromProducer(func() actor.Actor {
		return NewSubredditActor(metrics)
	})
	subredditPID := context.Spawn(subredditProps)

	// Spawn post actor
	postProps := actor.PropsFromProducer(func() actor.Actor {
		return NewPostActor(metrics)
	})
	postPID := context.Spawn(postProps)

	return &Engine{
		subredditActor: subredditPID,
		postActor:      postPID,
	}
}

// GetSubredditActor returns the PID of the subreddit actor
func (e *Engine) GetSubredditActor() *actor.PID {
	return e.subredditActor
}

// GetPostActor returns the PID of the post actor
func (e *Engine) GetPostActor() *actor.PID {
	return e.postActor
}
