package actors

import (
	stdctx "context" // Import standard context package with alias to avoid confusion
	"gator-swamp/internal/database"
	"gator-swamp/internal/models"
	"gator-swamp/internal/utils"
	"log"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/google/uuid"
)

// Message types for Subreddit operations
type (
	CreateSubredditMsg struct {
		Name        string
		Description string
		CreatorID   uuid.UUID
	}

	JoinSubredditMsg struct {
		SubredditID uuid.UUID
		UserID      uuid.UUID
	}

	LeaveSubredditMsg struct {
		SubredditID uuid.UUID
		UserID      uuid.UUID
	}

	ListSubredditsMsg struct{}

	GetSubredditMembersMsg struct {
		SubredditID uuid.UUID
	}

	GetSubredditByIDMsg struct {
		SubredditID uuid.UUID
	}

	GetSubredditByNameMsg struct {
		Name string
	}
)

// SubredditActor handles all subreddit-related operations
type SubredditActor struct {
	subredditsByName map[string]*models.Subreddit
	subredditsById   map[uuid.UUID]*models.Subreddit
	subredditMembers map[uuid.UUID]map[uuid.UUID]bool
	metrics          *utils.MetricsCollector
	context          actor.Context
	mongodb          *database.MongoDB
}

func NewSubredditActor(metrics *utils.MetricsCollector, mongodb *database.MongoDB) actor.Actor {
	return &SubredditActor{
		subredditsByName: make(map[string]*models.Subreddit),
		subredditsById:   make(map[uuid.UUID]*models.Subreddit),
		subredditMembers: make(map[uuid.UUID]map[uuid.UUID]bool),
		metrics:          metrics,
		mongodb:          mongodb,
	}
}

// Receive handles incoming messages
func (a *SubredditActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		a.context = context
		log.Printf("SubredditActor started")

	case *actor.Stopping:
		log.Printf("SubredditActor stopping")

	case *actor.Stopped:
		log.Printf("SubredditActor stopped")

	case *actor.Restarting:
		log.Printf("SubredditActor restarting")

	case *CreateSubredditMsg:
		a.handleCreateSubreddit(context, msg)

	case *GetSubredditByIDMsg:
		a.handleGetSubredditByID(context, msg)

	case *JoinSubredditMsg:
		a.handleJoinSubreddit(context, msg)

	case *LeaveSubredditMsg:
		a.handleLeaveSubreddit(context, msg)

	case *ListSubredditsMsg:
		a.handleListSubreddits(context)

	case *GetSubredditMembersMsg:
		a.handleGetMembers(context, msg)

	case *GetSubredditByNameMsg:
		a.handleGetSubredditByName(context, msg)

	case *GetCountsMsg:
		context.Respond(len(a.subredditsByName))
	}
}

// Handler functions for each message type
func (a *SubredditActor) handleCreateSubreddit(ctx actor.Context, msg *CreateSubredditMsg) {
	log.Printf("SubredditActor: Creating subreddit: %s", msg.Name)
	startTime := time.Now()

	// Check cache first
	if _, exists := a.subredditsByName[msg.Name]; exists {
		ctx.Respond(utils.NewAppError(utils.ErrDuplicate, "subreddit already exists", nil))
		return
	}

	newSubreddit := &models.Subreddit{
		ID:          uuid.New(),
		Name:        msg.Name,
		Description: msg.Description,
		CreatorID:   msg.CreatorID,
		CreatedAt:   time.Now(),
		Members:     1,
	}

	// Create a new context for MongoDB operations
	dbCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
	defer cancel()

	// Create the subreddit in MongoDB
	err := a.mongodb.CreateSubreddit(dbCtx, newSubreddit)
	if err != nil {
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to create subreddit", err))
		return
	}

	// Update the creator's subreddits list
	err = a.mongodb.UpdateUserSubreddits(dbCtx, msg.CreatorID, newSubreddit.ID, true)
	if err != nil {
		log.Printf("Warning: Failed to update creator's subreddit list: %v", err)
		// Don't fail the whole operation if this fails
	}

	// Store in local cache
	a.subredditsByName[msg.Name] = newSubreddit
	a.subredditsById[newSubreddit.ID] = newSubreddit
	a.subredditMembers[newSubreddit.ID] = map[uuid.UUID]bool{
		msg.CreatorID: true,
	}

	a.metrics.AddOperationLatency("create_subreddit", time.Since(startTime))
	log.Printf("SubredditActor: Successfully created subreddit: %s", newSubreddit.ID)
	ctx.Respond(newSubreddit)
}

