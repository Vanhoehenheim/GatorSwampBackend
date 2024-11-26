#!/bin/bash

# Store IDs
USER_ID="a3c9fddb-b486-4f3a-97e9-5020b021a58c"
SUBREDDIT_ID="51570c2f-846e-45c0-889b-9d4a2ab94acf"
POST_ID="f6fb2fa2-abd3-4c2b-99c0-cb7e31e9ec99"

echo "1. Joining the subreddit..."
curl -X POST http://localhost:8080/subreddit/members \
-H "Content-Type: application/json" \
-d '{
    "userId": "'$USER_ID'",
    "subredditId": "'$SUBREDDIT_ID'"
}'

echo -e "\n2. Getting subreddit details to verify membership..."
curl -X GET "http://localhost:8080/subreddit?id=$SUBREDDIT_ID"

echo -e "\n3. Getting user feed (after joining subreddit)..."
curl -X GET "http://localhost:8080/user/feed?userId=$USER_ID&limit=10"

echo -e "\n4. Attempting to vote again..."
curl -X POST http://localhost:8080/post/vote \
-H "Content-Type: application/json" \
-d '{
    "userId": "'$USER_ID'",
    "postId": "'$POST_ID'",
    "isUpvote": true
}'

echo -e "\n5. Getting post details to verify vote..."
curl -X GET "http://localhost:8080/post?id=$POST_ID"

echo -e "\n6. Checking user profile again..."
curl -X GET "http://localhost:8080/user/profile?userId=$USER_ID"