package engine

// Import necessary packages
import (
	"fmt"
	"gator-swamp/internal/engine/actors"
	"gator-swamp/internal/models" // Models for Subreddits and Posts
	"gator-swamp/internal/utils"  // Utility functions and error handling
	"log"
	"sort"
	"time" // Time utilities

	"github.com/asynkron/protoactor-go/actor" // ProtoActor for actor-based concurrency
	"github.com/google/uuid"                  // UUID generation for unique identifiers
)

// Add new message types
type (
	// Vote related messages
	VotePostMsg struct {
		PostID   uuid.UUID
		UserID   uuid.UUID
		IsUpvote bool
	}

	// Feed related messages
	GetUserFeedMsg struct {
		UserID uuid.UUID
		Limit  int
	}

	// Update PostActor messages
	UpdatePostKarmaMsg struct {
		PostID uuid.UUID
		Delta  int
	}
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
	subredditsByName map[string]*models.Subreddit
	subredditMembers map[uuid.UUID]map[uuid.UUID]bool
	metrics          *utils.MetricsCollector
	context          actor.Context // Add this to store context
}

// Constructor for SubredditActor
func NewSubredditActor(metrics *utils.MetricsCollector) actor.Actor {
	return &SubredditActor{
		subredditsByName: make(map[string]*models.Subreddit),
		subredditMembers: make(map[uuid.UUID]map[uuid.UUID]bool),
		metrics:          metrics,
	}
}

// Add Started method to initialize context
func (a *SubredditActor) Started(context actor.Context) {
	a.context = context
	log.Printf("SubredditActor started with context: %v", context)
}

// Receive method processes incoming messages and executes corresponding actions
func (a *SubredditActor) Receive(context actor.Context) {
	// Handle messages based on their type
	switch msg := context.Message().(type) {

	case *CreateSubredditMsg:
		log.Printf("SubredditActor: Creating subreddit: %s", msg.Name)
		startTime := time.Now()

		// Only check for duplicate subreddit
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
			Members:     1,
		}

		// Store the new subreddit
		a.subredditsByName[msg.Name] = newSubreddit
		a.subredditMembers[newSubreddit.ID] = map[uuid.UUID]bool{
			msg.CreatorID: true,
		}

		a.metrics.AddOperationLatency("create_subreddit", time.Since(startTime))
		log.Printf("SubredditActor: Successfully created subreddit: %s", newSubreddit.ID)
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
// Update PostActor to include voting and karma
type PostActor struct {
	postsByID      map[uuid.UUID]*models.Post
	subredditPosts map[uuid.UUID][]uuid.UUID
	postVotes      map[uuid.UUID]map[uuid.UUID]bool // PostID -> UserID -> IsUpvote
	metrics        *utils.MetricsCollector
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

	case *VotePostMsg:
		startTime := time.Now()
		post, exists := a.postsByID[msg.PostID]
		if !exists {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "Post not found", nil))
			return
		}

		// Initialize vote map for this post if it doesn't exist
		if _, exists := a.postVotes[msg.PostID]; !exists {
			a.postVotes[msg.PostID] = make(map[uuid.UUID]bool)
		}

		// Check previous vote
		previousVote, hasVoted := a.postVotes[msg.PostID][msg.UserID]

		// Calculate karma change
		karmaChange := 0
		if hasVoted {
			if previousVote != msg.IsUpvote {
				// Changing vote from down to up or up to down
				if msg.IsUpvote {
					karmaChange = 2
				} else {
					karmaChange = -2
				}
			}
		} else {
			// New vote
			if msg.IsUpvote {
				karmaChange = 1
			} else {
				karmaChange = -1
			}
		}

		// Update vote record and post karma
		a.postVotes[msg.PostID][msg.UserID] = msg.IsUpvote
		post.Karma += karmaChange

		// Update author's karma
		context.Send(context.Parent(), &actors.UpdateKarmaMsg{
			UserID: post.AuthorID,
			Delta:  karmaChange,
		})

		a.metrics.AddOperationLatency("vote_post", time.Since(startTime))
		context.Respond(post)

	case *GetUserFeedMsg:
		startTime := time.Now()

		// Get user's subscribed subreddits through Engine
		future := context.RequestFuture(
			context.Parent(), // Request to Engine
			&actors.GetUserProfileMsg{UserID: msg.UserID},
			5*time.Second,
		)
		result, err := future.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to get user profile", err))
			return
		}

		userProfile, ok := result.(*actors.UserState)
		if !ok || userProfile == nil {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "User not found", nil))
			return
		}

		// Rest of the code remains the same
		var feedPosts []*models.Post
		for _, subredditID := range userProfile.Subscriptions {
			if postIDs, exists := a.subredditPosts[subredditID]; exists {
				for _, postID := range postIDs {
					if post, exists := a.postsByID[postID]; exists {
						feedPosts = append(feedPosts, post)
					}
				}
			}
		}

		// Sort posts by karma and time (simple hot algorithm)
		sort.Slice(feedPosts, func(i, j int) bool {
			timeI := time.Since(feedPosts[i].CreatedAt).Hours()
			timeJ := time.Since(feedPosts[j].CreatedAt).Hours()
			scoreI := float64(feedPosts[i].Karma) / (timeI + 2.0)
			scoreJ := float64(feedPosts[j].Karma) / (timeJ + 2.0)
			return scoreI > scoreJ
		})

		// Apply limit if specified
		if msg.Limit > 0 && len(feedPosts) > msg.Limit {
			feedPosts = feedPosts[:msg.Limit]
		}

		a.metrics.AddOperationLatency("get_feed", time.Since(startTime))
		context.Respond(feedPosts)

	case *GetCountsMsg:
		count := len(a.postsByID)
		context.Respond(count)
	}
}

