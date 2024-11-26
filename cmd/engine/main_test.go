package main

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/assert"
    "gator-swamp/internal/engine/actors"
    "gator-swamp/internal/config"
    "gator-swamp/internal/utils"
)

func TestIntegrationFlow(t *testing.T) {
    // Initialize server
    cfg := config.DefaultConfig()
    metrics := utils.NewMetricsCollector()
    system := actor.NewActorSystem()
    gatorEngine := engine.NewEngine(system, metrics)

    server := &Server{
        system:  system,
        context: system.Root,
        engine:  gatorEngine,
        metrics: metrics,
    }

    // Initialize handlers
    registerHandler := server.handleUserRegistration()
    subredditHandler := server.handleSubreddits()
    postHandler := server.handlePost()
    commentHandler := server.handleComment()
    getCommentsHandler := server.handleGetPostComments()

    // Step 1: Create User 1
    user1Data := RegisterUserRequest{
        Username: "user1",
        Email:    "user1@example.com",
        Password: "password123",
        Karma:    100,
    }
    user1Bytes, _ := json.Marshal(user1Data)
    req1 := httptest.NewRequest("POST", "/user/register", bytes.NewBuffer(user1Bytes))
    req1.Header.Set("Content-Type", "application/json")
    w1 := httptest.NewRecorder()
    registerHandler.ServeHTTP(w1, req1)

    assert.Equal(t, http.StatusOK, w1.Code)
    var user1Response actors.UserState
    json.Unmarshal(w1.Body.Bytes(), &user1Response)
    user1ID := user1Response.ID
    t.Logf("User 1 created with ID: %s", user1ID)

    // Step 2: Create User 2
    user2Data := RegisterUserRequest{
        Username: "user2",
        Email:    "user2@example.com",
        Password: "password456",
        Karma:    100,
    }
    user2Bytes, _ := json.Marshal(user2Data)
    req2 := httptest.NewRequest("POST", "/user/register", bytes.NewBuffer(user2Bytes))
    req2.Header.Set("Content-Type", "application/json")
    w2 := httptest.NewRecorder()
    registerHandler.ServeHTTP(w2, req2)

    assert.Equal(t, http.StatusOK, w2.Code)
    var user2Response actors.UserState
    json.Unmarshal(w2.Body.Bytes(), &user2Response)
    user2ID := user2Response.ID
    t.Logf("User 2 created with ID: %s", user2ID)

    // Step 3: Create Subreddit
    subredditData := CreateSubredditRequest{
        Name:        "testsubreddit",
        Description: "Test Subreddit",
        CreatorID:   user1ID.String(),
    }
    subredditBytes, _ := json.Marshal(subredditData)
    req3 := httptest.NewRequest("POST", "/subreddit", bytes.NewBuffer(subredditBytes))
    req3.Header.Set("Content-Type", "application/json")
    w3 := httptest.NewRecorder()
    subredditHandler.ServeHTTP(w3, req3)

    assert.Equal(t, http.StatusOK, w3.Code)
    var subredditResponse SubredditResponse
    json.Unmarshal(w3.Body.Bytes(), &subredditResponse)
    subredditID := subredditResponse.ID
    t.Logf("Subreddit created with ID: %s", subredditID)

    // Step 4: User 1 creates a post
    postData := CreatePostRequest{
        Title:       "Test Post",
        Content:     "This is a test post",
        AuthorID:    user1ID.String(),
        SubredditID: subredditID,
    }
    postBytes, _ := json.Marshal(postData)
    req4 := httptest.NewRequest("POST", "/post", bytes.NewBuffer(postBytes))
    req4.Header.Set("Content-Type", "application/json")
    w4 := httptest.NewRecorder()
    postHandler.ServeHTTP(w4, req4)

    assert.Equal(t, http.StatusOK, w4.Code)
    var postResponse map[string]interface{}
    json.Unmarshal(w4.Body.Bytes(), &postResponse)
    postID := postResponse["id"].(string)
    t.Logf("Post created with ID: %s", postID)

    // Step 5: User 2 creates parent comment
    parentCommentData := CreateCommentRequest{
        Content:  "Parent comment by user2",
        AuthorID: user2ID.String(),
        PostID:   postID,
    }
    parentCommentBytes, _ := json.Marshal(parentCommentData)
    req5 := httptest.NewRequest("POST", "/comment", bytes.NewBuffer(parentCommentBytes))
    req5.Header.Set("Content-Type", "application/json")
    w5 := httptest.NewRecorder()
    commentHandler.ServeHTTP(w5, req5)

    assert.Equal(t, http.StatusOK, w5.Code)
    var parentCommentResponse map[string]interface{}
    json.Unmarshal(w5.Body.Bytes(), &parentCommentResponse)
    parentCommentID := parentCommentResponse["id"].(string)
    t.Logf("Parent comment created with ID: %s", parentCommentID)

    // Step 6: User 1 creates a reply
    replyData := CreateCommentRequest{
        Content:  "Reply from user1",
        AuthorID: user1ID.String(),
        PostID:   postID,
        ParentID: parentCommentID,
    }
    replyBytes, _ := json.Marshal(replyData)
    req6 := httptest.NewRequest("POST", "/comment", bytes.NewBuffer(replyBytes))
    req6.Header.Set("Content-Type", "application/json")
    w6 := httptest.NewRecorder()
    commentHandler.ServeHTTP(w6, req6)

    assert.Equal(t, http.StatusOK, w6.Code)
    var replyResponse map[string]interface{}
    json.Unmarshal(w6.Body.Bytes(), &replyResponse)
    replyID := replyResponse["id"].(string)
    t.Logf("Reply created with ID: %s", replyID)

    // Step 7: User 2 creates a nested reply
    nestedReplyData := CreateCommentRequest{
        Content:  "Nested reply from user2",
        AuthorID: user2ID.String(),
        PostID:   postID,
        ParentID: replyID,
    }
    nestedReplyBytes, _ := json.Marshal(nestedReplyData)
    req7 := httptest.NewRequest("POST", "/comment", bytes.NewBuffer(nestedReplyBytes))
    req7.Header.Set("Content-Type", "application/json")
    w7 := httptest.NewRecorder()
    commentHandler.ServeHTTP(w7, req7)

    assert.Equal(t, http.StatusOK, w7.Code)
    var nestedReplyResponse map[string]interface{}
    json.Unmarshal(w7.Body.Bytes(), &nestedReplyResponse)
    nestedReplyID := nestedReplyResponse["id"].(string)
    t.Logf("Nested reply created with ID: %s", nestedReplyID)

    // Step 8: Verify all comments exist
    req8 := httptest.NewRequest("GET", "/comment/post?postId="+postID, nil)
    w8 := httptest.NewRecorder()
    getCommentsHandler.ServeHTTP(w8, req8)

    assert.Equal(t, http.StatusOK, w8.Code)
    var commentsBeforeDelete []map[string]interface{}
    json.Unmarshal(w8.Body.Bytes(), &commentsBeforeDelete)
    assert.Equal(t, 3, len(commentsBeforeDelete), "Should have three comments before deletion")

    // Step 9: User 2 deletes their parent comment (should cascade delete all replies)
    req9 := httptest.NewRequest("DELETE", "/comment?commentId="+parentCommentID+"&authorId="+user2ID.String(), nil)
    w9 := httptest.NewRecorder()
    commentHandler.ServeHTTP(w9, req9)

    assert.Equal(t, http.StatusOK, w9.Code)
    var deleteResponse map[string]bool
    json.Unmarshal(w9.Body.Bytes(), &deleteResponse)
    assert.True(t, deleteResponse["success"], "Comment deletion should succeed")

    // Step 10: Verify all comments are deleted
    req10 := httptest.NewRequest("GET", "/comment/post?postId="+postID, nil)
    w10 := httptest.NewRecorder()
    getCommentsHandler.ServeHTTP(w10, req10)

    assert.Equal(t, http.StatusOK, w10.Code)
    var commentsAfterDelete []map[string]interface{}
    json.Unmarshal(w10.Body.Bytes(), &commentsAfterDelete)
    assert.Equal(t, 0, len(commentsAfterDelete), "All comments should be deleted")
}