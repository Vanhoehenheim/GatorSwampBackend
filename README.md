# Gator Swamp API Documentation

This document provides information about the Gator Swamp RESTful API endpoints, request formats, and response structures.

## Base URL

For local development: `http://localhost:8080`

## Authentication

Most endpoints require authentication using JSON Web Tokens (JWT). To authenticate requests, include an `Authorization` header with a Bearer token:

```
Authorization: Bearer <your_jwt_token>
```

You can obtain a JWT token by logging in through the `/user/login` endpoint.

## Public Endpoints

### Health Check

**Endpoint:** `GET /health`

Checks the health status of the API.

**Response:**
```json
{
  "status": "healthy",
  "subreddit_count": 10,
  "post_count": 45,
  "server_time": "2023-04-01T12:34:56Z"
}
```

### User Registration

**Endpoint:** `POST /user/register`

Registers a new user.

**Request Body:**
```json
{
  "username": "gator_user",
  "email": "user@example.com",
  "password": "secure_password",
  "karma": 0
}
```

**Response:**
```json
{
  "id": "uuid-string",
  "username": "gator_user",
  "email": "user@example.com",
  "karma": 0,
  "createdAt": "2023-04-01T12:34:56Z"
}
```

### User Login

**Endpoint:** `POST /user/login`

Authenticates a user and returns a JWT token.

**Request Body:**
```json
{
  "email": "user@example.com",
  "password": "secure_password"
}
```

**Response:**
```json
{
  "success": true,
  "token": "jwt_token_string",
  "userId": "uuid-string"
}
```

## Protected Endpoints

### Subreddits

#### List All Subreddits

**Endpoint:** `GET /subreddit`

Lists all available subreddits.

**Response:**
```json
[
  {
    "id": "uuid-string",
    "name": "gatortech",
    "description": "Tech discussions for gators",
    "createdAt": "2023-04-01T12:34:56Z",
    "creatorId": "uuid-string",
    "members": 150
  },
  // More subreddits...
]
```

#### Get Subreddit by ID

**Endpoint:** `GET /subreddit?id=<subreddit_id>`

Retrieves a specific subreddit by ID.

**Response:**
```json
{
  "id": "uuid-string",
  "name": "gatortech",
  "description": "Tech discussions for gators",
  "createdAt": "2023-04-01T12:34:56Z",
  "creatorId": "uuid-string",
  "members": 150
}
```

#### Get Subreddit by Name

**Endpoint:** `GET /subreddit?name=<subreddit_name>`

Retrieves a specific subreddit by name.

**Response:**
```json
{
  "id": "uuid-string",
  "name": "gatortech",
  "description": "Tech discussions for gators",
  "createdAt": "2023-04-01T12:34:56Z",
  "creatorId": "uuid-string",
  "members": 150
}
```

#### Create Subreddit

**Endpoint:** `POST /subreddit`

Creates a new subreddit.

**Request Body:**
```json
{
  "name": "newsubreddit",
  "description": "A new subreddit for discussions",
  "creatorId": "uuid-string"
}
```

**Response:**
```json
{
  "id": "uuid-string",
  "name": "newsubreddit",
  "description": "A new subreddit for discussions",
  "createdAt": "2023-04-01T12:34:56Z",
  "creatorId": "uuid-string",
  "members": 1
}
```

### Subreddit Membership

#### Get Subreddit Members

**Endpoint:** `GET /subreddit/members?id=<subreddit_id>`

Gets all members of a subreddit.

**Response:**
```json
[
  {
    "id": "uuid-string",
    "username": "user1",
    "joinedAt": "2023-04-01T12:34:56Z"
  },
  // More users...
]
```

#### Join Subreddit

**Endpoint:** `POST /subreddit/members`

Adds a user to a subreddit.

**Request Body:**
```json
{
  "subredditId": "uuid-string",
  "userId": "uuid-string"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Successfully joined subreddit"
}
```

#### Leave Subreddit

**Endpoint:** `DELETE /subreddit/members`

Removes a user from a subreddit.

**Request Body:**
```json
{
  "subredditId": "uuid-string",
  "userId": "uuid-string"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Successfully left subreddit"
}
```

### Posts

#### Create Post

**Endpoint:** `POST /post`

Creates a new post.

