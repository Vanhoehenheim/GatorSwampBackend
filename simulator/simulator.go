package simulator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

type SimConfig struct {
	NumUsers         int
	NumSubreddits    int
	SimulationTime   time.Duration
	PostFrequency    float64
	CommentFrequency float64
	VoteFrequency    float64
	RepostPercentage float64
	DisconnectRate   float64
	ReconnectRate    float64
	ZipfS            float64
	BatchSize        int
	EngineURL        string
}

type SimulationStats struct {
	mu               sync.RWMutex
	StartTime        time.Time
	TotalRequests    int64
	SuccessRequests  int64
	FailedRequests   int64
	AverageLatency   time.Duration
	ActiveUsers      int
	TotalPosts       int
	TotalComments    int
	TotalVotes       int
	RepostCount      int
	RequestLatencies []time.Duration
}

// Track simulated users with their actor state
type SimulatedUser struct {
	ID            uuid.UUID
	Username      string
	Email         string
	IsConnected   bool
	LastActive    time.Time
	Posts         []uuid.UUID        // Track posts created by this user
	Comments      []uuid.UUID        // Track comments made by this user
	VotedPosts    map[uuid.UUID]bool // Track which posts user has voted on
	Subscriptions []uuid.UUID        // Track subreddit subscriptions
}

type EnhancedSimulator struct {
	config     SimConfig
	stats      *SimulationStats
	users      []*SimulatedUser
	subreddits []uuid.UUID
	client     *http.Client
	mu         sync.RWMutex
}

func NewEnhancedSimulator(config SimConfig) *EnhancedSimulator {
	return &EnhancedSimulator{
		config: config,
		stats: &SimulationStats{
			StartTime:        time.Now(),
			RequestLatencies: make([]time.Duration, 0),
		},
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *EnhancedSimulator) Run(ctx context.Context) error {
	log.Printf("Starting enhanced simulation...")

	// Initialize users and subreddits first
	if err := s.initialize(ctx); err != nil {
		return fmt.Errorf("initialization failed: %v", err)
	}

	// Start concurrent simulations
	var wg sync.WaitGroup

	// Start activities
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.SimulateActivities(ctx); err != nil {
			log.Printf("Activities simulation error: %v", err)
		}
	}()

	// Simulate connection states
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.simulateConnectivity(ctx)
	}()

	// Collect metrics
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.collectMetrics(ctx)
	}()

	wg.Wait()
	return nil
}

func (s *EnhancedSimulator) initialize(ctx context.Context) error {
	log.Printf("Starting initialization...")

	// Phase 1: Create initial user base
	log.Printf("Phase 1: Creating %d users...", s.config.NumUsers)
	if err := s.createInitialUsers(ctx); err != nil {
		return fmt.Errorf("failed to create initial users: %v", err)
	}

	// Phase 2: Select some active users to create subreddits
	log.Printf("Phase 2: Creating %d subreddits...", s.config.NumSubreddits)
	if err := s.createSubredditsWithActiveUsers(ctx); err != nil {
		return fmt.Errorf("failed to create subreddits: %v", err)
	}

	// Phase 3: Simulate subreddit joins with Zipf distribution
	log.Printf("Phase 3: Simulating subreddit memberships...")
	if err := s.simulateSubredditJoins(ctx); err != nil {
		return fmt.Errorf("failed to simulate subreddit joins: %v", err)
	}

	log.Printf("Initialization completed successfully")
	return nil
}