// Engine coordinates communication between actors
type Engine struct {
	subredditActor *actor.PID
	postActor      *actor.PID
	userSupervisor *actor.PID         // Changed from userActor to userSupervisor
	context        *actor.RootContext // Add this
}

// NewEngine initializes the engine by spawning the required actors
// Update NewEngine to create UserSupervisor
func NewEngine(system *actor.ActorSystem, metrics *utils.MetricsCollector) *Engine {
	context := system.Root

	// Add debug logs
	log.Printf("Creating Engine with actors...")

	// Spawn UserSupervisor
	userSupProps := actor.PropsFromProducer(func() actor.Actor {
		log.Printf("Creating UserSupervisor")
		return actors.NewUserSupervisor()
	})
	userSupervisorPID := context.Spawn(userSupProps)
	log.Printf("UserSupervisor PID: %v", userSupervisorPID)

	// Spawn SubredditActor
	subredditProps := actor.PropsFromProducer(func() actor.Actor {
		log.Printf("Creating SubredditActor")
		return NewSubredditActor(metrics)
	})
	subredditPID := context.Spawn(subredditProps)
	log.Printf("SubredditActor PID: %v", subredditPID)

	// Spawn PostActor
	postProps := actor.PropsFromProducer(func() actor.Actor {
		log.Printf("Creating PostActor")
		return NewPostActor(metrics)
	})
	postPID := context.Spawn(postProps)
	log.Printf("PostActor PID: %v", postPID)

	return &Engine{
		subredditActor: subredditPID,
		postActor:      postPID,
		userSupervisor: userSupervisorPID,
		context:        context,
	}
}

