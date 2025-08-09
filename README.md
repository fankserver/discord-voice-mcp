# Discord Voice MCP Server

A pure MCP (Model Context Protocol) server for Discord voice channel transcription, written in Go. Control your Discord bot entirely through Claude Desktop or other MCP clients - no Discord commands needed.

## 📊 Specifications

| Component | Details |
|-----------|---------|
| Docker Image | **~12 MB** (minimal) / **~50 MB** (with ffmpeg) / **~500 MB** (whisper with GPU) |
| Binary Size | ~15 MB |
| Memory Usage | ~10-20 MB (base) / ~200-500 MB (with Whisper) |
| Language | Go 1.24 |
| MCP SDK | v0.2.0 (official Go SDK) |
| GPU Support | CUDA, ROCm, Vulkan (auto-detected) |

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

# Basic usage
docker run -i --rm \
  -e DISCORD_TOKEN="your-bot-token" \
  -e DISCORD_USER_ID="your-discord-user-id" \
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
└── session/
    └── manager.go       - Transcript session management

pkg/
└── transcriber/
    └── transcriber.go   - Transcription provider interface
```

### Key Design Principles

1. **MCP-First**: All control through MCP tools, no Discord text commands
2. **User-Centric**: Tools work with "your channel" via DISCORD_USER_ID
3. **Auto-Follow**: Bot can automatically follow you between channels
4. **Stateless Commands**: Each MCP tool call is independent
5. **Session-Based**: Transcripts organized by voice sessions

## 🔧 Technical Features

- **GPU Acceleration**: Automatic detection of NVIDIA/AMD/Intel GPUs for 5-10x faster transcription
- **Universal Image**: Single Docker image works on any hardware (GPU or CPU)
- **Lightweight**: 12MB minimal Docker image, 50MB with ffmpeg, 500MB with full GPU support
- **Fast Startup**: Sub-second initialization
- **Cross-Platform**: Compile for Windows, macOS, Linux, ARM
- **Concurrent**: Go's goroutines handle multiple audio streams efficiently
- **Clean Shutdown**: Proper resource cleanup with context cancellation
- **Structured Logging**: Configurable log levels for debugging

## 🛠️ Development

### Prerequisites
- Go 1.24+ 
- FFmpeg (for audio processing with normal Docker image)
- Discord Bot Token
- (Optional) Whisper.cpp and model file for real transcription

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
| `TRANSCRIBER_TYPE` | ❌ | Transcription provider (default: `mock`) | `mock`, `whisper`, `google` |
| `WHISPER_MODEL_PATH` | ⚠️ | Path to Whisper model (required if using `whisper`) | `/models/ggml-base.en.bin` |
| `AUDIO_BUFFER_DURATION_SEC` | ❌ | Buffer duration trigger (default: `2`) | `1`, `2`, `5` |
| `AUDIO_SILENCE_TIMEOUT_MS` | ❌ | Silence detection timeout (default: `1500`) | `500`, `1500`, `3000` |
| `AUDIO_MIN_BUFFER_MS` | ❌ | Minimum audio before transcription (default: `100`) | `50`, `100`, `200` |
| `WHISPER_USE_GPU` | ❌ | Enable GPU acceleration (default: `true`) | `true`, `false` |
| `CUDA_VISIBLE_DEVICES` | ❌ | Select NVIDIA GPU (default: `0`) | `0`, `1`, `all` |
| `HIP_VISIBLE_DEVICES` | ❌ | Select AMD GPU (default: `0`) | `0`, `1` |



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

## 🎤 Transcription Setup

### Mock Transcription (Default)
The server runs with mock transcription by default, which shows audio is being captured but doesn't transcribe actual content.

### Whisper Transcription with GPU Acceleration

The Whisper Docker image (`ghcr.io/fankserver/discord-voice-mcp:whisper`) includes **built-in GPU acceleration** for NVIDIA (CUDA), AMD (ROCm), and Intel/Other GPUs (Vulkan). The image automatically detects and uses available hardware acceleration, falling back to CPU if no GPU is available.

#### Supported Acceleration
- **NVIDIA GPUs**: CUDA acceleration (5-10x faster)
- **AMD GPUs**: ROCm acceleration (5-10x faster)  
- **Intel/Other GPUs**: Vulkan acceleration (3-5x faster)
- **CPU Fallback**: OpenBLAS acceleration (2-3x faster than baseline)

#### Download a Whisper Model
```bash
# Download base model (142 MB) - good balance of speed and accuracy
wget https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin -O models/ggml-base.bin

