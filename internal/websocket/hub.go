package websocket

import (
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MessageToSend defines the structure for sending a message to a specific user.
type MessageToSend struct {
	TargetUserID uuid.UUID
	Payload      []byte
}

// Hub maintains the set of active clients and broadcasts messages.
type Hub struct {
	// Registered clients. Maps user ID to a set of active client connections.
	Clients map[uuid.UUID]map[*Client]bool

	// Inbound messages from the clients (not used for sending DMs yet).
	Broadcast chan []byte

	// Channel for sending messages to specific users.
	SendDirect chan *MessageToSend

	// Register requests from the clients.
	Register chan *Client

	// Unregister requests from clients.
	Unregister chan *Client

	// Mutex to protect concurrent access to the clients map.
	mu sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		Broadcast:  make(chan []byte),
		SendDirect: make(chan *MessageToSend),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Clients:    make(map[uuid.UUID]map[*Client]bool),
	}
}

// Run starts the hub's processing loop.
func (h *Hub) Run() {
	log.Println("WebSocket Hub started.")
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			if _, ok := h.Clients[client.UserID]; !ok {
				h.Clients[client.UserID] = make(map[*Client]bool)
			}
			h.Clients[client.UserID][client] = true
			log.Printf("WebSocket Client registered for User %s. Total connections for user: %d", client.UserID, len(h.Clients[client.UserID]))
			h.mu.Unlock()

		case client := <-h.Unregister:
			h.mu.Lock()
			if userClients, ok := h.Clients[client.UserID]; ok {
				if _, clientOk := userClients[client]; clientOk {
					delete(userClients, client)
					// Note: Closing client.Send channel is typically handled by the writePump upon error or hub closure.
					if len(userClients) == 0 {
						delete(h.Clients, client.UserID)
						log.Printf("WebSocket Client unregistered. User %s has no more connections.", client.UserID)
					} else {
						log.Printf("WebSocket Client unregistered for User %s. Remaining connections: %d", client.UserID, len(userClients))
					}
				}
			}
			h.mu.Unlock()

		case message := <-h.Broadcast:
			h.mu.RLock()
			for _, userClients := range h.Clients {
				for client := range userClients {
					select {
					case client.Send <- message:
					default:
						log.Printf("Broadcast send buffer full for client of User %s", client.UserID)
					}
				}
			}
			h.mu.RUnlock()

		case directMessage := <-h.SendDirect:
			h.mu.RLock()
			if userClients, ok := h.Clients[directMessage.TargetUserID]; ok {
				if len(userClients) > 0 {
					log.Printf("Sending direct message to %d connections for User %s", len(userClients), directMessage.TargetUserID)
					for client := range userClients {
						select {
						case client.Send <- directMessage.Payload:
							log.Printf("Message successfully queued for client of User %s", client.UserID)
						default:
							log.Printf("Send channel full for client of User %s. Message dropped for this client.", client.UserID)
						}
					}
				} else {
					log.Printf("User %s found in map but has no active client connections.", directMessage.TargetUserID)
				}
			} else {
				log.Printf("User %s not connected, cannot send direct message.", directMessage.TargetUserID)
			}
			h.mu.RUnlock()
		}
	}
}

// SendDirectMessage allows other parts of the application (like actors) to send a message
// to a specific user via the WebSocket hub.
func (h *Hub) SendDirectMessage(targetUserID uuid.UUID, payload []byte) {
	message := &MessageToSend{
		TargetUserID: targetUserID,
		Payload:      payload,
	}
	select {
	case h.SendDirect <- message:
		log.Printf("Message queued in hub for User %s", targetUserID)
	case <-time.After(1 * time.Second):
		log.Printf("Timeout queuing message in hub's SendDirect channel for User %s. Hub might be busy or blocked.", targetUserID)
	}
}
