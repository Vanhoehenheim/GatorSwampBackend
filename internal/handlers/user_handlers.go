package handlers

import (
	"encoding/json"
	"fmt"
	"gator-swamp/internal/engine/actors"
	"gator-swamp/internal/middleware"
	"gator-swamp/internal/types"
	"log"
	"net/http"
	"strconv"
	"time"

	"gator-swamp/internal/utils"

	"github.com/google/uuid"
)

// RegisterUserRequest represents a request to register a new user
type RegisterUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Karma    int    `json:"karma"`
}

// LoginRequest represents a request to log in a user
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse represents a response to a login request
type LoginResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token,omitempty"`
	Error   string `json:"error,omitempty"`
	UserID  string `json:"userId"`
}

// HandleUserRegistration handles requests to register a new user
func (s *Server) HandleUserRegistration() http.HandlerFunc {
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

		future := s.Context.RequestFuture(
			s.Engine.GetUserSupervisor(),
			&actors.RegisterUserMsg{
				Username: req.Username,
				Email:    req.Email,
				Password: req.Password,
				Karma:    req.Karma,
			},
			s.RequestTimeout,
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

// HandleUserLogin handles requests to log in a user
func (s *Server) HandleUserLogin() http.HandlerFunc {
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

		future := s.Context.RequestFuture(
			s.Engine.GetUserSupervisor(),
			&actors.LoginMsg{
				Email:    req.Email,
				Password: req.Password,
			},
			s.RequestTimeout,
		)

		result, err := future.Result()
		if err != nil {
			log.Printf("HTTP Handler: Error getting login result: %v", err)
			http.Error(w, "Failed to process login", http.StatusInternalServerError)
			return
		}

		log.Printf("HTTP Handler: Received raw result: %+v", result)

		// Type assert the login response
		loginResp, ok := result.(*types.LoginResponse)
		if !ok {
			log.Printf("HTTP Handler: Invalid response type: %T", result)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Only generate token if login was successful
		if loginResp.Success {
			userID, err := uuid.Parse(loginResp.UserID)
			if err != nil {
				log.Printf("HTTP Handler: Invalid user ID format: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			// Generate JWT token
			token, err := middleware.GenerateToken(userID)
			if err != nil {
				log.Printf("HTTP Handler: Failed to generate token: %v", err)
				http.Error(w, "Failed to generate auth token", http.StatusInternalServerError)
				return
			}

			// Add token to response
			loginResp.Token = token
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(loginResp); err != nil {
			log.Printf("HTTP Handler: Failed to encode response: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}
}

// HandleUserProfile handles requests to get a user's profile
func (s *Server) HandleUserProfile() http.HandlerFunc {
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

		future := s.Context.RequestFuture(
			s.Engine.GetUserSupervisor(),
			&actors.GetUserProfileMsg{UserID: userID},
			s.RequestTimeout,
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

// HandleGetAllUsers handles requests to get all users
func (s *Server) HandleGetAllUsers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		log.Printf("HandleGetAllUsers: Fetching all users")

		// Use the DBAdapter to fetch users
		users, err := s.DB.GetAllUsers(r.Context())
		if err != nil {
			log.Printf("HandleGetAllUsers: Error fetching users: %v", err)
			// Check if it's an AppError
			if appErr, ok := err.(*utils.AppError); ok {
				// Map AppError code to HTTP status (add more cases as needed)
				statusCode := http.StatusInternalServerError
				if appErr.Code == utils.ErrDatabase {
					statusCode = http.StatusInternalServerError
				}
				http.Error(w, appErr.Error(), statusCode)
			} else {
				// Generic internal error for other types of errors
				http.Error(w, "Failed to fetch users", http.StatusInternalServerError)
			}
			return
		}

		log.Printf("HandleGetAllUsers: Returning %d users", len(users))

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(users); err != nil {
			log.Printf("HandleGetAllUsers: Error encoding response: %v", err)
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	}
}

// HandleGetFeed handles requests to get a user's feed
func (s *Server) HandleGetFeed() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get the UserID whose feed is requested (should be the authenticated user)
		userIDClaim := r.Context().Value(middleware.UserIDKey)
		userID, ok := userIDClaim.(uuid.UUID)
		if !ok {
			log.Printf("HandleGetFeed: Invalid user ID type in token context key. Expected uuid.UUID, got %T", userIDClaim)
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// Parse limit and offset from query parameters
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if limit <= 0 {
			limit = 20 // Default limit
		}
		if offset < 0 {
			offset = 0 // Default offset
		}

		// Send request via Engine to UserSupervisor
		future := s.Context.RequestFuture(s.EnginePID, &actors.GetUserFeedMsg{
			UserID:           userID, // User whose feed is requested
			Limit:            limit,
			Offset:           offset,
			RequestingUserID: userID, // User making the request
		}, s.RequestTimeout)

		result, err := future.Result()
		if err != nil {
			http.Error(w, "Failed to get feed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}
