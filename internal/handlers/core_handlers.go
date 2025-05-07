package handlers

import (
	"encoding/json"
	"fmt"
	"gator-swamp/internal/engine/actors"
	"gator-swamp/internal/middleware"
	"gator-swamp/internal/utils"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// CreatePostRequest represents a request to create a new post
type CreatePostRequest struct {
	Title       string `json:"title"`       // Post title
	Content     string `json:"content"`     // Post content
	AuthorID    string `json:"authorId"`    // Author ID (UUID as string)
	SubredditID string `json:"subredditId"` // Subreddit ID (UUID as string)
}

// VoteRequest represents a request to vote on a post
type VoteRequest struct {
	UserID     string `json:"userId"`
	PostID     string `json:"postId"`
	IsUpvote   bool   `json:"isUpvote"`
	RemoveVote bool   `json:"removeVote"` // New field to support vote toggling
}

// HandleHealth handles health check requests
func (s *Server) HandleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only allow GET requests
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get the subreddit count from SubredditActor
		futureSubreddits := s.Context.RequestFuture(s.Engine.GetSubredditActor(), &actors.GetCountsMsg{}, s.RequestTimeout)
		subredditResult, err := futureSubreddits.Result()
		if err != nil {
			http.Error(w, "Failed to get subreddit count", http.StatusInternalServerError)
			return
		}
		subredditCount := subredditResult.(int) // Parse the result

		// Get the post count from PostActor
		futurePosts := s.Context.RequestFuture(s.Engine.GetPostActor(), &actors.GetCountsMsg{}, s.RequestTimeout)
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
			"server_time":     time.Now(),
		})
	}
}

// Add this function to your server
func (s *Server) HandleSimpleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
		})
	}
}

// HandlePost handles post-related requests
func (s *Server) HandlePost() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			// Create new post
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

			future := s.Context.RequestFuture(s.EnginePID, &actors.CreatePostMsg{
				Title:       req.Title,
				Content:     req.Content,
				AuthorID:    authorID,
				SubredditID: subredditID,
			}, s.RequestTimeout)

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

		case http.MethodGet:
			// Get post by ID or get posts from a subreddit
			postID := r.URL.Query().Get("id")
			subredditID := r.URL.Query().Get("subredditId")

			if postID != "" {
				// Get post by ID
				id, err := uuid.Parse(postID)
				if err != nil {
					http.Error(w, "Invalid post ID format", http.StatusBadRequest)
					return
				}

				// ---- Start: Extract UserID from JWT ----
				var requestingUserID uuid.UUID = uuid.Nil // Default to Nil UUID if not found or error
				// Use the exported UserIDKey from the middleware package
				userIDClaim := r.Context().Value(middleware.UserIDKey)
				if userIDClaim != nil {
					// Assert the type to uuid.UUID directly, as stored by SetUserIDInContext
					parsedID, ok := userIDClaim.(uuid.UUID)
					if !ok {
						log.Printf("HandlePost GET: Invalid user ID type in token context key. Expected uuid.UUID, got %T", userIDClaim)
						// Depending on policy, might treat as anonymous or return error
						// http.Error(w, "Invalid user ID type in token", http.StatusInternalServerError)
						// return
					} else {
						requestingUserID = parsedID
					}
				} else {
					// User is likely not logged in, requestingUserID remains uuid.Nil
					log.Printf("HandlePost GET: No user ID found in context (user likely not authenticated)")
				}
				// ---- End: Extract UserID from JWT ----

				// Send message to actor including requesting user ID
				future := s.Context.RequestFuture(s.Engine.GetPostActor(),
					&actors.GetPostMsg{
						PostID:           id,
						RequestingUserID: requestingUserID, // Pass the extracted/parsed user ID
					},
					s.RequestTimeout)

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
				// Get posts from a subreddit
				id, err := uuid.Parse(subredditID)
				if err != nil {
					http.Error(w, "Invalid subreddit ID format", http.StatusBadRequest)
					return
				}

				future := s.Context.RequestFuture(s.Engine.GetPostActor(),
					&actors.GetSubredditPostsMsg{SubredditID: id},
					s.RequestTimeout)

				result, err := future.Result()
				if err != nil {
					http.Error(w, fmt.Sprintf("Failed to get subreddit posts: %v", err), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(result)
				return
			}

			http.Error(w, "Either post ID or subreddit ID is required", http.StatusBadRequest)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// HandleVote handles post voting
func (s *Server) HandleVote() http.HandlerFunc {
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

		future := s.Context.RequestFuture(s.EnginePID, &actors.VotePostMsg{
			PostID:     postID,
			UserID:     userID,
			IsUpvote:   req.IsUpvote,
			RemoveVote: req.RemoveVote, // Pass the RemoveVote parameter
		}, s.RequestTimeout)

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

// HandleRecentPosts returns the most recent posts across all subreddits
func (s *Server) HandleRecentPosts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if limit <= 0 {
			limit = 20 // Default limit
		}

		// Extract requesting user ID from context
		requestingUserID := uuid.Nil // Default to Nil if no user is authenticated
		if userIDStr, ok := r.Context().Value(middleware.UserIDKey).(string); ok {
			if parsedUUID, err := uuid.Parse(userIDStr); err == nil {
				requestingUserID = parsedUUID
			} else {
				log.Printf("HandleRecentPosts: Error parsing UserID from context: %v", err)
			}
		}

		// Send message to PostActor
		future := s.Context.RequestFuture(s.Engine.GetPostActor(), &actors.GetRecentPostsMsg{
			Limit:            limit,
			Offset:           offset,
			RequestingUserID: requestingUserID, // Pass the user ID
		}, s.RequestTimeout)

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
