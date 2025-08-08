# Discord Voice MCP Server

A Model Context Protocol (MCP) server that bridges Discord voice channels with Claude Code, enabling voice-based development workflows. Speak in Discord, and your words become context in Claude Code.

## Quick Start

### Option 1: Docker (Recommended)

```bash
# Clone the repository
git clone https://github.com/fankserver/discord-voice-mcp.git
cd discord-voice-mcp

# Copy environment file and add your Discord credentials
cp .env.example .env
# Edit .env with your DISCORD_TOKEN and DISCORD_CLIENT_ID

# Run with Docker Compose
docker-compose up -d

# View logs
docker-compose logs -f
```

### Option 2: Pre-built Docker Image

```bash
# Pull and run from GitHub Container Registry
docker run -d \
  --name discord-voice-mcp \
  -e DISCORD_TOKEN="your-token" \
  -e DISCORD_CLIENT_ID="your-client-id" \
  -v discord-models:/app/models \
  ghcr.io/fankserver/discord-voice-mcp:latest
```

### Option 3: Local Installation

```bash
# Prerequisites: Node.js 18+, ffmpeg
npm install

# Set up Discord credentials in .env
cp .env.example .env

# Start the server
npm start
```

## Claude Desktop Integration

Add to your Claude Desktop config:
- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`
- Linux: `~/.config/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "discord-voice": {
      "command": "docker",
      "args": ["exec", "-i", "discord-voice-mcp", "node", "/app/src/mcp-server.js"]
    }
  }
}
```

For local installation, use:
```json
{
  "mcpServers": {
    "discord-voice": {
      "command": "node",
      "args": ["/path/to/discord-voice-mcp/src/mcp-server.js"]
    }
  }
}
```

## Usage in Claude Code

```
You: Start transcribing the voice channel
Claude: [Starts voice session and joins channel]

You: Show me what was said in the last 5 minutes
Claude: [Retrieves transcript with timestamps]

You: Stop transcribing
Claude: [Stops session and leaves channel]
```

## Discord Bot Setup

1. Create a bot at https://discord.com/developers/applications
2. Get your bot token and client ID
3. Invite bot to server with voice permissions:
   ```
   https://discord.com/api/oauth2/authorize?client_id=YOUR_CLIENT_ID&permissions=3145728&scope=bot
   ```

## Configuration

### Environment Variables

```env
# Required
DISCORD_TOKEN=your_bot_token
DISCORD_CLIENT_ID=your_client_id

# Optional
DISCORD_GUILD_ID=specific_guild_id
TRANSCRIPTION_PROVIDER=whisper  # whisper, vosk, or google
LOG_LEVEL=info
```

### Transcription Providers

**Whisper.cpp** (Default)
- Offline, high accuracy
- Auto-downloads model on first run
- Models: tiny, base, small, medium, large

**Vosk** (Alternative)
- Offline, lower resource usage
- Real-time streaming
- Models: small (40MB), medium (128MB), large (1.8GB)

**Google Cloud** (Cloud-based)
- Requires service account key in `credentials/google-cloud-key.json`
- Best accuracy, requires internet

## Development

```bash
# Run with hot reload
npm run dev

# Run with Docker development mode
docker-compose -f docker-compose.dev.yml up

# Access container shell
docker exec -it discord-voice-mcp /bin/bash
```

## MCP Tools Available

| Tool | Description |
|------|-------------|
| `start_voice_session` | Start transcription session |
| `stop_voice_session` | Stop current session |
| `get_transcript` | Retrieve transcript |
| `join_voice_channel` | Join Discord channel |
| `leave_voice_channel` | Leave current channel |
| `list_active_sessions` | Show all sessions |
| `switch_provider` | Change transcription provider |
| `clear_transcript` | Clear session data |

## Troubleshooting

### Bot not joining channel
- Check bot has voice permissions in Discord
- Verify DISCORD_TOKEN is correct
- Ensure bot is in the server

### No transcription
- Check Docker logs: `docker-compose logs`
- Verify models downloaded: `docker exec discord-voice-mcp ls /app/models`
- Try smaller model if memory limited

### MCP not connecting
- Verify Claude Desktop config path is correct
- Use absolute paths in configuration
- Restart Claude Desktop after config changes

## Architecture

```
Discord Voice → Audio Stream → Transcription Provider → MCP Server → Claude Code
```

- **Discord Bot**: Captures voice channel audio using discord.js
- **Transcription**: Converts audio to text (Whisper/Vosk/Google)
- **MCP Server**: Exposes transcripts via Model Context Protocol
- **Session Manager**: Handles transcript storage and retrieval

## License

MIT

## Contributing

Pull requests welcome! Please keep changes focused and test before submitting.