**Request Body:**
```json
{
  "title": "My first post",
  "content": "This is the content of my post",
  "authorId": "uuid-string",
  "subredditId": "uuid-string"
}
```

**Response:**
```json
{
  "id": "uuid-string",
  "title": "My first post",
  "content": "This is the content of my post",
  "authorId": "uuid-string",
  "authorName": "username",
  "subredditId": "uuid-string",
  "subredditName": "subreddit-name",
  "voteCount": 0,
  "commentCount": 0,
  "createdAt": "2023-04-01T12:34:56Z"
}
```

#### Get Post by ID

**Endpoint:** `GET /post?id=<post_id>`

Retrieves a specific post by ID.

**Response:**
```json
{
  "id": "uuid-string",
  "title": "My first post",
  "content": "This is the content of my post",
  "authorId": "uuid-string",
  "authorName": "username",
  "subredditId": "uuid-string",
  "subredditName": "subreddit-name",
  "voteCount": 5,
  "commentCount": 2,
  "createdAt": "2023-04-01T12:34:56Z"
}
```

#### Get Posts by Subreddit

**Endpoint:** `GET /post?subredditId=<subreddit_id>`

Gets all posts in a specific subreddit.

**Response:**
```json
[
  {
    "id": "uuid-string",
    "title": "First post",
    "content": "Content of first post",
    "authorId": "uuid-string",
    "authorName": "username",
    "subredditId": "uuid-string",
    "subredditName": "subreddit-name",
    "voteCount": 5,
    "commentCount": 2,
    "createdAt": "2023-04-01T12:34:56Z"
  },
  // More posts...
]
```

### Voting

**Endpoint:** `POST /post/vote`

Vote on a post.

**Request Body:**
```json
{
  "userId": "uuid-string",
  "postId": "uuid-string",
  "isUpvote": true
}
```

**Response:**
```json
{
  "postId": "uuid-string",
  "voteCount": 6,
  "userVoted": true,
  "isUpvote": true
}
```

### User Feed

**Endpoint:** `GET /user/feed?userId=<user_id>&limit=<number>`

Gets personalized feed for a user (posts from subscribed subreddits).

**Response:**
```json
[
  {
    "id": "uuid-string",
    "title": "Post title",
    "content": "Post content",
    "authorId": "uuid-string",
    "authorName": "username",
    "subredditId": "uuid-string",
    "subredditName": "subreddit-name",
    "voteCount": 12,
    "commentCount": 5,
    "createdAt": "2023-04-01T12:34:56Z"
  },
  // More posts...
]
```

### Recent Posts

**Endpoint:** `GET /posts/recent`

Gets the most recent posts from all subreddits.

**Response:**
```json
[
  {
    "id": "uuid-string",
    "title": "Recent post",
    "content": "Content of recent post",
    "authorId": "uuid-string",
    "authorName": "username",
    "subredditId": "uuid-string",
    "subredditName": "subreddit-name",
    "voteCount": 3,
    "commentCount": 1,
    "createdAt": "2023-04-01T12:34:56Z"
  },
  // More posts...
]
```

### User Profile

**Endpoint:** `GET /user/profile?userId=<user_id>`

Gets the profile information for a user.

**Response:**
```json
{
  "id": "uuid-string",
  "username": "username",
  "email": "user@example.com",
  "karma": 120,
  "isConnected": true,
  "lastActive": "2023-04-01T12:34:56Z",
  "subredditID": ["uuid-1", "uuid-2"],
  "subredditName": ["subreddit1", "subreddit2"]
}
```

### Comments

#### Create Comment

**Endpoint:** `POST /comment`

Creates a new comment on a post or as a reply to another comment.

**Request Body:**
```json
{
  "content": "This is my comment",
  "authorId": "uuid-string",
  "postId": "uuid-string",
  "parentId": "uuid-string" // Optional, for replies
}
```

**Response:**
```json
{
  "id": "uuid-string",
  "content": "This is my comment",
  "authorId": "uuid-string",
  "authorName": "username",
  "postId": "uuid-string",
  "parentId": "uuid-string", // Optional
  "voteCount": 0,
  "createdAt": "2023-04-01T12:34:56Z"
}
```

#### Edit Comment

**Endpoint:** `PUT /comment`

