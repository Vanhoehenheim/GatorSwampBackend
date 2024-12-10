package actors

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"sync"
	"time"

	stdctx "context"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"gator-swamp/internal/database"
	"gator-swamp/internal/models"
	"gator-swamp/internal/types"
	"gator-swamp/internal/utils"
)

// UserSupervisor is responsible for supervising and managing UserActor instances.
// It ensures that each user has a corresponding actor and creates or retrieves them on-demand.
type UserSupervisor struct {
	userActors map[uuid.UUID]*actor.PID // Maps user IDs to their corresponding actor PIDs
	emailToID  map[string]uuid.UUID     // Maps emails to user IDs for quick lookup
	mu         sync.RWMutex             // Manages concurrent access to maps
	mongodb    *database.MongoDB
}

// NewUserSupervisor initializes a new UserSupervisor with MongoDB connection.
func NewUserSupervisor(mongodb *database.MongoDB) actor.Actor {
	return &UserSupervisor{
		userActors: make(map[uuid.UUID]*actor.PID),
		emailToID:  make(map[string]uuid.UUID),
		mongodb:    mongodb,
	}
}

// Message types for UserSupervisor and UserActor communication
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
		TargetType string // "post" or "comment"
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

// UserState represents the internal state of a user maintained by its actor.
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
	SubredditNames []string // New field
	VotedPosts     map[uuid.UUID]bool
	VotedComments  map[uuid.UUID]bool
}

// Receive is the main message handler for the UserSupervisor.
// It handles user registration, login, profile retrieval, and karma updates by delegating to UserActor instances.
func (s *UserSupervisor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {

	// Handle user registration requests
	case *RegisterUserMsg:
		s.mu.Lock()
		defer s.mu.Unlock()

		// Check if the email is already registered in MongoDB
		ctx := stdctx.Background()
		existingUser, _ := s.mongodb.GetUserByEmail(ctx, msg.Email)
		if existingUser != nil {
			log.Printf("Email already exists in MongoDB: %s", msg.Email)
			context.Respond(utils.NewAppError(utils.ErrDuplicate, "Email already registered", nil))
			return
		}

		// Create a new user actor for this user
		userID := uuid.New()
		props := actor.PropsFromProducer(func() actor.Actor {
			return NewUserActor(userID, msg, s.mongodb)
		})

		pid := context.Spawn(props)
		s.userActors[userID] = pid
		s.emailToID[msg.Email] = userID

		// Send the register message to the user actor and wait for a response
		future := context.RequestFuture(pid, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			log.Printf("Failed to create user: %v", err)
			context.Respond(utils.NewAppError(utils.ErrActorTimeout, "User creation failed", err))
			return
		}
		context.Respond(result)

	// Handle login requests
	case *LoginMsg:
		log.Printf("UserSupervisor: Processing login request for email: %s", msg.Email)

		// Fetch user from MongoDB by email
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

		// Check if an actor for this user already exists
		s.mu.RLock()
		pid, exists := s.userActors[user.ID]
		s.mu.RUnlock()

		if !exists {
			// Create a new actor for this existing user from MongoDB
			props := actor.PropsFromProducer(func() actor.Actor {
				return NewUserActor(user.ID, &RegisterUserMsg{
					Username: user.Username,
					Email:    user.Email,
					Password: "", // Actual password is from MongoDB
					Karma:    user.Karma,
				}, s.mongodb)
			})
			pid = context.Spawn(props)

			s.mu.Lock()
			s.userActors[user.ID] = pid
			s.emailToID[user.Email] = user.ID
			s.mu.Unlock()
		}

		// Forward the login message to the user actor
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

		// Respond with the login result (token or error)
		context.Respond(result)

		// Handle user profile retrieval
	case *GetUserProfileMsg:
		ctx := stdctx.Background()
		user, err := s.mongodb.GetUser(ctx, msg.UserID)
		if err != nil {
			if utils.IsErrorCode(err, utils.ErrUserNotFound) {
				context.Respond(nil)
				return
			}
			context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch user", err))
			return
		}

		// Get the names of all subreddits
		subredditNames := make([]string, 0, len(user.Subreddits))
		for _, subID := range user.Subreddits {
			subreddit, err := s.mongodb.GetSubredditByID(ctx, subID)
			if err != nil {
				log.Printf("Error fetching subreddit %s: %v", subID, err)
				continue
			}
			subredditNames = append(subredditNames, subreddit.Name)
		}

		// We don't have access to VotedPosts and VotedComments here
		// Those would need to come from the UserActor's state
		response := &UserState{
			ID:             user.ID,
			Username:       user.Username,
			Email:          user.Email,
			Karma:          user.Karma,
			IsConnected:    user.IsConnected,
			LastActive:     user.LastActive,
			Subreddits:     user.Subreddits,
			SubredditNames: subredditNames,
			// Initialize empty maps for voted posts/comments
			VotedPosts:    make(map[uuid.UUID]bool),
			VotedComments: make(map[uuid.UUID]bool),
		}

		context.Respond(response)

	// Handle karma updates
	case *UpdateKarmaMsg:
		s.mu.RLock()
		pid, exists := s.userActors[msg.UserID]
		s.mu.RUnlock()

		if !exists {
			log.Printf("UserSupervisor: User %s not found for karma update", msg.UserID)
			return
		}

		// Update MongoDB first
		ctx := stdctx.Background()
		err := s.mongodb.UpdateUserKarma(ctx, msg.UserID, msg.Delta)
		if err != nil {
			log.Printf("UserSupervisor: Failed to update karma in MongoDB for user %s: %v", msg.UserID, err)
			return
		}

		// Then update the actor's state
		log.Printf("UserSupervisor: Forwarding karma update to user actor %s", msg.UserID)
		context.Send(pid, msg)
	}
}

