package models

import (
	"time"

	"github.com/google/uuid"
)

type Post struct {
	ID             uuid.UUID
	Title          string
	Content        string
	AuthorID       uuid.UUID
	AuthorUsername string
	SubredditID    uuid.UUID
	SubredditName  string
	CreatedAt      time.Time
	Upvotes        int
	Downvotes      int
	Karma          int // Add Karma field to track post karma
}
