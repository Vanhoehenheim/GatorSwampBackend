package engine

// Import necessary packages
import (
	"gator-swamp/internal/models" // Models for Subreddits and Posts
	"gator-swamp/internal/utils"  // Utility functions and error handling
	"time"                        // Time utilities

	"github.com/asynkron/protoactor-go/actor" // ProtoActor for actor-based concurrency
	"github.com/google/uuid"                  // UUID generation for unique identifiers
)

// Message types for Subreddit operations

// GetCountsMsg retrieves the count of subreddits
type GetCountsMsg struct{}

// CreateSubredditMsg represents a request to create a new subreddit
type CreateSubredditMsg struct {
	Name        string    // Name of the subreddit
	Description string    // Description of the subreddit
	CreatorID   uuid.UUID // ID of the user creating the subreddit
}

// JoinSubredditMsg represents a request for a user to join a subreddit
type JoinSubredditMsg struct {
	SubredditID uuid.UUID // ID of the subreddit
	UserID      uuid.UUID // ID of the user joining
}

// LeaveSubredditMsg handles a user leaving a subreddit
type LeaveSubredditMsg struct {
	SubredditID uuid.UUID // ID of the subreddit
	UserID      uuid.UUID // ID of the user leaving
}

// ListSubredditsMsg retrieves all subreddits
type ListSubredditsMsg struct{}

// GetSubredditMembersMsg retrieves all members of a specific subreddit
type GetSubredditMembersMsg struct {
	SubredditID uuid.UUID // ID of the subreddit
}

// GetSubredditDetailsMsg gets detailed information about a subreddit
type GetSubredditDetailsMsg struct {
	SubredditID uuid.UUID // ID of the subreddit
}

// GetSubredditMsg retrieves a subreddit by name
type GetSubredditMsg struct {
	Name string // Name of the subreddit
}

// Message types for Post operations

// CreatePostMsg represents a request to create a new post
type CreatePostMsg struct {
	Title       string    // Title of the post
	Content     string    // Content of the post
	AuthorID    uuid.UUID // ID of the author
	SubredditID uuid.UUID // ID of the subreddit
}

// GetPostMsg retrieves a post by its ID
type GetPostMsg struct {
	PostID uuid.UUID // ID of the post
}

// GetSubredditPostsMsg retrieves all posts in a subreddit
type GetSubredditPostsMsg struct {
	SubredditID uuid.UUID // ID of the subreddit
}

// SubredditActor handles all subreddit-related operations
type SubredditActor struct {
	subredditsByName map[string]*models.Subreddit     // Map of subreddit names to their models
	subredditMembers map[uuid.UUID]map[uuid.UUID]bool // Map of subreddit IDs to their members
	metrics          *utils.MetricsCollector          // Metrics for performance tracking
}

// Constructor for SubredditActor
func NewSubredditActor(metrics *utils.MetricsCollector) actor.Actor {
	return &SubredditActor{
		subredditsByName: make(map[string]*models.Subreddit),     // Initialize subreddit storage
		subredditMembers: make(map[uuid.UUID]map[uuid.UUID]bool), // Initialize member storage
		metrics:          metrics,                                // Initialize metrics collector
	}
}

// Receive method processes incoming messages and executes corresponding actions
func (a *SubredditActor) Receive(context actor.Context) {
	// Handle messages based on their type
	switch msg := context.Message().(type) {

	case *CreateSubredditMsg:
		// Handle subreddit creation
		startTime := time.Now()

		// Check if a subreddit with the given name already exists
		if _, exists := a.subredditsByName[msg.Name]; exists {
			context.Respond(utils.NewAppError(utils.ErrDuplicate, "subreddit already exists", nil))
			return
		}

		// Create a new subreddit object
		newSubreddit := &models.Subreddit{
			ID:          uuid.New(),
			Name:        msg.Name,
			Description: msg.Description,
			CreatorID:   msg.CreatorID,
			CreatedAt:   time.Now(),
			Members:     1, // The creator is the first member
		}

		// Store the new subreddit and initialize its membership map
		a.subredditsByName[msg.Name] = newSubreddit
		a.subredditMembers[newSubreddit.ID] = map[uuid.UUID]bool{
			msg.CreatorID: true,
		}

		// Log the operation latency and respond with the created subreddit
		a.metrics.AddOperationLatency("create_subreddit", time.Since(startTime))
		context.Respond(newSubreddit)

	case *GetSubredditMsg:
		// Handle subreddit retrieval by name
		if subreddit, exists := a.subredditsByName[msg.Name]; exists {
			context.Respond(subreddit)
		} else {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
		}

	case *JoinSubredditMsg:
		// Handle user joining a subreddit
		startTime := time.Now()

		// Find the subreddit by ID
		var subreddit *models.Subreddit
		for _, s := range a.subredditsByName {
			if s.ID == msg.SubredditID {
				subreddit = s
				break
			}
		}

		// If subreddit does not exist, respond with an error
		if subreddit == nil {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
			return
		}

		// Add the user to the subreddit members
		if _, exists := a.subredditMembers[msg.SubredditID]; !exists {
			a.subredditMembers[msg.SubredditID] = make(map[uuid.UUID]bool)
		}
		a.subredditMembers[msg.SubredditID][msg.UserID] = true
		subreddit.Members++

		// Log operation latency and respond with success
		a.metrics.AddOperationLatency("join_subreddit", time.Since(startTime))
		context.Respond(true)

	case *LeaveSubredditMsg:
		// Handle user leaving a subreddit
		startTime := time.Now()

		// Find the subreddit by ID
		var subreddit *models.Subreddit
		for _, s := range a.subredditsByName {
			if s.ID == msg.SubredditID {
				subreddit = s
				break
			}
		}

		// If subreddit does not exist, respond with an error
		if subreddit == nil {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
			return
		}

		// Check if the user is a member of the subreddit
		members := a.subredditMembers[msg.SubredditID]
		if !members[msg.UserID] {
			context.Respond(utils.NewAppError(utils.ErrInvalidInput, "user is not a member", nil))
			return
		}

		// Remove the user from the subreddit members
		delete(a.subredditMembers[msg.SubredditID], msg.UserID)
		subreddit.Members--

		// Log operation latency and respond with success
		a.metrics.AddOperationLatency("leave_subreddit", time.Since(startTime))
		context.Respond(true)

	case *ListSubredditsMsg:
		// Handle request to list all subreddits
		subreddits := make([]*models.Subreddit, 0, len(a.subredditsByName))
		for _, sub := range a.subredditsByName {
			subreddits = append(subreddits, sub)
		}
		context.Respond(subreddits)

	case *GetSubredditMembersMsg:
		// Handle request to get members of a subreddit
		if members, exists := a.subredditMembers[msg.SubredditID]; exists {
			memberIDs := make([]uuid.UUID, 0, len(members))
			for userID := range members {
				memberIDs = append(memberIDs, userID)
			}
			context.Respond(memberIDs)
		} else {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
		}

	case *GetSubredditDetailsMsg:
		// Handle request to get subreddit details
		var subreddit *models.Subreddit
		for _, s := range a.subredditsByName {
			if s.ID == msg.SubredditID {
				subreddit = s
				break
			}
		}

		// If subreddit does not exist, respond with an error
		if subreddit == nil {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
			return
		}

		// Create a detailed response with subreddit and member count
		details := struct {
			Subreddit   *models.Subreddit
			MemberCount int
		}{
			Subreddit:   subreddit,
			MemberCount: len(a.subredditMembers[msg.SubredditID]),
		}
		context.Respond(details)
	case *GetCountsMsg:
		count := len(a.subredditsByName)
		context.Respond(count)
	}
}

