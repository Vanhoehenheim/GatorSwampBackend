package models

import (
	"time"

	"github.com/google/uuid"
)

type Post struct {
	ID              uuid.UUID `json:"id" db:"id"`
	Title           string    `json:"title" db:"title"`
	Content         string    `json:"content" db:"content"`
	AuthorID        uuid.UUID `json:"authorId" db:"author_id"`
	AuthorUsername  string    `json:"authorUsername" db:"author_username"` // Added db tag
	SubredditID     uuid.UUID `json:"subredditId" db:"subreddit_id"`
	SubredditName   string    `json:"subredditName" db:"subreddit_name"` // Added db tag
	CreatedAt       time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt       time.Time `json:"updatedAt" db:"updated_at"` // Added field
	Upvotes         int       `json:"upvotes" db:"upvotes"`      // Added db tag
	Downvotes       int       `json:"downvotes" db:"downvotes"`  // Added db tag
	Karma           int       `json:"karma" db:"karma"`
	CurrentUserVote *string   `json:"currentUserVote,omitempty" db:"current_user_vote"` // Added field for user's vote status (string: "up", "down", or nil)
	// UserVotes      map[string]bool `json:"userVotes"` // Removed; now handled by RecordVote and potentially a separate query
	CommentCount int `json:"commentCount" db:"comment_count"`
}
