package models

import (
	"time"

	"github.com/google/uuid"
)

type Subreddit struct {
	ID          uuid.UUID   `json:"id" db:"id"`
	Name        string      `json:"name" db:"name"`
	Description string      `json:"description" db:"description"`
	CreatorID   uuid.UUID   `json:"creatorId" db:"created_by"`
	Members     int         `json:"members" db:"member_count"`
	CreatedAt   time.Time   `json:"createdAt" db:"created_at"`
	Posts       []uuid.UUID `json:"posts"`
}
