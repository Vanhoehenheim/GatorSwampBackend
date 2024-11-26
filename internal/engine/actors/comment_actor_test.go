package actors

import (
    "testing"
    "time"
    "github.com/asynkron/protoactor-go/actor"
    "github.com/google/uuid"
    "github.com/stretchr/testify/assert"
)

func TestCommentActor(t *testing.T) {
    system := actor.NewActorSystem()
    props := actor.PropsFromProducer(func() actor.Actor {
        return NewCommentActor()
    })
    
    pid := system.Root.Spawn(props)

    // Test data
    authorID := uuid.New()
    postID := uuid.New()
    
    // Test creating a comment
    createMsg := &CreateCommentMsg{
        Content:  "Test comment",
        AuthorID: authorID,
        PostID:   postID,
    }
    
    future := system.Root.RequestFuture(pid, createMsg, 5*time.Second)
    result, err := future.Result()
    assert.NoError(t, err)
    
    comment := result.(*Comment)
    assert.Equal(t, "Test comment", comment.Content)
    assert.Equal(t, authorID, comment.AuthorID)
    
    // Test editing a comment
    editMsg := &EditCommentMsg{
        CommentID: comment.ID,
        AuthorID:  authorID,
        Content:   "Updated comment",
    }
    
    future = system.Root.RequestFuture(pid, editMsg, 5*time.Second)
    result, err = future.Result()
    assert.NoError(t, err)
    
    updatedComment := result.(*Comment)
    assert.Equal(t, "Updated comment", updatedComment.Content)
    
    // Test nested comments
    replyMsg := &CreateCommentMsg{
        Content:   "Reply comment",
        AuthorID:  authorID,
        PostID:    postID,
        ParentID:  &comment.ID,
    }
    
    future = system.Root.RequestFuture(pid, replyMsg, 5*time.Second)
    result, err = future.Result()
    assert.NoError(t, err)
    
    reply := result.(*Comment)
    assert.Equal(t, comment.ID, *reply.ParentID)
    
    // Test getting comments for a post
    getMsg := &GetCommentsForPostMsg{
        PostID: postID,
    }
    
    future = system.Root.RequestFuture(pid, getMsg, 5*time.Second)
    result, err = future.Result()
    assert.NoError(t, err)
    
    comments := result.([]*Comment)
    assert.Equal(t, 2, len(comments))
}