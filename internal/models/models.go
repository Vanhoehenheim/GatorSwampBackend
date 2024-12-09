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

type Subreddit struct {
	ID          uuid.UUID
	Name        string
	Description string
	CreatorID   uuid.UUID
	Members     int
	CreatedAt   time.Time
	Posts       []uuid.UUID
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
	Karma       int // Add Karma field to track post karma
}

// Comment represents a comment on a post or another comment
type Comment struct {
	ID          uuid.UUID   `json:"id"`
	Content     string      `json:"content"`
	AuthorID    uuid.UUID   `json:"authorId"`
	PostID      uuid.UUID   `json:"postId"`
	SubredditID uuid.UUID   `json:"subredditId"`
	ParentID    *uuid.UUID  `json:"parentId,omitempty"`
	Children    []uuid.UUID `json:"children"`
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
	IsDeleted   bool        `json:"isDeleted"`
	Upvotes     int         `json:"upvotes"`
	Downvotes   int         `json:"downvotes"`
	Karma       int         `json:"karma"`
}

// DirectMessage represents private messages between users
type DirectMessage struct {
	ID        uuid.UUID
	FromID    uuid.UUID
	ToID      uuid.UUID
	Content   string
	CreatedAt time.Time
	IsRead    bool
	IsDeleted bool
}
