package models

// VoteContentType represents the type of content being voted on.
type VoteContentType string

const (
	PostVote    VoteContentType = "post"
	CommentVote VoteContentType = "comment"
)

// VoteDirection represents the direction of a vote.
type VoteDirection string

const (
	VoteUp   VoteDirection = "up"
	VoteDown VoteDirection = "down"
	VoteNone VoteDirection = "none" // Used to indicate vote removal
)
