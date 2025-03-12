package models

import (
	"time"

	"github.com/google/uuid"
)

type Subreddit struct {
	ID          uuid.UUID
	Name        string
	Description string
	CreatorID   uuid.UUID
	Members     int
	CreatedAt   time.Time
	Posts       []uuid.UUID
}
