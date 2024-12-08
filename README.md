# Reddit Clone API Documentation

## Base URL
```
http://localhost:8080
```

## Authentication
Most endpoints require authentication. After login, include the token in the Authorization header:
```
Authorization: Bearer <token>
```

## User Management

### Register User
- **POST** `/user/register`
- Creates a new user account
```json
// Request
{
  "username": "string",
  "email": "string",
  "password": "string",
  "karma": number
}

// Response
{
  "id": "uuid",
  "username": "string",
  "email": "string",
  "karma": number
}
```

### Login
- **POST** `/user/login`
- Authenticates a user
```json
// Request
{
  "email": "string",
  "password": "string"
}

// Response
{
  "success": boolean,
  "token": "string",
  "error": "string" // only if success is false
}
```

### Get User Profile
- **GET** `/user/profile?userId=<uuid>`
- Retrieves user profile information
```json
// Response
{
  "id": "uuid",
  "username": "string",
  "karma": number,
  "posts": ["uuid"],
  "comments": ["uuid"],
  "subscriptions": ["uuid"]
}
```

## Subreddits

### Create Subreddit
- **POST** `/subreddit`
- Creates a new subreddit
```json
// Request
{
  "name": "string",
  "description": "string",
  "creatorId": "uuid"
}

// Response
{
  "id": "uuid",
  "name": "string",
  "description": "string",
  "creatorId": "uuid",
  "createdAt": "timestamp",
  "members": number
}
```

### List Subreddits
- **GET** `/subreddit`
- Retrieves all subreddits
```json
// Response
[
  {
    "id": "uuid",
    "name": "string",
    "description": "string",
    "members": number,
    "createdAt": "timestamp"
  }
]
```

### Manage Subreddit Membership
- **POST** `/subreddit/members`
- Join a subreddit
```json
// Request
{
  "subredditId": "uuid",
  "userId": "uuid"
}

// Response
{
  "success": boolean
}
```

- **DELETE** `/subreddit/members`
- Leave a subreddit
```json
// Request
{
  "subredditId": "uuid",
  "userId": "uuid"
}
```

### Get Subreddit Members
- **GET** `/subreddit/members?id=<uuid>`
- Lists members of a subreddit
```json
// Response
[
  "uuid" // Array of user IDs
]
```

## Posts

### Create Post
- **POST** `/post`
- Creates a new post
```json
// Request
{
  "title": "string",
  "content": "string",
  "authorId": "uuid",
  "subredditId": "uuid"
}

// Response
{
  "id": "uuid",
  "title": "string",
  "content": "string",
  "authorId": "uuid",
  "subredditId": "uuid",
  "createdAt": "timestamp",
  "upvotes": number,
  "downvotes": number,
  "karma": number
}
```

### Get Post
- **GET** `/post?id=<uuid>`
- Retrieves a specific post
```json
// Response
{
  "id": "uuid",
  "title": "string",
  "content": "string",
  "authorId": "uuid",
  "subredditId": "uuid",
  "createdAt": "timestamp",
  "upvotes": number,
  "downvotes": number,
  "karma": number
}
```

### Get Subreddit Posts
- **GET** `/post?subredditId=<uuid>`
- Retrieves all posts in a subreddit
```json
// Response
[
  {
    "id": "uuid",
    "title": "string",
    "content": "string",
    "authorId": "uuid",
    "subredditId": "uuid",
    "createdAt": "timestamp",
    "upvotes": number,
    "downvotes": number,
    "karma": number
  }
]
```

### Vote on Post
- **POST** `/post/vote`
- Upvote or downvote a post
```json
// Request
{
  "userId": "uuid",
  "postId": "uuid",
  "isUpvote": boolean
}

// Response
{
  "id": "uuid",
  "upvotes": number,
  "downvotes": number,
  "karma": number
}
```

### Get User Feed
- **GET** `/user/feed?userId=<uuid>&limit=<number>`
- Retrieves personalized feed for a user
```json
// Response
[
  {
    "id": "uuid",
    "title": "string",
    "content": "string",
    "authorId": "uuid",
    "subredditId": "uuid",
    "createdAt": "timestamp",
    "upvotes": number,
    "downvotes": number,
    "karma": number
  }
]
```

## Comments

### Create Comment
- **POST** `/comment`
- Creates a new comment
```json
// Request
{
  "content": "string",
  "authorId": "uuid",
  "postId": "uuid",
  "parentId": "uuid" // optional, for replies
}

// Response
{
  "id": "uuid",
  "content": "string",
  "authorId": "uuid",
  "postId": "uuid",
  "parentId": "uuid",
  "createdAt": "timestamp",
  "upvotes": number,
  "downvotes": number,
  "karma": number
}
```

### Get Post Comments
- **GET** `/comment/post?postId=<uuid>`
- Retrieves all comments for a post
```json
// Response
[
  {
    "id": "uuid",
    "content": "string",
    "authorId": "uuid",
    "postId": "uuid",
    "parentId": "uuid",
    "children": ["uuid"],
    "createdAt": "timestamp",
    "upvotes": number,
    "downvotes": number,
    "karma": number
  }
]
```

### Vote on Comment
- **POST** `/comment/vote`
- Upvote or downvote a comment
```json
// Request
{
  "commentId": "uuid",
  "userId": "uuid",
  "isUpvote": boolean
}

// Response
{
  "id": "uuid",
  "upvotes": number,
  "downvotes": number,
  "karma": number
}
```

## Direct Messages

### Send Message
- **POST** `/messages`
- Sends a direct message to another user
```json
// Request
{
  "fromId": "uuid",
  "toId": "uuid",
  "content": "string"
}

// Response
{
  "id": "uuid",
  "fromId": "uuid",
  "toId": "uuid",
  "content": "string",
  "createdAt": "timestamp",
  "isRead": boolean
}
```

### Get User Messages
- **GET** `/messages?userId=<uuid>`
- Retrieves all messages for a user
```json
// Response
[
  {
    "id": "uuid",
    "fromId": "uuid",
    "toId": "uuid",
    "content": "string",
    "createdAt": "timestamp",
    "isRead": boolean
  }
]
```

### Get Conversation
- **GET** `/messages/conversation?user1=<uuid>&user2=<uuid>`
- Retrieves conversation between two users
```json
// Response
[
  {
    "id": "uuid",
    "fromId": "uuid",
    "toId": "uuid",
    "content": "string",
    "createdAt": "timestamp",
    "isRead": boolean
  }
]
```

### Mark Message as Read
- **POST** `/messages/read`
- Marks a message as read
```json
// Request
{
  "messageId": "uuid",
  "userId": "uuid"
}

// Response
{
  "success": boolean
}
```

## System Health

### Health Check
- **GET** `/health`
- Checks system health and gets basic stats
```json
// Response
{
  "status": "string",
  "subreddit_count": number,
  "post_count": number
}
```

## Error Responses
All endpoints may return error responses in the following format:
```json
{
  "code": "string",
  "message": "string",
  "origin": "string"
}
```

Common error codes:
- `NOT_FOUND`: Resource not found
- `INVALID_INPUT`: Invalid request data
- `UNAUTHORIZED`: Authentication required or insufficient permissions
- `DUPLICATE`: Resource already exists
- `ACTOR_TIMEOUT`: Internal processing timeout
