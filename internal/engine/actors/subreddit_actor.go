package actors

import (
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
func (a *SubredditActor) handleCreateSubreddit(context actor.Context, msg *CreateSubredditMsg) {
	log.Printf("SubredditActor: Creating subreddit: %s", msg.Name)
	startTime := time.Now()

	if _, exists := a.subredditsByName[msg.Name]; exists {
		context.Respond(utils.NewAppError(utils.ErrDuplicate, "subreddit already exists", nil))
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

	// Store in both maps
	a.subredditsByName[msg.Name] = newSubreddit
	a.subredditsById[newSubreddit.ID] = newSubreddit
	a.subredditMembers[newSubreddit.ID] = map[uuid.UUID]bool{
		msg.CreatorID: true,
	}

	a.metrics.AddOperationLatency("create_subreddit", time.Since(startTime))
	log.Printf("SubredditActor: Successfully created subreddit: %s", newSubreddit.ID)
	context.Respond(newSubreddit)
}

func (a *SubredditActor) handleGetSubreddit(context actor.Context, msg *GetSubredditMsg) {
	if subreddit, exists := a.subredditsByName[msg.Name]; exists {
		context.Respond(subreddit)
	} else {
		context.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
	}
}

func (a *SubredditActor) handleJoinSubreddit(context actor.Context, msg *JoinSubredditMsg) {
	startTime := time.Now()

	subreddit, exists := a.subredditsById[msg.SubredditID]
	if !exists {
		context.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
		return
	}

	if _, exists := a.subredditMembers[msg.SubredditID]; !exists {
		a.subredditMembers[msg.SubredditID] = make(map[uuid.UUID]bool)
	}
	a.subredditMembers[msg.SubredditID][msg.UserID] = true
	subreddit.Members++

	log.Printf("SubredditActor: User %s joined subreddit %s", msg.UserID, msg.SubredditID)
	a.metrics.AddOperationLatency("join_subreddit", time.Since(startTime))
	context.Respond(true)
}
func (a *SubredditActor) handleLeaveSubreddit(context actor.Context, msg *LeaveSubredditMsg) {
	startTime := time.Now()

	var subreddit *models.Subreddit
	for _, s := range a.subredditsByName {
		if s.ID == msg.SubredditID {
			subreddit = s
			break
		}
	}

	if subreddit == nil {
		context.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
		return
	}

	members := a.subredditMembers[msg.SubredditID]
	if !members[msg.UserID] {
		context.Respond(utils.NewAppError(utils.ErrInvalidInput, "user is not a member", nil))
		return
	}

	delete(a.subredditMembers[msg.SubredditID], msg.UserID)
	subreddit.Members--

	a.metrics.AddOperationLatency("leave_subreddit", time.Since(startTime))
	context.Respond(true)
}

func (a *SubredditActor) handleListSubreddits(context actor.Context) {
	subreddits := make([]*models.Subreddit, 0, len(a.subredditsByName))
	for _, sub := range a.subredditsByName {
		subreddits = append(subreddits, sub)
	}
	context.Respond(subreddits)
}

func (a *SubredditActor) handleGetMembers(context actor.Context, msg *GetSubredditMembersMsg) {
	if members, exists := a.subredditMembers[msg.SubredditID]; exists {
		memberIDs := make([]uuid.UUID, 0, len(members))
		for userID := range members {
			memberIDs = append(memberIDs, userID)
		}
		context.Respond(memberIDs)
	} else {
		context.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
	}
}

func (a *SubredditActor) handleGetDetails(context actor.Context, msg *GetSubredditDetailsMsg) {
	var subreddit *models.Subreddit
	for _, s := range a.subredditsByName {
		if s.ID == msg.SubredditID {
			subreddit = s
			break
		}
	}

	if subreddit == nil {
		context.Respond(utils.NewAppError(utils.ErrNotFound, "subreddit not found", nil))
		return
	}

	details := struct {
		Subreddit   *models.Subreddit
		MemberCount int
	}{
		Subreddit:   subreddit,
		MemberCount: len(a.subredditMembers[msg.SubredditID]),
	}
	context.Respond(details)
}