func (s *EnhancedSimulator) createInitialUsers(ctx context.Context) error {
	s.users = make([]*SimulatedUser, 0, s.config.NumUsers)
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a limited number of workers to not overwhelm the actor system
	numWorkers := 5 // Reduced number of workers
	userJobs := make(chan int, numWorkers)
	results := make(chan *SimulatedUser, numWorkers)

	var wg sync.WaitGroup

	// Create rate limiter for all workers to share
	rateLimiter := time.NewTicker(200 * time.Millisecond) // 5 requests per second
	defer rateLimiter.Stop()

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			client := &http.Client{
				Timeout: 5 * time.Second, // Increased timeout
			}

			for userNum := range userJobs {
				// Wait for rate limiter
				<-rateLimiter.C

				user := &SimulatedUser{
					Username:      fmt.Sprintf("user_%d", userNum),
					Email:         fmt.Sprintf("user_%d@test.com", userNum),
					IsConnected:   true,
					VotedPosts:    make(map[uuid.UUID]bool),
					Posts:         make([]uuid.UUID, 0),
					Comments:      make([]uuid.UUID, 0),
					Subscriptions: make([]uuid.UUID, 0),
				}

				// Implement exponential backoff for retries
				var err error
				for retries := 0; retries < 3; retries++ {
					if err = s.registerUserWithClient(ctx, user, client); err == nil {
						results <- user
						break
					}
					backoffDuration := time.Duration(math.Pow(2, float64(retries))) * time.Second
					log.Printf("Worker %d: Retry %d for user %s after %v delay",
						workerID, retries+1, user.Username, backoffDuration)
					time.Sleep(backoffDuration)
				}

				if err != nil {
					log.Printf("Worker %d: Failed to register user %s after retries: %v",
						workerID, user.Username, err)
				}
			}
		}(i)
	}

	// Send jobs to workers
	go func() {
		for i := 0; i < s.config.NumUsers; i++ {
			userJobs <- i
		}
		close(userJobs)
	}()

	// Close results when workers are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results and track progress
	successCount := 0
	progressTicker := time.NewTicker(2 * time.Second)
	defer progressTicker.Stop()

	for user := range results {
		s.users = append(s.users, user)
		successCount++

		select {
		case <-progressTicker.C:
			log.Printf("Progress: %d/%d users created (%.2f%%)",
				successCount, s.config.NumUsers,
				float64(successCount)/float64(s.config.NumUsers)*100)
		default:
		}

		// Add small delay between successful registrations
		time.Sleep(50 * time.Millisecond)
	}

	log.Printf("Successfully created %d users", len(s.users))
	return nil
}

func (s *EnhancedSimulator) registerUserWithClient(ctx context.Context, user *SimulatedUser, client *http.Client) error {
	data := map[string]interface{}{
		"username": user.Username,
		"email":    user.Email,
		"password": "testpass123",
		"karma":    300,
	}

	// First verify if user already exists
	existingResp, err := s.makeRequestWithClient(client, "GET",
		fmt.Sprintf("/user/profile?username=%s", user.Username), nil)
	if err == nil {
		// User might already exist, try to parse the response
		var existingUser struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(existingResp, &existingUser) == nil && existingUser.ID != "" {
			userID, err := uuid.Parse(existingUser.ID)
			if err == nil {
				user.ID = userID
				return nil // User already exists, no need to register
			}
		}
	}

	// User doesn't exist, proceed with registration
	resp, err := s.makeRequestWithClient(client, "POST", "/user/register", data)
	if err != nil {
		return fmt.Errorf("failed to register user: %v", err)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse registration response: %v", err)
	}

	registeredID, err := uuid.Parse(result.ID)
	if err != nil {
		return fmt.Errorf("invalid user ID returned: %v", err)
	}

	user.ID = registeredID

	// Give the actor system time to process the registration
	time.Sleep(200 * time.Millisecond)

	return nil
}

// func (s *EnhancedSimulator) createInitialUsers(ctx context.Context) error {
// 	s.users = make([]*SimulatedUser, 0, s.config.NumUsers)

// 	// Create a rate limiter for registration requests
// 	// rateLimiter := time.NewTicker(200 * time.Millisecond) // 5 requests per second
// 	// new rateLimiter with 20 requests per second
// 	rateLimiter := time.NewTicker(50 * time.Millisecond)
// 	defer rateLimiter.Stop()

// 	for i := 0; i < s.config.NumUsers; i++ {
// 		user := &SimulatedUser{
// 			Username:      fmt.Sprintf("user_%d", i),
// 			Email:         fmt.Sprintf("user_%d@test.com", i),
// 			IsConnected:   true,
// 			VotedPosts:    make(map[uuid.UUID]bool),
// 			Posts:         make([]uuid.UUID, 0),
// 			Comments:      make([]uuid.UUID, 0),
// 			Subscriptions: make([]uuid.UUID, 0),
// 		}

