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
	db               database.DBAdapter
}

func NewSubredditActor(metrics *utils.MetricsCollector, db database.DBAdapter) actor.Actor {
	return &SubredditActor{
		subredditsByName: make(map[string]*models.Subreddit),
		subredditsById:   make(map[uuid.UUID]*models.Subreddit),
		subredditMembers: make(map[uuid.UUID]map[uuid.UUID]bool),
		metrics:          metrics,
		db:               db,
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

	// Create a new context for DB operations
	dbCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
	defer cancel()

	// Create the subreddit in DB
	err := a.db.CreateSubreddit(dbCtx, newSubreddit)
	if err != nil {
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to create subreddit", err))
		return
	}

	// Update the creator's subreddits list
	err = a.db.UpdateUserSubreddits(dbCtx, msg.CreatorID, newSubreddit.ID, true)
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

	// If not in cache, try DB
	if subreddit == nil {
		dbCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
		defer cancel()

		var err error
		subreddit, err = a.db.GetSubredditByID(dbCtx, msg.SubredditID)
		if err != nil {
			log.Printf("Error fetching subreddit from DB: %v", err)
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

	// Use actual member count from DB
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

	// If not in cache, try DB
	if subreddit == nil {
		dbCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
		defer cancel()

		var err error
		subreddit, err = a.db.GetSubredditByName(dbCtx, msg.Name)
		if err != nil {
			log.Printf("Error fetching subreddit from DB: %v", err)
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
	log.Printf("User %s joining subreddit %s", msg.UserID, msg.SubredditID)
	startTime := time.Now()

	subreddit, exists := a.subredditsById[msg.SubredditID]
	if !exists {
		ctx.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
		return
	}

	if a.subredditMembers[msg.SubredditID] == nil {
		a.subredditMembers[msg.SubredditID] = make(map[uuid.UUID]bool)
	}

	if a.subredditMembers[msg.SubredditID][msg.UserID] {
		ctx.Respond(utils.NewAppError(utils.ErrDuplicate, "user already a member", nil))
		return
	}

	dbCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
	defer cancel()

	// Update member count and user's list in DB
	err := a.db.UpdateSubredditMemberCount(dbCtx, msg.SubredditID, 1)
	if err != nil {
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to update member count", err))
		return
	}
	err = a.db.UpdateUserSubreddits(dbCtx, msg.UserID, msg.SubredditID, true)
	if err != nil {
		// Attempt to rollback member count update - best effort
		_ = a.db.UpdateSubredditMemberCount(dbCtx, msg.SubredditID, -1)
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to update user's subreddit list", err))
		return
	}

	// Update cache
	a.subredditMembers[msg.SubredditID][msg.UserID] = true
	subreddit.Members++
	a.metrics.AddOperationLatency("join_subreddit", time.Since(startTime))
	log.Printf("User %s successfully joined subreddit %s", msg.UserID, msg.SubredditID)
	ctx.Respond(true)
}

func (a *SubredditActor) handleLeaveSubreddit(ctx actor.Context, msg *LeaveSubredditMsg) {
	log.Printf("User %s leaving subreddit %s", msg.UserID, msg.SubredditID)
	startTime := time.Now()

	subreddit, exists := a.subredditsById[msg.SubredditID]
	if !exists {
		ctx.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
		return
	}

	if a.subredditMembers[msg.SubredditID] == nil || !a.subredditMembers[msg.SubredditID][msg.UserID] {
		ctx.Respond(utils.NewAppError(utils.ErrNotFound, "user not a member", nil))
		return
	}

	dbCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
	defer cancel()

	// Update member count and user's list in DB
	err := a.db.UpdateSubredditMemberCount(dbCtx, msg.SubredditID, -1)
	if err != nil {
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to update member count", err))
		return
	}
	err = a.db.UpdateUserSubreddits(dbCtx, msg.UserID, msg.SubredditID, false)
	if err != nil {
		// Attempt to rollback member count update - best effort
		_ = a.db.UpdateSubredditMemberCount(dbCtx, msg.SubredditID, 1)
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to update user's subreddit list", err))
		return
	}

	// Update cache
	delete(a.subredditMembers[msg.SubredditID], msg.UserID)
	subreddit.Members--
	a.metrics.AddOperationLatency("leave_subreddit", time.Since(startTime))
	log.Printf("User %s successfully left subreddit %s", msg.UserID, msg.SubredditID)
	ctx.Respond(true)
}

func (a *SubredditActor) handleListSubreddits(ctx actor.Context) {
	log.Println("SubredditActor: Listing all subreddits")
	dbCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 10*time.Second)
	defer cancel()

	// TODO: Add GetAllSubreddits to DBAdapter interface
	subreddits, err := a.db.GetAllSubreddits(dbCtx)
	if err != nil {
		log.Printf("Error fetching subreddits from DB: %v", err)
		ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to fetch subreddits", err))
		return
	}

	log.Printf("SubredditActor: Found %d subreddits", len(subreddits))
	ctx.Respond(subreddits)
}

func (a *SubredditActor) handleGetMembers(ctx actor.Context, msg *GetSubredditMembersMsg) {
	log.Printf("SubredditActor: Getting members for subreddit: %s", msg.SubredditID)
	startTime := time.Now()

	// Always fetch from DB for now to ensure freshness, bypassing cache check.
	log.Printf("SubredditActor: Fetching members from DB for %s.", msg.SubredditID)
	dbCtx, cancel := stdctx.WithTimeout(stdctx.Background(), 5*time.Second)
	defer cancel()

	memberIDs, err := a.db.GetSubredditMemberIDs(dbCtx, msg.SubredditID)
	if err != nil {
		log.Printf("SubredditActor: Error fetching members from DB: %v", err)
		// Check if it's a specific AppError or just a general DB error
		if appErr, ok := err.(*utils.AppError); ok {
			ctx.Respond(appErr) // Respond with the specific AppError
		} else {
			ctx.Respond(utils.NewAppError(utils.ErrDatabase, "failed to fetch members", err))
		}
		return
	}

	// Update cache (optional, but good practice if we reintroduce caching later)
	if _, exists := a.subredditMembers[msg.SubredditID]; !exists {
		a.subredditMembers[msg.SubredditID] = make(map[uuid.UUID]bool)
	}
	// Clear existing cache entries for this subreddit before adding new ones
	// This handles cases where members might have left
	for k := range a.subredditMembers[msg.SubredditID] {
		delete(a.subredditMembers[msg.SubredditID], k)
	}
	for _, id := range memberIDs {
		a.subredditMembers[msg.SubredditID][id] = true
	}

	a.metrics.AddOperationLatency("get_subreddit_members_db_fetch", time.Since(startTime))
	log.Printf("SubredditActor: Successfully fetched %d members from DB for %s", len(memberIDs), msg.SubredditID)
	ctx.Respond(memberIDs) // Respond with fetched members
}
