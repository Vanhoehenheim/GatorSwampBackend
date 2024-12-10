package main

// Import necessary packages
import (
	"context"
	"encoding/json"               // JSON encoding and decoding
	"fmt"                         // String formatting
	"gator-swamp/internal/config" // Configuration handling
	"gator-swamp/internal/database"
	"gator-swamp/internal/engine"        // Engine for managing actors
	"gator-swamp/internal/engine/actors" // For UserActors, SubredditActors, and PostActors
	"gator-swamp/internal/models"
	"gator-swamp/internal/utils" // Utility functions and metrics
	"log"                        // Logging
	"net/http"                   // HTTP server
	"strconv"                    // String conversion utilities
	"time"                       // Time utilities

	"github.com/asynkron/protoactor-go/actor" // ProtoActor for actor-based concurrency
	"github.com/google/uuid"                  // UUID generation for unique identifiers
)

// Request structs for handling JSON input

// CreateSubredditRequest represents a request to create a new subreddit
type CreateSubredditRequest struct {
	Name               string `json:"name"`        // Subreddit name
	Description        string `json:"description"` // Subreddit description
	CreatorID          string `json:"creatorId"`   // Creator ID (UUID as string)
	directMessageActor *actor.PID
}

// CreatePostRequest represents a request to create a new post
type CreatePostRequest struct {
	Title       string `json:"title"`       // Post title
	Content     string `json:"content"`     // Post content
	AuthorID    string `json:"authorId"`    // Author ID (UUID as string)
	SubredditID string `json:"subredditId"` // Subreddit ID (UUID as string)
}

// SubredditResponse is a response struct representing a subreddit with relevant details
type SubredditResponse struct {
	ID          string    `json:"id"`          // Subreddit ID
	Name        string    `json:"name"`        // Subreddit name
	Description string    `json:"description"` // Subreddit description
	Members     int       `json:"members"`     // Number of members
	CreatedAt   time.Time `json:"createdAt"`   // Timestamp of creation
}

// Server holds all server dependencies, including the actor system and engine
type Server struct {
	system             *actor.ActorSystem
	context            *actor.RootContext
	engine             *engine.Engine
	enginePID          *actor.PID // Add this field
	metrics            *utils.MetricsCollector
	commentActor       *actor.PID //Added comment actor
	directMessageActor *actor.PID
	mongodb            *database.MongoDB // Add MongoDB to server struct

	// Remove userActor field as we'll use engine.GetUserSupervisor()
}

// User-related request structures
type RegisterUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Karma    int    `json:"karma"`
}

// User-related login
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token,omitempty"`
	Error   string `json:"error,omitempty"`
	UserID  string `json:"userId"`
}

// Add new request types
type VoteRequest struct {
	UserID   string `json:"userId"`
	PostID   string `json:"postId"`
	IsUpvote bool   `json:"isUpvote"`
}

type GetFeedRequest struct {
	UserID string `json:"userId"`
	Limit  int    `json:"limit"`
}

// Creating Comments struct
type CreateCommentRequest struct {
	Content  string `json:"content"`
	AuthorID string `json:"authorId"`
	PostID   string `json:"postId"`
	ParentID string `json:"parentId,omitempty"` // Optional, for replies
}

// Editing Comments struct
type EditCommentRequest struct {
	CommentID string `json:"commentId"`
	AuthorID  string `json:"authorId"`
	Content   string `json:"content"`
}
type SendMessageRequest struct {
	FromID  string `json:"fromId"`
	ToID    string `json:"toId"`
	Content string `json:"content"`
}

