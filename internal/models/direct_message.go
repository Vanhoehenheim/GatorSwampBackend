package models

import (
	"time"

	"github.com/google/uuid"
)

type DirectMessage struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	FromID    uuid.UUID  `json:"fromId" db:"sender_id"`
	ToID      uuid.UUID  `json:"toId" db:"receiver_id"`
	Content   string     `json:"content" db:"content"`
	CreatedAt time.Time  `json:"createdAt" db:"created_at"`
	ReadAt    *time.Time `json:"readAt,omitempty" db:"read_at"`
	IsRead    bool       `json:"isRead"`
	IsDeleted bool       `json:"-"`
}
