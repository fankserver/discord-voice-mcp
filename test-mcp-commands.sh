#!/bin/bash

# MCP Command Test Script for Discord Voice MCP Server
# This script tests the MCP server by sending various commands

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${GREEN}Discord Voice MCP Server - MCP Command Tester${NC}"
echo "=============================================="
echo ""

# Check environment
if [ -z "$DISCORD_TOKEN" ] || [ -z "$DISCORD_USER_ID" ]; then
    echo -e "${YELLOW}Please set DISCORD_TOKEN and DISCORD_USER_ID first${NC}"
    echo "  export DISCORD_TOKEN='your-bot-token'"
    echo "  export DISCORD_USER_ID='your-user-id'"
    exit 1
fi

IMAGE="${1:-discord-voice-mcp:latest}"

echo "Using image: $IMAGE"
echo ""

# Function to send MCP command and get response
send_mcp_command() {
    local command="$1"
    local description="$2"
    
    echo -e "${BLUE}Testing: $description${NC}"
    echo "Command: $command"
    echo ""
    
    # Send command and capture response (timeout after 5 seconds)
    echo "$command" | timeout 5 docker run -i --rm \
        -e DISCORD_TOKEN="$DISCORD_TOKEN" \
        -e DISCORD_USER_ID="$DISCORD_USER_ID" \
        -e LOG_LEVEL="error" \
        "$IMAGE" 2>/dev/null | grep -E '"jsonrpc"|"result"|"error"' | head -20 || true
    
    echo ""
    echo "---"
    echo ""
}

# Test 1: Initialize MCP connection
echo -e "${GREEN}Test 1: Initialize MCP Connection${NC}"
send_mcp_command \
    '{"jsonrpc":"2.0","method":"mcp_lifecycle/initialize","params":{"protocolVersion":"1.1.0","capabilities":{}},"id":1}' \
    "Initializing MCP protocol"

# Test 2: List available tools
echo -e "${GREEN}Test 2: List Available Tools${NC}"
send_mcp_command \
    '{"jsonrpc":"2.0","method":"tools/list","params":{},"id":2}' \
    "Listing available MCP tools"

# Test 3: Get bot status
echo -e "${GREEN}Test 3: Get Bot Status${NC}"
send_mcp_command \
    '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"get_bot_status","arguments":{}},"id":3}' \
    "Getting bot connection status"

# Test 4: Join user's voice channel
echo -e "${GREEN}Test 4: Join My Voice Channel${NC}"
send_mcp_command \
    '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"join_my_voice_channel","arguments":{}},"id":4}' \
    "Attempting to join your voice channel"

# Test 5: Enable follow mode
echo -e "${GREEN}Test 5: Enable Follow Mode${NC}"
send_mcp_command \
    '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"follow_me","arguments":{"enabled":true}},"id":5}' \
    "Enabling auto-follow mode"

echo -e "${GREEN}Tests completed!${NC}"
echo ""
echo "Note: Some commands may fail if:"
echo "  - The bot is not in the same Discord server as you"
echo "  - You are not in a voice channel"
echo "  - The bot doesn't have proper permissions"
echo ""
echo "To run the server interactively for manual testing:"
echo "  ./test-docker.sh"