# Or download other model sizes:
# - tiny (39 MB) - fastest, lower accuracy
# - small (466 MB) - better accuracy
# - medium (1.5 GB) - high accuracy
# - large (3 GB) - best accuracy
```

#### Run with GPU Acceleration

**NVIDIA GPU:**
```bash
docker run -i --rm --gpus all \
  -e DISCORD_TOKEN="your-bot-token" \
  -e DISCORD_USER_ID="your-discord-user-id" \
  -e TRANSCRIBER_TYPE="whisper" \
  -e WHISPER_MODEL_PATH="/models/ggml-base.bin" \
  -v $(pwd)/models:/models:ro \
  ghcr.io/fankserver/discord-voice-mcp:whisper
```

**AMD GPU:**
```bash
docker run -i --rm \
  --device=/dev/kfd --device=/dev/dri --group-add video \
  -e DISCORD_TOKEN="your-bot-token" \
  -e DISCORD_USER_ID="your-discord-user-id" \
  -e TRANSCRIBER_TYPE="whisper" \
  -e WHISPER_MODEL_PATH="/models/ggml-base.bin" \
  -v $(pwd)/models:/models:ro \
  ghcr.io/fankserver/discord-voice-mcp:whisper
```

**Intel/Other GPUs (via Vulkan):**
```bash
docker run -i --rm --device=/dev/dri \
  -e DISCORD_TOKEN="your-bot-token" \
  -e DISCORD_USER_ID="your-discord-user-id" \
  -e TRANSCRIBER_TYPE="whisper" \
  -e WHISPER_MODEL_PATH="/models/ggml-base.bin" \
  -v $(pwd)/models:/models:ro \
  ghcr.io/fankserver/discord-voice-mcp:whisper
```

**CPU-Only (with OpenBLAS acceleration):**
```bash
docker run -i --rm \
  -e DISCORD_TOKEN="your-bot-token" \
  -e DISCORD_USER_ID="your-discord-user-id" \
  -e TRANSCRIBER_TYPE="whisper" \
  -e WHISPER_MODEL_PATH="/models/ggml-base.bin" \
  -v $(pwd)/models:/models:ro \
  ghcr.io/fankserver/discord-voice-mcp:whisper
```

### Google Speech-to-Text (Cloud)
The Google Speech-to-Text transcriber is a stub implementation that returns "Google transcription not implemented in PoC". Full implementation requires Google Cloud credentials integration.

## 🚀 GPU Acceleration Performance

The Whisper Docker image includes automatic GPU detection and acceleration:

| Hardware | Real-Time Factor | 10s Audio Processing Time | Speedup |
|----------|-----------------|--------------------------|---------|
| **CPU (no acceleration)** | 0.5x | ~5 seconds | Baseline |
| **CPU (OpenBLAS)** | 0.2x | ~2 seconds | 2-3x |
| **Intel GPU (Vulkan)** | 0.1x | ~1 second | 5x |
| **AMD GPU (ROCm)** | 0.05x | ~0.5 seconds | 10x |
| **NVIDIA GPU (CUDA)** | 0.05x | ~0.5 seconds | 10x |

*Lower Real-Time Factor is better. 0.1x means 10x faster than real-time.*

### Building with Custom GPU Support

```bash
# Build with all GPU backends (default)
docker build -f Dockerfile.whisper -t discord-voice-mcp:whisper .

# Build with CUDA only
docker build -f Dockerfile.whisper \
  --build-arg GGML_CUDA=ON \
  --build-arg GGML_ROCM=OFF \
  --build-arg GGML_VULKAN=OFF \
  -t discord-voice-mcp:cuda .

# Build CPU-only (smallest image)
docker build -f Dockerfile.whisper \
  --build-arg GGML_CUDA=OFF \
  --build-arg GGML_ROCM=OFF \
  --build-arg GGML_VULKAN=OFF \
  -t discord-voice-mcp:cpu .
```

## ⚙️ Audio Processing Configuration

The audio processing behavior can be customized using environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `AUDIO_BUFFER_DURATION_SEC` | `2` | Buffer duration in seconds before triggering transcription |
| `AUDIO_SILENCE_TIMEOUT_MS` | `1500` | Silence duration in milliseconds that triggers transcription |
| `AUDIO_MIN_BUFFER_MS` | `100` | Minimum audio duration in milliseconds before transcription |
| `WHISPER_LANGUAGE` | `auto` | Language code for Whisper transcription (e.g., "en", "de", "es", "auto") |
| `WHISPER_THREADS` | CPU cores | Number of threads for Whisper processing (defaults to runtime.NumCPU()) |
| `WHISPER_BEAM_SIZE` | `1` | Beam size for Whisper (1 = fastest, 5 = most accurate) |

### Examples

**Quick transcription with short pauses:**
```bash
# Trigger after 1 second buffer or 500ms silence
docker run -i --rm \
  -e DISCORD_TOKEN="your-bot-token" \
  -e DISCORD_USER_ID="your-discord-user-id" \
  -e AUDIO_BUFFER_DURATION_SEC="1" \
  -e AUDIO_SILENCE_TIMEOUT_MS="500" \
  -e AUDIO_MIN_BUFFER_MS="50" \
  ghcr.io/fankserver/discord-voice-mcp:latest
