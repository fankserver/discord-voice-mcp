#!/bin/bash

echo "🚀 Discord Voice MCP Setup Script"
echo "================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check Node.js version
echo "Checking Node.js version..."
NODE_VERSION=$(node -v | cut -d'v' -f2 | cut -d'.' -f1)
if [ "$NODE_VERSION" -lt 18 ]; then
    echo -e "${RED}❌ Node.js 18+ is required. Current version: $(node -v)${NC}"
    exit 1
fi
echo -e "${GREEN}✅ Node.js version: $(node -v)${NC}"
echo ""

# Install npm dependencies
echo "Installing npm dependencies..."
npm install
echo -e "${GREEN}✅ Dependencies installed${NC}"
echo ""

# Create necessary directories
echo "Creating directories..."
mkdir -p models
mkdir -p temp
mkdir -p sessions
mkdir -p exports
mkdir -p credentials
echo -e "${GREEN}✅ Directories created${NC}"
echo ""

# Setup .env file
if [ ! -f .env ]; then
    echo "Creating .env file from template..."
    cp .env.example .env
    echo -e "${YELLOW}⚠️  Please edit .env file with your Discord bot token and other credentials${NC}"
else
    echo -e "${GREEN}✅ .env file already exists${NC}"
fi
echo ""

# Provider selection
echo "Which transcription providers would you like to set up?"
echo "1) Vosk (Free, Offline, Recommended)"
echo "2) Whisper.cpp (Free, Offline, High Accuracy)"
echo "3) Google Cloud Speech (Cloud-based, Requires API Key)"
echo "4) All providers"
echo ""
read -p "Enter your choice (1-4): " PROVIDER_CHOICE

case $PROVIDER_CHOICE in
    1)
        echo ""
        echo "Setting up Vosk..."
        ./scripts/install-vosk.sh
        ;;
    2)
        echo ""
        echo "Setting up Whisper.cpp..."
        ./scripts/install-whisper.sh
        ;;
    3)
        echo ""
        echo "Setting up Google Cloud Speech..."
        echo -e "${YELLOW}Please place your Google Cloud service account JSON key in ./credentials/google-cloud-key.json${NC}"
        echo -e "${YELLOW}Update GOOGLE_APPLICATION_CREDENTIALS in .env file${NC}"
        ;;
    4)
        echo ""
        echo "Setting up all providers..."
        ./scripts/install-vosk.sh
        ./scripts/install-whisper.sh
        echo -e "${YELLOW}For Google Cloud: Place your service account JSON key in ./credentials/google-cloud-key.json${NC}"
        ;;
    *)
        echo -e "${RED}Invalid choice${NC}"
        ;;
esac

echo ""
echo "================================="
echo -e "${GREEN}🎉 Setup complete!${NC}"
echo ""
echo "Next steps:"
echo "1. Edit .env file with your Discord bot token"
echo "2. Configure your chosen transcription provider credentials"
echo "3. Add the MCP server to Claude Desktop configuration"
echo "4. Run 'npm start' to start the MCP server"
echo ""
echo "Claude Desktop configuration path:"
echo "  macOS: ~/Library/Application Support/Claude/claude_desktop_config.json"
echo "  Windows: %APPDATA%\\Claude\\claude_desktop_config.json"
echo "  Linux: ~/.config/Claude/claude_desktop_config.json"