// 		// Implement retry logic
// 		var err error
// 		for retries := 0; retries < 3; retries++ {
// 			<-rateLimiter.C // Wait for rate limiter

// 			if err = s.registerUserWithRetry(ctx, user); err == nil {
// 				break
// 			}

// 			log.Printf("Retry %d for user %s: %v", retries+1, user.Username, err)
// 			time.Sleep(time.Duration(retries+1) * time.Second) // Exponential backoff
// 		}

// 		if err != nil {
// 			log.Printf("Failed to register user %s after retries: %v", user.Username, err)
// 			continue
// 		}

// 		s.users = append(s.users, user)

// 		// Log progress every 10 users
// 		if (i+1)%10 == 0 {
// 			log.Printf("Created %d/%d users...", i+1, s.config.NumUsers)
// 		}
// 	}

// 	log.Printf("Successfully created %d users", len(s.users))
// 	return nil
// }

func (s *EnhancedSimulator) createSubredditsWithActiveUsers(ctx context.Context) error {
	// Select top 10% of users as potential subreddit creators
	numCreators := len(s.users) / 10
	creators := make([]*SimulatedUser, numCreators)
	copy(creators, s.users[:numCreators])

	// Shuffle the creators to randomize subreddit creation
	rand.Shuffle(len(creators), func(i, j int) {
		creators[i], creators[j] = creators[j], creators[i]
	})

	s.subreddits = make([]uuid.UUID, 0, s.config.NumSubreddits)

	for i := 0; i < s.config.NumSubreddits; i++ {
		creator := creators[i%len(creators)] // Cycle through creators
		subredditID := uuid.New()

		// Create themed subreddits
		theme := getRandomTheme()
		name := fmt.Sprintf("%s_%d", theme, i)
		description := fmt.Sprintf("A community for %s enthusiasts", theme)

		log.Printf("Creating subreddit '%s' with creator %s...", name, creator.Username)
		if err := s.createSubreddit(ctx, subredditID, name, description, creator.ID); err != nil {
			log.Printf("Failed to create subreddit %s: %v", name, err)
			continue
		}

		s.subreddits = append(s.subreddits, subredditID)
		// Automatically subscribe creator to their subreddit
		creator.Subscriptions = append(creator.Subscriptions, subredditID)

		// Small delay between creations
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// Helper function to generate random subreddit themes
func getRandomTheme() string {
	themes := []string{
		"gaming", "tech", "science", "music", "movies",
		"books", "sports", "food", "travel", "art",
		"photography", "fitness", "programming", "news", "memes",
		"history", "nature", "pets", "fashion", "diy",
	}
	return themes[rand.Intn(len(themes))]
}

func (s *EnhancedSimulator) simulateSubredditJoins(ctx context.Context) error {
	// Calculate popularity distribution using Zipf's law
	subredditPopularity := make([]int, len(s.subreddits))
	zipf := rand.NewZipf(rand.New(rand.NewSource(time.Now().UnixNano())),
		s.config.ZipfS, 1, uint64(len(s.users)))

	// For each user, determine number of subreddits to join
	for _, user := range s.users {
		// Skip users who already created subreddits
		if len(user.Subscriptions) > 0 {
			continue
		}

		// User joins 1 to 20 subreddits based on Zipf distribution
		numJoins := (int(zipf.Uint64()) % 20) + 1

		// Get available subreddits
		availableSubs := make([]uuid.UUID, len(s.subreddits))
		copy(availableSubs, s.subreddits)
		rand.Shuffle(len(availableSubs), func(i, j int) {
			availableSubs[i], availableSubs[j] = availableSubs[j], availableSubs[i]
		})

		// Join subreddits
		for i := 0; i < numJoins && i < len(availableSubs); i++ {
			subredditID := availableSubs[i]
			if err := s.joinSubreddit(ctx, user.ID, subredditID); err != nil {
				log.Printf("Failed to join subreddit: %v", err)
				continue
			}
			user.Subscriptions = append(user.Subscriptions, subredditID)
			subredditPopularity[i]++
		}

		// Small delay between users
		time.Sleep(50 * time.Millisecond)
	}

	// Log popularity statistics
	log.Printf("\nSubreddit Membership Statistics:")
	for i, count := range subredditPopularity {
		if count > 0 {
			log.Printf("Subreddit %d: %d members", i, count)
		}
	}

	return nil
}

func (s *EnhancedSimulator) getZipfNumber(max int) int {
	zipf := rand.NewZipf(rand.New(rand.NewSource(time.Now().UnixNano())),
		s.config.ZipfS, 1, uint64(max))
	return int(zipf.Uint64()) + 1
}

// Helper method to make HTTP requests
func (s *EnhancedSimulator) makeRequest(method, endpoint string, data interface{}) ([]byte, error) {
	var body []byte
	var err error

	if data != nil {
		body, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, s.config.EngineURL+endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := s.client.Do(req)
	s.recordRequestMetrics(start, err)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	return ioutil.ReadAll(resp.Body)
}

func (s *EnhancedSimulator) simulateConnectivity(ctx context.Context) {
	log.Printf("Starting connectivity simulation...")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			for _, user := range s.users {
				// Handle disconnection for connected users
				if user.IsConnected {
					if rand.Float64() < s.config.DisconnectRate {
						user.IsConnected = false
						s.stats.mu.Lock()
						s.stats.ActiveUsers--
						s.stats.mu.Unlock()

						// Update user connection status in the engine
						data := map[string]interface{}{
							"userId": user.ID.String(),
							"status": false,
						}
						s.makeRequest("PUT", "/user/profile", data) // Ignore error as this is just simulation
					}
				} else {
					// Handle reconnection for disconnected users
					if rand.Float64() < s.config.ReconnectRate {
						user.IsConnected = true
						s.stats.mu.Lock()
						s.stats.ActiveUsers++
						s.stats.mu.Unlock()

						// Update user connection status in the engine
						data := map[string]interface{}{
							"userId": user.ID.String(),
							"status": true,
						}
						s.makeRequest("PUT", "/user/profile", data) // Ignore error as this is just simulation
					}
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *EnhancedSimulator) recordRequestMetrics(start time.Time, err error) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()

	latency := time.Since(start)
	s.stats.TotalRequests++
	s.stats.RequestLatencies = append(s.stats.RequestLatencies, latency)

	if err != nil {
		s.stats.FailedRequests++
	} else {
		s.stats.SuccessRequests++
	}

	totalLatency := s.stats.AverageLatency * time.Duration(s.stats.TotalRequests-1)
	s.stats.AverageLatency = (totalLatency + latency) / time.Duration(s.stats.TotalRequests)
}

func (s *EnhancedSimulator) registerUserWithRetry(ctx context.Context, user *SimulatedUser) error {
	data := map[string]interface{}{
		"username": user.Username,
		"email":    user.Email,
		"password": "testpass123",
		"karma":    300,
	}

	// Create custom client with shorter timeout
	client := &http.Client{
		Timeout: 800 * time.Millisecond,
	}

	// Register user
	resp, err := s.makeRequestWithClient(client, "POST", "/user/register", data)
	if err != nil {
		return fmt.Errorf("failed to register user: %v", err)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse registration response: %v", err)
	}

	// Parse user ID from response
	registeredID, err := uuid.Parse(result.ID)
	if err != nil {
		return fmt.Errorf("invalid user ID returned: %v", err)
	}

	// Update the user's ID with the one returned from the server
	user.ID = registeredID

	// Wait a bit before trying to login
	time.Sleep(100 * time.Millisecond)

	// Login is optional - don't fail if it doesn't work
	loginData := map[string]interface{}{
		"email":    user.Email,
		"password": "testpass123",
	}

	_, loginErr := s.makeRequestWithClient(client, "POST", "/user/login", loginData)
	if loginErr != nil {
		log.Printf("Note: Failed to login user %s: %v", user.Username, loginErr)
		// Continue anyway as this isn't critical
	}

	return nil
}

func (s *EnhancedSimulator) makeRequestWithClient(client *http.Client, method, endpoint string, data interface{}) ([]byte, error) {
	var body []byte
	var err error

	if data != nil {
		body, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, s.config.EngineURL+endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := client.Do(req)
	s.recordRequestMetrics(start, err)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	return ioutil.ReadAll(resp.Body)
}

func (s *EnhancedSimulator) createSubreddit(ctx context.Context, id uuid.UUID, name, description string, creatorID uuid.UUID) error {
	data := map[string]interface{}{
		"name":        name,
		"description": description,
		"creatorId":   creatorID.String(),
	}

	resp, err := s.makeRequest("POST", "/subreddit", data)
	if err != nil {
		return fmt.Errorf("failed to create subreddit: %v", err)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse subreddit response: %v", err)
	}

	return nil
}

func (s *EnhancedSimulator) joinSubreddit(ctx context.Context, userID, subredditID uuid.UUID) error {
	data := map[string]interface{}{
		"userId":      userID.String(),
		"subredditId": subredditID.String(),
	}

	_, err := s.makeRequest("POST", "/subreddit/members", data)
	if err != nil {
		return fmt.Errorf("failed to join subreddit: %v", err)
	}

	return nil
}

func (s *EnhancedSimulator) collectMetrics(ctx context.Context) {
	log.Printf("Starting metrics collection...")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.stats.mu.RLock()
			elapsed := time.Since(s.stats.StartTime)
			requestRate := float64(s.stats.TotalRequests) / elapsed.Seconds()
			successRate := 0.0
			if s.stats.TotalRequests > 0 {
				successRate = float64(s.stats.SuccessRequests) / float64(s.stats.TotalRequests) * 100
			}

			// Calculate active users
			activeUsers := 0
			s.mu.RLock()
			for _, user := range s.users {
				if user.IsConnected {
					activeUsers++
				}
			}
			s.mu.RUnlock()

			// Update active users count
			s.stats.ActiveUsers = activeUsers

			log.Printf("\nSimulation Metrics (%.1f seconds elapsed):", elapsed.Seconds())
			log.Printf("- Request Rate: %.2f req/sec", requestRate)
			log.Printf("- Success Rate: %.1f%%", successRate)
			log.Printf("- Average Latency: %v", s.stats.AverageLatency)
			log.Printf("- Active Users: %d/%d", activeUsers, len(s.users))
			log.Printf("- Total Posts: %d (Reposts: %d)", s.stats.TotalPosts, s.stats.RepostCount)
			log.Printf("- Total Comments: %d", s.stats.TotalComments)
			log.Printf("- Total Votes: %d", s.stats.TotalVotes)
			log.Printf("- Failed Requests: %d", s.stats.FailedRequests)

			s.stats.mu.RUnlock()
		}
	}
}

// SimulationMetrics holds the metrics of the simulation
type SimulationMetrics struct {
	TotalUsers        int
	ActiveUsers       int
	TotalPosts        int
	TotalComments     int
	TotalVotes        int
	RepostCount       int
	AverageLatency    time.Duration
	ErrorCount        int
	RequestsPerSecond float64
}

// GetMetrics returns the current simulation metrics
func (s *EnhancedSimulator) GetMetrics() SimulationMetrics {
	s.stats.mu.RLock()
	defer s.stats.mu.RUnlock()

	elapsed := time.Since(s.stats.StartTime)
	requestRate := float64(s.stats.TotalRequests) / elapsed.Seconds()

	return SimulationMetrics{
		TotalUsers:        len(s.users),
		ActiveUsers:       s.stats.ActiveUsers,
		TotalPosts:        s.stats.TotalPosts,
		TotalComments:     s.stats.TotalComments,
		TotalVotes:        s.stats.TotalVotes,
		RepostCount:       s.stats.RepostCount,
		AverageLatency:    s.stats.AverageLatency,
		ErrorCount:        int(s.stats.FailedRequests),
		RequestsPerSecond: requestRate,
	}
}
