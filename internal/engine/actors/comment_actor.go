package actors

import (
	"gator-swamp/internal/utils"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/google/uuid"
)

// Message types for CommentActor
type (
	CreateCommentMsg struct {
		Content  string     `json:"content"`
		AuthorID uuid.UUID  `json:"authorId"`
		PostID   uuid.UUID  `json:"postId"`
		ParentID *uuid.UUID `json:"parentId,omitempty"`
	}

	EditCommentMsg struct {
		CommentID uuid.UUID `json:"commentId"`
		AuthorID  uuid.UUID `json:"authorId"`
		Content   string    `json:"content"`
	}

	DeleteCommentMsg struct {
		CommentID uuid.UUID `json:"commentId"`
		AuthorID  uuid.UUID `json:"authorId"`
	}

	GetCommentMsg struct {
		CommentID uuid.UUID `json:"commentId"`
	}

	GetCommentsForPostMsg struct {
		PostID uuid.UUID `json:"postId"`
	}

	VoteCommentMsg struct {
		CommentID uuid.UUID `json:"commentId"`
		UserID    uuid.UUID `json:"userId"`
		IsUpvote  bool      `json:"isUpvote"`
	}

	cascadeDeleteMsg struct {
		commentID uuid.UUID
	}
)

// Comment represents a single comment
type Comment struct {
	ID        uuid.UUID   `json:"id"`
	Content   string      `json:"content"`
	AuthorID  uuid.UUID   `json:"authorId"`
	PostID    uuid.UUID   `json:"postId"`
	ParentID  *uuid.UUID  `json:"parentId,omitempty"`
	Children  []uuid.UUID `json:"children"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
	IsDeleted bool        `json:"isDeleted"`
	Upvotes   int         `json:"upvotes"`
	Downvotes int         `json:"downvotes"`
	Karma     int         `json:"karma"`
}

// CommentActor manages comment operations
type CommentActor struct {
	comments     map[uuid.UUID]*Comment
	postComments map[uuid.UUID][]uuid.UUID
	commentVotes map[uuid.UUID]map[uuid.UUID]bool
	enginePID    *actor.PID
}

func NewCommentActor(enginePID *actor.PID) *CommentActor {
	return &CommentActor{
		comments:     make(map[uuid.UUID]*Comment),
		postComments: make(map[uuid.UUID][]uuid.UUID),
		commentVotes: make(map[uuid.UUID]map[uuid.UUID]bool),
		enginePID:    enginePID,
	}
}

func (a *CommentActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *CreateCommentMsg:
		commentID := uuid.New()
		now := time.Now()

		comment := &Comment{
			ID:        commentID,
			Content:   msg.Content,
			AuthorID:  msg.AuthorID,
			PostID:    msg.PostID,
			ParentID:  msg.ParentID,
			Children:  make([]uuid.UUID, 0),
			CreatedAt: now,
			UpdatedAt: now,
			Upvotes:   0,
			Downvotes: 0,
			Karma:     0,
		}

		a.comments[commentID] = comment
		a.postComments[msg.PostID] = append(a.postComments[msg.PostID], commentID)

		if msg.ParentID != nil {
			if parent, exists := a.comments[*msg.ParentID]; exists {
				parent.Children = append(parent.Children, commentID)
			}
		}

		context.Respond(comment)

	case *DeleteCommentMsg:
		if comment, exists := a.comments[msg.CommentID]; exists {
			if comment.AuthorID == msg.AuthorID {
				a.deleteCommentAndChildren(msg.CommentID)
				context.Respond(true)
			} else {
				context.Respond(false)
			}
		} else {
			context.Respond(false)
		}

	case *EditCommentMsg:
		if comment, exists := a.comments[msg.CommentID]; exists {
			if comment.AuthorID == msg.AuthorID && !comment.IsDeleted {
				comment.Content = msg.Content
				comment.UpdatedAt = time.Now()
				context.Respond(comment)
			} else {
				context.Respond(nil)
			}
		} else {
			context.Respond(nil)
		}

	case *GetCommentMsg:
		if comment, exists := a.comments[msg.CommentID]; exists {
			context.Respond(comment)
		} else {
			context.Respond(nil)
		}

	case *GetCommentsForPostMsg:
		if commentIDs, exists := a.postComments[msg.PostID]; exists {
			comments := make([]*Comment, 0)
			for _, id := range commentIDs {
				if comment, exists := a.comments[id]; exists && !comment.IsDeleted {
					comments = append(comments, comment)
				}
			}
			context.Respond(comments)
		} else {
			context.Respond([]*Comment{})
		}

	case *VoteCommentMsg:
		a.handleVote(context, msg)
	}
}

func (a *CommentActor) deleteCommentAndChildren(commentID uuid.UUID) {
	if comment, exists := a.comments[commentID]; exists {
		comment.IsDeleted = true
		comment.Content = "[deleted]"
		comment.UpdatedAt = time.Now()

		childrenToDelete := make([]uuid.UUID, len(comment.Children))
		copy(childrenToDelete, comment.Children)

		for _, childID := range childrenToDelete {
			a.deleteCommentAndChildren(childID)
		}
	}
}

func (a *CommentActor) handleVote(context actor.Context, msg *VoteCommentMsg) {
	comment, exists := a.comments[msg.CommentID]
	if !exists || comment.IsDeleted {
		context.Respond(utils.NewAppError(utils.ErrNotFound, "Comment not found", nil))
		return
	}

	if _, exists := a.commentVotes[msg.CommentID]; !exists {
		a.commentVotes[msg.CommentID] = make(map[uuid.UUID]bool)
	}

	karmaChange := 0

	if previousVote, voted := a.commentVotes[msg.CommentID][msg.UserID]; voted {
		if previousVote == msg.IsUpvote {
			context.Respond(utils.NewAppError(utils.ErrDuplicate, "Already voted", nil))
			return
		}

		if msg.IsUpvote {
			comment.Downvotes--
			comment.Upvotes++
			karmaChange = 2
		} else {
			comment.Upvotes--
			comment.Downvotes++
			karmaChange = -2
		}
	} else {
		if msg.IsUpvote {
			comment.Upvotes++
			karmaChange = 1
		} else {
			comment.Downvotes++
			karmaChange = -1
		}
	}

	a.commentVotes[msg.CommentID][msg.UserID] = msg.IsUpvote
	comment.Karma = comment.Upvotes - comment.Downvotes

	if karmaChange != 0 {
		context.Send(a.enginePID, &UpdateKarmaMsg{
			UserID: comment.AuthorID,
			Delta:  karmaChange,
		})
	}

	context.Respond(comment)
}
