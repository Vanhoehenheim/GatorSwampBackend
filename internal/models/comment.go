package models

import (
	"time"

	"github.com/google/uuid"
)

type Comment struct {
	ID              uuid.UUID   `json:"id" db:"id"`
	Content         string      `json:"content" db:"content"`
	AuthorID        uuid.UUID   `json:"authorId" db:"author_id"`
	AuthorUsername  string      `json:"authorUsername" db:"author_username"`
	PostID          uuid.UUID   `json:"postId" db:"post_id"`
	SubredditID     uuid.UUID   `json:"subredditId" db:"subreddit_id"`
	ParentID        *uuid.UUID  `json:"parentId,omitempty" db:"parent_id"`
	Children        []uuid.UUID `json:"children"` // Not in comments table
	CreatedAt       time.Time   `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time   `json:"updatedAt" db:"updated_at"`
	IsDeleted       bool        `json:"isDeleted"`                // Not in comments table
	Upvotes         int         `json:"upvotes" db:"upvotes"`     // Added db tag
	Downvotes       int         `json:"downvotes" db:"downvotes"` // Added db tag
	Karma           int         `json:"karma" db:"karma"`
	CurrentUserVote *string     `json:"currentUserVote,omitempty" db:"current_user_vote"`
}
