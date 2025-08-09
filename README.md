# Discord Voice MCP Server

A pure MCP (Model Context Protocol) server for Discord voice channel transcription, written in Go. Control your Discord bot entirely through Claude Desktop or other MCP clients - no Discord commands needed.

## 📊 Specifications

| Component | Details |
|-----------|---------|
| Docker Image | **11 MB** (Alpine-based) |
| Binary Size | ~12 MB |
| Memory Usage | ~10-20 MB |
| Language | Go 1.24 |
| MCP SDK | v0.2.0 (official Go SDK) |

## 🚀 Quick Start

### Prerequisites

1. **Create a Discord Bot** at https://discord.com/developers/applications
2. **Get your Discord User ID** (Enable Developer Mode in Discord settings → Right-click your username → Copy User ID)
3. **Invite bot to your server** with the following permissions:

### Required Discord Bot Permissions

| Permission | Why It's Needed |
|------------|----------------|
| **View Channels** | See available voice channels |
| **Connect** | Join voice channels |
| **Speak** | Transmit audio in voice channels |
| **Use Voice Activity** | Detect when users are speaking |

Minimum permission integer: `3145728` (for OAuth2 URL generator)

### Discord Bot Setup

1. Go to [Discord Developer Portal](https://discord.com/developers/applications)
2. Create a new application and bot
3. Copy the bot token
4. Generate an invite link:
   - Go to OAuth2 → URL Generator
   - Select scopes: `bot`
   - Select permissions: `View Channels`, `Connect`, `Speak`, `Use Voice Activity`
   - Or use this template URL (replace `YOUR_CLIENT_ID`):
   ```
   https://discord.com/api/oauth2/authorize?client_id=YOUR_CLIENT_ID&permissions=3145728&scope=bot
   ```

### Run with Docker (Recommended)

```bash
# Run the MCP server with your user ID
docker run -i --rm \
  -e DISCORD_TOKEN="your-bot-token" \
  -e DISCORD_USER_ID="your-discord-user-id" \
  ghcr.io/fankserver/discord-voice-mcp:latest

# Or with auto-follow enabled by default
docker run -i --rm \
  -e DISCORD_TOKEN="your-bot-token" \
  -e DISCORD_USER_ID="your-discord-user-id" \
  -e AUTO_FOLLOW="true" \
  ghcr.io/fankserver/discord-voice-mcp:latest
```

### Configure Claude Desktop

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "discord-voice": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-e", "DISCORD_TOKEN=your-bot-token",
        "-e", "DISCORD_USER_ID=your-discord-user-id",
        "ghcr.io/fankserver/discord-voice-mcp:latest"
      ]
    }
  }
}
```

### Cross-Compile for Any Platform

```bash
# Windows
GOOS=windows GOARCH=amd64 go build -o discord-voice-mcp.exe

# macOS
GOOS=darwin GOARCH=amd64 go build -o discord-voice-mcp-mac

# Linux ARM (Raspberry Pi)
GOOS=linux GOARCH=arm64 go build -o discord-voice-mcp-arm
```

## 📦 Architecture

This is a pure MCP server that connects to Discord. All control is through MCP tools - no Discord commands.

```
cmd/discord-voice-mcp/
└── main.go              - Entry point, MCP server startup

internal/
├── mcp/
│   └── server.go        - MCP tool implementations
├── bot/
│   └── bot.go           - Discord voice connection handler
├── audio/
│   └── processor.go     - Audio capture & processing
├── session/
│   └── manager.go       - Transcript session management
└── transcriber/
    └── interface.go     - Transcription provider interface
```

### Key Design Principles

1. **MCP-First**: All control through MCP tools, no Discord text commands
2. **User-Centric**: Tools work with "your channel" via DISCORD_USER_ID
3. **Auto-Follow**: Bot can automatically follow you between channels
4. **Stateless Commands**: Each MCP tool call is independent
5. **Session-Based**: Transcripts organized by voice sessions

## 🔧 Technical Features

- **Lightweight**: 11MB Docker image using Alpine Linux
- **Fast Startup**: Sub-second initialization
- **Cross-Platform**: Compile for Windows, macOS, Linux, ARM
- **Concurrent**: Go's goroutines handle multiple audio streams efficiently
- **Clean Shutdown**: Proper resource cleanup with context cancellation
- **Structured Logging**: Configurable log levels for debugging

## 🛠️ Development

### Prerequisites
- Go 1.22+ 
- FFmpeg (for audio processing)
- Discord Bot Token

### Build & Test

```bash
# Get dependencies
go mod download

