package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
)

func (s *EnhancedSimulator) SimulateActivities(ctx context.Context) error {
	log.Printf("Starting activities simulation...")

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel() // Ensure the context is canceled after the simulation ends

	log.Printf("Starting activities simulation...")

	var wg sync.WaitGroup

	// Start post simulation
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.simulatePosts(ctx)
	}()

	// Start comment simulation
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.simulateComments(ctx)
	}()

	// Start vote simulation
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.simulateVotes(ctx)
	}()

	wg.Wait()
	return nil
}

func (s *EnhancedSimulator) simulatePosts(ctx context.Context) {
	log.Printf("Starting post simulation...")

	// For 100 posts/hour, we want approximately 2 posts per minute
	tickInterval := 500 * time.Millisecond // Check every half second
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	log.Printf("Post simulation starting with tick interval: %v", tickInterval)

	const numWorkers = 5
	postJobs := make(chan *SimulatedUser, s.config.NumUsers)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for user := range postJobs {
				if !user.IsConnected || len(user.Subscriptions) == 0 {
					continue
				}

				// Calculate probability for this tick
				// For 100 posts/hour across all users, with 2 ticks/second
				// probability = (100/3600)/(2) = 0.014 per tick per user
				if rand.Float64() < (s.config.PostFrequency/3600.0)/2.0 {
					subredditID := user.Subscriptions[rand.Intn(len(user.Subscriptions))]
					title := fmt.Sprintf("Post by %s at %d", user.Username, time.Now().Unix())
					content := fmt.Sprintf("Content from %s: %s", user.Username, time.Now().Format(time.RFC3339))

					data := map[string]interface{}{
						"title":       title,
						"content":     content,
						"authorId":    user.ID.String(),
						"subredditId": subredditID.String(),
					}

					start := time.Now()
					if _, err := s.makeRequest("POST", "/post", data); err == nil {
						s.stats.mu.Lock()
						s.stats.TotalPosts++
						s.stats.mu.Unlock()
						log.Printf("Created post by user %s", user.Username)
					}
					s.recordRequestMetrics(start, nil)
				}
			}
		}(i)
	}

	// Main event loop
	for {
		select {
		case <-ctx.Done():
			close(postJobs)
			wg.Wait()
			return
		case <-ticker.C:
			s.mu.RLock()
			for _, user := range s.users {
				if user.IsConnected {
					postJobs <- user
				}
			}
			s.mu.RUnlock()
		}
	}
}

func (s *EnhancedSimulator) simulateComments(ctx context.Context) {
	log.Printf("Starting comment simulation...")

	tickInterval := 500 * time.Millisecond
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	const numWorkers = 5
	commentJobs := make(chan *SimulatedUser, s.config.NumUsers)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for user := range commentJobs {
				if !user.IsConnected {
					continue
				}

				if rand.Float64() < (s.config.CommentFrequency/3600.0)/2.0 {
					postID, err := s.getRandomPostToComment(user)
					if err != nil {
						continue
					}

					data := map[string]interface{}{
						"content":  fmt.Sprintf("Comment from %s at %s", user.Username, time.Now().Format(time.RFC3339)),
						"authorId": user.ID.String(),
						"postId":   postID.String(),
					}

					start := time.Now()
					if _, err := s.makeRequest("POST", "/comment", data); err == nil {
						s.stats.mu.Lock()
						s.stats.TotalComments++
						s.stats.mu.Unlock()
						log.Printf("Created comment by user %s", user.Username)
					}
					s.recordRequestMetrics(start, err)
				}
			}
		}(i)
	}

	for {
		select {
		case <-ctx.Done():
			close(commentJobs)
			wg.Wait()
			return
		case <-ticker.C:
			s.mu.RLock()
			for _, user := range s.users {
				if user.IsConnected {
					commentJobs <- user
				}
			}
			s.mu.RUnlock()
		}
	}
}

func (s *EnhancedSimulator) simulateVotes(ctx context.Context) {
	log.Printf("Starting vote simulation...")

	tickInterval := 500 * time.Millisecond
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	const numWorkers = 5
	voteJobs := make(chan *SimulatedUser, s.config.NumUsers)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for user := range voteJobs {
				if !user.IsConnected {
					continue
				}

				if rand.Float64() < (s.config.VoteFrequency/3600.0)/2.0 {
					postID, err := s.getRandomPostToVote(user)
					if err != nil {
						continue
					}

					if user.VotedPosts[postID] {
						continue
					}

					isUpvote := rand.Float64() < 0.7
					data := map[string]interface{}{
						"userId":   user.ID.String(),
						"postId":   postID.String(),
						"isUpvote": isUpvote,
					}

					start := time.Now()
					if _, err := s.makeRequest("POST", "/post/vote", data); err == nil {
						s.mu.Lock()
						user.VotedPosts[postID] = true
						s.stats.TotalVotes++
						s.mu.Unlock()
						log.Printf("Created vote by user %s (upvote: %v)", user.Username, isUpvote)
					}
					s.recordRequestMetrics(start, err)
				}
			}
		}(i)
	}

	for {
		select {
		case <-ctx.Done():
			close(voteJobs)
			wg.Wait()
			return
		case <-ticker.C:
			s.mu.RLock()
			for _, user := range s.users {
				if user.IsConnected {
					voteJobs <- user
				}
			}
			s.mu.RUnlock()
		}
	}
}

// Helper functions

func (s *EnhancedSimulator) getRandomPostToComment(user *SimulatedUser) (uuid.UUID, error) {
	if len(user.Subscriptions) == 0 {
		return uuid.Nil, fmt.Errorf("no subscriptions")
	}

	subredditID := user.Subscriptions[rand.Intn(len(user.Subscriptions))]
	resp, err := s.makeRequest("GET", fmt.Sprintf("/post?subredditId=%s", subredditID), nil)
	if err != nil {
		return uuid.Nil, err
	}

	var posts []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp, &posts); err != nil {
		return uuid.Nil, err
	}

	if len(posts) == 0 {
		return uuid.Nil, fmt.Errorf("no posts found")
	}

	postID, err := uuid.Parse(posts[rand.Intn(len(posts))].ID)
	if err != nil {
		return uuid.Nil, err
	}

	return postID, nil
}

func (s *EnhancedSimulator) getRandomComment(postID uuid.UUID) (uuid.UUID, error) {
	resp, err := s.makeRequest("GET", fmt.Sprintf("/comment/post?postId=%s", postID), nil)
	if err != nil {
		return uuid.Nil, err
	}

	var comments []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp, &comments); err != nil {
		return uuid.Nil, err
	}

	if len(comments) == 0 {
		return uuid.Nil, fmt.Errorf("no comments found")
	}

	commentID, err := uuid.Parse(comments[rand.Intn(len(comments))].ID)
	if err != nil {
		return uuid.Nil, err
	}

	return commentID, nil
}

func (s *EnhancedSimulator) getRandomPostToVote(user *SimulatedUser) (uuid.UUID, error) {
	return s.getRandomPostToComment(user) // Reuse the same logic
}