// getOrCreateUserActor ensures that a user actor exists for the given userID.
// If it doesn't, it fetches the user from MongoDB and creates a new actor.
func (s *UserSupervisor) getOrCreateUserActor(context actor.Context, userID uuid.UUID) (*actor.PID, error) {
	s.mu.RLock()
	pid, exists := s.userActors[userID]
	s.mu.RUnlock()

	if exists {
		return pid, nil
	}

	// Fetch user details from MongoDB
	ctx := stdctx.Background()
	user, err := s.mongodb.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Create a new actor if none exists
	props := actor.PropsFromProducer(func() actor.Actor {
		return NewUserActor(user.ID, &RegisterUserMsg{
			Username: user.Username,
			Email:    user.Email,
			Password: user.HashedPassword, // Use hashed password directly
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

// UserActor is responsible for managing the state of a single user.
// It handles messages related to user registration, login, profile updates, voting, etc.
type UserActor struct {
	id      uuid.UUID
	state   *UserState
	mongodb *database.MongoDB
}

// NewUserActor creates a new user actor with initial user state, typically during registration or actor creation for an existing user.
func NewUserActor(id uuid.UUID, msg *RegisterUserMsg, mongodb *database.MongoDB) *UserActor {
	return &UserActor{
		id: id,
		state: &UserState{
			ID:            id,
			Username:      msg.Username,
			Email:         msg.Email,
			Karma:         300, // Default initial karma
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

// hashPassword securely hashes a user password using bcrypt
func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

// generateToken creates a secure random token for authentication purposes
func generateToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// Receive is the main message handler for the UserActor. It processes incoming messages related to user operations.
func (a *UserActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {

	// Handle user registration inside the user actor
	case *RegisterUserMsg:
		hashedPassword, err := hashPassword(msg.Password)
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrInvalidInput, "Failed to hash password", err))
			return
		}

		// Update actor state with newly registered user info
		a.state.Username = msg.Username
		a.state.Email = msg.Email
		a.state.HashedPassword = hashedPassword
		a.state.Karma = 300
		a.state.Subreddits = make([]uuid.UUID, 0)

		// Create a user model for MongoDB storage
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

		// Persist the user in MongoDB
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

	// Handle user profile updates (username/email)
	case *UpdateProfileMsg:
		if a.state.ID == msg.UserID {
			a.state.Username = msg.NewUsername
			a.state.Email = msg.NewEmail
			context.Respond(true)
		} else {
			context.Respond(false)
		}

	// Handle karma updates
	case *UpdateKarmaMsg:
		if a.state.ID == msg.UserID {
			log.Printf("UserActor: Updating karma for user %s by %d", msg.UserID, msg.Delta)
			a.state.Karma += msg.Delta
		}

	// Handle user profile retrieval
	case *GetUserProfileMsg:
		ctx := stdctx.Background()
		user, err := a.mongodb.GetUser(ctx, msg.UserID)
		if err != nil {
			if utils.IsErrorCode(err, utils.ErrUserNotFound) {
				context.Respond(nil) // User not found
				return
			}
			log.Printf("Error fetching user from MongoDB: %v", err)
			context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch user", err))
			return
		}

		// Update actor state from the database record
		a.state = &UserState{
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

		context.Respond(a.state)

	// Handle user login
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
			log.Printf("Login failed - Password mismatch: %v", err)
			context.Respond(&types.LoginResponse{
				Success: false,
				Error:   "Invalid credentials",
			})
			return
		}

		// Generate a new auth token for the session
		token, err := generateToken()
		if err != nil {
			log.Printf("Failed to generate auth token: %v", err)
			context.Respond(&types.LoginResponse{
				Success: false,
				Error:   "Authentication error",
			})
			return
		}

		// Update user activity in MongoDB
		err = a.mongodb.UpdateUserActivity(ctx, user.ID, true)
		if err != nil {
			log.Printf("Warning: Failed to update user activity in MongoDB: %v", err)
		}

		// Update actor state with new auth token and connection status
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
			UserID:  user.ID.String(),
		})

	// Handle voting (upvotes/downvotes on posts or comments)
	case *VoteMsg:
		var previousVote bool
		var exists bool

		// Check if user already voted on this target
		if msg.TargetType == "post" {
			previousVote, exists = a.state.VotedPosts[msg.TargetID]
		} else {
			previousVote, exists = a.state.VotedComments[msg.TargetID]
		}

		// If the vote is the same as before, respond with an error
		if exists && previousVote == msg.IsUpvote {
			context.Respond(utils.NewAppError(utils.ErrDuplicate, "Already voted", nil))
			return
		}

		// Record the new vote
		if msg.TargetType == "post" {
			a.state.VotedPosts[msg.TargetID] = msg.IsUpvote
		} else {
			a.state.VotedComments[msg.TargetID] = msg.IsUpvote
		}

		context.Respond(true)

	// Handle feed additions — checks if the user follows the subreddit before adding
	case *AddToFeedMsg:
		for _, subID := range a.state.Subreddits {
			if subID == msg.SubredditID {
				context.Respond(true)
				return
			}
		}
		context.Respond(false)

	// Handle user connection events
	case *ConnectUserMsg:
		a.state.IsConnected = true
		a.state.LastActive = time.Now()
		context.Respond(true)

	// Handle user disconnection events
	case *DisconnectUserMsg:
		a.state.IsConnected = false
		context.Respond(true)
	}
}
