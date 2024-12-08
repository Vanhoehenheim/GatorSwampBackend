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

	GetSubredditMsg struct {
		Name string
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

	GetSubredditDetailsMsg struct {
		SubredditID uuid.UUID
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

	case *GetSubredditMsg:
		a.handleGetSubreddit(context, msg)

	case *JoinSubredditMsg:
		a.handleJoinSubreddit(context, msg)

	case *LeaveSubredditMsg:
		a.handleLeaveSubreddit(context, msg)

	case *ListSubredditsMsg:
		a.handleListSubreddits(context)

	case *GetSubredditMembersMsg:
		a.handleGetMembers(context, msg)

	case *GetSubredditDetailsMsg:
		a.handleGetDetails(context, msg)

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

func (a *SubredditActor) handleGetSubreddit(ctx actor.Context, msg *GetSubredditMsg) {
	if subreddit, exists := a.subredditsByName[msg.Name]; exists {
		ctx.Respond(subreddit)
	} else {
		ctx.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
	}
}

func (a *SubredditActor) handleJoinSubreddit(ctx actor.Context, msg *JoinSubredditMsg) {
	startTime := time.Now()

	subreddit, exists := a.subredditsById[msg.SubredditID]
	if !exists {
		ctx.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
		return
	}

	// Check if user is already a member
	if members, exists := a.subredditMembers[msg.SubredditID]; exists {
		if members[msg.UserID] {
			ctx.Respond(utils.NewAppError(utils.ErrDuplicate, "user is already a member", nil))
			return
		}
	}

	dbCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
	defer cancel()

	// Update MongoDB subreddit members count
	err := a.mongodb.UpdateSubredditMembers(dbCtx, msg.SubredditID, 1)
	if err != nil {
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to update member count", err))
		return
	}

	// Update user's subreddits list
	err = a.mongodb.UpdateUserSubreddits(dbCtx, msg.UserID, msg.SubredditID, true)
	if err != nil {
		log.Printf("Warning: Failed to update user's subreddit list: %v", err)
		// Rollback the member count update
		rollbackErr := a.mongodb.UpdateSubredditMembers(dbCtx, msg.SubredditID, -1)
		if rollbackErr != nil {
			log.Printf("Error rolling back member count: %v", rollbackErr)
		}
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to update user's subreddit list", err))
		return
	}

	// Update local cache
	if _, exists := a.subredditMembers[msg.SubredditID]; !exists {
		a.subredditMembers[msg.SubredditID] = make(map[uuid.UUID]bool)
	}
	a.subredditMembers[msg.SubredditID][msg.UserID] = true
	subreddit.Members++

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
	if members, exists := a.subredditMembers[msg.SubredditID]; exists {
		memberIDs := make([]uuid.UUID, 0, len(members))
		for userID := range members {
			memberIDs = append(memberIDs, userID)
		}
		ctx.Respond(memberIDs)
	} else {
		ctx.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
	}
}

func (a *SubredditActor) handleGetDetails(ctx actor.Context, msg *GetSubredditDetailsMsg) {
	var subreddit *models.Subreddit
	for _, s := range a.subredditsByName {
		if s.ID == msg.SubredditID {
			subreddit = s
			break
		}
	}

	if subreddit == nil {
		ctx.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
		return
	}

	details := struct {
		Subreddit   *models.Subreddit
		MemberCount int
	}{
		Subreddit:   subreddit,
		MemberCount: len(a.subredditMembers[msg.SubredditID]),
	}
	ctx.Respond(details)
}
