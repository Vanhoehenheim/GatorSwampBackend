package engine

import (
	"fmt"
	"gator-swamp/internal/database"
	"gator-swamp/internal/engine/actors"
	"gator-swamp/internal/utils"
	"log"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/google/uuid"
)

// Add new message types
type (
	// Vote related messages
	VotePostMsg struct {
		PostID     uuid.UUID
		UserID     uuid.UUID
		IsUpvote   bool
		RemoveVote bool // New field to support vote toggling
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

// Engine coordinates communication between actors
type Engine struct {
	context        *actor.RootContext // Use RootContext
	metrics        *utils.MetricsCollector
	db             database.DBAdapter // Database adapter interface
	userSupervisor *actor.PID
	subredditActor *actor.PID
	postActor      *actor.PID
	commentActor   *actor.PID
}

// NewEngine creates a new engine instance with all required actors
func NewEngine(system *actor.ActorSystem, metrics *utils.MetricsCollector, db database.DBAdapter) *Engine {
	context := system.Root
	log.Printf("Creating Engine with actors...")

	// Create the Engine first
	e := &Engine{
		context: context, // Assign RootContext here
		metrics: metrics,
		db:      db, // Assign the db interface
	}

	// Create props with Engine's PID
	engineProps := actor.PropsFromProducer(func() actor.Actor {
		return e
	})
	enginePID := context.Spawn(engineProps)

	// Now create other actors with enginePID
	supervisorProps := actor.PropsFromProducer(func() actor.Actor {
		// TODO: Update NewUserSupervisor signature
		return actors.NewUserSupervisor(e.db) // Pass db interface
	})

	subredditProps := actor.PropsFromProducer(func() actor.Actor {
		// TODO: Update NewSubredditActor signature
		return actors.NewSubredditActor(metrics, e.db) // Pass db interface
	})

	// Create the CommentActor first
	commentProps := actor.PropsFromProducer(func() actor.Actor {
		// TODO: Update NewCommentActor signature
		return actors.NewCommentActor(enginePID, e.db) // Pass db interface
	})

	userSupervisorPID := context.Spawn(supervisorProps)
	subredditPID := context.Spawn(subredditProps)
	commentPID := context.Spawn(commentProps)

	// Create PostActor and pass CommentActor PID to it
	postProps := actor.PropsFromProducer(func() actor.Actor {
		// TODO: Update NewPostActor signature
		return actors.NewPostActor(metrics, enginePID, e.db, commentPID) // Pass db interface
	})
	postPID := context.Spawn(postProps)

	e.userSupervisor = userSupervisorPID
	e.subredditActor = subredditPID
	e.commentActor = commentPID
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

	case *actors.CreateSubredditMsg:
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
		// Get user profile to check subreddit membership
		userFuture := context.RequestFuture(e.GetUserSupervisor(),
			&actors.GetUserProfileMsg{UserID: msg.AuthorID},
			5*time.Second)

		result, err := userFuture.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to validate user", err))
			return
		}

		userState, ok := result.(*actors.UserState)
		if !ok || userState == nil {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "User not found", nil))
			return
		}

		// Check if user is a member of the subreddit
		isMember := false
		for _, subID := range userState.Subreddits {
			if subID == msg.SubredditID {
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
		result, err = future.Result()
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

	case *actors.GetUserFeedMsg:
		// First validate user exists
		userFuture := context.RequestFuture(e.userSupervisor,
			&actors.GetUserProfileMsg{UserID: msg.UserID},
			5*time.Second)

		result, err := userFuture.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to validate user", err))
			return
		}

		userState, ok := result.(*actors.UserState)
		if !ok || userState == nil {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "User not found", nil))
			return
		}

		// Forward to PostActor to get feed
		future := context.RequestFuture(e.postActor, msg, 5*time.Second)
		result, err = future.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to get user feed", err))
			return
		}
		context.Respond(result)
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
	case *actors.CreateSubredditMsg,
		*actors.JoinSubredditMsg,
		*actors.LeaveSubredditMsg,
		*actors.ListSubredditsMsg,
		*actors.GetSubredditMembersMsg,
		*actors.GetSubredditByIDMsg,
		*actors.GetSubredditByNameMsg,
		*actors.GetCountsMsg:
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
		*actors.UpdateProfileMsg:
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

func (e *Engine) GetCommentActor() *actor.PID {
	return e.commentActor
}

func (e *Engine) GetDB() database.DBAdapter {
	return e.db
}
