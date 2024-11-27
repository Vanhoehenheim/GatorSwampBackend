#!/bin/bash

# Register user
echo "1. Registering user..."
USER_RESPONSE=$(curl -s -X POST http://localhost:8080/user/register \
-H "Content-Type: application/json" \
-d '{
    "username": "testuser",
    "email": "test@example.com",
    "password": "pass123",
    "karma": 300
}')
echo "User Response: $USER_RESPONSE"
USER_ID=$(echo $USER_RESPONSE | jq -r '.ID')
echo "User ID: $USER_ID"

sleep 1

# Create subreddit
echo -e "\n2. Creating subreddit..."
SUB_RESPONSE=$(curl -s -X POST http://localhost:8080/subreddit \
-H "Content-Type: application/json" \
-d '{
    "name": "testsubreddit",
    "description": "Test subreddit",
    "creatorId": "'$USER_ID'"
}')
echo "Subreddit Response: $SUB_RESPONSE"
SUB_ID=$(echo $SUB_RESPONSE | jq -r '.ID')
echo "Subreddit ID: $SUB_ID"

sleep 1

# Check initial members
echo -e "\n3. Checking initial members..."
curl -s -X GET "http://localhost:8080/subreddit/members?id=$SUB_ID"

sleep 1

# Leave subreddit
echo -e "\n4. Leaving subreddit..."
LEAVE_RESPONSE=$(curl -s -X DELETE http://localhost:8080/subreddit/members \
-H "Content-Type: application/json" \
-d '{
    "subredditId": "'$SUB_ID'",
    "userId": "'$USER_ID'"
}')
echo "Leave Response: $LEAVE_RESPONSE"

sleep 1

# Try to post after leaving (should fail)
echo -e "\n5. Trying to post after leaving (should fail)..."
POST_RESPONSE=$(curl -s -X POST http://localhost:8080/post \
-H "Content-Type: application/json" \
-d '{
    "title": "Test Post",
    "content": "This should fail",
    "authorId": "'$USER_ID'",
    "subredditId": "'$SUB_ID'"
}')
echo "Post Response: $POST_RESPONSE"

sleep 1

# Check final members
echo -e "\n6. Checking final members..."
curl -s -X GET "http://localhost:8080/subreddit/members?id=$SUB_ID"