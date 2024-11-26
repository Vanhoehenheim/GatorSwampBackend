#!/bin/bash

# First user registration
echo "1. Registering first user (poster)..."
POSTER_RESPONSE=$(curl -s -X POST http://localhost:8080/user/register \
-H "Content-Type: application/json" \
-d '{
   "username": "poster",
   "email": "poster@example.com", 
   "password": "pass123",
   "karma": 300
}')
echo "Poster Response: $POSTER_RESPONSE"
POSTER_ID=$(echo $POSTER_RESPONSE | jq -r '.ID')
echo "Poster ID: $POSTER_ID"

# Second user registration
echo -e "\n2. Registering second user (voter)..."
VOTER_RESPONSE=$(curl -s -X POST http://localhost:8080/user/register \
-H "Content-Type: application/json" \
-d '{
   "username": "voter",
   "email": "voter@example.com",
   "password": "pass123",  
   "karma": 300
}')
echo "Voter Response: $VOTER_RESPONSE"
VOTER_ID=$(echo $VOTER_RESPONSE | jq -r '.ID')
echo "Voter ID: $VOTER_ID"

# First user creates subreddit
echo -e "\n3. Creating subreddit..."
SUBREDDIT_RESPONSE=$(curl -s -X POST http://localhost:8080/subreddit \
-H "Content-Type: application/json" \
-d '{
   "name": "testsubreddit",
   "description": "A test subreddit",
   "creatorId": "'$POSTER_ID'"
}')
echo "Subreddit Response: $SUBREDDIT_RESPONSE"
SUBREDDIT_ID=$(echo $SUBREDDIT_RESPONSE | jq -r '.ID')
echo "Subreddit ID: $SUBREDDIT_ID"

# First user creates post
echo -e "\n4. Creating post..."
POST_RESPONSE=$(curl -s -X POST http://localhost:8080/post \
-H "Content-Type: application/json" \
-d '{
   "title": "Test Post",
   "content": "This is a test post content",
   "authorId": "'$POSTER_ID'",
   "subredditId": "'$SUBREDDIT_ID'"
}')
echo "Post Response: $POST_RESPONSE"
POST_ID=$(echo $POST_RESPONSE | jq -r '.ID')
echo "Post ID: $POST_ID"

# Second user upvotes the post
echo -e "\n5. Second user upvoting the post..."
VOTE_RESPONSE=$(curl -s -X POST http://localhost:8080/post/vote \
-H "Content-Type: application/json" \
-d '{
   "userId": "'$VOTER_ID'",
   "postId": "'$POST_ID'",
   "isUpvote": true
}')
echo "Vote Response: $VOTE_RESPONSE"

# Check poster's updated karma
echo -e "\n6. Checking poster's updated profile..."
curl -s -X GET "http://localhost:8080/user/profile?userId=$POSTER_ID"