func (a *SubredditActor) handleGetSubredditByID(ctx actor.Context, msg *GetSubredditByIDMsg) {
	log.Printf("Fetching subreddit details for ID: %s", msg.SubredditID)

	// First check cache
	var subreddit *models.Subreddit
	for _, s := range a.subredditsByName {
		if s.ID == msg.SubredditID {
			subreddit = s
			break
		}
	}

	// If not in cache, try MongoDB
	if subreddit == nil {
		dbCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
		defer cancel()

		var err error
		subreddit, err = a.mongodb.GetSubredditByID(dbCtx, msg.SubredditID)
		if err != nil {
			log.Printf("Error fetching subreddit from MongoDB: %v", err)
			ctx.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", err))
			return
		}

		// Update cache if found
		if subreddit != nil {
			a.subredditsByName[subreddit.Name] = subreddit
			a.subredditsById[subreddit.ID] = subreddit

			if _, exists := a.subredditMembers[subreddit.ID]; !exists {
				a.subredditMembers[subreddit.ID] = make(map[uuid.UUID]bool)
			}
		}
	}

	if subreddit == nil {
		ctx.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
		return
	}

	// Use actual member count from MongoDB
	// _, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
	// defer cancel()

	response := struct {
		ID          string      `json:"ID"`
		Name        string      `json:"Name"`
		Description string      `json:"Description"`
		CreatorID   string      `json:"CreatorID"`
		Members     int         `json:"Members"`
		CreatedAt   time.Time   `json:"CreatedAt"`
		Posts       []uuid.UUID `json:"Posts"`
	}{
		ID:          subreddit.ID.String(),
		Name:        subreddit.Name,
		Description: subreddit.Description,
		CreatorID:   subreddit.CreatorID.String(),
		Members:     subreddit.Members, // Use the value from the model
		CreatedAt:   subreddit.CreatedAt,
		Posts:       subreddit.Posts,
	}

	log.Printf("Successfully fetched subreddit details for ID: %s", msg.SubredditID)
	ctx.Respond(response)
}

func (a *SubredditActor) handleGetSubredditByName(ctx actor.Context, msg *GetSubredditByNameMsg) {
	log.Printf("Fetching subreddit details for name: %s", msg.Name)

	// First check cache
	var subreddit *models.Subreddit
	if cached, exists := a.subredditsByName[msg.Name]; exists {
		subreddit = cached
	}

	// If not in cache, try MongoDB
	if subreddit == nil {
		dbCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
		defer cancel()

		var err error
		subreddit, err = a.mongodb.GetSubredditByName(dbCtx, msg.Name)
		if err != nil {
			log.Printf("Error fetching subreddit from MongoDB: %v", err)
			ctx.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", err))
			return
		}

		if subreddit != nil {
			// Update cache
			a.subredditsByName[subreddit.Name] = subreddit
			a.subredditsById[subreddit.ID] = subreddit

			if _, exists := a.subredditMembers[subreddit.ID]; !exists {
				a.subredditMembers[subreddit.ID] = make(map[uuid.UUID]bool)
			}
		}
	}

	if subreddit == nil {
		ctx.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
		return
	}

	response := struct {
		ID          string      `json:"ID"`
		Name        string      `json:"Name"`
		Description string      `json:"Description"`
		CreatorID   string      `json:"CreatorID"`
		Members     int         `json:"Members"`
		CreatedAt   time.Time   `json:"CreatedAt"`
		Posts       []uuid.UUID `json:"Posts"`
	}{
		ID:          subreddit.ID.String(),
		Name:        subreddit.Name,
		Description: subreddit.Description,
		CreatorID:   subreddit.CreatorID.String(),
		Members:     subreddit.Members, // Use the value from the model
		CreatedAt:   subreddit.CreatedAt,
		Posts:       subreddit.Posts,
	}

	log.Printf("Successfully fetched subreddit details for name: %s", msg.Name)
	ctx.Respond(response)
}