// PostActor handles post-related operations
type PostActor struct {
	postsByID      map[uuid.UUID]*models.Post // Map of post IDs to their models
	subredditPosts map[uuid.UUID][]uuid.UUID  // Map of subreddit IDs to their posts
	metrics        *utils.MetricsCollector    // Metrics for performance tracking
}

// Constructor for PostActor
func NewPostActor(metrics *utils.MetricsCollector) actor.Actor {
	return &PostActor{
		postsByID:      make(map[uuid.UUID]*models.Post), // Initialize post storage
		subredditPosts: make(map[uuid.UUID][]uuid.UUID),  // Initialize subreddit post mapping
		metrics:        metrics,                          // Initialize metrics collector
	}
}

// Receive method processes incoming messages related to posts
func (a *PostActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {

	case *CreatePostMsg:
		// Handle post creation
		startTime := time.Now()

		// Create a new post object
		newPost := &models.Post{
			ID:          uuid.New(),
			Title:       msg.Title,
			Content:     msg.Content,
			AuthorID:    msg.AuthorID,
			SubredditID: msg.SubredditID,
			CreatedAt:   time.Now(),
		}

		// Store the post and update the subreddit-post mapping
		a.postsByID[newPost.ID] = newPost
		a.subredditPosts[msg.SubredditID] = append(a.subredditPosts[msg.SubredditID], newPost.ID)

		// Log the operation latency and respond with the created post
		a.metrics.AddOperationLatency("create_post", time.Since(startTime))
		context.Respond(newPost)

	case *GetPostMsg:
		// Handle post retrieval by ID
		if post, exists := a.postsByID[msg.PostID]; exists {
			context.Respond(post)
		} else {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "post not found", nil))
		}

	case *GetSubredditPostsMsg:
		// Handle retrieval of all posts in a subreddit
		if postIDs, exists := a.subredditPosts[msg.SubredditID]; exists {
			posts := make([]*models.Post, 0, len(postIDs))
			for _, postID := range postIDs {
				posts = append(posts, a.postsByID[postID])
			}
			context.Respond(posts)
		} else {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found or has no posts", nil))
		}
	case *GetCountsMsg:
		count := len(a.postsByID)
		context.Respond(count)
	}
}

// Engine coordinates communication between actors
type Engine struct {
	subredditActor *actor.PID // PID of the SubredditActor
	postActor      *actor.PID // PID of the PostActor
}

// NewEngine initializes the engine by spawning the required actors
func NewEngine(system *actor.ActorSystem, metrics *utils.MetricsCollector) *Engine {
	context := system.Root

	// Spawn the SubredditActor and get its PID
	subredditProps := actor.PropsFromProducer(func() actor.Actor {
		return NewSubredditActor(metrics) // Create a new instance of SubredditActor
	})
	subredditPID := context.Spawn(subredditProps)

	// Spawn the PostActor and get its PID
	postProps := actor.PropsFromProducer(func() actor.Actor {
		return NewPostActor(metrics) // Create a new instance of PostActor
	})
	postPID := context.Spawn(postProps)

	// Return a new Engine instance with references to the actors' PIDs
	return &Engine{
		subredditActor: subredditPID,
		postActor:      postPID,
	}
}

// GetSubredditActor returns the PID of the SubredditActor
func (e *Engine) GetSubredditActor() *actor.PID {
	return e.subredditActor
}

// GetPostActor returns the PID of the PostActor
func (e *Engine) GetPostActor() *actor.PID {
	return e.postActor
}
