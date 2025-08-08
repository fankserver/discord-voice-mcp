#!/bin/bash

# Discord Voice MCP Docker Setup Script
# Usage: ./docker-setup.sh [OPTIONS]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
PROVIDER="vosk"
VOSK_MODEL="small"
WHISPER_MODEL="base.en"
ACTION="setup"
COMPOSE_FILE="docker-compose.yml"
ENV_FILE=".env"

# Function to print colored output
print_color() {
    color=$1
    message=$2
    echo -e "${color}${message}${NC}"
}

# Function to print usage
usage() {
    cat << EOF
Discord Voice MCP Docker Setup Script

USAGE:
    ./docker-setup.sh [OPTIONS]

OPTIONS:
    -h, --help              Show this help message
    -p, --provider TYPE     Set transcription provider (vosk|whisper|google) [default: vosk]
    -v, --vosk-model SIZE   Set Vosk model size (small|medium|large) [default: small]
    -w, --whisper-model MODEL Set Whisper model (tiny.en|base.en|small.en|medium.en|large) [default: base.en]
    -t, --token TOKEN       Set Discord bot token
    -c, --client-id ID      Set Discord client ID
    -g, --guild-id ID       Set Discord guild ID
    -a, --action ACTION     Action to perform (setup|build|run|stop|logs|clean) [default: setup]
    --cpu-limit CPUS        Set CPU limit (e.g., 2) [default: 2]
    --memory-limit MEM      Set memory limit (e.g., 2G) [default: 2G]
    --interactive           Run setup interactively
    --dev                   Run in development mode with hot reload

ACTIONS:
    setup   - Interactive setup and configuration
    build   - Build Docker image only
    run     - Run the container (builds if needed)
    stop    - Stop the running container
    logs    - Show container logs
    clean   - Remove container and images
    shell   - Open shell in running container

EXAMPLES:
    # Interactive setup
    ./docker-setup.sh --interactive

    # Quick setup with Vosk
    ./docker-setup.sh -p vosk -t YOUR_BOT_TOKEN -c CLIENT_ID -g GUILD_ID -a run

    # Setup with Whisper
    ./docker-setup.sh -p whisper -w small.en -t YOUR_BOT_TOKEN -a run

    # Setup with Google Cloud
    ./docker-setup.sh -p google -t YOUR_BOT_TOKEN -a run

    # View logs
    ./docker-setup.sh -a logs

    # Stop container
    ./docker-setup.sh -a stop

EOF
    exit 0
}

# Function for interactive setup
interactive_setup() {
    print_color "$BLUE" "🚀 Discord Voice MCP Docker Interactive Setup"
    echo "============================================="
    echo ""

    # Provider selection
    print_color "$YELLOW" "Select transcription provider:"
    echo "1) Vosk (Free, Offline, Lightweight)"
    echo "2) Whisper.cpp (Free, Offline, High Accuracy)"
    echo "3) Google Cloud (Cloud-based, Requires API Key)"
    read -p "Enter choice (1-3): " provider_choice

    case $provider_choice in
        1)
            PROVIDER="vosk"
            print_color "$YELLOW" "\nSelect Vosk model size:"
            echo "1) Small (40 MB) - Fast, lower accuracy"
            echo "2) Medium (128 MB) - Balanced"
            echo "3) Large (1.8 GB) - Best accuracy"
            read -p "Enter choice (1-3): " vosk_choice
            case $vosk_choice in
                1) VOSK_MODEL="small" ;;
                2) VOSK_MODEL="medium" ;;
                3) VOSK_MODEL="large" ;;
                *) VOSK_MODEL="small" ;;
            esac
            ;;
        2)
            PROVIDER="whisper"
            print_color "$YELLOW" "\nSelect Whisper model:"
            echo "1) tiny.en (39 MB) - Fastest"
            echo "2) base.en (74 MB) - Balanced"
            echo "3) small.en (244 MB) - Better accuracy"
            echo "4) medium.en (769 MB) - High accuracy"
            echo "5) large-v3 (1.5 GB) - Best accuracy, multilingual"
            read -p "Enter choice (1-5): " whisper_choice
            case $whisper_choice in
                1) WHISPER_MODEL="tiny.en" ;;
                2) WHISPER_MODEL="base.en" ;;
                3) WHISPER_MODEL="small.en" ;;
                4) WHISPER_MODEL="medium.en" ;;
                5) WHISPER_MODEL="large-v3" ;;
                *) WHISPER_MODEL="base.en" ;;
            esac
            ;;
        3)
            PROVIDER="google"
            print_color "$YELLOW" "\nGoogle Cloud Speech requires a service account key."
            echo "Place your key file at: ./credentials/google-cloud-key.json"
            read -p "Press Enter when ready..."
            ;;
        *)
            PROVIDER="vosk"
            VOSK_MODEL="small"
            ;;
    esac

    # Discord configuration
    print_color "$YELLOW" "\nDiscord Bot Configuration:"
    read -p "Enter Discord Bot Token: " DISCORD_TOKEN
    read -p "Enter Discord Client ID: " DISCORD_CLIENT_ID
    read -p "Enter Discord Guild ID (optional): " DISCORD_GUILD_ID

    # Resource limits
    print_color "$YELLOW" "\nResource Limits (press Enter for defaults):"
    read -p "CPU limit (default: 2): " cpu_input
    CPU_LIMIT=${cpu_input:-2}
    read -p "Memory limit (default: 2G): " mem_input
    MEMORY_LIMIT=${mem_input:-2G}

    # Save configuration
    save_env_file
}

