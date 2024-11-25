package main

import (
	"encoding/json"
	"fmt"
	"gator-swamp/internal/config"
	"gator-swamp/internal/engine"
	"gator-swamp/internal/utils"
	"log"
	"net/http"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/google/uuid"
)

// Request structs for JSON handling
type CreateSubredditRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatorID   string `json:"creatorId"`
}

type CreatePostRequest struct {
	Title       string `json:"title"`
	Content     string `json:"content"`
	AuthorID    string `json:"authorId"`
	SubredditID string `json:"subredditId"`
}

// Server holds all dependencies
type Server struct {
	system  *actor.ActorSystem
	context *actor.RootContext
	engine  *engine.Engine
	metrics *utils.MetricsCollector
}

func main() {
	// Initialize components
	cfg := config.DefaultConfig()
	metrics := utils.NewMetricsCollector()

	// Initialize actor system
	system := actor.NewActorSystem()

	// Initialize engine with actors
	gatorEngine := engine.NewEngine(system, metrics)

	// Create server instance
	server := &Server{
		system:  system,
		context: system.Root,
		engine:  gatorEngine,
		metrics: metrics,
	}

	// Set up HTTP handlers
	http.HandleFunc("/health", server.handleHealth)
	http.HandleFunc("/subreddit", server.handleSubreddit)
	http.HandleFunc("/post", server.handlePost)

	// Start server
	serverAddr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("Starting server on %s", serverAddr)
	if err := http.ListenAndServe(serverAddr, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Get subreddit count
	futureSubreddits := s.context.RequestFuture(s.engine.GetSubredditActor(), &engine.GetCountsMsg{}, 5*time.Second)
	subredditResult, err := futureSubreddits.Result()
	if err != nil {
		http.Error(w, "Failed to get subreddit count", http.StatusInternalServerError)
		return
	}
	subredditCount := subredditResult.(int)

	// Get post count
	futurePosts := s.context.RequestFuture(s.engine.GetPostActor(), &engine.GetCountsMsg{}, 5*time.Second)
	postsResult, err := futurePosts.Result()
	if err != nil {
		http.Error(w, "Failed to get post count", http.StatusInternalServerError)
		return
	}
	postCount := postsResult.(int)

	response := fmt.Sprintf("GatorSwap Status:\n"+
		"- Total Subreddits: %d\n"+
		"- Total Posts: %d\n",
		subredditCount,
		postCount,
	)

	fmt.Fprint(w, response)
}

func (s *Server) handleSubreddit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreateSubredditRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	// Convert string ID to UUID
	creatorID, err := uuid.Parse(req.CreatorID)
	if err != nil {
		http.Error(w, "Invalid creatorId format", http.StatusBadRequest)
		return
	}

	// Create message
	createMsg := &engine.CreateSubredditMsg{
		Name:        req.Name,
		Description: req.Description,
		CreatorID:   creatorID,
	}

	// Send to actor
	future := s.context.RequestFuture(s.engine.GetSubredditActor(), createMsg, 5*time.Second)
	result, err := future.Result()
	if err != nil {
		http.Error(w, "Failed to create subreddit", http.StatusInternalServerError)
		return
	}

	// Check for application error
	if appErr, ok := result.(*utils.AppError); ok {
		http.Error(w, appErr.Message, http.StatusBadRequest)
		return
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.Title == "" || req.Content == "" {
		http.Error(w, "Title and content are required", http.StatusBadRequest)
		return
	}

	// Convert string IDs to UUIDs
	authorID, err := uuid.Parse(req.AuthorID)
	if err != nil {
		http.Error(w, "Invalid authorId format", http.StatusBadRequest)
		return
	}

	subredditID, err := uuid.Parse(req.SubredditID)
	if err != nil {
		http.Error(w, "Invalid subredditId format", http.StatusBadRequest)
		return
	}

	// Create message
	createMsg := &engine.CreatePostMsg{
		Title:       req.Title,
		Content:     req.Content,
		AuthorID:    authorID,
		SubredditID: subredditID,
	}

	// Send to actor
	future := s.context.RequestFuture(s.engine.GetPostActor(), createMsg, 5*time.Second)
	result, err := future.Result()
	if err != nil {
		http.Error(w, "Failed to create post", http.StatusInternalServerError)
		return
	}

	// Check for application error
	if appErr, ok := result.(*utils.AppError); ok {
		http.Error(w, appErr.Message, http.StatusBadRequest)
		return
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
