package main

import (
	"context"
	"fmt"
	"gator-swamp/internal/config"
	"gator-swamp/internal/database"
	"gator-swamp/internal/engine"
	"gator-swamp/internal/engine/actors" // Import actors package
	"gator-swamp/internal/handlers"
	"gator-swamp/internal/middleware"
	"gator-swamp/internal/utils"
	"gator-swamp/internal/websocket"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Configure logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Gator Swamp API server...")

	// Load configuration
	config, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// --- BEGIN DEBUG LOGGING ---
	log.Printf("[DEBUG] Loaded DB Config - Type: %s", config.Database.Type)
	log.Printf("[DEBUG] Loaded DB Config - URI: %s", config.Database.URI)
	log.Printf("[DEBUG] Loaded DB Config - Host: %s", config.Database.Host)
	log.Printf("[DEBUG] Loaded DB Config - Port: %d", config.Database.Port)
	log.Printf("[DEBUG] Loaded DB Config - User: %s", config.Database.User)
	log.Printf("[DEBUG] Loaded DB Config - Name: %s", config.Database.Name)
	log.Printf("[DEBUG] Loaded DB Config - SSLMode: %s", config.Database.SSLMode)
	// Check if password field exists and log placeholder if it does
	if config.Database.Password != "" {
		log.Printf("[DEBUG] Loaded DB Config - Password: [SET]")
	} else {
		log.Printf("[DEBUG] Loaded DB Config - Password: [NOT SET]")
	}
	// --- END DEBUG LOGGING ---

	// Initialize Actor System
	system := actor.NewActorSystem()
	rootContext := system.Root // Use system.Root based on engine.go

	// Initialize Metrics Collector (but don't register it with Prometheus here)
	metrics := utils.NewMetricsCollector()
	// REMOVED: utils.RegisterMetrics(metrics) // Incorrect function call

	// Initialize Database (PostgreSQL only)
	dbAdapter, err := database.NewPostgresDB(config.Database.URI)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer dbAdapter.Close(context.Background()) // Ensure DB connection is closed on exit
	if err := dbAdapter.InitializeTables(context.Background()); err != nil {
		log.Fatalf("Failed to initialize tables: %v", err)
	}

	// Initialize WebSocket Hub
	hub := websocket.NewHub()
	go hub.Run() // Run the hub in a separate goroutine

	// Initialize Engine Actor
	engineInstance := engine.NewEngine(system, metrics, dbAdapter)
	engineProps := actor.PropsFromProducer(func() actor.Actor { return engineInstance })
	enginePID, err := rootContext.SpawnNamed(engineProps, "engine-actor")
	if err != nil {
		log.Fatalf("Failed to spawn engine actor instance: %v", err)
	}

	// Get PIDs for actors managed BY the Engine
	commentActorPID := engineInstance.GetCommentActor()
	postActorPID := engineInstance.GetPostActor()
	subredditActorPID := engineInstance.GetSubredditActor()
	userSupervisorPID := engineInstance.GetUserSupervisor()

	// Spawn DirectMessageActor directly, passing the DB adapter and Hub
	directMessageActorPID := rootContext.Spawn(actor.PropsFromProducer(func() actor.Actor {
		return actors.NewDirectMessageActor(dbAdapter, hub)
	}))
	log.Printf("Direct Message actor started with PID: %s", directMessageActorPID.String())

	// Initialize Server with dependencies including the hub
	server := handlers.NewServer(
		system,         // Pass ActorSystem
		rootContext,    // Pass RootContext
		engineInstance, // Pass Engine instance
		enginePID,      // Pass Engine PID
		metrics,        // Pass Metrics Collector
		commentActorPID,
		directMessageActorPID, // Pass the directly spawned PID
		dbAdapter,
		hub,
		postActorPID,
		subredditActorPID,
		userSupervisorPID,
		5*time.Second, // Example Request Timeout
	)

	// Setup HTTP routes
	mux := http.NewServeMux()

	// CORS configuration
	corsConfig := middleware.CORSConfig{
		AllowedOrigins: config.AllowedOrigins,
		AllowedMethods: strings.Split("GET,POST,PUT,DELETE,OPTIONS", ","), // Split string into slice
		AllowedHeaders: strings.Split("Content-Type,Authorization", ","),  // Split string into slice
		MaxAge:         86400,
		// AllowCredentials defaults true in DefaultCORSConfig
	}

	// Add Prometheus metrics endpoint if enabled
	if config.Server.MetricsEnabled {
		mux.Handle("/metrics", promhttp.Handler())
	}

	// Public routes
	mux.HandleFunc("/health", middleware.ApplyCORS(server.HandleSimpleHealth(), &corsConfig))
	mux.HandleFunc("/health/full", middleware.ApplyCORS(server.HandleHealth(), &corsConfig))
	mux.HandleFunc("/user/register", middleware.ApplyCORS(server.HandleUserRegistration(), &corsConfig))
	mux.HandleFunc("/user/login", middleware.ApplyCORS(server.HandleUserLogin(), &corsConfig))

	// Protected routes (Apply JWT middleware)
	mux.HandleFunc("/subreddit",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleSubreddits(), "/subreddit"), &corsConfig))
	mux.HandleFunc("/subreddit/members",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleSubredditMembers(), "/subreddit/members"), &corsConfig))
	mux.HandleFunc("/post",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandlePost(), "/post"), &corsConfig))
	mux.HandleFunc("/post/vote",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleVote(), "/post/vote"), &corsConfig))
	mux.HandleFunc("/user/feed",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleGetFeed(), "/user/feed"), &corsConfig))
	mux.HandleFunc("/user/profile",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleUserProfile(), "/user/profile"), &corsConfig))
	mux.HandleFunc("/comment",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleComment(), "/comment"), &corsConfig))
	mux.HandleFunc("/comment/post",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleGetPostComments(), "/comment/post"), &corsConfig))
	mux.HandleFunc("/messages",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleDirectMessages(), "/messages"), &corsConfig))
	mux.HandleFunc("/messages/conversation",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleConversation(), "/messages/conversation"), &corsConfig))
	mux.HandleFunc("/messages/read",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleMarkMessageRead(), "/messages/read"), &corsConfig))
	mux.HandleFunc("/comment/vote",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleCommentVote(), "/comment/vote"), &corsConfig))
	mux.HandleFunc("/posts/recent",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleRecentPosts(), "/posts/recent"), &corsConfig))
	mux.HandleFunc("/users",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleGetAllUsers(), "/users"), &corsConfig))

	// WebSocket endpoint
	mux.HandleFunc("/ws", server.HandleWebSocket())

	// Set up HTTP server
	serverAddr := fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port)
	httpServer := &http.Server{
		Addr:         serverAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting HTTP server on %s", serverAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Graceful shutdown handling
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// Block until a signal is received.
	sig := <-signalChan
	log.Printf("Received signal: %s. Shutting down gracefully...", sig)

	// Create a deadline to wait for.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown failed: %v", err)
	}

	// Stop the actor system
	system.Shutdown()
	log.Println("Actor system shut down.")

	log.Println("Server gracefully stopped.")
}
