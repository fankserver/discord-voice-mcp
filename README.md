# Discord Voice MCP Server

A Model Context Protocol (MCP) server that bridges Discord voice channels with Claude Code, enabling voice-based development workflows. Speak your ideas in Discord, and they appear as context in Claude Code for refinement and implementation.

## Features

- 🎙️ **Real-time voice transcription** from Discord voice channels
- 🔄 **Multiple transcription providers**: Vosk (offline), Whisper.cpp (offline), Google Cloud Speech
- 💾 **Session management**: Save and retrieve transcription sessions
- 🔌 **Claude Code integration**: Direct MCP integration for seamless workflow
- 🎯 **Provider switching**: Hot-swap between transcription providers on the fly

## Quick Start

### 🐳 Docker Installation (Recommended)

#### Option 1: Use Pre-built Images from GitHub Container Registry

```bash
# Pull and run directly (no build needed!)
docker run -d \
  --name discord-voice-mcp \
  -e DISCORD_TOKEN="your-token" \
  -e DISCORD_CLIENT_ID="your-client-id" \
  -e TRANSCRIPTION_PROVIDER="vosk" \
  -v $(pwd)/models:/app/models \
  ghcr.io/fankserver/discord-voice-mcp:latest
```

See [GHCR_USAGE.md](GHCR_USAGE.md) for using pre-built images.

#### Option 2: Build Locally with Setup Script

```bash
# Clone repository
git clone https://github.com/yourusername/discord-voice-mcp.git
cd discord-voice-mcp

# Run interactive setup
chmod +x docker-setup.sh
./docker-setup.sh --interactive
```

See [DOCKER_README.md](DOCKER_README.md) for detailed Docker instructions.

### 📦 Manual Installation

Prerequisites:
- Node.js 18+
- Discord Bot Token
- Claude Desktop

```bash
# Clone and setup
chmod +x scripts/setup.sh
./scripts/setup.sh
```

3. Configure your Discord bot:
   - Create a bot at https://discord.com/developers/applications
   - Copy the bot token to `.env` file
   - Invite bot to your server with voice permissions

4. Choose and install a transcription provider:
   - **Vosk** (Recommended): `./scripts/install-vosk.sh`
   - **Whisper.cpp**: `./scripts/install-whisper.sh`
   - **Google Cloud**: Add credentials to `./credentials/google-cloud-key.json`

5. Configure Claude Desktop:
   - Copy the configuration from `claude_desktop_config.json`
   - Add to your Claude Desktop config file:
     - macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
     - Windows: `%APPDATA%\Claude\claude_desktop_config.json`
     - Linux: `~/.config/Claude/claude_desktop_config.json`

6. Start the MCP server:
```bash
npm start
```

## Usage in Claude Code

### Start a transcription session:
```
You: Start a voice transcription session for our meeting

Claude: I'll start a voice transcription session for you.
[Uses tool: start_voice_session]
✅ Started voice session: "meeting"
Now transcribing voice channel audio...
```

### Join a Discord voice channel:
```
You: Join the voice channel [channel-id] in server [guild-id]

Claude: [Uses tool: join_voice_channel]
🔊 Joined voice channel
Ready to transcribe audio using vosk
```

### Get transcript:
```
You: Show me the transcript from the last 5 minutes

Claude: [Uses tool: get_transcript with lastNMinutes: 5]
[10:23:45] You: So we need a real-time data pipeline
[10:23:52] You: It should handle WebSocket connections
[10:24:03] You: And process events using a queue system
```

### Switch transcription provider:
```
You: Switch to whisper for better accuracy

Claude: [Uses tool: switch_provider]
🔄 Switched transcription provider:
vosk → whisper
```

## Transcription Providers

### Vosk (Default)
- ✅ Completely free and offline
- ✅ Low resource usage
- ✅ Real-time streaming
- ⚡ 50MB - 2GB models

### Whisper.cpp
- ✅ Free and offline
- ✅ High accuracy
- ✅ 100+ languages
- 💾 140MB - 1.5GB models

### Google Cloud Speech
- ☁️ Cloud-based processing
- ✅ Real-time streaming
- ✅ Speaker diarization
- 💰 Pay-per-use (free tier available)

## Project Structure

```
discord-voice-mcp/
├── src/
│   ├── mcp-server.js           # MCP server implementation
│   ├── discord-bot.js          # Discord voice handling
│   ├── session-manager.js      # Transcript session management
│   └── services/
│       └── transcription.js    # Multi-provider transcription
├── scripts/
│   ├── setup.sh               # Main setup script
│   ├── install-vosk.sh        # Vosk model installer
│   └── install-whisper.sh     # Whisper.cpp installer
├── models/                    # Transcription models
├── sessions/                  # Saved sessions
├── .env                      # Configuration
└── package.json
```

## MCP Tools Available

| Tool | Description |
|------|-------------|
| `start_voice_session` | Start a new transcription session |
| `stop_voice_session` | Stop the current session |
| `get_transcript` | Retrieve session transcript |
| `join_voice_channel` | Join a Discord voice channel |
| `leave_voice_channel` | Leave the current channel |
| `switch_provider` | Change transcription provider |
| `list_active_sessions` | Show all sessions |
| `clear_transcript` | Clear session transcript |

## Configuration

### Environment Variables (.env)

```env
# Discord
DISCORD_TOKEN=your_bot_token
DISCORD_CLIENT_ID=your_client_id
DISCORD_GUILD_ID=your_guild_id

# Transcription Provider (vosk, whisper, google)
TRANSCRIPTION_PROVIDER=vosk

# Vosk
VOSK_MODEL_PATH=./models/vosk-model-en-us-0.22

# Whisper.cpp
WHISPER_MODEL_PATH=./models/ggml-base.en.bin
WHISPER_EXECUTABLE=./whisper.cpp/main

# Google Cloud
GOOGLE_APPLICATION_CREDENTIALS=./credentials/google-cloud-key.json
```

## Troubleshooting

### MCP Server not connecting
- Check Claude Desktop config path
- Verify absolute paths in configuration
- Check logs with `npm start`

### No audio being transcribed
- Ensure bot has voice channel permissions
- Check that bot is not self-deafened
- Verify transcription provider is initialized

### Provider-specific issues

**Vosk:**
- Download appropriate model size for your system
- Check model path in .env

**Whisper.cpp:**
- Ensure build tools are installed (gcc/clang, make)
- Run `make clean && make` in whisper.cpp directory

**Google Cloud:**
- Verify service account JSON key
- Check API quotas and billing

## Development

### Run in development mode:
```bash
npm run dev
```

### Test Discord bot only:
```bash
npm run test-bot
```

### Export session transcripts:
Sessions are automatically saved to `./sessions/` directory as JSON files.

## Privacy & Security

- 🔒 Never commit `.env` file with tokens
- 🎙️ Inform users when recording
- 💾 Sessions stored locally only
- 🔐 Use environment variables for all credentials

## License

MIT

## Contributing

Contributions welcome! Please submit PRs with:
- New transcription providers
- Additional MCP tools
- Performance improvements
- Bug fixes

## Support

For issues and questions:
- Open an issue on GitHub
- Check existing sessions in `./sessions/` directory
- Review logs for error messages