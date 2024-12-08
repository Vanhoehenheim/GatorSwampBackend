user1 : 5a313582-97f7-4666-920b-d860e9f13bdc
user2: 387ed7d3-b40b-4621-9569-4d93efbb0198
Subreddit: 64026593-29cf-4ca9-a0e0-6d07f03fcdb5
Post: 7d1c0565-b228-44e4-a905-e9f73fea2047
Comment: 54c9ea92-44f7-4053-8d6a-2b2a5835ab02

# Create User 1
curl -X POST http://localhost:8080/user/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "user1",
    "email": "user1@example.com",
    "password": "password123",
    "karma": 300
  }'

# Create User 2
curl -X POST http://localhost:8080/user/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "user2",
    "email": "user2@example.com",
    "password": "password123",
    "karma": 300
  }'

# User 1 creates subreddit
curl -X POST http://localhost:8080/subreddit \
  -H "Content-Type: application/json" \
  -d '{
    "name": "testsubreddit",
    "description": "A test subreddit",
    "creatorId": "5a313582-97f7-4666-920b-d860e9f13bdc"
  }'

# User 2 joins the subreddit
curl -X POST http://localhost:8080/subreddit/members \
  -H "Content-Type: application/json" \
  -d '{
    "subredditId": "64026593-29cf-4ca9-a0e0-6d07f03fcdb5",
    "userId": "387ed7d3-b40b-4621-9569-4d93efbb0198"
  }'

# User 1 creates post
curl -X POST http://localhost:8080/post \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Test Post",
    "content": "This is a test post",
    "authorId": "5a313582-97f7-4666-920b-d860e9f13bdc",
    "subredditId": "64026593-29cf-4ca9-a0e0-6d07f03fcdb5"
  }'

# User 1 comments on the post
curl -X POST http://localhost:8080/comment \
  -H "Content-Type: application/json" \
  -d '{
    "content": "This is a test comment",
    "authorId": "5a313582-97f7-4666-920b-d860e9f13bdc",
    "postId": "7d1c0565-b228-44e4-a905-e9f73fea2047"
  }'

# User 2 upvotes the comment
curl -X POST http://localhost:8080/comment/vote \
  -H "Content-Type: application/json" \
  -d '{
    "commentId": "54c9ea92-44f7-4053-8d6a-2b2a5835ab02",
    "userId": "387ed7d3-b40b-4621-9569-4d93efbb0198",
    "isUpvote": true
  }'

# Check User 1's karma after the upvote
curl "http://localhost:8080/user/profile?userId=5a313582-97f7-4666-920b-d860e9f13bdc"