// Make Engine implement the Actor interface
func (e *Engine) Receive(context actor.Context) {
	log.Printf("Engine received message: %T", context.Message())
	switch msg := context.Message().(type) {
	case *actors.GetUserProfileMsg:
		// Forward user profile requests to UserSupervisor
		future := context.RequestFuture(e.userSupervisor, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to get user profile", err))
			return
		}
		context.Respond(result)

	case *CreateSubredditMsg:
		log.Printf("Engine: Processing CreateSubredditMsg for creator: %s", msg.CreatorID)

		// 1. Validate user first
		userFuture := context.RequestFuture(e.userSupervisor,
			&actors.GetUserProfileMsg{UserID: msg.CreatorID},
			5*time.Second)

		userResult, err := userFuture.Result()
		if err != nil {
			log.Printf("Engine: Error getting user profile: %v", err)
			context.Respond(utils.NewAppError(utils.ErrActorTimeout,
				fmt.Sprintf("Failed to validate user: %v", err), err))
			return
		}

		userState, ok := userResult.(*actors.UserState)
		if !ok || userState == nil {
			log.Printf("Engine: User not found")
			context.Respond(utils.NewAppError(utils.ErrNotFound, "User not found", nil))
			return
		}

		// 2. Check karma requirement
		if userState.Karma < 100 {
			log.Printf("Engine: Insufficient karma for user %s", msg.CreatorID)
			context.Respond(utils.NewAppError(utils.ErrInvalidInput,
				fmt.Sprintf("Insufficient karma (required: 100, current: %d)", userState.Karma), nil))
			return
		}

		// 3. Forward to SubredditActor for creation
		log.Printf("Engine: User validated, forwarding to SubredditActor")
		future := context.RequestFuture(e.subredditActor, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			log.Printf("Engine: Error creating subreddit: %v", err)
			context.Respond(utils.NewAppError(utils.ErrActorTimeout,
				fmt.Sprintf("Failed to create subreddit: %v", err), err))
			return
		}

		log.Printf("Engine: Subreddit creation completed")
		context.Respond(result)

	case *CreatePostMsg:
		// First validate user is member of subreddit
		memberFuture := context.RequestFuture(e.subredditActor,
			&GetSubredditMembersMsg{SubredditID: msg.SubredditID},
			5*time.Second)

		memberResult, err := memberFuture.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to validate membership", err))
			return
		}

		members, ok := memberResult.([]uuid.UUID)
		if !ok {
			context.Respond(utils.NewAppError(utils.ErrInvalidInput, "Invalid member list", nil))
			return
		}

		isMember := false
		for _, memberID := range members {
			if memberID == msg.AuthorID {
				isMember = true
				break
			}
		}

		if !isMember {
			context.Respond(utils.NewAppError(utils.ErrUnauthorized, "User must be a member to post", nil))
			return
		}

		// Then forward to PostActor
		future := context.RequestFuture(e.postActor, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to create post", err))
			return
		}
		context.Respond(result)

	case *actors.UpdateKarmaMsg:
		// Route karma updates to UserSupervisor
		future := context.RequestFuture(e.userSupervisor, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to update karma", err))
			return
		}
		context.Respond(result)

	case *VotePostMsg:
		// First validate user exists
		userFuture := context.RequestFuture(e.userSupervisor,
			&actors.GetUserProfileMsg{UserID: msg.UserID},
			5*time.Second)

		_, err := userFuture.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to validate user", err))
			return
		}

		// Then forward to PostActor
		future := context.RequestFuture(e.postActor, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to process vote", err))
			return
		}
		context.Respond(result)

	default:
		// Forward unknown messages to appropriate actor based on message type
		switch {
		case isSubredditMessage(msg):
			future := context.RequestFuture(e.subredditActor, msg, 5*time.Second)
			result, err := future.Result()
			if err != nil {
				context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to process subreddit request", err))
				return
			}
			context.Respond(result)

		case isPostMessage(msg):
			future := context.RequestFuture(e.postActor, msg, 5*time.Second)
			result, err := future.Result()
			if err != nil {
				context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to process post request", err))
				return
			}
			context.Respond(result)

		case isUserMessage(msg):
			future := context.RequestFuture(e.userSupervisor, msg, 5*time.Second)
			result, err := future.Result()
			if err != nil {
				context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to process user request", err))
				return
			}
			context.Respond(result)
		}
	}
}

// Add getter for UserSupervisor
func (e *Engine) GetUserSupervisor() *actor.PID {
	return e.userSupervisor
}

// GetSubredditActor returns the PID of the SubredditActor
func (e *Engine) GetSubredditActor() *actor.PID {
	return e.subredditActor
}

// GetPostActor returns the PID of the PostActor
func (e *Engine) GetPostActor() *actor.PID {
	return e.postActor
}

// Helper functions to identify message types
func isSubredditMessage(msg interface{}) bool {
	switch msg.(type) {
	case *CreateSubredditMsg, *JoinSubredditMsg, *LeaveSubredditMsg,
		*ListSubredditsMsg, *GetSubredditMembersMsg, *GetSubredditDetailsMsg:
		return true
	default:
		return false
	}
}

func isPostMessage(msg interface{}) bool {
	switch msg.(type) {
	case *CreatePostMsg, *GetPostMsg, *GetSubredditPostsMsg, *VotePostMsg:
		return true
	default:
		return false
	}
}

func isUserMessage(msg interface{}) bool {
	switch msg.(type) {
	case *actors.RegisterUserMsg, *actors.UpdateProfileMsg,
		*actors.UpdateKarmaMsg, *actors.GetUserProfileMsg, *actors.LoginMsg:
		return true
	default:
		return false
	}
}
