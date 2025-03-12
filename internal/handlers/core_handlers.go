package handlers

import (
	"encoding/json"
	"fmt"
	"gator-swamp/internal/engine/actors"
	"gator-swamp/internal/utils"
	"net/http"
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
	UserID   string `json:"userId"`
	PostID   string `json:"postId"`
	IsUpvote bool   `json:"isUpvote"`
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

				future := s.Context.RequestFuture(s.Engine.GetPostActor(),
					&actors.GetPostMsg{PostID: id},
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
			PostID:   postID,
			UserID:   userID,
			IsUpvote: req.IsUpvote,
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

		limit := 10 // Default limit
		// You can add logic to parse a limit parameter from the query string if needed

		// Send request to PostActor through Engine
		future := s.Context.RequestFuture(
			s.Engine.GetPostActor(),
			&actors.GetRecentPostsMsg{Limit: limit},
			s.RequestTimeout,
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
