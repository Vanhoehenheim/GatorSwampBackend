#!/bin/bash

# Array of random post titles and content
titles=("Tech Post" "Science News" "Gaming Discussion" "Movie Review" "Book Talk")
contents=("Technology discussion" "Scientific breakthrough" "Gaming strategies" "Movie analysis" "Book recommendations")

# Function to generate random string for usernames
generate_random_string() {
    cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w ${1:-8} | head -n 1
}

# Arrays to store IDs
declare -a user_ids
declare -a post_ids
subreddit_id=""

# Register a new user
register_user() {
    local username="user_$(generate_random_string 5)"
    local email="${username}@example.com"
    
    echo "Registering user: $username"
    local response=$(curl -s -X POST http://localhost:8080/user/register \
    -H "Content-Type: application/json" \
    -d '{
       "username": "'$username'",
       "email": "'$email'",
       "password": "pass123",
       "karma": 300
    }')
    
    if [[ $response == *"ID"* ]]; then
        local user_id=$(echo $response | jq -r '.ID')
        user_ids+=("$user_id")
        echo "Created user with ID: $user_id"
    else
        echo "Error creating user: $response"
    fi
}

# Create subreddit
create_subreddit() {
    local creator_id=$1
    echo "Creating subreddit..."
    local response=$(curl -s -X POST http://localhost:8080/subreddit \
    -H "Content-Type: application/json" \
    -d '{
       "name": "mainsubreddit",
       "description": "Main testing subreddit",
       "creatorId": "'$creator_id'"
    }')
    
    if [[ $response == *"ID"* ]]; then
        subreddit_id=$(echo $response | jq -r '.ID')
        echo "Created subreddit with ID: $subreddit_id"
    else
        echo "Error creating subreddit: $response"
    fi
}

# Create a post
create_post() {
    local author_id=$1
    local s_id=$2
    local rand_index=$((RANDOM % ${#titles[@]}))
    local title="${titles[$rand_index]}"
    local content="${contents[$rand_index]}"
    
    echo -e "\nCreating post..."
    local response=$(curl -s -X POST http://localhost:8080/post \
    -H "Content-Type: application/json" \
    -d '{
   "title": "'"$title"'",
   "content": "'"$content"'",
   "authorId": "'"$author_id"'",
   "subredditId": "'"$s_id"'"
}')
    echo "Post Response: $response"
    
    if [[ $response == *"ID"* ]]; then
        local post_id=$(echo $response | jq -r '.ID')
        echo "Post ID: $post_id"
        post_ids+=("$post_id")
        return 0
    else
        echo "Error creating post: $response"
        return 1
    fi
}

# Vote on a post
vote_on_post() {
    local voter_id=$1
    local post_id=$2
    local is_upvote=$3
    
    if [[ -n $post_id ]]; then
        echo -e "\nUser $voter_id voting on post $post_id..."
        local response=$(curl -s -X POST http://localhost:8080/post/vote \
        -H "Content-Type: application/json" \
        -d '{
   "userId": "'"$voter_id"'",
   "postId": "'"$post_id"'",
   "isUpvote": '$is_upvote'
}')
        echo "Vote Response: $response"
        sleep 1
    fi
}

# Main simulation
echo "Starting user activity simulation..."

# Create users
for i in {1..3}; do
    register_user
    sleep 1
done

# Create initial subreddit with first user
if [ ${#user_ids[@]} -gt 0 ]; then
    create_subreddit "${user_ids[0]}"
    sleep 1
fi

# Create posts and simulate voting
if [ -n "$subreddit_id" ]; then
    # Let the subreddit creator make posts
    create_post "${user_ids[0]}" "$subreddit_id"
    sleep 1
            
    # Have other users vote on the post
    if [ $? -eq 0 ] && [ ${#post_ids[@]} -gt 0 ]; then
        for ((i=1; i<${#user_ids[@]}; i++)); do
            vote_on_post "${user_ids[$i]}" "${post_ids[-1]}" true
            sleep 1
        done
    fi
fi

# Check final karma for all users
echo -e "\nFinal karma check for all users:"
for user_id in "${user_ids[@]}"; do
    echo "Checking karma for user $user_id"
    curl -s -X GET "http://localhost:8080/user/profile?userId=$user_id"
    echo -e "\n"
done

echo "Simulation completed!"