package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID             uuid.UUID   `json:"id" db:"id"`
	Username       string      `json:"username" db:"username"`
	Email          string      `json:"email" db:"email"`
	HashedPassword string      `json:"-" db:"password_hash"`
	Karma          int         `json:"karma" db:"karma"`
	CreatedAt      time.Time   `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time   `json:"updatedAt" db:"updated_at"`
	LastActive     time.Time   `json:"lastActive" db:"last_active"`
	IsConnected    bool        `json:"isConnected" db:"is_connected"`
	Subreddits     []uuid.UUID `json:"subreddits"`
}