Edits an existing comment.

**Request Body:**
```json
{
  "commentId": "uuid-string",
  "authorId": "uuid-string",
  "content": "Updated comment content"
}
```

**Response:**
```json
{
  "id": "uuid-string",
  "content": "Updated comment content",
  "authorId": "uuid-string",
  "authorName": "username",
  "postId": "uuid-string",
  "parentId": "uuid-string", // Optional
  "voteCount": 0,
  "createdAt": "2023-04-01T12:34:56Z",
  "updatedAt": "2023-04-01T13:34:56Z"
}
```

#### Get Comments for Post

**Endpoint:** `GET /comment/post?postId=<post_id>`

Gets all comments for a specific post.

**Response:**
```json
[
  {
    "id": "uuid-string",
    "content": "Comment content",
    "authorId": "uuid-string",
    "authorName": "username",
    "postId": "uuid-string",
    "parentId": null,
    "voteCount": 3,
    "createdAt": "2023-04-01T12:34:56Z",
    "replies": [
      {
        "id": "uuid-string",
        "content": "Reply content",
        "authorId": "uuid-string",
        "authorName": "another_user",
        "postId": "uuid-string",
        "parentId": "uuid-string",
        "voteCount": 1,
        "createdAt": "2023-04-01T13:34:56Z"
      }
    ]
  },
  // More comments...
]
```

#### Vote on Comment

**Endpoint:** `POST /comment/vote`

Vote on a comment.

**Request Body:**
```json
{
  "commentId": "uuid-string",
  "userId": "uuid-string",
  "isUpvote": true
}
```

**Response:**
```json
{
  "commentId": "uuid-string",
  "voteCount": 4,
  "userVoted": true,
  "isUpvote": true
}
```

### Direct Messages

#### Send Message

**Endpoint:** `POST /messages`

Sends a direct message to another user.

**Request Body:**
```json
{
  "fromId": "uuid-string",
  "toId": "uuid-string",
  "content": "Hello, how are you?"
}
```

**Response:**
```json
{
  "id": "uuid-string",
  "fromId": "uuid-string",
  "fromUsername": "sender_username",
  "toId": "uuid-string",
  "toUsername": "recipient_username",
  "content": "Hello, how are you?",
  "read": false,
  "createdAt": "2023-04-01T12:34:56Z"
}
```

#### Get User Messages

**Endpoint:** `GET /messages?userId=<user_id>`

Gets all messages for a specific user.

**Response:**
```json
[
  {
    "id": "uuid-string",
    "fromId": "uuid-string",
    "fromUsername": "sender_username",
    "toId": "uuid-string",
    "toUsername": "recipient_username",
    "content": "Hello, how are you?",
    "read": true,
    "createdAt": "2023-04-01T12:34:56Z"
  },
  // More messages...
]
```

#### Get Conversation

**Endpoint:** `GET /messages/conversation?userId=<user_id>&otherUserId=<other_user_id>`

Gets the conversation between two specific users.

**Response:**
```json
[
  {
    "id": "uuid-string",
    "fromId": "uuid-string",
    "fromUsername": "sender_username",
    "toId": "uuid-string",
    "toUsername": "recipient_username",
    "content": "Hello, how are you?",
    "read": true,
    "createdAt": "2023-04-01T12:34:56Z"
  },
  // More messages...
]
```

#### Mark Message as Read

**Endpoint:** `POST /messages/read`

Marks a message as read.

**Request Body:**
```json
{
  "messageId": "uuid-string",
  "userId": "uuid-string"
}
```

**Response:**
```json
{
  "id": "uuid-string",
  "read": true,
  "updatedAt": "2023-04-01T13:34:56Z"
}
```

## Error Responses

All endpoints return appropriate HTTP status codes:

- `200 OK`: Successful request
- `400 Bad Request`: Invalid input or request parameters
- `401 Unauthorized`: Authentication required or failed
- `403 Forbidden`: Insufficient permissions
- `404 Not Found`: Resource not found
- `500 Internal Server Error`: Server error

Error response format:
```json
{
  "error": "Detailed error message"
}
```

## Rate Limiting

The API implements rate limiting to protect against abuse. Clients may receive a `429 Too Many Requests` status code if they exceed the allowed request rate.
