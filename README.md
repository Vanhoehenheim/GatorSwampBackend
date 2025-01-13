# Gator-Swamp: A Social Media application for Gators built with Go using Actor Models

Link To Frontend: https://github.com/Vanhoehenheim/Gator_Swamp_Frontend

## Overview
Gator-Swamp is a Reddit-like platform built using Go, implementing the Actor Model pattern with Proto.Actor for concurrent operations. The system uses MongoDB for persistence and provides a RESTful API interface for client applications.

## Features
- User Management
  - Registration and authentication
  - Karma system
  - User profiles with activity tracking
  - Direct messaging between users

- Content Management
  - Subreddit creation and management
  - Post creation and management
  - Nested comments with threading
  - Voting system for posts and comments

- Real-time Features
  - User activity tracking
  - Message read status
  - Karma updates

## Tech Stack
- **Language**: Go
- **Actor Framework**: Proto.Actor
- **Database**: MongoDB
- **Authentication**: Custom token-based system
- **API**: RESTful HTTP endpoints

## Architecture

### Actor System
The application uses the Actor Model for handling concurrent operations, with several key actors:
- **Engine**: Central coordinator for all operations
- **UserSupervisor**: Manages user-related operations and user actors
- **SubredditActor**: Handles subreddit operations
- **PostActor**: Manages post-related operations
- **CommentActor**: Handles comment operations
- **DirectMessageActor**: Manages private messaging

### Database Schema
The system uses MongoDB with the following collections:
- Users
- Subreddits
- Posts
- Comments
- Messages
- Votes

## API Endpoints

### User Management
- `POST /user/register`: Register new user
- `POST /user/login`: User login
- `GET /user/profile`: Get user profile
- `GET /user/feed`: Get user's personalized feed

### Subreddits
- `POST /subreddit`: Create new subreddit
- `GET /subreddit`: List subreddits
- `GET /subreddit/members`: Get subreddit members
- `POST /subreddit/members`: Join subreddit
- `DELETE /subreddit/members`: Leave subreddit

### Posts
- `POST /post`: Create new post
- `GET /post`: Get post by ID
- `POST /post/vote`: Vote on post
- `GET /posts/recent`: Get recent posts

### Comments
- `POST /comment`: Create comment
- `PUT /comment`: Edit comment
- `DELETE /comment`: Delete comment
- `GET /comment`: Get comment by ID
- `GET /comment/post`: Get comments for post
- `POST /comment/vote`: Vote on comment

### Messages
- `POST /messages`: Send direct message
- `GET /messages`: Get user messages
- `GET /messages/conversation`: Get conversation between users
- `POST /messages/read`: Mark message as read
- `DELETE /messages`: Delete message

## Setup and Configuration

### Prerequisites
- Go 1.x
- MongoDB
- Protocol Buffers

### Environment Variables
Configure the following environment variables:
```
MONGODB_URI=mongodb+srv://your-connection-string
PORT=8080
HOST=localhost
```

### Running the Application
1. Clone the repository
2. Install dependencies: `go mod download`
3. Start the server: `go run main.go`

### Running Tests
Execute the test suite:
```bash
go test ./...
```

## Performance and Scaling
The application includes several performance-oriented features:
- In-memory caching in actors
- MongoDB indexing for frequent queries
- Metrics collection for monitoring
- Connection pooling for database operations

## Security Features
- Password hashing using bcrypt
- Token-based authentication
- Input validation and sanitization
- Rate limiting capabilities
- Permission-based access control
