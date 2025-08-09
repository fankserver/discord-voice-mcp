#!/bin/bash

# Test script for Discord Voice MCP Server Docker images
# Usage: ./test-docker.sh

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Discord Voice MCP Server - Docker Test${NC}"
echo "========================================"
echo ""

# Check for required environment variables
if [ -z "$DISCORD_TOKEN" ]; then
    echo -e "${YELLOW}Warning: DISCORD_TOKEN not set${NC}"
    echo "Please set your Discord bot token:"
    echo "  export DISCORD_TOKEN='your-bot-token'"
    echo ""
    read -p "Enter Discord Token: " DISCORD_TOKEN
fi

if [ -z "$DISCORD_USER_ID" ]; then
    echo -e "${YELLOW}Warning: DISCORD_USER_ID not set${NC}"
    echo "Please set your Discord user ID:"
    echo "  export DISCORD_USER_ID='your-user-id'"
    echo ""
    read -p "Enter Discord User ID: " DISCORD_USER_ID
fi

# Choose which image to test
echo ""
echo "Which Docker image would you like to test?"
echo "1) discord-voice-mcp:latest (201MB - with ffmpeg for audio processing)"
echo "2) discord-voice-mcp:minimal (12.4MB - without ffmpeg, limited functionality)"
echo ""
read -p "Enter choice [1-2]: " choice

case $choice in
    1)
        IMAGE="discord-voice-mcp:latest"
        echo -e "${GREEN}Testing with full image (includes ffmpeg)${NC}"
        ;;
    2)
        IMAGE="discord-voice-mcp:minimal"
        echo -e "${YELLOW}Testing with minimal image (no ffmpeg - audio processing may not work)${NC}"
        ;;
    *)
        echo -e "${RED}Invalid choice${NC}"
        exit 1
        ;;
esac

echo ""
echo "Starting MCP server with Docker image: $IMAGE"
echo "============================================"
echo ""
echo "Environment:"
echo "  DISCORD_TOKEN: ${DISCORD_TOKEN:0:10}..."
echo "  DISCORD_USER_ID: $DISCORD_USER_ID"
echo "  IMAGE: $IMAGE"
echo ""
echo -e "${YELLOW}Note: This will start the MCP server on stdin/stdout${NC}"
echo -e "${YELLOW}You can send MCP commands as JSON to interact with it${NC}"
echo -e "${YELLOW}Press Ctrl+C to stop${NC}"
echo ""
echo "Starting in 3 seconds..."
sleep 3

# Run the Docker container
docker run -it --rm \
    -e DISCORD_TOKEN="$DISCORD_TOKEN" \
    -e DISCORD_USER_ID="$DISCORD_USER_ID" \
    -e LOG_LEVEL="${LOG_LEVEL:-info}" \
    "$IMAGE"