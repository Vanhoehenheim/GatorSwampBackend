#!/bin/bash

# First user registration
echo "1. Registering user 1 (poster)..."
USER1_RESPONSE=$(curl -s -X POST http://localhost:8080/user/register \
-H "Content-Type: application/json" \
-d '{
   "username": "poster1", 
   "email": "poster1@example.com",
   "password": "pass123",
   "karma": 300
}')
echo "User 1 Response: $USER1_RESPONSE"
USER1_ID=$(echo $USER1_RESPONSE | jq -r '.ID')
echo "User 1 ID: $USER1_ID"

sleep 1

# Second user registration
echo -e "\n2. Registering user 2 (poster)..."
USER2_RESPONSE=$(curl -s -X POST http://localhost:8080/user/register \
-H "Content-Type: application/json" \
-d '{
   "username": "poster2",
   "email": "poster2@example.com", 
   "password": "pass123",
   "karma": 300
}')
echo "User 2 Response: $USER2_RESPONSE" 
USER2_ID=$(echo $USER2_RESPONSE | jq -r '.ID')
echo "User 2 ID: $USER2_ID"

sleep 1

# Third user registration (voter)
echo -e "\n3. Registering user 3 (voter)..."
USER3_RESPONSE=$(curl -s -X POST http://localhost:8080/user/register \
-H "Content-Type: application/json" \
-d '{
   "username": "voter",
   "email": "voter@example.com",
   "password": "pass123",
   "karma": 300
}')
echo "User 3 Response: $USER3_RESPONSE"
USER3_ID=$(echo $USER3_RESPONSE | jq -r '.ID')
echo "User 3 ID: $USER3_ID"

sleep 1

#  4th user registration (voter)
echo -e "\n4. Registering user 4 (voter)..."
USER4_RESPONSE=$(curl -s -X POST http://localhost:8080/user/register \
-H "Content-Type: application/json" \
-d '{
   "username": "voter2",
   "email": "voter2@example.com",
    "password": "pass1234",
    "karma": 300
}')
echo "User 4 Response: $USER4_RESPONSE"
USER4_ID=$(echo $USER4_RESPONSE | jq -r '.ID')
echo "User 4 ID: $USER4_ID"

# User 1 creates first subreddit
echo -e "\n4. User 1 creating first subreddit..."
SUB1_RESPONSE=$(curl -s -X POST http://localhost:8080/subreddit \
-H "Content-Type: application/json" \
-d '{
   "name": "technology",
   "description": "Tech discussions",
   "creatorId": "'$USER1_ID'"
}')
echo "Subreddit 1 Response: $SUB1_RESPONSE"
SUB1_ID=$(echo $SUB1_RESPONSE | jq -r '.ID')
echo "Subreddit 1 ID: $SUB1_ID"

sleep 1

# User 2 creates second subreddit
echo -e "\n5. User 2 creating second subreddit..."
SUB2_RESPONSE=$(curl -s -X POST http://localhost:8080/subreddit \
-H "Content-Type: application/json" \
-d '{
   "name": "gaming", 
   "description": "Gaming discussions",
   "creatorId": "'$USER2_ID'"
}')
echo "Subreddit 2 Response: $SUB2_RESPONSE"
SUB2_ID=$(echo $SUB2_RESPONSE | jq -r '.ID')
echo "Subreddit 2 ID: $SUB2_ID"

sleep 1

# User 1 creates post in first subreddit
echo -e "\n6. User 1 creating post in first subreddit..."
POST1_RESPONSE=$(curl -s -X POST http://localhost:8080/post \
-H "Content-Type: application/json" \
-d '{
   "title": "Tech Post",
   "content": "Technology discussion",
   "authorId": "'$USER1_ID'",
   "subredditId": "'$SUB1_ID'"
}')
echo "Post 1 Response: $POST1_RESPONSE"
POST1_ID=$(echo $POST1_RESPONSE | jq -r '.ID')
echo "Post 1 ID: $POST1_ID"

sleep 1

# User 2 creates post in second subreddit
echo -e "\n7. User 2 creating post in second subreddit..."
POST2_RESPONSE=$(curl -s -X POST http://localhost:8080/post \
-H "Content-Type: application/json" \
-d '{
   "title": "Gaming Post",
   "content": "Gaming discussion", 
   "authorId": "'$USER2_ID'",
   "subredditId": "'$SUB2_ID'"
}')
echo "Post 2 Response: $POST2_RESPONSE"
POST2_ID=$(echo $POST2_RESPONSE | jq -r '.ID')
echo "Post 2 ID: $POST2_ID"

sleep 1

# User 3 upvotes both posts
echo -e "\n8. User 3 upvoting posts..."
echo "Upvoting Post 1..."
curl -s -X POST http://localhost:8080/post/vote \
-H "Content-Type: application/json" \
-d '{
   "userId": "'$USER3_ID'",
   "postId": "'$POST1_ID'",
   "isUpvote": true
}'

sleep 1

echo -e "\nUpvoting Post 2..."
curl -s -X POST http://localhost:8080/post/vote \
-H "Content-Type: application/json" \
-d '{
   "userId": "'$USER3_ID'",
   "postId": "'$POST2_ID'",
   "isUpvote": true
}'

sleep 1

# User 4 downvotes both posts
echo -e "\n9. User 4 downvoting posts..."
echo "Downvoting Post 1..."
curl -s -X POST http://localhost:8080/post/vote \
-H "Content-Type: application/json" \
-d '{
   "userId": "'$USER4_ID'",
   "postId": "'$POST1_ID'",
   "isUpvote": false
}'
sleep 1

echo -e "\nDownvoting Post 2..."
curl -s -X POST http://localhost:8080/post/vote \
-H "Content-Type: application/json" \
-d '{
   "userId": "'$USER4_ID'",
   "postId": "'$POST2_ID'",
   "isUpvote": false
}'
sleep 1

# Check karma for all users
echo -e "\n9. Checking user profiles..."
echo "User 1 profile:"
curl -s -X GET "http://localhost:8080/user/profile?userId=$USER1_ID"

echo -e "\nUser 2 profile:"
curl -s -X GET "http://localhost:8080/user/profile?userId=$USER2_ID"

echo -e "\nUser 3 profile:"
curl -s -X GET "http://localhost:8080/user/profile?userId=$USER3_ID"