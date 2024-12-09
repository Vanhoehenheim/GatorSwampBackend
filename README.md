# Complete API Testing Guide for Gator-Swamp

## Pre-existing Test Users
```
User 1 (Alice): "dbc3f210-8416-4fab-b3cb-1e6ac1589cdd"
User 2 (Bob): "ddf5e4af-2d12-4715-96fa-d1e48610a506"
```

## 1. User Management

### 1.1 User Registration
```json
POST http://localhost:8080/user/register
{
    "username": "testuser",
    "email": "test@example.com",
    "password": "password123",
    "karma": 100
}
```

### 1.2 User Login
```json
POST http://localhost:8080/user/login
{
    "email": "test@example.com",
    "password": "password123"
}
```

### 1.3 Get User Profile
```
GET http://localhost:8080/user/profile?userId=dbc3f210-8416-4fab-b3cb-1e6ac1589cdd
```

## 2. Subreddit Operations

### 2.1 Create Subreddit
```json
POST http://localhost:8080/subreddit
{
    "name": "testsubreddit",
    "description": "A test subreddit for API testing",
    "creatorId": "dbc3f210-8416-4fab-b3cb-1e6ac1589cdd"
}
```

### 2.2 List All Subreddits
```
GET http://localhost:8080/subreddit
```

### 2.3 Get Subreddit Members
```
GET http://localhost:8080/subreddit/members?id=23eadaec-66fd-4e6b-a2cb-02a97fd25c3f
```

### 2.4 Join Subreddit
```json
POST http://localhost:8080/subreddit/members
{
    "subredditId": "23eadaec-66fd-4e6b-a2cb-02a97fd25c3f",
    "userId": "ddf5e4af-2d12-4715-96fa-d1e48610a506"
}
```

### 2.5 Leave Subreddit
```json
DELETE http://localhost:8080/subreddit/members
{
    "subredditId": "23eadaec-66fd-4e6b-a2cb-02a97fd25c3f",
    "userId": "ddf5e4af-2d12-4715-96fa-d1e48610a506"
}
```

## 3. Post Operations

### 3.1 Create Post
```json
POST http://localhost:8080/post
{
    "title": "Test Post Title",
    "content": "This is test post content",
    "authorId": "dbc3f210-8416-4fab-b3cb-1e6ac1589cdd",
    "subredditId": "23eadaec-66fd-4e6b-a2cb-02a97fd25c3f"
}
```

### 3.2 Get Post by ID
```
GET http://localhost:8080/post?id=34894994-2524-4080-b52f-b1275fe67965
```

### 3.3 Get Posts by Subreddit
```
GET http://localhost:8080/post?subredditId=23eadaec-66fd-4e6b-a2cb-02a97fd25c3f
```

### 3.4 Vote on Post
```json
POST http://localhost:8080/post/vote
{
    "postId": "34894994-2524-4080-b52f-b1275fe67965",
    "userId": "ddf5e4af-2d12-4715-96fa-d1e48610a506",
    "isUpvote": true
}
```

### 3.5 Get User Feed
```
GET http://localhost:8080/user/feed?userId=dbc3f210-8416-4fab-b3cb-1e6ac1589cdd&limit=10
```

## 4. Comment Operations

### 4.1 Create Top-level Comment
```json
POST http://localhost:8080/comment
{
    "content": "This is a top-level comment",
    "authorId": "dbc3f210-8416-4fab-b3cb-1e6ac1589cdd",
    "postId": "34894994-2524-4080-b52f-b1275fe67965",
    "subredditId": "23eadaec-66fd-4e6b-a2cb-02a97fd25c3f"
}
```

### 4.2 Create Reply Comment
```json
POST http://localhost:8080/comment
{
    "content": "This is a reply to another comment",
    "authorId": "ddf5e4af-2d12-4715-96fa-d1e48610a506",
    "postId": "34894994-2524-4080-b52f-b1275fe67965",
    "subredditId": "23eadaec-66fd-4e6b-a2cb-02a97fd25c3f",
    "parentId": "b2993f49-125e-480e-87e3-bbe5a3ab5f73"
}
```

### 4.3 Edit Comment
```json
PUT http://localhost:8080/comment
{
    "commentId": "b2993f49-125e-480e-87e3-bbe5a3ab5f73",
    "authorId": "dbc3f210-8416-4fab-b3cb-1e6ac1589cdd",
    "content": "This is an edited comment"
}
```

### 4.4 Vote on Comment
```json
POST http://localhost:8080/comment/vote
{
    "commentId": "b2993f49-125e-480e-87e3-bbe5a3ab5f73",
    "userId": "ddf5e4af-2d12-4715-96fa-d1e48610a506",
    "isUpvote": true
}
```

### 4.5 Get Comments for Post
```
GET http://localhost:8080/comment/post?postId=34894994-2524-4080-b52f-b1275fe67965
```

### 4.6 Delete Comment
```
DELETE http://localhost:8080/comment?commentId=b2993f49-125e-480e-87e3-bbe5a3ab5f73&authorId=dbc3f210-8416-4fab-b3cb-1e6ac1589cdd
```

## 5. Direct Message Operations

### 5.1 Send Message
```json
POST http://localhost:8080/messages
{
    "fromId": "dbc3f210-8416-4fab-b3cb-1e6ac1589cdd",
    "toId": "ddf5e4af-2d12-4715-96fa-d1e48610a506",
    "content": "Hello! This is a test message."
}
```

### 5.2 Get User's Messages
```
GET http://localhost:8080/messages?userId=dbc3f210-8416-4fab-b3cb-1e6ac1589cdd
```

### 5.3 Get Conversation Between Users
```
GET http://localhost:8080/messages/conversation?user1=dbc3f210-8416-4fab-b3cb-1e6ac1589cdd&user2=ddf5e4af-2d12-4715-96fa-d1e48610a506
```

### 5.4 Mark Message as Read
```json
POST http://localhost:8080/messages/read
{
    "messageId": "MESSAGE_ID_FROM_PREVIOUS_RESPONSE",
    "userId": "ddf5e4af-2d12-4715-96fa-d1e48610a506"
}
```

### 5.5 Delete Message
```
DELETE http://localhost:8080/messages?messageId=MESSAGE_ID_HERE&userId=dbc3f210-8416-4fab-b3cb-1e6ac1589cdd
```

## 6. Health Check
```
GET http://localhost:8080/health
```

## Testing Scenarios

### User Flow
1. Register new user
2. Login with user
3. Get user profile
4. Create a subreddit
5. Join another subreddit
6. Create posts
7. Vote on posts
8. Check user feed

### Subreddit Flow
1. Create subreddit
2. List all subreddits
3. Get members
4. Have users join
5. Have users leave
6. Check member count updates

### Post Flow
1. Create post in subreddit
2. Get post by ID
3. Get all posts in subreddit
4. Vote on post
5. Check karma updates
6. Check user feed updates

### Comment Flow
1. Create top-level comment
2. Create reply to comment
3. Create nested reply
4. Edit a comment
5. Vote on comments
6. Delete a comment
7. Check nested comment structure

### Message Flow
1. Send message between users
2. Get user's messages
3. View conversation
4. Mark as read
5. Delete message
6. Verify message states

### Error Cases
1. Try to create duplicate subreddit
2. Try to vote twice on same post
3. Try to edit another user's comment
4. Try to access unauthorized content
5. Try to send message to non-existent user
