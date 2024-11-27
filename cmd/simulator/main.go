package main

import (
	"context"
	"log"
	"time"

	"gator-swamp/simulator" // This should match your module name
)

func main() {
	// Define simulation configuration
	config := simulator.SimConfig{
		NumUsers:         10,
		NumSubreddits:    5,
		SimulationTime:   10 * time.Minute,
		PostFrequency:    100.0,
		CommentFrequency: 60.0,
		VoteFrequency:    100.0,
		RepostPercentage: 0.1,
		DisconnectRate:   0.01,
		ReconnectRate:    0.05,
		ZipfS:            1.07,
		BatchSize:        50,
		EngineURL:        "http://localhost:8080",
	}

	sim := simulator.NewEnhancedSimulator(config)
	ctx, cancel := context.WithTimeout(context.Background(), config.SimulationTime)
	defer cancel()

	// if err := sim.Run(ctx); err != nil {
	// 	log.Fatalf("Simulation failed: %v", err)
	// }

	// Log configuration
	log.Printf("Starting simulation with configuration:")
	log.Printf("- Engine URL: %s", config.EngineURL)
	log.Printf("- Number of users: %d", config.NumUsers)
	log.Printf("- Number of subreddits: %d", config.NumSubreddits)
	log.Printf("- Simulation time: %v", config.SimulationTime)
	log.Printf("- Post frequency: %.2f posts/user/hour", config.PostFrequency)
	log.Printf("- Comment frequency: %.2f comments/user/hour", config.CommentFrequency)
	log.Printf("- Repost percentage: %.1f%%", config.RepostPercentage*100)
	log.Printf("- Disconnect rate: %.2f", config.DisconnectRate)
	log.Printf("- Reconnect rate: %.2f", config.ReconnectRate)
	log.Printf("- Zipf parameter: %.2f", config.ZipfS)

	// Start simulation
	if err := sim.Run(ctx); err != nil {
		log.Fatalf("Simulation failed: %v", err)
	}

	// Print final metrics
	metrics := sim.GetMetrics()
	log.Printf("\nSimulation completed. Final metrics:")
	log.Printf("- Total users: %d", metrics.TotalUsers)
	log.Printf("- Active users at end: %d", metrics.ActiveUsers)
	log.Printf("- Total posts: %d", metrics.TotalPosts)
	log.Printf("- Reposts: %d", metrics.RepostCount)
	log.Printf("- Error count: %d", metrics.ErrorCount)
}
