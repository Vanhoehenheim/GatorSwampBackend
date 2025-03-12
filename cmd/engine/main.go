package main

import (
	"context"
	"fmt"
	"gator-swamp/internal/config"
	"gator-swamp/internal/database"
	"gator-swamp/internal/engine"
	"gator-swamp/internal/engine/actors"
	"gator-swamp/internal/handlers"
	"gator-swamp/internal/middleware"
	"gator-swamp/internal/utils"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/asynkron/protoactor-go/actor"
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

	// Initialize MongoDB with configuration
	mongodb, err := database.NewMongoDB(config.MongoDBURI)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up graceful shutdown handler
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		sig := <-sigChan
		log.Printf("Received signal: %v, initiating graceful shutdown", sig)
		cancel()
	}()

	// Initialize metrics collector
	metrics := utils.NewMetricsCollector()

	// Initialize actor system
	system := actor.NewActorSystem()
	rootContext := system.Root

	// Initialize engine
	gatorEngine := engine.NewEngine(system, metrics, mongodb)
	engineProps := actor.PropsFromProducer(func() actor.Actor {
		return gatorEngine
	})
	enginePID := rootContext.Spawn(engineProps)

	// Initialize comment actor
	commentActor := rootContext.Spawn(actor.PropsFromProducer(func() actor.Actor {
		return actors.NewCommentActor(enginePID, mongodb)
	}))

	// Initialize direct message actor
	directMessageActor := rootContext.Spawn(actor.PropsFromProducer(func() actor.Actor {
		return actors.NewDirectMessageActor(mongodb)
	}))

	// Initialize server with all dependencies
	server := handlers.NewServer(
		system,
		rootContext,
		gatorEngine,
		enginePID,
		metrics,
		commentActor,
		directMessageActor,
		mongodb,
	)

	// Set up HTTP router with middleware
	mux := http.NewServeMux()

	// CORS configuration from app config
	corsConfig := &middleware.CORSConfig{
		AllowedOrigins:   config.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "Accept", "Origin", "X-Requested-With"},
		ExposedHeaders:   []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           86400, // 24 hours
	}

	// Public endpoints (no JWT required)
	// Then update your router to include both endpoints
	mux.HandleFunc("/health", middleware.ApplyCORS(server.HandleSimpleHealth(), corsConfig))
	mux.HandleFunc("/health/full", middleware.ApplyCORS(server.HandleHealth(), corsConfig))
	mux.HandleFunc("/user/register", middleware.ApplyCORS(server.HandleUserRegistration(), corsConfig))
	mux.HandleFunc("/user/login", middleware.ApplyCORS(server.HandleUserLogin(), corsConfig))

	// Protected endpoints (JWT required)
	mux.HandleFunc("/subreddit",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleSubreddits(), "/subreddit"), corsConfig))
	mux.HandleFunc("/subreddit/members",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleSubredditMembers(), "/subreddit/members"), corsConfig))
	mux.HandleFunc("/post",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandlePost(), "/post"), corsConfig))
	mux.HandleFunc("/post/vote",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleVote(), "/post/vote"), corsConfig))
	mux.HandleFunc("/user/feed",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleGetFeed(), "/user/feed"), corsConfig))
	mux.HandleFunc("/user/profile",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleUserProfile(), "/user/profile"), corsConfig))
	mux.HandleFunc("/comment",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleComment(), "/comment"), corsConfig))
	mux.HandleFunc("/comment/post",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleGetPostComments(), "/comment/post"), corsConfig))
	mux.HandleFunc("/messages",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleDirectMessages(), "/messages"), corsConfig))
	mux.HandleFunc("/messages/conversation",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleConversation(), "/messages/conversation"), corsConfig))
	mux.HandleFunc("/messages/read",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleMarkMessageRead(), "/messages/read"), corsConfig))
	mux.HandleFunc("/comment/vote",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleCommentVote(), "/comment/vote"), corsConfig))
	mux.HandleFunc("/posts/recent",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleRecentPosts(), "/posts/recent"), corsConfig))
	mux.HandleFunc("/users",
		middleware.ApplyCORS(middleware.ApplyJWTMiddleware(server.HandleGetAllUsers(), "/users"), corsConfig))

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

	// Wait for shutdown signal
	<-ctx.Done()
	log.Println("Server is shutting down...")

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Shutdown HTTP server
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// Close MongoDB connection
	if err := mongodb.Close(shutdownCtx); err != nil {
		log.Printf("Error closing MongoDB connection: %v", err)
	}

	log.Println("Server shutdown complete")
}
