package models

import (
	"time"

	"github.com/google/uuid"
)

type Comment struct {
	ID             uuid.UUID   `json:"id"`
	Content        string      `json:"content"`
	AuthorID       uuid.UUID   `json:"authorId"`
	AuthorUsername string      `json:"authorUsername"` // Added AuthorUsername field
	PostID         uuid.UUID   `json:"postId"`
	SubredditID    uuid.UUID   `json:"subredditId"`
	ParentID       *uuid.UUID  `json:"parentId,omitempty"`
	Children       []uuid.UUID `json:"children"`
	CreatedAt      time.Time   `json:"createdAt"`
	UpdatedAt      time.Time   `json:"updatedAt"`
	IsDeleted      bool        `json:"isDeleted"`
	Upvotes        int         `json:"upvotes"`
	Downvotes      int         `json:"downvotes"`
	Karma          int         `json:"karma"`
}
