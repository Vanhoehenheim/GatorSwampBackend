package handlers

import (
	"gator-swamp/internal/database"
	"gator-swamp/internal/engine"
	"gator-swamp/internal/utils"
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
	MongoDB            *database.MongoDB
	RequestTimeout     time.Duration
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
	mongodb *database.MongoDB,
) *Server {
	return &Server{
		System:             system,
		Context:            context,
		Engine:             engine,
		EnginePID:          enginePID,
		Metrics:            metrics,
		CommentActor:       commentActor,
		DirectMessageActor: directMessageActor,
		MongoDB:            mongodb,
		RequestTimeout:     5 * time.Second, // Default timeout for actor requests
	}
}
