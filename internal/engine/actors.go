package engine

// Import necessary packages
import (
	"fmt"
	"gator-swamp/internal/engine/actors"
	"gator-swamp/internal/models" // Models for Subreddits and Posts
	"gator-swamp/internal/utils"  // Utility functions and error handling
	"log"
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

// Engine coordinates communication between actors
type Engine struct {
	subredditActor *actor.PID
	postActor      *actor.PID
	userSupervisor *actor.PID              // Changed from userActor to userSupervisor
	context        *actor.RootContext      // Add this
	metrics        *utils.MetricsCollector // Added metrics field
}

// NewEngine creates a new engine instance with all required actors
func NewEngine(system *actor.ActorSystem, metrics *utils.MetricsCollector) *Engine {
	context := system.Root
	log.Printf("Creating Engine with actors...")

	// Create the Engine first
	e := &Engine{
		context: context,
		metrics: metrics,
	}

	// Create props with Engine's PID
	engineProps := actor.PropsFromProducer(func() actor.Actor {
		return e
	})
	enginePID := context.Spawn(engineProps)

	// Now create other actors with enginePID
	supervisorProps := actor.PropsFromProducer(func() actor.Actor {
		return actors.NewUserSupervisor()
	})

	subredditProps := actor.PropsFromProducer(func() actor.Actor {
		return NewSubredditActor(metrics)
	})

	postProps := actor.PropsFromProducer(func() actor.Actor {
		return actors.NewPostActor(metrics, enginePID) // Pass enginePID here
	})

	userSupervisorPID := context.Spawn(supervisorProps)
	subredditPID := context.Spawn(subredditProps)
	postPID := context.Spawn(postProps)

	e.userSupervisor = userSupervisorPID
	e.subredditActor = subredditPID
	e.postActor = postPID

	return e
}

// Make Engine implement the Actor interface
func (e *Engine) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		log.Printf("Engine started")

	case *actor.Stopping:
		log.Printf("Engine stopping")

	case *actor.Stopped:
		log.Printf("Engine stopped")

	case *actor.Restarting:
		log.Printf("Engine restarting")

	case *CreateSubredditMsg:
		log.Printf("Engine: Processing CreateSubredditMsg for creator: %s", msg.CreatorID)

		// Validate user exists and has sufficient karma
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

		// Check karma requirement
		if userState.Karma < 100 {
			log.Printf("Engine: Insufficient karma for user %s", msg.CreatorID)
			context.Respond(utils.NewAppError(utils.ErrInvalidInput,
				fmt.Sprintf("Insufficient karma (required: 100, current: %d)", userState.Karma), nil))
			return
		}

		// Forward to SubredditActor
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

	case *actors.CreatePostMsg:
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

		// Check if user is a member
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

		// Forward to PostActor
		future := context.RequestFuture(e.postActor, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to create post", err))
			return
		}
		context.Respond(result)

	case *actors.VotePostMsg:
		// Validate user exists
		userFuture := context.RequestFuture(e.userSupervisor,
			&actors.GetUserProfileMsg{UserID: msg.UserID},
			5*time.Second)

		_, err := userFuture.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to validate user", err))
			return
		}

		// Forward to PostActor
		future := context.RequestFuture(e.postActor, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to process vote", err))
			return
		}
		context.Respond(result)

	case *actors.UpdateKarmaMsg:
		log.Printf("Engine: Forwarding karma update to UserSupervisor")
		context.Send(e.userSupervisor, msg)

	default:
		// Route message based on type
		var targetPID *actor.PID
		var msgType string

		switch {
		case isSubredditMessage(msg):
			targetPID = e.subredditActor
			msgType = "subreddit"
		case isUserMessage(msg):
			targetPID = e.userSupervisor
			msgType = "user"
		case isPostMessage(msg):
			targetPID = e.postActor
			msgType = "post"
		default:
			log.Printf("Unknown message type: %T", msg)
			context.Respond(utils.NewAppError(utils.ErrInvalidInput, "Unknown message type", nil))
			return
		}

		future := context.RequestFuture(targetPID, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout,
				fmt.Sprintf("Failed to process %s request", msgType), err))
			return
		}
		context.Respond(result)
	}
}

// Helper functions to identify message types
func isSubredditMessage(msg interface{}) bool {
	switch msg.(type) {
	case *CreateSubredditMsg,
		*JoinSubredditMsg,
		*LeaveSubredditMsg,
		*ListSubredditsMsg,
		*GetSubredditMembersMsg,
		*GetSubredditDetailsMsg,
		*GetSubredditMsg,
		*GetCountsMsg:
		return true
	default:
		return false
	}
}

func isUserMessage(msg interface{}) bool {
	switch msg.(type) {
	case *actors.RegisterUserMsg,
		*actors.LoginMsg,
		*actors.GetUserProfileMsg,
		*actors.UpdateProfileMsg,
		*actors.UpdateKarmaMsg:
		return true
	default:
		return false
	}
}

func isPostMessage(msg interface{}) bool {
	switch msg.(type) {
	case *actors.CreatePostMsg,
		*actors.GetPostMsg,
		*actors.GetSubredditPostsMsg,
		*actors.VotePostMsg,
		*actors.GetUserFeedMsg,
		*actors.DeletePostMsg:
		return true
	default:
		return false
	}
}

// Getter methods for actor PIDs
func (e *Engine) GetUserSupervisor() *actor.PID {
	return e.userSupervisor
}

func (e *Engine) GetSubredditActor() *actor.PID {
	return e.subredditActor
}

func (e *Engine) GetPostActor() *actor.PID {
	return e.postActor
}
