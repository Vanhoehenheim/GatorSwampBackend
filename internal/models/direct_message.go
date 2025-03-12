package models

import (
	"time"

	"github.com/google/uuid"
)

type DirectMessage struct {
	ID        uuid.UUID
	FromID    uuid.UUID
	ToID      uuid.UUID
	Content   string
	CreatedAt time.Time
	IsRead    bool
	IsDeleted bool
}
