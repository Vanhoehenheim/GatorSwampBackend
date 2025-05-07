package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"gator-swamp/internal/engine/actors"
	"gator-swamp/internal/middleware"
	"gator-swamp/internal/utils"

	"github.com/google/uuid"
)

// CreateCommentRequest represents a request to create a new comment
type CreateCommentRequest struct {
	Content  string `json:"content"`
	AuthorID string `json:"authorId"`
	PostID   string `json:"postId"`
	ParentID string `json:"parentId,omitempty"` // Optional, for replies
}

// EditCommentRequest represents a request to edit an existing comment
type EditCommentRequest struct {
	CommentID string `json:"commentId"`
	AuthorID  string `json:"authorId"`
	Content   string `json:"content"`
}

// CommentVoteRequest represents a request to vote on a comment
type CommentVoteRequest struct {
	UserID     string `json:"userId"`
	CommentID  string `json:"commentId"`
	IsUpvote   bool   `json:"isUpvote"`
	RemoveVote bool   `json:"removeVote,omitempty"` // Added optional field
}

// HandleComment handles comment-related operations
func (s *Server) HandleComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			// Create comment
			log.Printf("Received comment creation request")
			var req CreateCommentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				log.Printf("Error decoding request: %v", err)
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			log.Printf("Creating comment for post: %s by author: %s and parent: %s", req.PostID, req.AuthorID, req.ParentID)

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
			future := s.Context.RequestFuture(s.CommentActor, &actors.CreateCommentMsg{
				Content:  req.Content,
				AuthorID: authorID,
				PostID:   postID,
				ParentID: parentID,
			}, s.RequestTimeout)

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
			// Edit comment
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

			future := s.Context.RequestFuture(s.CommentActor, &actors.EditCommentMsg{
				CommentID: commentID,
				AuthorID:  authorID,
				Content:   req.Content,
			}, s.RequestTimeout)

			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to edit comment", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)

		case http.MethodDelete:
			// Delete comment
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

			future := s.Context.RequestFuture(s.CommentActor, &actors.DeleteCommentMsg{
				CommentID: cID,
				AuthorID:  aID,
			}, s.RequestTimeout)

			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to delete comment", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": result.(bool)})

		case http.MethodGet:
			// Get a specific comment
			commentID := r.URL.Query().Get("commentId")
			if commentID == "" {
				http.Error(w, "Missing comment ID", http.StatusBadRequest)
				return
			}

			cID, err := uuid.Parse(commentID)
			if err != nil {
				http.Error(w, "Invalid comment ID", http.StatusBadRequest)
				return
			}

			future := s.Context.RequestFuture(s.CommentActor, &actors.GetCommentMsg{
				CommentID: cID,
			}, s.RequestTimeout)

			result, err := future.Result()
			if err != nil {
				http.Error(w, "Failed to get comment", http.StatusInternalServerError)
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
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// HandleGetPostComments retrieves all comments for a given post
func (s *Server) HandleGetPostComments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		postIDStr := r.URL.Query().Get("postId")
		if postIDStr == "" {
			http.Error(w, "Missing post ID", http.StatusBadRequest)
			return
		}

		pID, err := uuid.Parse(postIDStr)
		if err != nil {
			http.Error(w, "Invalid post ID", http.StatusBadRequest)
			return
		}

		// Extract requesting user ID from JWT token
		var requestingUserID uuid.UUID
		userIDFromToken, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
		if ok {
			requestingUserID = userIDFromToken
		} else {
			// If UserIDKey is not present or not a UUID, send uuid.Nil
			// This allows unauthenticated users to still fetch comments, but without their vote status.
			requestingUserID = uuid.Nil
			log.Printf("No authenticated user found or UserIDKey is not a UUID, using uuid.Nil for requestingUserID in HandleGetPostComments")
		}

		future := s.Context.RequestFuture(s.CommentActor, &actors.GetCommentsForPostMsg{
			PostID:           pID,
			RequestingUserID: requestingUserID, // Pass the user ID
		}, s.RequestTimeout)

		result, err := future.Result()
		if err != nil {
			// Check if the error is an AppError and handle it specifically
			if appErr, ok := err.(*utils.AppError); ok {
				log.Printf("AppError fetching comments for post %s: %v", pID, appErr)
				http.Error(w, appErr.Message, utils.AppErrorToHTTPStatus(appErr.Code))
				return
			}
			// Generic error
			log.Printf("Error fetching comments for post %s: %v", pID, err)
			http.Error(w, "Failed to get comments", http.StatusInternalServerError)
			return
		}

		// Check if the result itself is an AppError (e.g., from the actor logic, not just future.Result() error)
		if appErr, ok := result.(*utils.AppError); ok {
			log.Printf("AppError (from actor result) fetching comments for post %s: %v", pID, appErr)
			http.Error(w, appErr.Message, utils.AppErrorToHTTPStatus(appErr.Code))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			log.Printf("Error encoding comments response for post %s: %v", pID, err)
			// Avoid writing another http.Error if headers already sent.
			return
		}
	}
}

// HandleCommentVote handles voting on comments
func (s *Server) HandleCommentVote() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req CommentVoteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			http.Error(w, "Invalid user ID", http.StatusBadRequest)
			return
		}

		commentID, err := uuid.Parse(req.CommentID)
		if err != nil {
			http.Error(w, "Invalid comment ID", http.StatusBadRequest)
			return
		}

		// Send the message to the CommentActor
		future := s.Context.RequestFuture(s.CommentActor, &actors.VoteCommentMsg{
			CommentID:  commentID,
			UserID:     userID,
			IsUpvote:   req.IsUpvote,
			RemoveVote: req.RemoveVote, // Include RemoveVote
		}, s.RequestTimeout)

		result, err := future.Result()
		if err != nil {
			// Basic error handling for actor communication failure
			log.Printf("Error requesting comment vote from actor: %v", err)
			http.Error(w, "Failed to process vote", http.StatusInternalServerError)
			return
		}

		// Check if the result itself is an AppError from the actor
		if appErr, ok := result.(*utils.AppError); ok {
			var statusCode int
			switch appErr.Code {
			case utils.ErrNotFound:
				statusCode = http.StatusNotFound
			case utils.ErrDuplicate:
				statusCode = http.StatusConflict
			case utils.ErrDatabase:
				statusCode = http.StatusInternalServerError
			default:
				statusCode = http.StatusInternalServerError
			}
			http.Error(w, appErr.Error(), statusCode)
			return
		}

		// Check if the result indicates success (assuming actor responds with {Success: true})
		if successResp, ok := result.(*struct{ Success bool }); ok && successResp.Success {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		} else {
			// Handle unexpected result type or failure indication
			log.Printf("Unexpected result from CommentActor vote: %T - %v", result, result)
			http.Error(w, "Unexpected result from vote processing", http.StatusInternalServerError)
		}
	}
}
