package handlers

import (
	"encoding/json"
	"net/http"

	"gator-swamp/internal/engine/actors"

	"github.com/google/uuid"
)

// SendMessageRequest represents a request to send a direct message
type SendMessageRequest struct {
	FromID  string `json:"fromId"`
	ToID    string `json:"toId"`
	Content string `json:"content"`
}

// HandleDirectMessages handles sending and retrieving direct messages
func (s *Server) HandleDirectMessages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			// Send a direct message
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

			future := s.Context.RequestFuture(s.DirectMessageActor, msg, s.RequestTimeout)
			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to send message", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		case http.MethodGet:
			// Get messages for a user
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
			future := s.Context.RequestFuture(s.DirectMessageActor, msg, s.RequestTimeout)
			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to get messages", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		case http.MethodDelete:
			// Delete a message
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

			future := s.Context.RequestFuture(s.DirectMessageActor, msg, s.RequestTimeout)
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

// HandleConversation gets messages between two specific users
func (s *Server) HandleConversation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID := r.URL.Query().Get("userId")
		otherID := r.URL.Query().Get("otherUserId")

		if userID == "" || otherID == "" {
			http.Error(w, "Both user IDs required", http.StatusBadRequest)
			return
		}

		parsedUserID, err := uuid.Parse(userID)
		if err != nil {
			http.Error(w, "Invalid user ID", http.StatusBadRequest)
			return
		}

		parsedOtherID, err := uuid.Parse(otherID)
		if err != nil {
			http.Error(w, "Invalid other user ID", http.StatusBadRequest)
			return
		}

		msg := &actors.GetConversationMsg{
			UserID1: parsedUserID,
			UserID2: parsedOtherID,
		}

		future := s.Context.RequestFuture(s.DirectMessageActor, msg, s.RequestTimeout)
		result, err := future.Result()
		if err != nil {
			http.Error(w, "Failed to get conversation", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// HandleMarkMessageRead marks a message as read
func (s *Server) HandleMarkMessageRead() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			MessageIds []string `json:"messageIds"`
			UserID     string   `json:"userId"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			http.Error(w, "Invalid user ID", http.StatusBadRequest)
			return
		}

		results := make(map[string]bool)
		for _, mid := range req.MessageIds {
			messageID, err := uuid.Parse(mid)
			if err != nil {
				results[mid] = false
				continue
			}
			msg := &actors.MarkMessageReadMsg{
				MessageID: messageID,
				UserID:    userID,
			}
			future := s.Context.RequestFuture(s.DirectMessageActor, msg, s.RequestTimeout)
			result, err := future.Result()
			if err != nil {
				results[mid] = false
				continue
			}
			if success, ok := result.(bool); ok && success {
				results[mid] = true
			} else {
				results[mid] = false
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}
