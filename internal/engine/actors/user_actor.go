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
	db         database.DBAdapter       // Database adapter interface
}

// NewUserSupervisor initializes a new UserSupervisor with DBAdapter.
func NewUserSupervisor(db database.DBAdapter) actor.Actor {
	return &UserSupervisor{
		userActors: make(map[uuid.UUID]*actor.PID),
		emailToID:  make(map[string]uuid.UUID),
		db:         db, // Assign the db interface
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

	GetUserProfileMsg struct {
		UserID uuid.UUID
	}

	LoginMsg struct {
		Email    string
		Password string
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
}

// Receive is the main message handler for the UserSupervisor.
// It handles user registration, login, profile retrieval, and karma updates by delegating to UserActor instances.
func (s *UserSupervisor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {

	// Handle user registration requests
	case *RegisterUserMsg:
		s.mu.Lock()
		defer s.mu.Unlock()

		// Check if the email is already registered
		ctx := stdctx.Background()
		// TODO: Add GetUserByEmail to DBAdapter interface
		existingUser, _ := s.db.GetUserByEmail(ctx, msg.Email)
		if existingUser != nil {
			log.Printf("Email already exists in DB: %s", msg.Email)
			context.Respond(utils.NewAppError(utils.ErrDuplicate, "Email already registered", nil))
			return
		}

		// Create a new user actor for this user
		userID := uuid.New()
		props := actor.PropsFromProducer(func() actor.Actor {
			// TODO: Update NewUserActor signature
			return NewUserActor(userID, msg, s.db)
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

		// Fetch user from DB by email
		ctx := stdctx.Background()
		// TODO: Add GetUserByEmail to DBAdapter interface
		user, err := s.db.GetUserByEmail(ctx, msg.Email)
		if err != nil {
			log.Printf("UserSupervisor: User not found in DB: %v", err)
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
			// Create a new actor for this existing user from DB
			props := actor.PropsFromProducer(func() actor.Actor {
				// TODO: Update NewUserActor signature
				return NewUserActor(user.ID, &RegisterUserMsg{
					Username: user.Username,
					Email:    user.Email,
					Password: "", // Actual password is from DB
					Karma:    user.Karma,
				}, s.db)
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
		// TODO: Add GetUser to DBAdapter interface
		user, err := s.db.GetUser(ctx, msg.UserID)
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
			// TODO: Add GetSubredditByID to DBAdapter interface
			subreddit, err := s.db.GetSubredditByID(ctx, subID)
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
		}

		context.Respond(response)
	}
}

// getOrCreateUserActor ensures that a user actor exists for the given userID.
// If it doesn't, it fetches the user from the database and creates a new actor.
func (s *UserSupervisor) getOrCreateUserActor(context actor.Context, userID uuid.UUID) (*actor.PID, error) {
	s.mu.RLock()
	pid, exists := s.userActors[userID]
	s.mu.RUnlock()

	if exists {
		return pid, nil
	}

	// Fetch user details from the database
	ctx := stdctx.Background()
	// TODO: Add GetUser to DBAdapter interface
	user, err := s.db.GetUser(ctx, userID)
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
		}, s.db)
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
	id    uuid.UUID
	state *UserState
	db    database.DBAdapter
}

// NewUserActor creates a new user actor with initial user state, typically during registration or actor creation for an existing user.
func NewUserActor(id uuid.UUID, msg *RegisterUserMsg, db database.DBAdapter) *UserActor {
	return &UserActor{
		id: id,
		state: &UserState{
			ID:          id,
			Username:    msg.Username,
			Email:       msg.Email,
			Karma:       300, // Default initial karma
			IsConnected: true,
			LastActive:  time.Now(),
			Posts:       make([]uuid.UUID, 0),
			Comments:    make([]uuid.UUID, 0),
			Subreddits:  make([]uuid.UUID, 0),
		},
		db: db,
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
		log.Printf("UserActor [%s]: Registering new user", a.id)

		// Hash password
		hashedPassword, err := hashPassword(msg.Password)
		if err != nil {
			context.Respond(utils.NewAppError(utils.ErrInvalidInput, "Failed to hash password", err))
			return
		}

		// Create a user model for database storage
		user := &models.User{
			ID:             a.id,
			Username:       msg.Username,
			Email:          msg.Email,
			HashedPassword: hashedPassword,
			Karma:          msg.Karma,
			CreatedAt:      time.Now(),
			LastActive:     time.Now(),
			IsConnected:    true,
			Subreddits:     a.state.Subreddits,
		}

		// Persist the user in the database
		ctx := stdctx.Background()
		// TODO: Add SaveUser to DBAdapter interface
		if err := a.db.SaveUser(ctx, user); err != nil {
			log.Printf("Failed to save user to DB: %v", err)
			context.Respond(utils.NewAppError(utils.ErrInvalidInput, "Failed to save user", err))
			return
		}

		log.Printf("Successfully created user %s in DB", a.id)

		context.Respond(&UserState{
			ID:       a.id,
			Username: msg.Username,
			Email:    msg.Email,
			Karma:    msg.Karma,
		})

	// Handle user profile updates (username/email)
	case *UpdateProfileMsg:
		if a.id == msg.UserID {
			a.state.Username = msg.NewUsername
			a.state.Email = msg.NewEmail
			context.Respond(true)
		} else {
			context.Respond(false)
		}

	// Handle user profile retrieval
	case *GetUserProfileMsg:
		// Fetch latest persistent data from DB
		ctx := stdctx.Background()
		user, err := a.db.GetUser(ctx, msg.UserID)
		if err != nil {
			if utils.IsErrorCode(err, utils.ErrNotFound) {
				context.Respond(nil) // User not found
				return
			}
			log.Printf("Error fetching user %s from DB for profile: %v", msg.UserID, err)
			context.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to fetch user profile data", err))
			return
		}

		// Update actor state with fetched persistent data,
		// preserving volatile state (IsConnected, LastActive, AuthToken)
		a.state.Username = user.Username
		a.state.Email = user.Email
		a.state.Karma = user.Karma
		a.state.HashedPassword = user.HashedPassword // Keep password hash synchronized
		a.state.Subreddits = user.Subreddits
		// a.state.IsConnected is managed by Connect/Disconnect messages
		// a.state.LastActive is managed by Connect/Login messages
		// a.state.AuthToken is managed by Login messages

		// Fetch subreddit names based on the updated list
		subredditNames := make([]string, 0, len(a.state.Subreddits))
		for _, subID := range a.state.Subreddits {
			subreddit, err := a.db.GetSubredditByID(ctx, subID)
			if err != nil {
				log.Printf("Error fetching subreddit %s for user %s profile: %v", subID, msg.UserID, err)
				// Optionally add a placeholder or skip
				continue
			}
			subredditNames = append(subredditNames, subreddit.Name)
		}
		a.state.SubredditNames = subredditNames

		// Respond with the updated actor state
		context.Respond(a.state)

	// Handle user login
	case *LoginMsg:
		log.Printf("Processing login request for email: %s", msg.Email)

		ctx := stdctx.Background()
		user, err := a.db.GetUserByEmail(ctx, msg.Email)
		if err != nil {
			log.Printf("Login failed - Error fetching user from DB: %v", err)
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

		// Update user activity in the database
		err = a.db.UpdateUserActivity(ctx, user.ID, true)
		if err != nil {
			log.Printf("Warning: Failed to update user activity in DB: %v", err)
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
			Subreddits:     user.Subreddits,
		}

		log.Printf("Login successful for user: %s", user.Username)

		context.Respond(&types.LoginResponse{
			Success: true,
			Token:   token,
			UserID:  user.ID.String(),
		})

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

	default:
		log.Printf("UserActor %s received unknown message type: %T", a.id, msg)
	}
}
