# GatorSwap: Actor-Based Reddit Clone
## Project Structure and Implementation Guide

## Current Base Setup

### 1. Project Structure
```
gator-swamp/
├── cmd/
│   └── engine/
│       └── main.go           # Server entry point, HTTP handlers
├── internal/
│   ├── config/
│   │   └── config.go        # Server configuration
│   ├── engine/
│   │   └── actor.go         # Core actor implementations
│   ├── models/
│   │   └── models.go        # Shared data structures
│   └── utils/
       ├── errors.go         # Error handling
       └── metrics.go        # Performance tracking
```

### 2. Core Components Implemented

#### Data Models (`models.go`)
```go
- User: Basic user information
- Subreddit: Community structure
- Post: Content container
- Comment: Hierarchical comments
- DirectMessage: User communications
```

#### Actor System (`actor.go`)
```go
// Currently Implemented
- SubredditActor: Manages subreddits
- PostActor: Manages posts
- Message Types: CreateSubreddit, JoinSubreddit, CreatePost, etc.

// To Be Implemented
- UserActor (Abhay)
- CommentActor (Abhay)
- Supervisors (Both)
```

## Division of Responsibilities

### Your Part (Core Engine)

1. Subreddit Management
```go
SubredditSupervisor
├── SubredditActor1
├── SubredditActor2
└── SubredditActorN
```
- Create/manage subreddits
- Handle memberships
- Maintain subreddit state

2. Post Management
```go
PostSupervisor
├── PostActor1
├── PostActor2
└── PostActorN
```
- Post creation/deletion
- Feed generation
- Post state management

### Abhay's Part (User Interaction)

1. User Management
```go
UserSupervisor
├── UserActor1
├── UserActor2
└── UserActorN
```
- User registration/authentication
- Karma management
- User state maintenance

2. Interaction Systems
```go
CommentSupervisor
├── CommentActor1
├── CommentActor2
└── CommentActorN
```
- Comment handling
- Voting system
- Direct messaging

## Implementation Guidelines

### 1. Actor Communication Patterns

```
User Action → UserActor → SubredditActor → PostActor
                      ↘              ↘
                        CommentActor   MetricsActor
```

Example Message Flow:
```go
// Creating a post
UserActor 
    → SubredditActor (verify membership)
    → PostActor (create post)
    → SubredditActor (update state)
```

### 2. Key Considerations

For You:
1. Subreddit Operations
   - Maintain thread safety using actor model
   - Handle concurrent membership updates
   - Manage subreddit state efficiently

2. Post Management
   - Implement efficient feed generation
   - Handle post updates/deletions
   - Maintain post ordering

For Abhay:
1. User Management
   - Each user becomes an actor
   - Handle concurrent karma updates
   - Manage user sessions

2. Comment System
   - Implement hierarchical comment structure
   - Handle concurrent voting
   - Maintain comment state

### 3. Testing Strategy

1. Actor Testing
```go
// Example test structure
func TestSubredditActor_CreateSubreddit(t *testing.T) {
    // Setup actor system
    // Send message
    // Verify response
}
```

2. Integration Testing
```go
// Test interaction between actors
User creates post → Verify in subreddit → Verify in feed
```

## Message Types Reference

### Currently Implemented
```go
// Your messages
CreateSubredditMsg
JoinSubredditMsg
CreatePostMsg
GetPostMsg
GetCountsMsg

// To be added by Abhay
CreateUserMsg
CreateCommentMsg
VoteMsg
```

## Adding New Features

### For You:
1. Add Supervisor Actors
```go
type SubredditSupervisor struct {
    subreddits map[string]*actor.PID
}
```

2. Implement Feed Generation
```go
type GenerateFeedMsg struct {
    UserID uuid.UUID
    Limit  int
}
```

### For Abhay:
1. Implement User Actor
```go
type UserActor struct {
    userState    *models.User
    karma        int
    votedPosts   map[uuid.UUID]bool
}
```

2. Implement Comment Actor
```go
type CommentActor struct {
    comments     map[uuid.UUID]*models.Comment
    parentChild  map[uuid.UUID][]uuid.UUID
}
```

## Performance Considerations

1. Message Patterns
- Use RequestFuture for synchronous operations
- Use Tell for fire-and-forget operations
- Handle timeouts appropriately

2. State Management
- Keep actor state minimal
- Use appropriate data structures
- Consider memory usage

## Next Steps

For You:
1. Implement SubredditSupervisor
2. Add feed generation logic
3. Implement post sorting/filtering

For Abhay:
1. Set up UserActor structure
2. Implement comment system
3. Add voting mechanism

## Testing the System

1. Create a subreddit:
```bash
curl -X POST http://localhost:8080/subreddit \
  -H "Content-Type: application/json" \
  -d '{
    "name": "golang",
    "description": "Go discussions",
    "creatorId": "123e4567-e89b-12d3-a456-426614174000"
  }'
```

2. Check system health:
```bash
curl http://localhost:8080/health
```

## Common Pitfalls to Avoid

1. Actor Design
- Don't share state between actors
- Use messages for all communication
- Handle actor failures gracefully

2. Message Handling
- Always respond to messages
- Handle unknown message types
- Include proper error handling

3. Performance
- Don't create too many actors
- Keep message size reasonable
- Use appropriate timeouts

Need any specific section explained in more detail?