func main() {
	// Initialize MongoDB
	mongoURI := "mongodb+srv://panangadanprajay:golangReddit123!@cluster0.wpgh3.mongodb.net/?retryWrites=true&w=majority&appName=Cluster0"
	mongodb, err := database.NewMongoDB(mongoURI)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer func() {
		if err := mongodb.Close(context.Background()); err != nil {
			log.Printf("Error closing MongoDB connection: %v", err)
		}
	}()
	if err := mongodb.FixCommentSubreddits(context.Background()); err != nil {
		log.Printf("Error fixing comment subreddits: %v", err)
	}
	cfg := config.DefaultConfig()
	metrics := utils.NewMetricsCollector()
	system := actor.NewActorSystem()
	// Pass mongodb to engine
	gatorEngine := engine.NewEngine(system, metrics, mongodb)
	engineProps := actor.PropsFromProducer(func() actor.Actor {
		return gatorEngine
	})
	enginePID := system.Root.Spawn(engineProps)

	server := &Server{
		system:    system,
		context:   system.Root,
		engine:    gatorEngine,
		enginePID: enginePID,
		metrics:   metrics,
		mongodb:   mongodb, // Add MongoDB to server struct
	}
	// Initialize actors with MongoDB
	server.commentActor = system.Root.Spawn(actor.PropsFromProducer(func() actor.Actor {
		return actors.NewCommentActor(enginePID, mongodb)
	}))
	log.Printf("Initialized CommentActor with PID: %v", server.commentActor)

	server.directMessageActor = system.Root.Spawn(actor.PropsFromProducer(func() actor.Actor {
		return actors.NewDirectMessageActor(mongodb)
	}))
	// Set up HTTP endpoints
	http.HandleFunc("/health", corsMiddleware(server.handleHealth()))
	http.HandleFunc("/subreddit", corsMiddleware(server.handleSubreddits()))
	http.HandleFunc("/subreddit/members", corsMiddleware(server.handleSubredditMembers()))
	http.HandleFunc("/post", corsMiddleware(server.handlePost()))
	http.HandleFunc("/user/register", corsMiddleware(server.handleUserRegistration()))
	http.HandleFunc("/user/login", corsMiddleware(server.handleUserLogin()))
	http.HandleFunc("/post/vote", corsMiddleware(server.handleVote()))
	http.HandleFunc("/user/feed", corsMiddleware(server.handleGetFeed()))
	http.HandleFunc("/user/profile", corsMiddleware(server.handleUserProfile()))
	http.HandleFunc("/comment", corsMiddleware(server.handleComment()))
	http.HandleFunc("/comment/post", corsMiddleware(server.handleGetPostComments()))
	http.HandleFunc("/messages", corsMiddleware(server.handleDirectMessages()))
	http.HandleFunc("/messages/conversation", corsMiddleware(server.handleConversation()))
	http.HandleFunc("/messages/read", corsMiddleware(server.handleMarkMessageRead()))
	http.HandleFunc("/comment/vote", corsMiddleware(server.handleCommentVote()))
	http.HandleFunc("/posts/recent", corsMiddleware(server.handleRecentPosts()))

	serverAddr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("Starting server on %s", serverAddr)
	if err := http.ListenAndServe(serverAddr, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*") // For development
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// handleHealth checks the health of the system
func (s *Server) handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get the subreddit count from SubredditActor
		futureSubreddits := s.context.RequestFuture(s.engine.GetSubredditActor(), &actors.GetCountsMsg{}, 5*time.Second)
		subredditResult, err := futureSubreddits.Result()
		if err != nil {
			http.Error(w, "Failed to get subreddit count", http.StatusInternalServerError)
			return
		}
		subredditCount := subredditResult.(int) // Parse the result

		// Get the post count from PostActor
		futurePosts := s.context.RequestFuture(s.engine.GetPostActor(), &actors.GetCountsMsg{}, 5*time.Second)
		postResult, err := futurePosts.Result()
		if err != nil {
			http.Error(w, "Failed to get post count", http.StatusInternalServerError)
			return
		}
		postCount := postResult.(int) // Parse the result

		// Respond with the subreddit and post counts
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":          "healthy",
			"subreddit_count": subredditCount,
			"post_count":      postCount,
		})
	}
}

