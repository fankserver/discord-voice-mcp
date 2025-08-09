#!/bin/bash

# MCP Inspector test script for Discord Voice MCP Server
# This script runs the MCP Inspector to debug the Discord bot

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}Discord Voice MCP Server - MCP Inspector${NC}"
echo "========================================="
echo ""

# Check if binary exists
if [ ! -f "./discord-voice-mcp" ]; then
    echo -e "${YELLOW}Building binary first...${NC}"
    go build -o discord-voice-mcp cmd/discord-voice-mcp/main.go
    echo -e "${GREEN}Binary built successfully!${NC}"
    echo ""
fi

# Check environment variables
if [ -z "$DISCORD_TOKEN" ]; then
    echo -e "${YELLOW}DISCORD_TOKEN not set${NC}"
    read -p "Enter Discord Bot Token: " DISCORD_TOKEN
    export DISCORD_TOKEN
fi

if [ -z "$DISCORD_USER_ID" ]; then
    echo -e "${YELLOW}DISCORD_USER_ID not set${NC}"
    read -p "Enter your Discord User ID: " DISCORD_USER_ID
    export DISCORD_USER_ID
fi

echo ""
echo -e "${BLUE}Configuration:${NC}"
echo "  Token: ${DISCORD_TOKEN:0:10}..."
echo "  User ID: $DISCORD_USER_ID"
echo ""

echo -e "${GREEN}Starting MCP Inspector...${NC}"
echo -e "${YELLOW}The inspector will open at http://localhost:5173${NC}"
echo -e "${YELLOW}Press Ctrl+C to stop${NC}"
echo ""

# Run the MCP Inspector with our binary and environment variables
npx @modelcontextprotocol/inspector \
    -e DISCORD_TOKEN="$DISCORD_TOKEN" \
    -e DISCORD_USER_ID="$DISCORD_USER_ID" \
    ./discord-voice-mcp