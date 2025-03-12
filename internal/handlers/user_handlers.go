package handlers

import (
	"encoding/json"
	"fmt"
	"gator-swamp/internal/engine/actors"
	"gator-swamp/internal/middleware"
	"gator-swamp/internal/types"
	"log"
	"net/http"
	"time"

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

		ctx := r.Context()
		cursor, err := s.MongoDB.Users.Find(ctx, map[string]interface{}{})
		if err != nil {
			http.Error(w, "Failed to fetch users", http.StatusInternalServerError)
			return
		}
		defer cursor.Close(ctx)

		var users []struct {
			ID       string    `json:"id"`
			Username string    `json:"username"`
			Email    string    `json:"email"`
			Karma    int       `json:"karma"`
			JoinedAt time.Time `json:"joinedAt"`
		}

		for cursor.Next(ctx) {
			var user struct {
				ID        string    `bson:"_id"`
				Username  string    `bson:"username"`
				Email     string    `bson:"email"`
				Karma     int       `bson:"karma"`
				CreatedAt time.Time `bson:"createdAt"`
			}
			if err := cursor.Decode(&user); err != nil {
				continue
			}
			users = append(users, struct {
				ID       string    `json:"id"`
				Username string    `json:"username"`
				Email    string    `json:"email"`
				Karma    int       `json:"karma"`
				JoinedAt time.Time `json:"joinedAt"`
			}{
				ID:       user.ID,
				Username: user.Username,
				Email:    user.Email,
				Karma:    user.Karma,
				JoinedAt: user.CreatedAt,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
	}
}

// HandleGetFeed handles requests to get a user's feed
func (s *Server) HandleGetFeed() http.HandlerFunc {
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

		// Get limit from query params, default to 50
		limit := 50
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			fmt.Sscanf(limitStr, "%d", &limit)
		}

		// Send to Engine
		future := s.Context.RequestFuture(s.EnginePID, &actors.GetUserFeedMsg{
			UserID: userID,
			Limit:  limit,
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
