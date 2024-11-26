package main

// Import necessary packages
import (
	"encoding/json"                      // JSON encoding and decoding
	"fmt"                                // String formatting
	"gator-swamp/internal/config"        // Configuration handling
	"gator-swamp/internal/engine"        // Engine for managing actors
	"gator-swamp/internal/engine/actors" // For UserActors
	"gator-swamp/internal/utils"         // Utility functions and metrics
	"log"                                // Logging
	"net/http"                           // HTTP server
	"strconv"                            // String conversion utilities
	"time"                               // Time utilities

	"github.com/asynkron/protoactor-go/actor" // ProtoActor for actor-based concurrency
	"github.com/google/uuid"                  // UUID generation for unique identifiers
)

// Request structs for handling JSON input

// CreateSubredditRequest represents a request to create a new subreddit
type CreateSubredditRequest struct {
	Name        string `json:"name"`        // Subreddit name
	Description string `json:"description"` // Subreddit description
	CreatorID   string `json:"creatorId"`   // Creator ID (UUID as string)
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
	system       *actor.ActorSystem
	context      *actor.RootContext
	engine       *engine.Engine
	enginePID    *actor.PID // Add this field
	metrics      *utils.MetricsCollector
	commentActor *actor.PID //Added comment actor
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

func main() {
	cfg := config.DefaultConfig()
	metrics := utils.NewMetricsCollector()
	system := actor.NewActorSystem()
	gatorEngine := engine.NewEngine(system, metrics)
	engineProps := actor.PropsFromProducer(func() actor.Actor {
		return gatorEngine
	})
	enginePID := system.Root.Spawn(engineProps)

	server := &Server{
		system:    system,
		context:   system.Root,
		engine:    gatorEngine,
		enginePID: enginePID, // Store the PID
		metrics:   metrics,
	}
	// Initialize comment actor
	server.commentActor = system.Root.Spawn(actor.PropsFromProducer(func() actor.Actor {
		return actors.NewCommentActor()
	}))
	// Set up HTTP endpoints
	http.HandleFunc("/health", server.handleHealth())
	http.HandleFunc("/subreddit", server.handleSubreddits())
	http.HandleFunc("/subreddit/members", server.handleSubredditMembers())
	http.HandleFunc("/post", server.handlePost())
	http.HandleFunc("/user/register", server.handleUserRegistration())
	http.HandleFunc("/user/login", server.handleUserLogin())
	http.HandleFunc("/post/vote", server.handleVote())    // New endpoint
	http.HandleFunc("/user/feed", server.handleGetFeed()) // New endpoint
	http.HandleFunc("/user/profile", server.handleUserProfile())
	http.HandleFunc("/comment", server.handleComment())              //Comment actor
	http.HandleFunc("/comment/post", server.handleGetPostComments()) //Post comment actor

	serverAddr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("Starting server on %s", serverAddr)
	if err := http.ListenAndServe(serverAddr, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// handleHealth checks the health of the system
func (s *Server) handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get the subreddit count from SubredditActor
		futureSubreddits := s.context.RequestFuture(s.engine.GetSubredditActor(), &engine.GetCountsMsg{}, 5*time.Second)
		subredditResult, err := futureSubreddits.Result()
		if err != nil {
			http.Error(w, "Failed to get subreddit count", http.StatusInternalServerError)
			return
		}
		subredditCount := subredditResult.(int) // Parse the result

		// Get the post count from PostActor
		futurePosts := s.context.RequestFuture(s.engine.GetPostActor(), &engine.GetCountsMsg{}, 5*time.Second)
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
			future := s.context.RequestFuture(s.engine.GetSubredditActor(), &engine.ListSubredditsMsg{}, 5*time.Second)
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
			msg := &engine.CreateSubredditMsg{
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
		// Get the subreddit ID from the query parameters
		subredditID := r.URL.Query().Get("id")
		if subredditID == "" {
			http.Error(w, "Subreddit ID required", http.StatusBadRequest)
			return
		}

		// Convert subreddit ID to UUID
		id, err := uuid.Parse(subredditID)
		if err != nil {
			http.Error(w, "Invalid subreddit ID", http.StatusBadRequest)
			return
		}

		// Create message to get subreddit members
		msg := &engine.GetSubredditMembersMsg{SubredditID: id}
		future := s.context.RequestFuture(s.engine.GetSubredditActor(), msg, 5*time.Second)
		result, err := future.Result()
		if err != nil {
			http.Error(w, "Failed to get members", http.StatusInternalServerError)
			return
		}

		// Respond with the list of subreddit members
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// handlePost handles post-related requests, such as creating a new post
func (s *Server) handlePost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// Parse request to create a post
			var req CreatePostRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			// Convert AuthorID and SubredditID to UUIDs and create post message
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

			// Handle the response from the actor
			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to create post", http.StatusInternalServerError)
				return
			}

			// Respond with the created post details
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		} else {
			// Handle unsupported HTTP methods
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

		future := s.context.RequestFuture(s.engine.GetUserSupervisor(), &actors.LoginMsg{
			Email:    req.Email,
			Password: req.Password,
		}, 5*time.Second)

		result, err := future.Result()
		if err != nil {
			http.Error(w, "Failed to process login", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// Add user profile handler
func (s *Server) handleUserProfile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get userID from query parameters
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

		// Request user profile from UserSupervisor through Engine
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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
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

		future := s.context.RequestFuture(s.engine.GetPostActor(), &engine.GetUserFeedMsg{
			UserID: userID,
			Limit:  limit,
		}, 5*time.Second)

		result, err := future.Result()
		if err != nil {
			http.Error(w, "Failed to get feed", http.StatusInternalServerError)
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
			var req CreateCommentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			authorID, err := uuid.Parse(req.AuthorID)
			if err != nil {
				http.Error(w, "Invalid author ID", http.StatusBadRequest)
				return
			}

			postID, err := uuid.Parse(req.PostID)
			if err != nil {
				http.Error(w, "Invalid post ID", http.StatusBadRequest)
				return
			}

			var parentID *uuid.UUID
			if req.ParentID != "" {
				parsed, err := uuid.Parse(req.ParentID)
				if err != nil {
					http.Error(w, "Invalid parent comment ID", http.StatusBadRequest)
					return
				}
				parentID = &parsed
			}

			future := s.context.RequestFuture(s.commentActor, &actors.CreateCommentMsg{
				Content:  req.Content,
				AuthorID: authorID,
				PostID:   postID,
				ParentID: parentID,
			}, 5*time.Second)

			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to create comment", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

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
