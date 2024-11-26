# GatorSwamp


# Create User 1
curl -X POST http://localhost:8080/user/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "user1",
    "email": "user1@example.com",
    "password": "password123",
    "karma": 100
  }'

# Create User 2
curl -X POST http://localhost:8080/user/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "user2",
    "email": "user2@example.com",
    "password": "password456",
    "karma": 100
  }'

# User 1
curl -X POST http://localhost:8080/subreddit \
  -H "Content-Type: application/json" \
  -d '{
    "name": "testsubreddit",
    "description": "Test Subreddit",
    "creatorId": "79fd6bfb-f585-4352-8111-9885d71993a9"
  }'
  
  # User 1
  curl -X POST http://localhost:8080/post \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Test Post",
    "content": "This is a test post",
    "authorId": "79fd6bfb-f585-4352-8111-9885d71993a9",
    "subredditId": "8aa74519-16c0-4668-b3bc-eaacbaf88086"
  }'
# User 2
  curl -X POST http://localhost:8080/comment \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Parent comment by user2",
    "authorId": "b7197cbf-edd5-4778-a27b-c0d4fdcb08bb",
    "postId": "a0b946f1-14ea-4bf0-ba2e-31c7d573150b"
  }'

 # User 1
  curl -X POST http://localhost:8080/comment \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Reply from user1",
    "authorId": "79fd6bfb-f585-4352-8111-9885d71993a9",
    "postId": "a0b946f1-14ea-4bf0-ba2e-31c7d573150b",
    "parentId": "2101799b-82f3-4e31-8f1c-c9186ed9b909"
  }'

  curl http://localhost:8080/comment/post?postId="a0b946f1-14ea-4bf0-ba2e-31c7d573150b

  curl -X DELETE "http://localhost:8080/comment?commentId=cf4ff484-c206-4c75-bad7-1d00d6b9d802&authorId="b7197cbf-edd5-4778-a27b-c0d4fdcb08bb"

  curl "http://localhost:8080/comment/post?postId="a0b946f1-14ea-4bf0-ba2e-31c7d573150b"
"
  9f1c4bf3-4446-4f54-9982-2e4a4b113846