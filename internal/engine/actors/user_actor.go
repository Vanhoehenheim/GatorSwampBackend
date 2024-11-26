package actors

import (
	"crypto/rand"
	"encoding/base64"
	"gator-swamp/internal/utils"
	"sync"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// UserSupervisor manages all user actors
type UserSupervisor struct {
	userActors map[uuid.UUID]*actor.PID
	emailToID  map[string]uuid.UUID
	mu         sync.RWMutex
}

func NewUserSupervisor() actor.Actor {
	return &UserSupervisor{
		userActors: make(map[uuid.UUID]*actor.PID),
		emailToID:  make(map[string]uuid.UUID),
	}
}

// Message types for UserActor
type (
	RegisterUserMsg struct {
		Username string
		Email    string
		Password string
		Karma    int
	}

	UpdateProfileMsg struct {
		UserID      uuid.UUID
		NewUsername string
		NewEmail    string
	}

	UpdateKarmaMsg struct {
		UserID uuid.UUID
		Delta  int // Positive for upvote, negative for downvote
	}

	GetUserProfileMsg struct {
		UserID uuid.UUID
	}

	LoginMsg struct {
		Email    string
		Password string
	}

	LoginResponse struct {
		Success bool
		Token   string
		Error   string
	}

	VoteMsg struct {
		UserID     uuid.UUID
		TargetID   uuid.UUID // Post or Comment ID
		TargetType string    // "post" or "comment"
		IsUpvote   bool
	}

	GetFeedMsg struct {
		UserID uuid.UUID
		Limit  int
	}

	AddToFeedMsg struct {
		PostID      uuid.UUID
		SubredditID uuid.UUID
	}
)

// UserState represents the internal state
type UserState struct {
	ID             uuid.UUID
	Username       string
	Email          string
	Karma          int
	Posts          []uuid.UUID
	Comments       []uuid.UUID
	HashedPassword string
	AuthToken      string
	Subscriptions  []uuid.UUID        // Subreddit IDs
	VotedPosts     map[uuid.UUID]bool // PostID -> isUpvote
	VotedComments  map[uuid.UUID]bool // CommentID -> isUpvote
}

func (s *UserSupervisor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *RegisterUserMsg:
		s.mu.Lock()
		defer s.mu.Unlock()

		// Check if email already exists
		if _, exists := s.emailToID[msg.Email]; exists {
			context.Respond(utils.NewAppError(utils.ErrDuplicate, "Email already registered", nil))
			return
		}

		// Create new user ID and actor
		userID := uuid.New()
		props := actor.PropsFromProducer(func() actor.Actor {
			return NewUserActor(userID, msg)
		})

		pid := context.Spawn(props)
		s.userActors[userID] = pid
		s.emailToID[msg.Email] = userID

		// Forward registration to new actor
		future := context.RequestFuture(pid, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "User creation failed", err))
			return
		}
		context.Respond(result)

	case *LoginMsg:
		s.mu.RLock()
		userID, exists := s.emailToID[msg.Email]
		s.mu.RUnlock()

		if !exists {
			context.Respond(&LoginResponse{Success: false, Error: "Invalid credentials"})
			return
		}

		// Forward to specific user actor
		future := context.RequestFuture(s.userActors[userID], msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			context.Respond(&LoginResponse{Success: false, Error: "Login failed"})
			return
		}
		context.Respond(result)

	case *GetUserProfileMsg:
		s.mu.RLock()
		pid, exists := s.userActors[msg.UserID]
		s.mu.RUnlock()

		if !exists {
			context.Respond(utils.NewAppError(utils.ErrNotFound, "User not found", nil))
			return
		}

		future := context.RequestFuture(pid, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "Failed to get profile", err))
			return
		}
		context.Respond(result)
	}
}

// UserActor manages user-related operations
type UserActor struct {
	id    uuid.UUID
	state *UserState
}

func NewUserActor(id uuid.UUID, msg *RegisterUserMsg) *UserActor {
	return &UserActor{
		id: id,
		state: &UserState{
			ID:            id,
			Username:      msg.Username,
			Email:         msg.Email,
			Karma:         200,
			Posts:         make([]uuid.UUID, 0),
			Comments:      make([]uuid.UUID, 0),
			VotedPosts:    make(map[uuid.UUID]bool),
			VotedComments: make(map[uuid.UUID]bool),
			Subscriptions: make([]uuid.UUID, 0),
		},
	}
}

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (a *UserActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *RegisterUserMsg:
		// Hash password before storing
		hashedPassword, err := hashPassword(msg.Password)
		if err != nil {
			context.Respond(nil)
			return
		}

		a.state.Username = msg.Username
		a.state.Email = msg.Email
		a.state.HashedPassword = hashedPassword
		a.state.Karma = 300

		context.Respond(&UserState{
			ID:       a.state.ID,
			Username: a.state.Username,
			Email:    a.state.Email,
			Karma:    a.state.Karma,
		})

	case *UpdateProfileMsg:
		if a.state.ID == msg.UserID {
			a.state.Username = msg.NewUsername
			a.state.Email = msg.NewEmail
			context.Respond(true)
		} else {
			context.Respond(false)
		}

	case *UpdateKarmaMsg:
		if a.state.ID == msg.UserID {
			a.state.Karma += msg.Delta
			context.Respond(a.state.Karma)
		}

	case *GetUserProfileMsg:
		if a.state.ID == msg.UserID {
			context.Respond(a.state)
		} else {
			context.Respond(nil)
		}

	case *LoginMsg:
		if a.state.Email != msg.Email {
			context.Respond(&LoginResponse{Success: false, Error: "Invalid credentials"})
			return
		}

		err := bcrypt.CompareHashAndPassword([]byte(a.state.HashedPassword), []byte(msg.Password))
		if err != nil {
			context.Respond(&LoginResponse{Success: false, Error: "Invalid credentials"})
			return
		}

		token, err := generateToken()
		if err != nil {
			context.Respond(&LoginResponse{Success: false, Error: "Authentication error"})
			return
		}

		a.state.AuthToken = token
		context.Respond(&LoginResponse{
			Success: true,
			Token:   token,
		})
	case *VoteMsg:
		// Check if already voted
		var previousVote bool
		var exists bool

		if msg.TargetType == "post" {
			previousVote, exists = a.state.VotedPosts[msg.TargetID]
		} else {
			previousVote, exists = a.state.VotedComments[msg.TargetID]
		}

		if exists && previousVote == msg.IsUpvote {
			context.Respond(utils.NewAppError(utils.ErrDuplicate, "Already voted", nil))
			return
		}

		// Record the vote
		if msg.TargetType == "post" {
			a.state.VotedPosts[msg.TargetID] = msg.IsUpvote
		} else {
			a.state.VotedComments[msg.TargetID] = msg.IsUpvote
		}

		context.Respond(true)

	case *AddToFeedMsg:
		// Add post to user's feed if subscribed
		for _, subID := range a.state.Subscriptions {
			if subID == msg.SubredditID {
				context.Respond(true)
				return
			}
		}
		context.Respond(false)
	}
}
