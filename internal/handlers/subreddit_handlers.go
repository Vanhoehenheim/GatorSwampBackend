package handlers

import (
	"encoding/json"
	"fmt"
	"gator-swamp/internal/engine/actors"
	"gator-swamp/internal/utils"
	"net/http"

	"github.com/google/uuid"
)

// CreateSubredditRequest represents a request to create a new subreddit
type CreateSubredditRequest struct {
	Name        string `json:"name"`        // Subreddit name
	Description string `json:"description"` // Subreddit description
	CreatorID   string `json:"creatorId"`   // Creator ID (UUID as string)
}

// HandleSubreddits handles requests related to subreddits
func (s *Server) HandleSubreddits() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// Check query parameters
			name := r.URL.Query().Get("name")
			id := r.URL.Query().Get("id")

			// If neither parameter is provided, list all subreddits
			if name == "" && id == "" {
				future := s.Context.RequestFuture(s.Engine.GetSubredditActor(), &actors.ListSubredditsMsg{}, s.RequestTimeout)
				result, err := future.Result()
				if err != nil {
					http.Error(w, "Failed to get subreddits", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(result)
				return
			}

			// If ID is provided
			if id != "" {
				subredditID, err := uuid.Parse(id)
				if err != nil {
					http.Error(w, "Invalid subreddit ID format", http.StatusBadRequest)
					return
				}

				future := s.Context.RequestFuture(s.Engine.GetSubredditActor(),
					&actors.GetSubredditByIDMsg{SubredditID: subredditID},
					s.RequestTimeout)

				result, err := future.Result()
				if err != nil {
					http.Error(w, "Failed to get subreddit", http.StatusInternalServerError)
					return
				}

				if appErr, ok := result.(*utils.AppError); ok {
					if appErr.Code == utils.ErrNotFound {
						http.Error(w, "Subreddit not found", http.StatusNotFound)
					} else {
						http.Error(w, appErr.Error(), http.StatusInternalServerError)
					}
					return
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(result)
				return
			}

			// If name is provided
			if name != "" {
				future := s.Context.RequestFuture(s.Engine.GetSubredditActor(),
					&actors.GetSubredditByNameMsg{Name: name},
					s.RequestTimeout)

				result, err := future.Result()
				if err != nil {
					http.Error(w, "Failed to get subreddit", http.StatusInternalServerError)
					return
				}

				if appErr, ok := result.(*utils.AppError); ok {
					if appErr.Code == utils.ErrNotFound {
						http.Error(w, "Subreddit not found", http.StatusNotFound)
					} else {
						http.Error(w, appErr.Error(), http.StatusInternalServerError)
					}
					return
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(result)
				return
			}

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
			future := s.Context.RequestFuture(s.EnginePID, msg, s.RequestTimeout)
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

// HandleSubredditMembers handles subreddit membership operations
func (s *Server) HandleSubredditMembers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// Get subreddit members
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
			future := s.Context.RequestFuture(s.Engine.GetSubredditActor(), msg, s.RequestTimeout)
			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to get members", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		case http.MethodPost:
			// Join a subreddit
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

			future := s.Context.RequestFuture(s.Engine.GetSubredditActor(),
				&actors.JoinSubredditMsg{
					SubredditID: subredditID,
					UserID:      userID,
				}, s.RequestTimeout)

			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to join subreddit", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		case http.MethodDelete:
			// Leave a subreddit
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

			future := s.Context.RequestFuture(s.Engine.GetSubredditActor(),
				&actors.LeaveSubredditMsg{
					SubredditID: subredditID,
					UserID:      userID,
				}, s.RequestTimeout)

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
