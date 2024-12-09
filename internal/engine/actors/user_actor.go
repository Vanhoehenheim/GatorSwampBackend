package actors

import (
	"crypto/rand"
	"encoding/base64"
	"gator-swamp/internal/database"
	"gator-swamp/internal/models"
	"gator-swamp/internal/types"
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
	Subreddits     []uuid.UUID
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
		log.Printf("UserSupervisor: Processing login request for email: %s", msg.Email)

		// Get user from MongoDB
		ctx := stdctx.Background()
		user, err := s.mongodb.GetUserByEmail(ctx, msg.Email)
		if err != nil {
			log.Printf("UserSupervisor: User not found in MongoDB: %v", err)
			context.Respond(&types.LoginResponse{
				Success: false,
				Error:   "Invalid credentials",
			})
			return
		}

		// Check if we already have an actor for this user
		s.mu.RLock()
		pid, exists := s.userActors[user.ID]
		s.mu.RUnlock()

		if !exists {
			// Create new actor for this user
			props := actor.PropsFromProducer(func() actor.Actor {
				return NewUserActor(user.ID, &RegisterUserMsg{
					Username: user.Username,
					Email:    user.Email,
					Password: "", // Password will be set from MongoDB data
					Karma:    user.Karma,
				}, s.mongodb)
			})

			pid = context.Spawn(props)

			s.mu.Lock()
			s.userActors[user.ID] = pid
			s.emailToID[user.Email] = user.ID
			s.mu.Unlock()
		}

		// Forward login request to the user actor
		future := context.RequestFuture(pid, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			log.Printf("UserSupervisor: Login request to user actor failed: %v", err)
			context.Respond(&types.LoginResponse{
				Success: false,
				Error:   "Login failed",
			})
			return
		}

		// Forward the response
		context.Respond(result)

	case *GetUserProfileMsg:
		log.Printf("UserSupervisor: Getting profile for user ID: %s", msg.UserID)

		pid, err := s.getOrCreateUserActor(context, msg.UserID)
		if err != nil {
			log.Printf("UserSupervisor: Failed to get/create user actor: %v", err)
			context.Respond(utils.NewAppError(utils.ErrNotFound, "User not found", err))
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

func (s *UserSupervisor) getOrCreateUserActor(context actor.Context, userID uuid.UUID) (*actor.PID, error) {
	s.mu.RLock()
	pid, exists := s.userActors[userID]
	s.mu.RUnlock()

	if exists {
		return pid, nil
	}

	// Get user from MongoDB
	ctx := stdctx.Background()
	user, err := s.mongodb.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Create new actor for this user
	props := actor.PropsFromProducer(func() actor.Actor {
		return NewUserActor(user.ID, &RegisterUserMsg{
			Username: user.Username,
			Email:    user.Email,
			Password: user.HashedPassword,
			Karma:    user.Karma,
		}, s.mongodb)
	})

	pid = context.Spawn(props)

	s.mu.Lock()
	s.userActors[user.ID] = pid
	s.emailToID[user.Email] = user.ID
	s.mu.Unlock()

	return pid, nil
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
			Subreddits:    make([]uuid.UUID, 0),
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
			Subreddits:     a.state.Subreddits,
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
		ctx := stdctx.Background()
		user, err := a.mongodb.GetUser(ctx, msg.UserID)
		if err != nil {
			if utils.IsErrorCode(err, utils.ErrUserNotFound) {
				context.Respond(nil)
				return
			}
			log.Printf("Error fetching user from MongoDB: %v", err)
			context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch user", err))
			return
		}

		// Convert MongoDB user to UserState
		state := &UserState{
			ID:             user.ID,
			Username:       user.Username,
			Email:          user.Email,
			Karma:          user.Karma,
			IsConnected:    user.IsConnected,
			LastActive:     user.LastActive,
			HashedPassword: user.HashedPassword,
			Subreddits:     user.Subreddits,
			VotedPosts:     make(map[uuid.UUID]bool),
			VotedComments:  make(map[uuid.UUID]bool),
		}

		// Update in-memory state
		a.state = state
		context.Respond(state)

	case *LoginMsg:
		log.Printf("Processing login request for email: %s", msg.Email)

		ctx := stdctx.Background()
		user, err := a.mongodb.GetUserByEmail(ctx, msg.Email)
		if err != nil {
			log.Printf("Login failed - Error fetching user from MongoDB: %v", err)
			context.Respond(&types.LoginResponse{
				Success: false,
				Error:   "Invalid credentials",
			})
			return
		}

		// Verify password
		err = bcrypt.CompareHashAndPassword([]byte(user.HashedPassword), []byte(msg.Password))
		if err != nil {
			log.Printf("Login failed - Password comparison error: %v", err)
			context.Respond(&types.LoginResponse{
				Success: false,
				Error:   "Invalid credentials",
			})
			return
		}

		// Generate authentication token
		token, err := generateToken()
		if err != nil {
			log.Printf("Failed to generate auth token: %v", err)
			context.Respond(&types.LoginResponse{
				Success: false,
				Error:   "Authentication error",
			})
			return
		}

		// Update user's last active time and connected status
		err = a.mongodb.UpdateUserActivity(ctx, user.ID, true)
		if err != nil {
			log.Printf("Warning: Failed to update user activity in MongoDB: %v", err)
		}

		// Update actor's state
		a.state = &UserState{
			ID:             user.ID,
			Username:       user.Username,
			Email:          user.Email,
			Karma:          user.Karma,
			IsConnected:    true,
			LastActive:     time.Now(),
			AuthToken:      token,
			HashedPassword: user.HashedPassword,
			VotedPosts:     make(map[uuid.UUID]bool),
			VotedComments:  make(map[uuid.UUID]bool),
			Subreddits:     user.Subreddits,
		}

		log.Printf("Login successful for user: %s", user.Username)

		context.Respond(&types.LoginResponse{
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