# Function to save .env file
save_env_file() {
    cat > $ENV_FILE << EOF
# Discord Configuration
DISCORD_TOKEN=${DISCORD_TOKEN}
DISCORD_CLIENT_ID=${DISCORD_CLIENT_ID}
DISCORD_GUILD_ID=${DISCORD_GUILD_ID}

# Provider Configuration
PROVIDER=${PROVIDER}
VOSK_MODEL=${VOSK_MODEL}
WHISPER_MODEL=${WHISPER_MODEL}

# Resource Limits
CPU_LIMIT=${CPU_LIMIT:-2}
MEMORY_LIMIT=${MEMORY_LIMIT:-2G}
CPU_RESERVE=${CPU_RESERVE:-0.5}
MEMORY_RESERVE=${MEMORY_RESERVE:-512M}

# Logging
LOG_LEVEL=${LOG_LEVEL:-info}

# WebSocket Port
WS_PORT=${WS_PORT:-3001}
EOF

    print_color "$GREEN" "✅ Configuration saved to .env"
}

# Function to build Docker image
docker_build() {
    print_color "$BLUE" "🔨 Building Docker image..."
    
    # Load .env if exists
    if [ -f "$ENV_FILE" ]; then
        export $(cat $ENV_FILE | grep -v '^#' | xargs)
    fi

    # For optimized build, we don't need build args anymore
    docker-compose build

    print_color "$GREEN" "✅ Docker image built successfully"
}

# Function to run container
docker_run() {
    print_color "$BLUE" "🚀 Starting Discord Voice MCP..."
    
    # Check if .env exists
    if [ ! -f "$ENV_FILE" ]; then
        print_color "$RED" "❌ No .env file found. Run with --interactive or provide credentials"
        exit 1
    fi

    # Build if image doesn't exist
    if ! docker images | grep -q "discord-voice-mcp"; then
        docker_build
    fi

    docker-compose up -d
    
    print_color "$GREEN" "✅ Container started successfully"
    print_color "$YELLOW" "📝 View logs with: ./docker-setup.sh -a logs"
}

# Function to stop container
docker_stop() {
    print_color "$BLUE" "⏹️ Stopping Discord Voice MCP..."
    docker-compose down
    print_color "$GREEN" "✅ Container stopped"
}

# Function to show logs
docker_logs() {
    docker-compose logs -f --tail=100
}

# Function to clean up
docker_clean() {
    print_color "$YELLOW" "🧹 Cleaning up Docker resources..."
    docker-compose down -v
    docker rmi discord-voice-mcp:latest 2>/dev/null || true
    print_color "$GREEN" "✅ Cleanup complete"
}

# Function to open shell
docker_shell() {
    docker-compose exec discord-voice-mcp /bin/bash
}

# Function to generate Claude config
generate_claude_config() {
    CONTAINER_ID=$(docker ps -q -f name=discord-voice-mcp)
    
    if [ -z "$CONTAINER_ID" ]; then
        print_color "$RED" "❌ Container not running. Start it first with: ./docker-setup.sh -a run"
        exit 1
    fi

    cat > claude_desktop_docker_config.json << EOF
{
  "mcpServers": {
    "discord-voice": {
      "command": "docker",
      "args": [
        "exec",
        "-i",
        "discord-voice-mcp",
        "node",
        "/app/src/mcp-server.js"
      ]
    }
  }
}
EOF

    print_color "$GREEN" "✅ Claude Desktop configuration saved to: claude_desktop_docker_config.json"
    print_color "$YELLOW" "Add this to your Claude Desktop configuration file"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            usage
            ;;
        -p|--provider)
            PROVIDER="$2"
            shift 2
            ;;
        -v|--vosk-model)
            VOSK_MODEL="$2"
            shift 2
            ;;
        -w|--whisper-model)
            WHISPER_MODEL="$2"
            shift 2
            ;;
        -t|--token)
            DISCORD_TOKEN="$2"
            shift 2
            ;;
        -c|--client-id)
            DISCORD_CLIENT_ID="$2"
            shift 2
            ;;
        -g|--guild-id)
            DISCORD_GUILD_ID="$2"
            shift 2
            ;;
        -a|--action)
            ACTION="$2"
            shift 2
            ;;
        --cpu-limit)
            CPU_LIMIT="$2"
            shift 2
            ;;
        --memory-limit)
            MEMORY_LIMIT="$2"
            shift 2
            ;;
        --interactive)
            ACTION="interactive"
            shift
            ;;
        --dev)
            COMPOSE_FILE="docker-compose.dev.yml"
            shift
            ;;
        *)
            print_color "$RED" "Unknown option: $1"
            usage
            ;;
    esac
done

# Main execution
case $ACTION in
    setup|interactive)
        interactive_setup
        docker_build
        docker_run
        generate_claude_config
        ;;
    build)
        docker_build
        ;;
    run)
        docker_run
        generate_claude_config
        ;;
    stop)
        docker_stop
        ;;
    logs)
        docker_logs
        ;;
    clean)
        docker_clean
        ;;
    shell)
        docker_shell
        ;;
    *)
        print_color "$RED" "Unknown action: $ACTION"
        usage
        ;;
esac