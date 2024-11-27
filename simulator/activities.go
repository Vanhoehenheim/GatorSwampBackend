package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"gator-swamp/internal/models"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ErrorResponse struct {
	Code    string  `json:"Code"`
	Message string  `json:"Message"`
	Origin  *string `json:"Origin"`
}

func (s *EnhancedSimulator) SimulateActivities(ctx context.Context) error {
	log.Printf("Starting activities simulation...")

	// Create a channel to signal when we have enough posts to start comments/votes
	postsAvailable := make(chan struct{})

	var wg sync.WaitGroup

	// Start post simulation
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.simulatePosts(ctx, postsAvailable)
	}()

	// Wait for some posts to be created before starting comments and votes
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.stats.mu.RLock()
				if s.stats.TotalPosts >= 10 { // Wait for at least 10 posts
					s.stats.mu.RUnlock()
					close(postsAvailable)
					return
				}
				s.stats.mu.RUnlock()
			}
		}
	}()

	// Start comment simulation after some posts are available
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		case <-postsAvailable:
			log.Printf("Starting comments after posts available...")
			s.simulateComments(ctx)
		}
	}()

	// Start vote simulation after some posts are available
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		case <-postsAvailable:
			log.Printf("Starting votes after posts available...")
			s.simulateVotes(ctx)
		}
	}()

	wg.Wait()
	return nil
}

func (s *EnhancedSimulator) simulatePosts(ctx context.Context, postsAvailable chan struct{}) {
	log.Printf("Starting post simulation...")

	tickInterval := 500 * time.Millisecond
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

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

				if rand.Float64() < (s.config.PostFrequency/3600.0)/2.0 {
					subredditID := user.Subscriptions[rand.Intn(len(user.Subscriptions))]

					// Ensure membership before posting
					joinData := map[string]interface{}{
						"subredditId": subredditID.String(),
						"userId":      user.ID.String(),
					}

					// Try to join/rejoin the subreddit before posting
					joinResp, err := s.makeRequest("POST", "/subreddit/members", joinData)
					if err != nil {
						log.Printf("Debug: Failed to verify subreddit membership: %v", err)
						continue
					}
					log.Printf("Debug: Join response for user %s in subreddit %s: %s",
						user.ID, subredditID, string(joinResp))

					// Small delay after joining
					time.Sleep(50 * time.Millisecond)

					// Create the post
					postData := map[string]interface{}{
						"title":       fmt.Sprintf("Post by %s at %d", user.Username, time.Now().Unix()),
						"content":     fmt.Sprintf("Content from %s: %s", user.Username, time.Now().Format(time.RFC3339)),
						"authorId":    user.ID.String(),
						"subredditId": subredditID.String(),
					}

					log.Printf("Debug: Creating post for user %s in subreddit %s with data: %+v",
						user.ID, subredditID, postData)

					start := time.Now()
					resp, err := s.makeRequest("POST", "/post", postData)
					if err != nil {
						log.Printf("Debug: Error creating post: %v", err)
						s.recordRequestMetrics(start, err)
						continue
					}
					log.Printf("Debug: Post creation response: %s", string(resp))

					s.stats.mu.Lock()
					postCount := s.stats.TotalPosts + 1
					s.stats.TotalPosts = postCount
					s.stats.mu.Unlock()

					log.Printf("Created post by user %s (Total: %d) in subreddit %s",
						user.Username, postCount, subredditID)
					s.recordRequestMetrics(start, nil)

					// If we hit the threshold, signal that posts are available
					if postCount == 10 {
						select {
						case <-postsAvailable: // Check if already closed
						default:
							close(postsAvailable)
						}
					}
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
					select {
					case postJobs <- user:
					default: // Don't block if channel is full
					}
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
						log.Printf("Debug: Worker %d failed to get random post: %v", workerID, err)
						continue
					}

					data := map[string]interface{}{
						"content":  fmt.Sprintf("Comment from %s at %s", user.Username, time.Now().Format(time.RFC3339)),
						"authorId": user.ID.String(),
						"postId":   postID.String(),
					}

					start := time.Now()
					resp, err := s.makeRequest("POST", "/comment", data)
					if err != nil {
						log.Printf("Debug: Worker %d failed to create comment: %v", workerID, err)
					} else {
						s.stats.mu.Lock()
						s.stats.TotalComments++
						commentCount := s.stats.TotalComments
						s.stats.mu.Unlock()
						log.Printf("Created comment by user %s (Total: %d)", user.Username, commentCount)
						log.Printf("Debug: Comment response: %s", string(resp))
					}
					s.recordRequestMetrics(start, err)
				}
			}
		}(i)
	}

	// Rest of the function remains the same
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
					select {
					case commentJobs <- user:
					default: // Don't block if channel is full
					}
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

	shuffledSubs := make([]uuid.UUID, len(user.Subscriptions))
	copy(shuffledSubs, user.Subscriptions)
	rand.Shuffle(len(shuffledSubs), func(i, j int) {
		shuffledSubs[i], shuffledSubs[j] = shuffledSubs[j], shuffledSubs[i]
	})

	for _, subredditID := range shuffledSubs {
		log.Printf("Debug: Fetching posts for subreddit %s", subredditID)

		resp, err := s.makeRequest("GET", fmt.Sprintf("/post?subredditId=%s", subredditID), nil)
		if err != nil {
			log.Printf("Debug: Error making request: %v", err)
			continue
		}

		// First try to parse as error response
		var errorResp ErrorResponse
		if err := json.Unmarshal(resp, &errorResp); err == nil && errorResp.Code == "NOT_FOUND" {
			log.Printf("Debug: No posts found in subreddit %s", subredditID)
			continue
		}

		// If it's not an error response, try to parse as array of posts
		var posts []models.Post
		if err := json.Unmarshal(resp, &posts); err != nil {
			log.Printf("Debug: Error parsing posts: %v", err)
			log.Printf("Debug: Raw API response: %s", string(resp))
			continue
		}

		if len(posts) == 0 {
			log.Printf("Debug: Empty posts array for subreddit %s", subredditID)
			continue
		}

		// Select a random post
		selectedPost := posts[rand.Intn(len(posts))]
		log.Printf("Debug: Successfully found post %s to comment on", selectedPost.ID)
		return selectedPost.ID, nil
	}

	return uuid.Nil, fmt.Errorf("no posts found in any subscribed subreddits")
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