```

**Longer recordings with natural pauses:**
```bash
# Allow 3 second pauses, 5 second buffer
docker run -i --rm \
  -e DISCORD_TOKEN="your-bot-token" \
  -e DISCORD_USER_ID="your-discord-user-id" \
  -e AUDIO_BUFFER_DURATION_SEC="5" \
  -e AUDIO_SILENCE_TIMEOUT_MS="3000" \
  -e AUDIO_MIN_BUFFER_MS="200" \
  ghcr.io/fankserver/discord-voice-mcp:latest
```

**Multilingual transcription (preserve original language):**
```bash
# Auto-detect and preserve original language
docker run -i --rm \
  -e DISCORD_TOKEN="your-bot-token" \
  -e DISCORD_USER_ID="your-discord-user-id" \
  -e WHISPER_LANGUAGE="auto" \
  ghcr.io/fankserver/discord-voice-mcp:latest
```

**Force specific language:**
```bash
# Force German transcription only
docker run -i --rm \
  -e DISCORD_TOKEN="your-bot-token" \
  -e DISCORD_USER_ID="your-discord-user-id" \
  -e WHISPER_LANGUAGE="de" \
  ghcr.io/fankserver/discord-voice-mcp:latest
```

**Optimize for faster transcription (reduce delay):**
```bash
# Use more threads and smaller beam size for speed
docker run -i --rm \
  -e DISCORD_TOKEN="your-bot-token" \
  -e DISCORD_USER_ID="your-discord-user-id" \
  -e WHISPER_THREADS="8" \
  -e WHISPER_BEAM_SIZE="1" \
  -e AUDIO_SILENCE_TIMEOUT_MS="1000" \
  ghcr.io/fankserver/discord-voice-mcp:whisper
```

**Optimize for accuracy (slower but better quality):**
```bash
# Use default threads but larger beam size
docker run -i --rm \
  -e DISCORD_TOKEN="your-bot-token" \
  -e DISCORD_USER_ID="your-discord-user-id" \
  -e WHISPER_THREADS="4" \
  -e WHISPER_BEAM_SIZE="5" \
  ghcr.io/fankserver/discord-voice-mcp:whisper
```

## 🎯 Use Cases

### Personal Assistant
- **Meeting Transcription** - Record Discord voice meetings
- **Study Groups** - Capture study session discussions
- **Gaming Sessions** - Document strategy discussions
- **Podcast Recording** - Transcribe Discord podcasts

### Technical Benefits
- **Resource Efficiency** - Runs on Raspberry Pi or small VPS
- **Fast Deployment** - 12-50MB images deploy instantly
- **Cost Efficiency** - Small container footprint (12-50MB images)
- **Cross-Platform** - Single binary for any OS
- **Claude Integration** - Native MCP support

## ✅ Features

### Implemented
- ✅ **Pure MCP Control** - No Discord text commands needed
- ✅ **User-Centric Tools** - "Join my channel" functionality  
- ✅ **Auto-Follow Mode** - Bot follows you automatically
- ✅ **GPU Acceleration** - CUDA, ROCm, Vulkan support with auto-detection
- ✅ **Minimal Docker Images** - 12MB minimal, 50MB with ffmpeg, 500MB with GPU
- ✅ **Voice Connection** - Stable Discord voice handling
- ✅ **Session Management** - Organized transcript storage
- ✅ **Audio Pipeline** - Real-time PCM processing
- ✅ **MCP SDK Integration** - Using official Go SDK v0.2.0
- ✅ **Whisper Transcription** - Complete implementation with whisper.cpp + GPU acceleration

### In Progress
- 🚧 **Google Speech Integration** - Currently stub implementation
- 🚧 **Real-time Updates** - Live transcript streaming
- 🚧 **Multi-user Support** - Track multiple speakers

## 🔮 Roadmap

### Phase 1: Transcription (Current)
- [x] Integrate whisper.cpp for offline transcription (completed)
- [ ] Add Google Cloud Speech-to-Text (stub exists)
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