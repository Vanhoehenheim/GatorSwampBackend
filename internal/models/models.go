package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID
	Username  string
	CreatedAt time.Time
	Karma     int
}

type Subreddit struct {
	ID          uuid.UUID
	Name        string
	Description string
	CreatorID   uuid.UUID
	Members     int
	CreatedAt   time.Time
}

type Post struct {
	ID          uuid.UUID
	Title       string
	Content     string
	AuthorID    uuid.UUID
	SubredditID uuid.UUID
	CreatedAt   time.Time
	Upvotes     int
	Downvotes   int
}

// Comment represents a comment on a post or another comment
type Comment struct {
	ID        uuid.UUID
	Content   string
	AuthorID  uuid.UUID
	PostID    uuid.UUID
	ParentID  *uuid.UUID // Null for top-level comments
	CreatedAt time.Time
	Upvotes   int
	Downvotes int
}

// DirectMessage represents private messages between users
type DirectMessage struct {
	ID        uuid.UUID
	FromID    uuid.UUID
	ToID      uuid.UUID
	Content   string
	CreatedAt time.Time
	IsRead    bool
}