# Run tests
go test ./...

# Build with optimizations
go build -ldflags="-w -s" -o discord-voice-mcp

# Check binary size
ls -lh discord-voice-mcp
# -rwxr-xr-x  1 user  staff  15M  discord-voice-mcp
```

### Environment Variables

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|  
| `DISCORD_TOKEN` | ✅ | Bot token from Discord Developer Portal | `MTIz...` |
| `DISCORD_USER_ID` | ✅ | Your Discord user ID for "my channel" commands | `123456789012345678` |
| `LOG_LEVEL` | ❌ | Logging verbosity (default: `info`) | `debug`, `info`, `warn`, `error` |



## 🔌 MCP Tools

### Available Commands

| Tool | Description | Parameters |
|------|-------------|------------|
| `join_my_voice_channel` | Join the voice channel where you are | None |
| `follow_me` | Auto-follow you between voice channels | `enabled`: boolean |
| `join_specific_channel` | Join a specific channel by ID | `guildId`, `channelId` |
| `leave_voice_channel` | Leave current voice channel | None |
| `get_bot_status` | Get bot connection status | None |
| `list_sessions` | List all transcription sessions | None |
| `get_transcript` | Get transcript for a session | `sessionId` |
| `export_session` | Export session to JSON | `sessionId` |

### Example Usage in Claude Desktop

```
# Join your current voice channel
"Use the join_my_voice_channel tool"

# Enable auto-follow so bot follows you
"Enable follow_me to track my movements"

# Check bot status
"What's the bot status?"

# Get transcripts
"List all sessions and show me the latest transcript"
```

## 🎯 Use Cases

### Personal Assistant
- **Meeting Transcription** - Record Discord voice meetings
- **Study Groups** - Capture study session discussions
- **Gaming Sessions** - Document strategy discussions
- **Podcast Recording** - Transcribe Discord podcasts

### Technical Benefits
- **Resource Efficiency** - Runs on Raspberry Pi or small VPS
- **Fast Deployment** - 11MB images deploy instantly
- **Cost Savings** - 95% less memory usage
- **Cross-Platform** - Single binary for any OS
- **Claude Integration** - Native MCP support

## ✅ Features

### Implemented
- ✅ **Pure MCP Control** - No Discord text commands needed
- ✅ **User-Centric Tools** - "Join my channel" functionality  
- ✅ **Auto-Follow Mode** - Bot follows you automatically
- ✅ **Minimal Docker Images** - Only 11MB
- ✅ **Voice Connection** - Stable Discord voice handling
- ✅ **Session Management** - Organized transcript storage
- ✅ **Audio Pipeline** - Real-time PCM processing
- ✅ **MCP SDK Integration** - Using official Go SDK v0.2.0

### In Progress
- 🚧 **Transcription** - Whisper/Google Speech integration
- 🚧 **Real-time Updates** - Live transcript streaming
- 🚧 **Multi-user Support** - Track multiple speakers

## 🔮 Roadmap

### Phase 1: Transcription (Current)
- [ ] Integrate whisper.cpp for offline transcription
- [ ] Add Google Cloud Speech-to-Text
- [ ] Implement real-time streaming transcripts

### Phase 2: Enhanced Features
- [ ] Speaker diarization (who said what)
- [ ] Sentiment analysis
- [ ] Keyword detection and alerts
- [ ] Multi-language support

### Phase 3: Scaling
- [ ] Kubernetes deployment manifests
- [ ] Multi-guild support
- [ ] Webhook integrations
- [ ] Transcript search API


## 🤝 Contributing

Contributions are welcome! Areas of interest:
- Transcription provider implementations (Whisper, Google Speech)
- Additional MCP tools and features
- Performance optimizations
- Documentation improvements

Please ensure all tests pass before submitting PRs:
```bash
go test ./...
```

## 📄 License

MIT