// handleSubreddits handles requests related to subreddits, such as listing all subreddits or creating a new one
func (s *Server) handleSubreddits() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// Handle listing all subreddits
			future := s.context.RequestFuture(s.engine.GetSubredditActor(), &actors.ListSubredditsMsg{}, 5*time.Second)
			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to get subreddits", http.StatusInternalServerError)
				return
			}

			// Respond with the list of subreddits
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		case http.MethodPost:
			var req CreateSubredditRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			creatorID, err := uuid.Parse(req.CreatorID)
			if err != nil {
				http.Error(w, "Invalid creator ID format", http.StatusBadRequest)
				return
			}

			// Create the message
			msg := &actors.CreateSubredditMsg{
				Name:        req.Name,
				Description: req.Description,
				CreatorID:   creatorID,
			}

			// Send to Engine for validation and processing
			future := s.context.RequestFuture(s.enginePID, msg, 5*time.Second)
			result, err := future.Result()
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to create subreddit: %v", err), http.StatusInternalServerError)
				return
			}

			// Check for application errors
			if appErr, ok := result.(*utils.AppError); ok {
				var statusCode int
				switch appErr.Code {
				case utils.ErrNotFound:
					statusCode = http.StatusNotFound
				case utils.ErrInvalidInput:
					statusCode = http.StatusBadRequest
				case utils.ErrUnauthorized:
					statusCode = http.StatusUnauthorized
				default:
					statusCode = http.StatusInternalServerError
				}
				http.Error(w, appErr.Error(), statusCode)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// handleSubredditMembers handles requests to retrieve the members of a specific subreddit
func (s *Server) handleSubredditMembers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// Existing GET logic
			subredditID := r.URL.Query().Get("id")
			if subredditID == "" {
				http.Error(w, "Subreddit ID required", http.StatusBadRequest)
				return
			}

			id, err := uuid.Parse(subredditID)
			if err != nil {
				http.Error(w, "Invalid subreddit ID", http.StatusBadRequest)
				return
			}

			msg := &actors.GetSubredditMembersMsg{SubredditID: id}
			future := s.context.RequestFuture(s.engine.GetSubredditActor(), msg, 5*time.Second)
			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to get members", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		case http.MethodPost:
			// New POST logic for joining
			var req struct {
				SubredditID string `json:"subredditId"`
				UserID      string `json:"userId"`
			}

			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}

			subredditID, err := uuid.Parse(req.SubredditID)
			if err != nil {
				http.Error(w, "Invalid subreddit ID format", http.StatusBadRequest)
				return
			}

			userID, err := uuid.Parse(req.UserID)
			if err != nil {
				http.Error(w, "Invalid user ID format", http.StatusBadRequest)
				return
			}

			future := s.context.RequestFuture(s.engine.GetSubredditActor(),
				&actors.JoinSubredditMsg{
					SubredditID: subredditID,
					UserID:      userID,
				}, 5*time.Second)

			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to join subreddit", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		case http.MethodDelete:
			// New DELETE logic for leaving
			var req struct {
				SubredditID string `json:"subredditId"`
				UserID      string `json:"userId"`
			}

			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}

			subredditID, err := uuid.Parse(req.SubredditID)
			if err != nil {
				http.Error(w, "Invalid subreddit ID format", http.StatusBadRequest)
				return
			}

			userID, err := uuid.Parse(req.UserID)
			if err != nil {
				http.Error(w, "Invalid user ID format", http.StatusBadRequest)
				return
			}

			future := s.context.RequestFuture(s.engine.GetSubredditActor(),
				&actors.LeaveSubredditMsg{
					SubredditID: subredditID,
					UserID:      userID,
				}, 5*time.Second)

			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to leave subreddit", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// handlePost handles post-related requests, such as creating a new post
