package actors

import (
    "github.com/asynkron/protoactor-go/actor"
    "github.com/google/uuid"
    "time"
)

// Message types for CommentActor
type (
    CreateCommentMsg struct {
        Content   string    `json:"content"`
        AuthorID  uuid.UUID `json:"authorId"`
        PostID    uuid.UUID `json:"postId"`
        ParentID  *uuid.UUID `json:"parentId,omitempty"` // Optional, for nested comments
    }

    EditCommentMsg struct {
        CommentID uuid.UUID `json:"commentId"`
        AuthorID  uuid.UUID `json:"authorId"` // To verify ownership
        Content   string    `json:"content"`
    }

    DeleteCommentMsg struct {
        CommentID uuid.UUID `json:"commentId"`
        AuthorID  uuid.UUID `json:"authorId"` // To verify ownership
    }

	cascadeDeleteMsg struct {
        commentID uuid.UUID
    }

    GetCommentMsg struct {
        CommentID uuid.UUID `json:"commentId"`
    }

    GetCommentsForPostMsg struct {
        PostID uuid.UUID `json:"postId"`
    }
)

// Comment represents a single comment
type Comment struct {
    ID        uuid.UUID  `json:"id"`
    Content   string     `json:"content"`
    AuthorID  uuid.UUID  `json:"authorId"`
    PostID    uuid.UUID  `json:"postId"`
    ParentID  *uuid.UUID `json:"parentId,omitempty"`
    Children  []uuid.UUID `json:"children"`
    CreatedAt time.Time   `json:"createdAt"`
    UpdatedAt time.Time   `json:"updatedAt"`
    IsDeleted bool        `json:"isDeleted"`
}

// CommentActor manages comment operations
type CommentActor struct {
    comments map[uuid.UUID]*Comment
    postComments map[uuid.UUID][]uuid.UUID // Maps PostID to comment IDs
}

func NewCommentActor() *CommentActor {
    return &CommentActor{
        comments: make(map[uuid.UUID]*Comment),
        postComments: make(map[uuid.UUID][]uuid.UUID),
    }
}

func (a *CommentActor) deleteCommentAndChildren(commentID uuid.UUID) {
    if comment, exists := a.comments[commentID]; exists {
        // Mark this comment as deleted
        comment.IsDeleted = true
        comment.Content = "[deleted]"
        comment.UpdatedAt = time.Now()

        // Store original children before recursion
        childrenToDelete := make([]uuid.UUID, len(comment.Children))
        copy(childrenToDelete, comment.Children)

        // Recursively delete all child comments
        for _, childID := range childrenToDelete {
            a.deleteCommentAndChildren(childID)
        }
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
        }
        
        a.comments[commentID] = comment
        
        // Add to post's comments
        a.postComments[msg.PostID] = append(a.postComments[msg.PostID], commentID)
        
        // If this is a reply, add it to parent's children
        if msg.ParentID != nil {
            if parent, exists := a.comments[*msg.ParentID]; exists {
                parent.Children = append(parent.Children, commentID)
            }
        }
        
        context.Respond(comment)

    case *DeleteCommentMsg:
        if comment, exists := a.comments[msg.CommentID]; exists {
            if comment.AuthorID == msg.AuthorID {
                // Use cascade deletion instead of simple deletion
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
    }
}