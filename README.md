# Comprehensive Reddit Clone API Documentation

## Table of Contents
- [Internal Data Structures](#internal-data-structures)
- [Database Schema](#database-schema)
- [Subreddit Endpoints](#subreddit-endpoints)
- [Comment Endpoints](#comment-endpoints)
- [Direct Message Endpoints](#direct-message-endpoints)
- [Error Handling](#error-handling)

## Internal Data Structures

### Subreddit-Related Structures

#### Subreddit Model (models.Subreddit)
```go
type Subreddit struct {
    ID          uuid.UUID
    Name        string
    Description string
    CreatorID   uuid.UUID
    Members     int
    CreatedAt   time.Time
    Posts       []uuid.UUID
}
```

#### SubredditDB (Database Document)
```go
type SubredditDB struct {
    ID          string    `bson:"_id"`
    Name        string    `bson:"name"`
    Description string    `bson:"description"`
    CreatorID   string    `bson:"creatorId"`
    Members     int       `bson:"members"`
    CreatedAt   time.Time `bson:"createdAt"`
    Posts       []string  `bson:"posts"`
}
```

### Comment-Related Structures

#### Comment Model (models.Comment)
```go
type Comment struct {
    ID          uuid.UUID   `json:"id"`
    Content     string      `json:"content"`
    AuthorID    uuid.UUID   `json:"authorId"`
    PostID      uuid.UUID   `json:"postId"`
    SubredditID uuid.UUID   `json:"subredditId"`
    ParentID    *uuid.UUID  `json:"parentId,omitempty"`
    Children    []uuid.UUID `json:"children"`
    CreatedAt   time.Time   `json:"createdAt"`
    UpdatedAt   time.Time   `json:"updatedAt"`
    IsDeleted   bool        `json:"isDeleted"`
    Upvotes     int         `json:"upvotes"`
    Downvotes   int         `json:"downvotes"`
    Karma       int         `json:"karma"`
}
```

#### CommentDocument (Database Document)
```go
type CommentDocument struct {
    ID          string    `bson:"_id"`
    Content     string    `bson:"content"`
    AuthorID    string    `bson:"authorId"`
    PostID      string    `bson:"postId"`
    SubredditID string    `bson:"subredditId"`
    ParentID    *string   `bson:"parentId,omitempty"`
    Children    []string  `bson:"children"`
    CreatedAt   time.Time `bson:"createdAt"`
    UpdatedAt   time.Time `bson:"updatedAt"`
    IsDeleted   bool      `bson:"isDeleted"`
    Upvotes     int       `bson:"upvotes"`
    Downvotes   int       `bson:"downvotes"`
    Karma       int       `bson:"karma"`
}
```

### Direct Message-Related Structures

#### DirectMessage Model (models.DirectMessage)
```go
type DirectMessage struct {
    ID        uuid.UUID
    FromID    uuid.UUID
    ToID      uuid.UUID
    Content   string
    CreatedAt time.Time
    IsRead    bool
    IsDeleted bool
}
```

#### DirectMessageDocument (Database Document)
```go
type DirectMessageDocument struct {
    ID        string    `bson:"_id"`
    FromID    string    `bson:"fromId"`
    ToID      string    `bson:"toId"`
    Content   string    `bson:"content"`
    CreatedAt time.Time `bson:"createdAt"`
    IsRead    bool      `bson:"isRead"`
    IsDeleted bool      `bson:"isDeleted"`
}
```

## Subreddit Endpoints

### List All Subreddits
`GET /subreddit`

**Response Body Example**
```json
[
    {
        "ID": "123e4567-e89b-12d3-a456-426614174000",
        "Name": "GatorSwamp",
        "Description": "A place for all things Gator related!",
        "CreatorID": "123e4567-e89b-12d3-a456-426614174111",
        "Members": 150,
        "CreatedAt": "2024-04-01T12:00:00Z",
        "Posts": [
            "123e4567-e89b-12d3-a456-426614174222",
            "123e4567-e89b-12d3-a456-426614174333"
        ]
    },
    {
        "ID": "123e4567-e89b-12d3-a456-426614174444",
        "Name": "TechTalks",
        "Description": "Discussion about technology",
        "CreatorID": "123e4567-e89b-12d3-a456-426614174555",
        "Members": 75,
        "CreatedAt": "2024-04-02T14:30:00Z",
        "Posts": []
    }
]
```

### Create Subreddit
`POST /subreddit`

**Request Body Example**
```json
{
    "name": "GatorSwamp",
    "description": "A place for all things Gator related!",
    "creatorId": "123e4567-e89b-12d3-a456-426614174000"
}
```

**Response Body Example**
```json
{
    "ID": "123e4567-e89b-12d3-a456-426614174111",
    "Name": "GatorSwamp",
    "Description": "A place for all things Gator related!",
    "CreatorID": "123e4567-e89b-12d3-a456-426614174000",
    "Members": 1,
    "CreatedAt": "2024-04-01T12:00:00Z",
    "Posts": []
}
```

**Error Response Examples**
```json
// 400 Bad Request - Invalid UUID
{
    "error": "Invalid creator ID format"
}

// 409 Conflict - Duplicate Name
{
    "error": "subreddit with name GatorSwamp already exists"
}

// 401 Unauthorized - Insufficient Karma
{
    "error": "Insufficient karma (required: 100, current: 50)"
}
```

### Get Subreddit Members
`GET /subreddit/members?id=123e4567-e89b-12d3-a456-426614174000`

**Response Body Example**
```json
[
    "123e4567-e89b-12d3-a456-426614174111",
    "123e4567-e89b-12d3-a456-426614174222",
    "123e4567-e89b-12d3-a456-426614174333"
]
```

### Join Subreddit
`POST /subreddit/members`

**Request Body Example**
```json
{
    "subredditId": "123e4567-e89b-12d3-a456-426614174000",
    "userId": "123e4567-e89b-12d3-a456-426614174111"
}
```

**Success Response Example**
```json
true
```

### Leave Subreddit
`DELETE /subreddit/members`

**Request Body Example**
```json
{
    "subredditId": "123e4567-e89b-12d3-a456-426614174000",
    "userId": "123e4567-e89b-12d3-a456-426614174111"
}
```

**Success Response Example**
```json
true
```

## Comment Endpoints

### Create Comment
`POST /comment`

**Request Body Example**
```json
{
    "content": "This is a great post!",
    "authorId": "123e4567-e89b-12d3-a456-426614174000",
    "postId": "123e4567-e89b-12d3-a456-426614174111",
    "parentId": "123e4567-e89b-12d3-a456-426614174222" // Optional
}
```

**Response Body Example**
```json
{
    "id": "123e4567-e89b-12d3-a456-426614174333",
    "content": "This is a great post!",
    "authorId": "123e4567-e89b-12d3-a456-426614174000",
    "postId": "123e4567-e89b-12d3-a456-426614174111",
    "subredditId": "123e4567-e89b-12d3-a456-426614174444",
    "parentId": "123e4567-e89b-12d3-a456-426614174222",
    "children": [],
    "createdAt": "2024-04-01T12:00:00Z",
    "updatedAt": "2024-04-01T12:00:00Z",
    "isDeleted": false,
    "upvotes": 0,
    "downvotes": 0,
    "karma": 0
}
```

### Edit Comment
`PUT /comment`

**Request Body Example**
```json
{
    "commentId": "123e4567-e89b-12d3-a456-426614174000",
    "authorId": "123e4567-e89b-12d3-a456-426614174111",
    "content": "Updated comment content"
}
```

**Response Body Example**
```json
{
    "id": "123e4567-e89b-12d3-a456-426614174000",
    "content": "Updated comment content",
    "authorId": "123e4567-e89b-12d3-a456-426614174111",
    "postId": "123e4567-e89b-12d3-a456-426614174222",
    "subredditId": "123e4567-e89b-12d3-a456-426614174333",
    "parentId": null,
    "children": [
        "123e4567-e89b-12d3-a456-426614174444",
        "123e4567-e89b-12d3-a456-426614174555"
    ],
    "createdAt": "2024-04-01T12:00:00Z",
    "updatedAt": "2024-04-01T13:00:00Z",
    "isDeleted": false,
    "upvotes": 5,
    "downvotes": 2,
    "karma": 3
}
```

### Get Post Comments
`GET /comment/post?postId=123e4567-e89b-12d3-a456-426614174000`

**Response Body Example**
```json
[
    {
        "id": "123e4567-e89b-12d3-a456-426614174111",
        "content": "Parent comment",
        "authorId": "123e4567-e89b-12d3-a456-426614174222",
        "postId": "123e4567-e89b-12d3-a456-426614174000",
        "subredditId": "123e4567-e89b-12d3-a456-426614174333",
        "parentId": null,
        "children": [
            "123e4567-e89b-12d3-a456-426614174444"
        ],
        "createdAt": "2024-04-01T12:00:00Z",
        "updatedAt": "2024-04-01T12:00:00Z",
        "isDeleted": false,
        "upvotes": 10,
        "downvotes": 2,
        "karma": 8
    },
    {
        "id": "123e4567-e89b-12d3-a456-426614174444",
        "content": "Reply to parent",
        "authorId": "123e4567-e89b-12d3-a456-426614174555",
        "postId": "123e4567-e89b-12d3-a456-426614174000",
        "subredditId": "123e4567-e89b-12d3-a456-426614174333",
        "parentId": "123e4567-e89b-12d3-a456-426614174111",
        "children": [],
        "createdAt": "2024-04-01T12:30:00Z",
        "updatedAt": "2024-04-01T12:30:00Z",
        "isDeleted": false,
        "upvotes": 5,
        "downvotes": 1,
        "karma": 4
    }
]
```

### Vote on Comment
`POST /comment/vote`

**Request Body Example**
```json
{
    "commentId": "123e4567-e89b-12d3-a456-426614174000",
    "userId": "123e4567-e89b-12d3-a456-426614174111",
    "isUpvote": true
}
```

**Response Body Example**
```json
{
    "id": "123e4567-e89b-12d3-a456-426614174000",
    "content": "Comment content",
    "authorId": "123e4567-e89b-12d3-a456-426614174222",
    "postId": "123e4567-e89b-12d3-a456-426614174333",
    "subredditId": "123e4567-e89b-12d3-a456-426614174444",
    "parentId": null,
    "children": [],
    "createdAt": "2024-04-01T12:00:00Z",
    "updatedAt": "2024-04-01T12:00:00Z",
    "isDeleted": false,
    "upvotes": 6,
    "downvotes": 2,
    "karma": 4
}
```

## Direct Message Endpoints

### Send Message
`POST /messages`

**Request Body Example**
```json
{
    "fromId": "123e4567-e89b-12d3-a456-426614174000",
    "toId": "123e4567-e89b-12d3-a456-426614174111",
    "content": "Hello! How are you?"
}
```

**Response Body Example**
```json
{
    "ID": "123e4567-e89b-12d3-a456-426614174222",
    "FromID": "123e4567-e89b-12d3-a456-426614174000",
    "ToID": "123e4567-e89b-12d3-a456-426614174111",
    "Content": "Hello! How are you?",
    "CreatedAt": "2024-04-01T12:00:00Z",
    "IsRead": false,
    "IsDeleted": false
}
```

### Get User Messages
`GET /messages?userId=123e4567-e89b-12d3-a456-426614174000`

**Response Body Example**
```json
[
    {
        "ID": "123e4567-e89b-12d3-a456-426614174111",
        "FromID": "123e4567-e89b-12d3-a456-426614174000",
        "ToID": "123e4567-e89b-12d3-a456-426614174222",
        "Content": "Hello! How are you?",
        "CreatedAt": "2024-04-01T12:00:00Z",
        "IsRead": true,
        "IsDeleted": false
    },
    {
        "ID": "123e4567-e89b-12d3-a456-426614174333",
        "FromID": "123e4567-e89b-12d3-a456-426614174222",
        "ToID": "123e4567-e89b-12d3-a456-426614174000",
        "Content": "I'm good, thanks! How about you?",
        "CreatedAt": "2024-04-01T12:05:00Z",
        "IsRead": false,
        "IsDeleted": false
    }
]
```

### Get Conversation
`GET /messages/conversation?user1=123e4567-e89b-12d3-a456-426614174000&user2=123e4567-e89b-12d3-a456-426614174111`

**Response Body Example**
```json
[
    {
        "ID": "123e4567-e89b-12d3-a456-426614174222",
        "FromID": "123e4567-e89b-12d3-a456-426614174000",
        "ToID": "123e4567-e89b-12d3-a456-426614174111",
        "Content": "Hey! Want to collaborate on the project?",
        "CreatedAt": "2024-04-01T10:00:00Z",
        "IsRead": true,
        "IsDeleted": false
    },
    {
        "ID": "123e4567-e89b-12d3-a456-426614174333",
        "FromID": "123e4567-e89b-12d3-a456-426614174111",
        "ToID": "123e4567-e89b-12d3-a456-426614174000",
        "Content": "Sure! When do you want to start?",
        "CreatedAt": "2024-04-01T10:05:00Z",
        "IsRead": true,
        "IsDeleted": false
    }
]
```

### Mark Message as Read
`POST /messages/read`

**Request Body Example**
```json
{
    "messageId": "123e4567-e89b-12d3-a456-426614174000",
    "userId": "123e4567-e89b-12d3-a456-426614174111"
}
```

**Response Body Example**
```json
{
    "success": true
}
```

### Delete Message
`DELETE /messages?messageId=123e4567-e89b-12d3-a456-426614174000&userId=123e4567-e89b-12d3-a456-426614174111`

**Response Body Example**
```json
{
    "success": true
}
```

## Additional Implementation Details

### Data Type Transformations

#### UUID Handling
1. Frontend sends/receives UUIDs as strings
2. Backend converts using:
```go
// String to UUID
id, err := uuid.Parse(idString)

// UUID to String
idString := id.String()
```

#### DateTime Handling
1. Frontend receives ISO 8601 strings
2. Backend uses time.Time internally
3. MongoDB stores dates in native DateTime format
4. JSON marshaling automatically converts between formats

### Caching Mechanisms

#### Subreddit Actor Cache
```go
type SubredditActor struct {
    subredditsByName map[string]*models.Subreddit
    subredditsById   map[uuid.UUID]*models.Subreddit
    subredditMembers map[uuid.UUID]map[uuid.UUID]bool
}
```

#### Comment Actor Cache
```go
type CommentActor struct {
    comments     map[uuid.UUID]*models.Comment
    postComments map[uuid.UUID][]uuid.UUID
    commentVotes map[uuid.UUID]map[uuid.UUID]bool
}
```

#### Direct Message Actor Cache
```go
type DirectMessageActor struct {
    messages     map[uuid.UUID]*models.DirectMessage
    userMessages map[uuid.UUID]map[uuid.UUID][]*models.DirectMessage
}
```

### Error Handling

#### AppError Structure
```go
type AppError struct {
    Code    string
    Message string
    Origin  error
}
```

#### Common Error Codes
```go
const (
    ErrNotFound     = "NOT_FOUND"
    ErrDuplicate    = "DUPLICATE"
    ErrInvalidInput = "INVALID_INPUT"
    ErrUnauthorized = "UNAUTHORIZED"
    ErrForbidden    = "FORBIDDEN"
    ErrDatabase     = "DATABASE_ERROR"
)
```

#### Error Response Example
```json
{
    "code": "NOT_FOUND",
    "message": "Subreddit not found",
    "details": "Failed to find subreddit with ID: 123e4567-e89b-12d3-a456-426614174000"
}
```

### WebSocket Considerations
While not currently implemented, the system is designed to support future WebSocket integration for:
- Real-time message delivery
- Comment updates
- Karma changes
- Member join/leave notifications

### Database Indexes
```go
// Comment Indexes
[
    { "postId": 1, "createdAt": -1 },
    { "authorId": 1 },
    { "parentId": 1 }
]

// Subreddit Indexes
[
    { "name": 1 } // Unique index
]

// Message Indexes
[
    { "fromId": 1, "toId": 1, "createdAt": -1 },
    { "toId": 1, "isRead": 1 }
]
```

### Performance Considerations
1. Comment tree traversal is optimized using the Children array
2. Messages are paginated by default (limit 50)
3. Caching reduces database load
4. Soft deletes preserve data integrity
5. Batch operations for vote processing

### Security Notes
1. All UUIDs are validated before processing
2. Author verification for edits/deletes
3. Recipient verification for message operations
4. Karma requirements for certain operations
5. MongoDB injection prevention through proper BSON usage