// handlePost handles post-related requests, such as creating a new post
func (s *Server) handlePost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		// create new post
		case http.MethodPost:
			var req CreatePostRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			authorID, err := uuid.Parse(req.AuthorID)
			if err != nil {
				http.Error(w, "Invalid author ID format", http.StatusBadRequest)
				return
			}

			subredditID, err := uuid.Parse(req.SubredditID)
			if err != nil {
				http.Error(w, "Invalid subreddit ID format", http.StatusBadRequest)
				return
			}

			future := s.context.RequestFuture(s.enginePID, &actors.CreatePostMsg{
				Title:       req.Title,
				Content:     req.Content,
				AuthorID:    authorID,
				SubredditID: subredditID,
			}, 5*time.Second)

			result, err := future.Result()
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to create post: %v", err), http.StatusInternalServerError)
				return
			}

			// Check for application errors
			if appErr, ok := result.(*utils.AppError); ok {
				var statusCode int
				switch appErr.Code {
				case utils.ErrNotFound:
					statusCode = http.StatusNotFound
				case utils.ErrDatabase:
					statusCode = http.StatusInternalServerError
				case utils.ErrInvalidInput:
					statusCode = http.StatusBadRequest
				case utils.ErrUnauthorized:
					statusCode = http.StatusUnauthorized
				default:
					statusCode = http.StatusInternalServerError
				}
				http.Error(w, appErr.Error(), statusCode)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		// get post by post ID or subreddit ID
		case http.MethodGet:
			postID := r.URL.Query().Get("id")
			subredditID := r.URL.Query().Get("subredditId")

			if postID != "" {
				id, err := uuid.Parse(postID)
				if err != nil {
					http.Error(w, "Invalid post ID format", http.StatusBadRequest)
					return
				}

				future := s.context.RequestFuture(s.engine.GetPostActor(),
					&actors.GetPostMsg{PostID: id},
					5*time.Second)

				result, err := future.Result()
				if err != nil {
					http.Error(w, fmt.Sprintf("Failed to get post: %v", err), http.StatusInternalServerError)
					return
				}

				// Check for application errors
				if appErr, ok := result.(*utils.AppError); ok {
					var statusCode int
					switch appErr.Code {
					case utils.ErrNotFound:
						statusCode = http.StatusNotFound
					case utils.ErrDatabase:
						statusCode = http.StatusInternalServerError
					default:
						statusCode = http.StatusInternalServerError
					}
					http.Error(w, appErr.Error(), statusCode)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(result)
				return
			}

			if subredditID != "" {
				id, err := uuid.Parse(subredditID)
				if err != nil {
					http.Error(w, "Invalid subreddit ID format", http.StatusBadRequest)
					return
				}

				log.Printf("Fetching posts for subreddit: %s", id)
				future := s.context.RequestFuture(s.engine.GetPostActor(),
					&actors.GetSubredditPostsMsg{SubredditID: id},
					5*time.Second)

				result, err := future.Result()
				if err != nil {
					log.Printf("Error getting subreddit posts: %v", err)
					http.Error(w, fmt.Sprintf("Failed to get subreddit posts: %v", err), http.StatusInternalServerError)
					return
				}

				// Check for application errors
				if appErr, ok := result.(*utils.AppError); ok {
					var statusCode int
					switch appErr.Code {
					case utils.ErrNotFound:
						statusCode = http.StatusNotFound
					case utils.ErrDatabase:
						statusCode = http.StatusInternalServerError
					default:
						statusCode = http.StatusInternalServerError
					}
					http.Error(w, appErr.Error(), statusCode)
					return
				}

				// Check if we got an empty result
				posts, ok := result.([]*models.Post)
				if ok && len(posts) == 0 {
					// Return empty array instead of error for no posts
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode([]*models.Post{})
					return
				}

				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(result); err != nil {
					log.Printf("Error encoding response: %v", err)
					http.Error(w, "Error encoding response", http.StatusInternalServerError)
					return
				}
				return
			}

			http.Error(w, "Either post ID or subreddit ID is required", http.StatusBadRequest)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// Handler for registration
// Update registration handler to use UserSupervisor
func (s *Server) handleUserRegistration() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req RegisterUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// Use Engine's UserSupervisor instead
		future := s.context.RequestFuture(
			s.engine.GetUserSupervisor(),
			&actors.RegisterUserMsg{
				Username: req.Username,
				Email:    req.Email,
				Password: req.Password,
				Karma:    req.Karma,
			},
			5*time.Second,
		)

		result, err := future.Result()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to register user: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// Handler for login
// Update login handler to use UserSupervisor
func (s *Server) handleUserLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		log.Printf("HTTP Handler: Received login request for email: %s", req.Email)

		future := s.context.RequestFuture(
			s.engine.GetUserSupervisor(),
			&actors.LoginMsg{
				Email:    req.Email,
				Password: req.Password,
			},
			5*time.Second,
		)

		result, err := future.Result()
		if err != nil {
			log.Printf("HTTP Handler: Error getting login result: %v", err)
			http.Error(w, "Failed to process login", http.StatusInternalServerError)
			return
		}

		log.Printf("HTTP Handler: Received raw result: %+v", result)

		// Important: directly encode the result without type assertion
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			log.Printf("HTTP Handler: Failed to encode response: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}
}

// Add user profile handler
func (s *Server) handleUserProfile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userIDStr := r.URL.Query().Get("userId")
		if userIDStr == "" {
			http.Error(w, "User ID required", http.StatusBadRequest)
			return
		}

		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			http.Error(w, "Invalid user ID format", http.StatusBadRequest)
			return
		}

		future := s.context.RequestFuture(
			s.engine.GetUserSupervisor(),
			&actors.GetUserProfileMsg{UserID: userID},
			5*time.Second,
		)

		result, err := future.Result()
		if err != nil {
			http.Error(w, "Failed to get user profile", http.StatusInternalServerError)
			return
		}

		if result == nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		userState, ok := result.(*actors.UserState)
		if !ok {
			http.Error(w, "Invalid response type", http.StatusInternalServerError)
			return
		}

		// Create response in the format you requested
		response := struct {
			ID            string    `json:"id"`
			Username      string    `json:"username"`
			Email         string    `json:"email"`
			Karma         int       `json:"karma"`
			IsConnected   bool      `json:"isConnected"`
			LastActive    time.Time `json:"lastActive"`
			SubredditID   []string  `json:"subredditID"`
			SubredditName []string  `json:"subredditName"`
		}{
			ID:          userState.ID.String(),
			Username:    userState.Username,
			Email:       userState.Email,
			Karma:       userState.Karma,
			IsConnected: userState.IsConnected,
			LastActive:  userState.LastActive,
		}

		// Convert UUID slices to string slices
		response.SubredditID = make([]string, len(userState.Subreddits))
		for i, id := range userState.Subreddits {
			response.SubredditID[i] = id.String()
		}
		response.SubredditName = userState.SubredditNames

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func (s *Server) handleVote() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req VoteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			http.Error(w, "Invalid user ID format", http.StatusBadRequest)
			return
		}

		postID, err := uuid.Parse(req.PostID)
		if err != nil {
			http.Error(w, "Invalid post ID format", http.StatusBadRequest)
			return
		}

		// Send to Engine instead of PostActor directly
		future := s.context.RequestFuture(s.enginePID, &actors.VotePostMsg{
			PostID:   postID,
			UserID:   userID,
			IsUpvote: req.IsUpvote,
		}, 5*time.Second)

		result, err := future.Result()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to process vote: %v", err), http.StatusInternalServerError)
			return
		}

		// Check for application errors
		if appErr, ok := result.(*utils.AppError); ok {
			var statusCode int
			switch appErr.Code {
			case utils.ErrNotFound:
				statusCode = http.StatusNotFound
			case utils.ErrUnauthorized:
				statusCode = http.StatusUnauthorized
			case utils.ErrDuplicate:
				statusCode = http.StatusConflict
			default:
				statusCode = http.StatusInternalServerError
			}
			http.Error(w, appErr.Error(), statusCode)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// Add new handler for getting user feed
func (s *Server) handleGetFeed() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID, err := uuid.Parse(r.URL.Query().Get("userId"))
		if err != nil {
			http.Error(w, "Invalid user ID", http.StatusBadRequest)
			return
		}

		limit := 50 // Default limit
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
				limit = parsedLimit
			}
		}

		// Send to Engine instead of PostActor directly
		future := s.context.RequestFuture(s.enginePID, &actors.GetUserFeedMsg{
			UserID: userID,
			Limit:  limit,
		}, 5*time.Second)

		result, err := future.Result()
		if err != nil {
			http.Error(w, "Failed to get feed", http.StatusInternalServerError)
			return
		}

		// Check for application errors
		if appErr, ok := result.(*utils.AppError); ok {
			var statusCode int
			switch appErr.Code {
			case utils.ErrNotFound:
				statusCode = http.StatusNotFound
			case utils.ErrUnauthorized:
				statusCode = http.StatusUnauthorized
			default:
				statusCode = http.StatusInternalServerError
			}
			http.Error(w, appErr.Error(), statusCode)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

func (s *Server) handleComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			log.Printf("Received comment creation request")
			var req CreateCommentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				log.Printf("Error decoding request: %v", err)
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			log.Printf("Creating comment for post: %s by author: %s", req.PostID, req.AuthorID)

			authorID, err := uuid.Parse(req.AuthorID)
			if err != nil {
				log.Printf("Error parsing author ID: %v", err)
				http.Error(w, "Invalid author ID", http.StatusBadRequest)
				return
			}

			postID, err := uuid.Parse(req.PostID)
			if err != nil {
				log.Printf("Error parsing post ID: %v", err)
				http.Error(w, "Invalid post ID", http.StatusBadRequest)
				return
			}

			var parentID *uuid.UUID
			if req.ParentID != "" {
				parsed, err := uuid.Parse(req.ParentID)
				if err != nil {
					log.Printf("Error parsing parent ID: %v", err)
					http.Error(w, "Invalid parent comment ID", http.StatusBadRequest)
					return
				}
				parentID = &parsed
			}

			log.Printf("Sending CreateCommentMsg to comment actor")
			future := s.context.RequestFuture(s.commentActor, &actors.CreateCommentMsg{
				Content:  req.Content,
				AuthorID: authorID,
				PostID:   postID,
				ParentID: parentID,
			}, 5*time.Second)

			result, err := future.Result()
			if err != nil {
				log.Printf("Error getting result from comment actor: %v", err)
				http.Error(w, "Failed to create comment", http.StatusInternalServerError)
				return
			}

			log.Printf("Received result from comment actor: %+v", result)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(result); err != nil {
				log.Printf("Error encoding response: %v", err)
				http.Error(w, "Failed to encode response", http.StatusInternalServerError)
				return
			}
			log.Printf("Successfully sent response")
		case http.MethodPut:
			var req EditCommentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			commentID, err := uuid.Parse(req.CommentID)
			if err != nil {
				http.Error(w, "Invalid comment ID", http.StatusBadRequest)
				return
			}

			authorID, err := uuid.Parse(req.AuthorID)
			if err != nil {
				http.Error(w, "Invalid author ID", http.StatusBadRequest)
				return
			}

			future := s.context.RequestFuture(s.commentActor, &actors.EditCommentMsg{
				CommentID: commentID,
				AuthorID:  authorID,
				Content:   req.Content,
			}, 5*time.Second)

			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to edit comment", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		case http.MethodDelete:
			commentID := r.URL.Query().Get("commentId")
			authorID := r.URL.Query().Get("authorId")

			if commentID == "" || authorID == "" {
				http.Error(w, "Missing comment ID or author ID", http.StatusBadRequest)
				return
			}

			cID, err := uuid.Parse(commentID)
			if err != nil {
				http.Error(w, "Invalid comment ID", http.StatusBadRequest)
				return
			}

			aID, err := uuid.Parse(authorID)
			if err != nil {
				http.Error(w, "Invalid author ID", http.StatusBadRequest)
				return
			}

			future := s.context.RequestFuture(s.commentActor, &actors.DeleteCommentMsg{
				CommentID: cID,
				AuthorID:  aID,
			}, 5*time.Second)

			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to delete comment", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": result.(bool)})
		}
	}
}

