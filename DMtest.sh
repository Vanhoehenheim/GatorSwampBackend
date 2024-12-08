#!/bin/bash

# Set base URL
BASE_URL="http://localhost:8080"

# Function to extract ID from response
extract_id() {
    echo $1 | grep -o '"id":"[^"]*' | grep -o '[^"]*$'
}

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Function to print colored output
print_success() {
    echo -e "${GREEN}$1${NC}"
}

print_error() {
    echo -e "${RED}$1${NC}"
}

echo "Starting Direct Messaging Test Script..."

# Create User 1
echo "Creating User 1..."
USER1_RESPONSE=$(curl -s -X POST $BASE_URL/user/register \
    -H "Content-Type: application/json" \
    -d '{
        "username": "alice",
        "email": "alice@example.com",
        "password": "password123",
        "karma": 100
    }')

USER1_ID=$(extract_id "$USER1_RESPONSE")
if [ -z "$USER1_ID" ]; then
    print_error "Failed to create User 1"
    exit 1
fi
print_success "Created User 1 with ID: $USER1_ID"

# Create User 2
echo "Creating User 2..."
USER2_RESPONSE=$(curl -s -X POST $BASE_URL/user/register \
    -H "Content-Type: application/json" \
    -d '{
        "username": "bob",
        "email": "bob@example.com",
        "password": "password123",
        "karma": 100
    }')

USER2_ID=$(extract_id "$USER2_RESPONSE")
if [ -z "$USER2_ID" ]; then
    print_error "Failed to create User 2"
    exit 1
fi
print_success "Created User 2 with ID: $USER2_ID"

# Wait for user creation to complete
sleep 2

# Send a message from User 1 to User 2
echo "Sending message from User 1 to User 2..."
MESSAGE1_RESPONSE=$(curl -s -X POST $BASE_URL/messages \
    -H "Content-Type: application/json" \
    -d "{
        \"fromId\": \"$USER1_ID\",
        \"toId\": \"$USER2_ID\",
        \"content\": \"Hello Bob! How are you?\"
    }")

MESSAGE1_ID=$(extract_id "$MESSAGE1_RESPONSE")
if [ -z "$MESSAGE1_ID" ]; then
    print_error "Failed to send message from User 1"
else
    print_success "Sent message from User 1, Message ID: $MESSAGE1_ID"
fi

# Send a reply from User 2 to User 1
echo "Sending reply from User 2 to User 1..."
MESSAGE2_RESPONSE=$(curl -s -X POST $BASE_URL/messages \
    -H "Content-Type: application/json" \
    -d "{
        \"fromId\": \"$USER2_ID\",
        \"toId\": \"$USER1_ID\",
        \"content\": \"Hi Alice! I'm doing great, thanks for asking!\"
    }")

MESSAGE2_ID=$(extract_id "$MESSAGE2_RESPONSE")
if [ -z "$MESSAGE2_ID" ]; then
    print_error "Failed to send message from User 2"
else
    print_success "Sent message from User 2, Message ID: $MESSAGE2_ID"
fi

# Get all messages for User 1
echo "Getting all messages for User 1..."
USER1_MESSAGES=$(curl -s -X GET "$BASE_URL/messages?userId=$USER1_ID")
echo "User 1 Messages:"
echo "$USER1_MESSAGES" | json_pp

# Get all messages for User 2
echo "Getting all messages for User 2..."
USER2_MESSAGES=$(curl -s -X GET "$BASE_URL/messages?userId=$USER2_ID")
echo "User 2 Messages:"
echo "$USER2_MESSAGES" | json_pp

# Get conversation between User 1 and User 2
echo "Getting conversation between User 1 and User 2..."
CONVERSATION=$(curl -s -X GET "$BASE_URL/messages/conversation?user1=$USER1_ID&user2=$USER2_ID")
echo "Conversation:"
echo "$CONVERSATION" | json_pp

# Send another message from User 1
echo "Sending another message from User 1 to User 2..."
MESSAGE3_RESPONSE=$(curl -s -X POST $BASE_URL/messages \
    -H "Content-Type: application/json" \
    -d "{
        \"fromId\": \"$USER1_ID\",
        \"toId\": \"$USER2_ID\",
        \"content\": \"Would you like to meet for coffee?\"
    }")

MESSAGE3_ID=$(extract_id "$MESSAGE3_RESPONSE")
if [ -z "$MESSAGE3_ID" ]; then
    print_error "Failed to send second message from User 1"
else
    print_success "Sent second message from User 1, Message ID: $MESSAGE3_ID"
fi

# Get updated conversation
echo "Getting updated conversation..."
UPDATED_CONVERSATION=$(curl -s -X GET "$BASE_URL/messages/conversation?user1=$USER1_ID&user2=$USER2_ID")
echo "Updated Conversation:"
echo "$UPDATED_CONVERSATION" | json_pp

echo "Test script completed."