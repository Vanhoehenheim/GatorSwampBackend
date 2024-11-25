# Immediate Implementation Requirements

## Your Part (Engine Features)

### 1. Subreddit Core (HIGH PRIORITY)
```go
// Add these operations to SubredditActor
- Leave subreddit
- List all subreddits
- Get subreddit members
- Get subreddit details
```

### 2. Post Management (HIGH PRIORITY)
```go
// Add to PostActor
- Delete posts
- Update posts
- List posts by subreddit
- Generate user feeds
- Sort posts (by time, karma)
```

## Abhay's Part (User Features)

### 1. User Management (HIGH PRIORITY)
```go
// Create UserActor with:
- User registration
- Basic authentication
- Profile management
- Karma tracking
```

### 2. Comments (HIGH PRIORITY)
```go
// Create CommentActor with:
- Create comment
- Nested comments
- Edit/delete comments
- Comment retrieval
```

### 3. Interaction (MEDIUM PRIORITY)
```go
// Add to both User and Post actors:
- Upvote/downvote
- Direct messaging
- Message inbox/outbox
```

## Integration Points

1. Post Creation Flow:
```
User (Abhay) → Subreddit (You) → Post (You)
```

2. Comment Flow:
```
User (Abhay) → Post (You) → Comment (Abhay)
```

3. Karma System:
```
Vote (Abhay) → Post/Comment (Both) → User Karma (Abhay)
```

We can start implementing these features immediately, and add the simulator component later. Would you like to:
1. Start with any specific feature implementation?
2. See the message types for any component?
3. Discuss the integration points in detail?