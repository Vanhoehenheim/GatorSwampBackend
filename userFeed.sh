#!/bin/bash

# Test 1: User Feed Flow
echo "1. Testing User Feed Flow..."

# Create two users
echo "Creating User 1 (Content Creator)..."
USER1_RESPONSE=$(curl -s -X POST http://localhost:8080/user/register \
-H "Content-Type: application/json" \
-d '{
    "username": "creator",
    "email": "creator@example.com",
    "password": "pass123",
    "karma": 300
}')
USER1_ID=$(echo $USER1_RESPONSE | jq -r '.ID')
echo "User 1 ID: $USER1_ID"

echo "Creating User 2 (Subscriber)..."
USER2_RESPONSE=$(curl -s -X POST http://localhost:8080/user/register \
-H "Content-Type: application/json" \
-d '{
    "username": "subscriber",
    "email": "sub@example.com",
    "password": "pass123",
    "karma": 300
}')
USER2_ID=$(echo $USER2_RESPONSE | jq -r '.ID')

# Create subreddit
echo -e "\n2. Creating subreddit..."
SUB_RESPONSE=$(curl -s -X POST http://localhost:8080/subreddit \
-H "Content-Type: application/json" \
-d '{
    "name": "testfeed",
    "description": "Testing feed",
    "creatorId": "'$USER1_ID'"
}')
SUB_ID=$(echo $SUB_RESPONSE | jq -r '.ID')

# User 2 joins subreddit
echo -e "\n3. User 2 joining subreddit..."
curl -s -X POST http://localhost:8080/subreddit/members \
-H "Content-Type: application/json" \
-d '{
    "subredditId": "'$SUB_ID'",
    "userId": "'$USER2_ID'"
}'

# Create multiple posts
echo -e "\n4. Creating multiple posts..."
curl -s -X POST http://localhost:8080/post \
-H "Content-Type: application/json" \
-d '{
    "title": "First Post",
    "content": "Content 1",
    "authorId": "'$USER1_ID'",
    "subredditId": "'$SUB_ID'"
}'

sleep 1

curl -s -X POST http://localhost:8080/post \
-H "Content-Type: application/json" \
-d '{
    "title": "Second Post",
    "content": "Content 2",
    "authorId": "'$USER1_ID'",
    "subredditId": "'$SUB_ID'"
}'

# Get feed for User 2
echo -e "\n5. Getting feed for User 2..."
curl -s -X GET "http://localhost:8080/user/feed?userId=$USER2_ID&limit=10"