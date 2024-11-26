package main

// Import necessary packages
import (
	"encoding/json"               // JSON encoding and decoding
	"fmt"                         // String formatting
	"gator-swamp/internal/config" // Configuration handling
	"gator-swamp/internal/engine" // Engine for managing actors
	"gator-swamp/internal/utils"  // Utility functions and metrics
	"log"                         // Logging
	"net/http"                    // HTTP server
	"time"                        // Time utilities
	"gator-swamp/internal/engine/actors"  // For UserActors
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
// Request/Response to create a comment
type CreateCommentRequest struct {
    Content   string `json:"content"`
    AuthorID  string `json:"authorId"`
    PostID    string `json:"postId"`
    ParentID  string `json:"parentId,omitempty"`  // Optional, for replies
}

type EditCommentRequest struct {
    CommentID string `json:"commentId"`
    AuthorID  string `json:"authorId"`
    Content   string `json:"content"`
}

// Server holds all server dependencies, including the actor system and engine


// User-related request structures
type RegisterUserRequest struct {
    Username string `json:"username"`
    Email    string `json:"email"`
    Password string `json:"password"`
	Karma    int    `json:"Karma"`
}
//User-related login
type LoginRequest struct {
    Email    string `json:"email"`
    Password string `json:"password"`
}

type LoginResponse struct {
    Success bool   `json:"success"`
    Token   string `json:"token,omitempty"`
    Error   string `json:"error,omitempty"`
}
type Server struct {
	system  *actor.ActorSystem      // Actor system
	context *actor.RootContext      // Root context for actors
	engine  *engine.Engine          // Engine managing actors
	metrics *utils.MetricsCollector // Metrics collector
	userActor      *actor.PID        // User actor system
	commentActor *actor.PID   		//Comment Actor System 
}
func main() {
	// Initialize application components
	cfg := config.DefaultConfig()          // Load default configurations
	metrics := utils.NewMetricsCollector() // Initialize metrics collector

	// Initialize the ProtoActor system
	system := actor.NewActorSystem()

	// Initialize the engine with actors
	gatorEngine := engine.NewEngine(system, metrics)

	

	// Create the server instance with all dependencies
	server := &Server{
		system:  system,
		context: system.Root,
		engine:  gatorEngine,
		metrics: metrics,
	}

	 // Initialize user actor
	 server.userActor = system.Root.Spawn(actor.PropsFromProducer(func() actor.Actor {
        return actors.NewUserActor(nil)
    }))

	// Set up HTTP endpoints
	http.HandleFunc("/health", server.handleHealth())                      // Health check endpoint
	http.HandleFunc("/subreddit", server.handleSubreddits())               // Endpoint for subreddit-related operations
	http.HandleFunc("/subreddit/members", server.handleSubredditMembers()) // Endpoint for subreddit members
	http.HandleFunc("/post", server.handlePost())                          // Endpoint for post-related operations
    http.HandleFunc("/user/register", server.handleUserRegistration())     // Endpoint for register
    http.HandleFunc("/user/login", server.handleUserLogin())               //Endpoint for login
	// Start the HTTP server
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
			// Handle creating a new subreddit
			var req CreateSubredditRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			// Convert CreatorID to UUID and create subreddit message
			creatorID, err := uuid.Parse(req.CreatorID)
			if err != nil {
				http.Error(w, "Invalid creator ID format", http.StatusBadRequest)
				return
			}
			future := s.context.RequestFuture(s.engine.GetSubredditActor(), &engine.CreateSubredditMsg{
				Name:        req.Name,
				Description: req.Description,
				CreatorID:   creatorID,
			}, 5*time.Second)

			// Handle the response from the actor
			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to create subreddit", http.StatusInternalServerError)
				return
			}

			// Respond with the created subreddit details
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		default:
			// Handle unsupported HTTP methods
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
			future := s.context.RequestFuture(s.engine.GetPostActor(), &engine.CreatePostMsg{
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
//Handler for registration
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

        future := s.context.RequestFuture(s.userActor, &actors.RegisterUserMsg{
            Username: req.Username,
            Email:    req.Email,
            Password: req.Password,
			Karma: req.Karma,
        }, 5*time.Second)

        result, err := future.Result()
        if err != nil {
            http.Error(w, "Failed to register user", http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(result)
    }
}
//Handler for login
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

        future := s.context.RequestFuture(s.userActor, &actors.LoginMsg{
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