// Comment Actor
func (s *Server) handleGetPostComments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		postID := r.URL.Query().Get("postId")
		if postID == "" {
			http.Error(w, "Missing post ID", http.StatusBadRequest)
			return
		}

		pID, err := uuid.Parse(postID)
		if err != nil {
			http.Error(w, "Invalid post ID", http.StatusBadRequest)
			return
		}

		future := s.context.RequestFuture(s.commentActor, &actors.GetCommentsForPostMsg{
			PostID: pID,
		}, 5*time.Second)

		result, err := future.Result()
		if err != nil {
			http.Error(w, "Failed to get comments", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

func (s *Server) handleDirectMessages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var req SendMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}

			fromID, err := uuid.Parse(req.FromID)
			if err != nil {
				http.Error(w, "Invalid sender ID", http.StatusBadRequest)
				return
			}

			toID, err := uuid.Parse(req.ToID)
			if err != nil {
				http.Error(w, "Invalid recipient ID", http.StatusBadRequest)
				return
			}

			msg := &actors.SendDirectMessageMsg{
				FromID:  fromID,
				ToID:    toID,
				Content: req.Content,
			}

			future := s.context.RequestFuture(s.directMessageActor, msg, 5*time.Second)
			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to send message", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		case http.MethodGet:
			userID := r.URL.Query().Get("userId")
			if userID == "" {
				http.Error(w, "User ID required", http.StatusBadRequest)
				return
			}

			parsedID, err := uuid.Parse(userID)
			if err != nil {
				http.Error(w, "Invalid user ID", http.StatusBadRequest)
				return
			}

			msg := &actors.GetUserMessagesMsg{UserID: parsedID}
			future := s.context.RequestFuture(s.directMessageActor, msg, 5*time.Second)
			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to get messages", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		case http.MethodDelete:
			messageID := r.URL.Query().Get("messageId")
			userID := r.URL.Query().Get("userId")

			if messageID == "" || userID == "" {
				http.Error(w, "Message ID and User ID required", http.StatusBadRequest)
				return
			}

			parsedMessageID, err := uuid.Parse(messageID)
			if err != nil {
				http.Error(w, "Invalid message ID", http.StatusBadRequest)
				return
			}

			parsedUserID, err := uuid.Parse(userID)
			if err != nil {
				http.Error(w, "Invalid user ID", http.StatusBadRequest)
				return
			}

			msg := &actors.DeleteMessageMsg{
				MessageID: parsedMessageID,
				UserID:    parsedUserID,
			}

			future := s.context.RequestFuture(s.directMessageActor, msg, 5*time.Second)
			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to delete message", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": result.(bool)})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (s *Server) handleConversation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		user1ID := r.URL.Query().Get("user1")
		user2ID := r.URL.Query().Get("user2")
		if user1ID == "" || user2ID == "" {
			http.Error(w, "Both user IDs required", http.StatusBadRequest)
			return
		}

		parsedUser1ID, err := uuid.Parse(user1ID)
		if err != nil {
			http.Error(w, "Invalid user1 ID", http.StatusBadRequest)
			return
		}

		parsedUser2ID, err := uuid.Parse(user2ID)
		if err != nil {
			http.Error(w, "Invalid user2 ID", http.StatusBadRequest)
			return
		}

		msg := &actors.GetConversationMsg{
			UserID1: parsedUser1ID,
			UserID2: parsedUser2ID,
		}

		future := s.context.RequestFuture(s.directMessageActor, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			http.Error(w, "Failed to get conversation", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

func (s *Server) handleMarkMessageRead() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			MessageID string `json:"messageId"`
			UserID    string `json:"userId"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		messageID, err := uuid.Parse(req.MessageID)
		if err != nil {
			http.Error(w, "Invalid message ID", http.StatusBadRequest)
			return
		}

		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			http.Error(w, "Invalid user ID", http.StatusBadRequest)
			return
		}

		msg := &actors.MarkMessageReadMsg{
			MessageID: messageID,
			UserID:    userID,
		}

		future := s.context.RequestFuture(s.directMessageActor, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			http.Error(w, "Failed to mark message as read", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": result.(bool)})
	}
}

func (s *Server) handleCommentVote() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			CommentID string `json:"commentId"`
			UserID    string `json:"userId"`
			IsUpvote  bool   `json:"isUpvote"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		commentID, err := uuid.Parse(req.CommentID)
		if err != nil {
			http.Error(w, "Invalid comment ID", http.StatusBadRequest)
			return
		}

		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			http.Error(w, "Invalid user ID", http.StatusBadRequest)
			return
		}

		msg := &actors.VoteCommentMsg{
			CommentID: commentID,
			UserID:    userID,
			IsUpvote:  req.IsUpvote,
		}

		future := s.context.RequestFuture(s.commentActor, msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			http.Error(w, "Failed to process vote", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

func (s *Server) handleRecentPosts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Send request to PostActor through Engine
		future := s.context.RequestFuture(
			s.engine.GetPostActor(),
			&actors.GetRecentPostsMsg{Limit: 10},
			5*time.Second,
		)

		result, err := future.Result()
		if err != nil {
			http.Error(w, "Failed to fetch recent posts", http.StatusInternalServerError)
			return
		}

		// Check for application errors
		if appErr, ok := result.(*utils.AppError); ok {
			http.Error(w, appErr.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}
