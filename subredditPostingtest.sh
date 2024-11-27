#!/bin/bash

# Register first user (subreddit creator)
echo "1. Registering first user..."
USER1_RESPONSE=$(curl -s -X POST http://localhost:8080/user/register \
-H "Content-Type: application/json" \
-d '{
    "username": "creator",
    "email": "creator@example.com",
    "password": "pass123",
    "karma": 300
}')
echo "User 1 Response: $USER1_RESPONSE"
USER1_ID=$(echo $USER1_RESPONSE | jq -r '.ID')
echo "User 1 ID: $USER1_ID"

sleep 1

# Register second user
echo -e "\n2. Registering second user..."
USER2_RESPONSE=$(curl -s -X POST http://localhost:8080/user/register \
-H "Content-Type: application/json" \
-d '{
    "username": "poster",
    "email": "poster@example.com",
    "password": "pass123",
    "karma": 300
}')
echo "User 2 Response: $USER2_RESPONSE"
USER2_ID=$(echo $USER2_RESPONSE | jq -r '.ID')
echo "User 2 ID: $USER2_ID"

sleep 1

# First user creates subreddit
echo -e "\n3. First user creating subreddit..."
SUB_RESPONSE=$(curl -s -X POST http://localhost:8080/subreddit \
-H "Content-Type: application/json" \
-d '{
    "name": "technology",
    "description": "Tech discussions",
    "creatorId": "'$USER1_ID'"
}')
echo "Subreddit Response: $SUB_RESPONSE"
SUB_ID=$(echo $SUB_RESPONSE | jq -r '.ID')
echo "Subreddit ID: $SUB_ID"

sleep 1

# First user creates post (should succeed as creator is automatically a member)
echo -e "\n4. First user creating post..."
POST1_RESPONSE=$(curl -s -X POST http://localhost:8080/post \
-H "Content-Type: application/json" \
-d '{
    "title": "First Post",
    "content": "Tech discussion",
    "authorId": "'$USER1_ID'",
    "subredditId": "'$SUB_ID'"
}')
echo "First Post Response: $POST1_RESPONSE"

sleep 1

# Second user tries to post without joining (should fail)
echo -e "\n5. Second user trying to post without joining (should fail)..."
POST2_RESPONSE=$(curl -s -X POST http://localhost:8080/post \
-H "Content-Type: application/json" \
-d '{
    "title": "Unauthorized Post",
    "content": "This should fail",
    "authorId": "'$USER2_ID'",
    "subredditId": "'$SUB_ID'"
}')
echo "Failed Post Response: $POST2_RESPONSE"

sleep 1

# Second user joins subreddit
echo -e "\n6. Second user joining subreddit..."
JOIN_RESPONSE=$(curl -s -X POST http://localhost:8080/subreddit/members \
-H "Content-Type: application/json" \
-d '{
    "subredditId": "'$SUB_ID'",
    "userId": "'$USER2_ID'"
}')
echo "Join Response: $JOIN_RESPONSE"

sleep 1

# Second user tries to post after joining (should succeed)
echo -e "\n7. Second user posting after joining (should succeed)..."
POST3_RESPONSE=$(curl -s -X POST http://localhost:8080/post \
-H "Content-Type: application/json" \
-d '{
    "title": "Authorized Post",
    "content": "This should work",
    "authorId": "'$USER2_ID'",
    "subredditId": "'$SUB_ID'"
}')
echo "Successful Post Response: $POST3_RESPONSE"