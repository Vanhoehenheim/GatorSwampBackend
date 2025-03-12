package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID             uuid.UUID   `json:"id"`
	Username       string      `json:"username"`
	Email          string      `json:"email"`
	HashedPassword string      `json:"-"` // Won't be included in JSON responses
	Karma          int         `json:"karma"`
	CreatedAt      time.Time   `json:"createdAt"`
	LastActive     time.Time   `json:"lastActive"`
	IsConnected    bool        `json:"isConnected"`
	Subreddits     []uuid.UUID `json:"subreddits" bson:"subreddits"`
}
