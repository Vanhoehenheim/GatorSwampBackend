package handlers

import (
	"gator-swamp/internal/database"
	"gator-swamp/internal/engine"
	"gator-swamp/internal/utils"
	"gator-swamp/internal/websocket"
	"time"

	"github.com/asynkron/protoactor-go/actor"
)

// Server holds all server dependencies, including the actor system and engine
type Server struct {
	System             *actor.ActorSystem
	Context            *actor.RootContext
	Engine             *engine.Engine
	EnginePID          *actor.PID
	Metrics            *utils.MetricsCollector
	CommentActor       *actor.PID
	DirectMessageActor *actor.PID
	DB                 database.DBAdapter
	RequestTimeout     time.Duration
	Hub                *websocket.Hub
	PostActor          *actor.PID
	SubredditActor     *actor.PID
	UserSupervisor     *actor.PID
}

// NewServer creates a new Server instance with the given components
func NewServer(
	system *actor.ActorSystem,
	context *actor.RootContext,
	engine *engine.Engine,
	enginePID *actor.PID,
	metrics *utils.MetricsCollector,
	commentActor *actor.PID,
	directMessageActor *actor.PID,
	db database.DBAdapter,
	hub *websocket.Hub,
	postActor *actor.PID,
	subredditActor *actor.PID,
	userSupervisor *actor.PID,
	timeout time.Duration,
) *Server {
	return &Server{
		System:             system,
		Context:            context,
		Engine:             engine,
		EnginePID:          enginePID,
		Metrics:            metrics,
		CommentActor:       commentActor,
		DirectMessageActor: directMessageActor,
		DB:                 db,
		RequestTimeout:     timeout,
		Hub:                hub,
		PostActor:          postActor,
		SubredditActor:     subredditActor,
		UserSupervisor:     userSupervisor,
	}
}
