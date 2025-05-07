package handlers

import (
	"gator-swamp/internal/middleware"
	"gator-swamp/internal/websocket"
	"log"
	"net/http"

	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
)

var upgrader = ws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper origin checking based on config
		// This should likely use s.Server.AllowedOrigins or similar
		return true
	},
}

// HandleWebSocket handles WebSocket connection requests.
func (s *Server) HandleWebSocket() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Authenticate using JWT from query parameter
		tokenString := r.URL.Query().Get("token")
		if tokenString == "" {
			log.Println("WebSocket connection failed: Missing token")
			http.Error(w, "Missing authentication token", http.StatusUnauthorized)
			return
		}
		log.Printf("WebSocket attempting auth with token: %s...", tokenString[:min(len(tokenString), 10)]) // Log prefix

		claims, err := middleware.ValidateToken(tokenString)
		if err != nil {
			log.Printf("WebSocket connection failed: Invalid token: %v", err)
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		userID := claims.UserID
		if userID == uuid.Nil {
			log.Println("WebSocket connection failed: Nil userID in token claims")
			http.Error(w, "Invalid user ID in token", http.StatusInternalServerError)
			return
		}
		log.Printf("WebSocket token validated for User %s", userID)

		// 2. Upgrade connection
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed for User %s: %v", userID, err)
			// Note: Cannot write HTTP error after successful upgrade attempt
			return
		}
		log.Printf("WebSocket connection upgraded for User %s", userID)

		// 3. Create and register the client (Use exported fields)
		client := &websocket.Client{
			Hub:    s.Hub,
			UserID: userID,
			Conn:   conn,
			Send:   make(chan []byte, 256),
		}
		client.Hub.Register <- client // Use exported Hub and Register

		log.Printf("WebSocket client registered for User %s", userID)

		// 4. Start read and write pumps (Use exported methods)
		go client.WritePump()
		go client.ReadPump()
	}
}

// Helper to avoid logging entire token
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