func (a *SubredditActor) handleJoinSubreddit(ctx actor.Context, msg *JoinSubredditMsg) {
	startTime := time.Now()

	// Create single context for all DB operations
	dbCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
	defer cancel()

	// First verify subreddit exists and get latest data from MongoDB
	subredditFromDB, err := a.mongodb.GetSubredditByID(dbCtx, msg.SubredditID)
	if err != nil {
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to get subreddit", err))
		return
	}
	if subredditFromDB == nil {
		ctx.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
		return
	}

	// Update local cache with latest data
	a.subredditsById[msg.SubredditID] = subredditFromDB
	a.subredditsByName[subredditFromDB.Name] = subredditFromDB

	// Initialize member map if doesn't exist
	if _, exists := a.subredditMembers[msg.SubredditID]; !exists {
		a.subredditMembers[msg.SubredditID] = make(map[uuid.UUID]bool)
	}

	// Check if user is already a member
	if a.subredditMembers[msg.SubredditID][msg.UserID] {
		ctx.Respond(utils.NewAppError(utils.ErrDuplicate, "user is already a member", nil))
		return
	}

	// Update MongoDB subreddit members count
	err = a.mongodb.UpdateSubredditMembers(dbCtx, msg.SubredditID, 1)
	if err != nil {
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to update member count", err))
		return
	}

	// Update user's subreddits list
	err = a.mongodb.UpdateUserSubreddits(dbCtx, msg.UserID, msg.SubredditID, true)
	if err != nil {
		// Rollback the member count update
		rollbackErr := a.mongodb.UpdateSubredditMembers(dbCtx, msg.SubredditID, -1)
		if rollbackErr != nil {
			log.Printf("Error rolling back member count: %v", rollbackErr)
		}
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to update user's subreddit list", err))
		return
	}

	// Update local cache
	a.subredditMembers[msg.SubredditID][msg.UserID] = true
	subredditFromDB.Members++

	log.Printf("SubredditActor: User %s joined subreddit %s", msg.UserID, msg.SubredditID)
	a.metrics.AddOperationLatency("join_subreddit", time.Since(startTime))
	ctx.Respond(true)
}

func (a *SubredditActor) handleLeaveSubreddit(ctx actor.Context, msg *LeaveSubredditMsg) {
	startTime := time.Now()

	subreddit := a.subredditsById[msg.SubredditID]
	if subreddit == nil {
		ctx.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
		return
	}

	members := a.subredditMembers[msg.SubredditID]
	if !members[msg.UserID] {
		ctx.Respond(utils.NewAppError(utils.ErrInvalidInput, "user is not a member", nil))
		return
	}

	dbCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
	defer cancel()

	// Update MongoDB subreddit members count
	err := a.mongodb.UpdateSubredditMembers(dbCtx, msg.SubredditID, -1)
	if err != nil {
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to update member count", err))
		return
	}

	// Update user's subreddits list
	err = a.mongodb.UpdateUserSubreddits(dbCtx, msg.UserID, msg.SubredditID, false)
	if err != nil {
		log.Printf("Warning: Failed to update user's subreddit list: %v", err)
		// Rollback the member count update
		rollbackErr := a.mongodb.UpdateSubredditMembers(dbCtx, msg.SubredditID, 1)
		if rollbackErr != nil {
			log.Printf("Error rolling back member count: %v", rollbackErr)
		}
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to update user's subreddit list", err))
		return
	}

	// Update local cache
	delete(a.subredditMembers[msg.SubredditID], msg.UserID)
	subreddit.Members--

	a.metrics.AddOperationLatency("leave_subreddit", time.Since(startTime))
	ctx.Respond(true)
}

func (a *SubredditActor) handleListSubreddits(ctx actor.Context) {
	dbCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
	defer cancel()

	// Get from MongoDB and update cache
	subreddits, err := a.mongodb.ListSubreddits(dbCtx)
	if err != nil {
		// If MongoDB fails, fall back to cache
		cachedSubreddits := make([]*models.Subreddit, 0, len(a.subredditsByName))
		for _, sub := range a.subredditsByName {
			cachedSubreddits = append(cachedSubreddits, sub)
		}
		ctx.Respond(cachedSubreddits)
		return
	}

	// Update cache with MongoDB data
	for _, sub := range subreddits {
		a.subredditsByName[sub.Name] = sub
		a.subredditsById[sub.ID] = sub
	}

	ctx.Respond(subreddits)
}

func (a *SubredditActor) handleGetMembers(ctx actor.Context, msg *GetSubredditMembersMsg) {
	log.Printf("Getting members for subreddit: %s", msg.SubredditID)
	std_ctx := stdctx.Background()
	memberIDs, err := a.mongodb.GetSubredditMembers(std_ctx, msg.SubredditID)
	if err != nil {
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "Failed to get subreddit members", err))
		return
	}

	if len(memberIDs) == 0 {
		// Decide if you want to return an empty list or an error
		ctx.Respond([]string{}) // or ctx.Respond(utils.NewAppError(...))
		return
	}

	log.Printf("Found %d members for subreddit: %s", len(memberIDs), msg.SubredditID)
	ctx.Respond(memberIDs)
}
