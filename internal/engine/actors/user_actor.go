package actors

import (
	"crypto/rand"
	"encoding/base64"
	"gator-swamp/internal/database"
	"gator-swamp/internal/models"
	"gator-swamp/internal/utils"
	"log"
	"sync"
	"time"

	stdctx "context"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// UserSupervisor manages all user actors
type UserSupervisor struct {
	userActors map[uuid.UUID]*actor.PID
	emailToID  map[string]uuid.UUID
	mu         sync.RWMutex
	mongodb    *database.MongoDB
}

func NewUserSupervisor(mongodb *database.MongoDB) actor.Actor {
	return &UserSupervisor{
		userActors: make(map[uuid.UUID]*actor.PID),
		emailToID:  make(map[string]uuid.UUID),
		mongodb:    mongodb,
	}
}

// Message types remain the same
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
		Delta  int
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
		TargetID   uuid.UUID
		TargetType string
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

	ConnectUserMsg struct {
		UserID uuid.UUID
	}

	DisconnectUserMsg struct {
		UserID uuid.UUID
	}
)

// UserState represents the internal state
type UserState struct {
	ID             uuid.UUID
	Username       string
	Email          string
	Karma          int
	IsConnected    bool
	LastActive     time.Time
	Posts          []uuid.UUID
	Comments       []uuid.UUID
	HashedPassword string
	AuthToken      string
	Subreddits  []uuid.UUID
	VotedPosts     map[uuid.UUID]bool
	VotedComments  map[uuid.UUID]bool
}

func (s *UserSupervisor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *RegisterUserMsg:
		s.mu.Lock()
		defer s.mu.Unlock()

		// Check if email exists in MongoDB first
		ctx := stdctx.Background()
		existingUser, _ := s.mongodb.GetUserByEmail(ctx, msg.Email)
		if existingUser != nil {
			log.Printf("Email already exists in MongoDB: %s", msg.Email)
			context.Respond(utils.NewAppError(utils.ErrDuplicate, "Email already registered", nil))
			return
		}

		// Create new user ID and actor
		userID := uuid.New()
		props := actor.PropsFromProducer(func() actor.Actor {
			return NewUserActor(userID, msg, s.mongodb)
		})

		pid := context.Spawn(props)
		s.userActors[userID] = pid
		s.emailToID[msg.Email] = userID

		future := context.RequestFuture(pid, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			log.Printf("Failed to create user: %v", err)
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

	case *UpdateKarmaMsg:
		s.mu.RLock()
		pid, exists := s.userActors[msg.UserID]
		s.mu.RUnlock()

		if !exists {
			log.Printf("UserSupervisor: User %s not found for karma update", msg.UserID)
			return
		}

		log.Printf("UserSupervisor: Forwarding karma update to user actor %s", msg.UserID)
		context.Send(pid, msg)
	}
}

type UserActor struct {
	id      uuid.UUID
	state   *UserState
	mongodb *database.MongoDB
}

func NewUserActor(id uuid.UUID, msg *RegisterUserMsg, mongodb *database.MongoDB) *UserActor {
	return &UserActor{
		id: id,
		state: &UserState{
			ID:            id,
			Username:      msg.Username,
			Email:         msg.Email,
			Karma:         300,
			IsConnected:   true,
			LastActive:    time.Now(),
			Posts:         make([]uuid.UUID, 0),
			Comments:      make([]uuid.UUID, 0),
			VotedPosts:    make(map[uuid.UUID]bool),
			VotedComments: make(map[uuid.UUID]bool),
			Subreddits: make([]uuid.UUID, 0),
		},
		mongodb: mongodb,
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
			context.Respond(utils.NewAppError(utils.ErrInvalidInput, "Failed to hash password", err))
			return
		}

		a.state.Username = msg.Username
		a.state.Email = msg.Email
		a.state.HashedPassword = hashedPassword
		a.state.Karma = 300
		a.state.Subreddits = make([]uuid.UUID, 0)

		// Create user model for MongoDB
		user := &models.User{
			ID:             a.state.ID,
			Username:       a.state.Username,
			Email:          a.state.Email,
			HashedPassword: hashedPassword,
			Karma:          a.state.Karma,
			CreatedAt:      time.Now(),
			LastActive:     time.Now(),
			IsConnected:    true,
			Subreddits: a.state.Subreddits,
		}

		// Save to MongoDB
		ctx := stdctx.Background()
		if err := a.mongodb.SaveUser(ctx, user); err != nil {
			log.Printf("Failed to save user to MongoDB: %v", err)
			context.Respond(utils.NewAppError(utils.ErrInvalidInput, "Failed to save user", err))
			return
		}

		log.Printf("Successfully created user %s in MongoDB", a.state.ID)

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
			log.Printf("UserActor: Updating karma for user %s by %d", msg.UserID, msg.Delta)
			a.state.Karma += msg.Delta
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

		if msg.TargetType == "post" {
			a.state.VotedPosts[msg.TargetID] = msg.IsUpvote
		} else {
			a.state.VotedComments[msg.TargetID] = msg.IsUpvote
		}

		context.Respond(true)

	case *AddToFeedMsg:
		for _, subID := range a.state.Subreddits {
			if subID == msg.SubredditID {
				context.Respond(true)
				return
			}
		}
		context.Respond(false)

	case *ConnectUserMsg:
		a.state.IsConnected = true
		a.state.LastActive = time.Now()
		context.Respond(true)

	case *DisconnectUserMsg:
		a.state.IsConnected = false
		context.Respond(true)
	}
}
