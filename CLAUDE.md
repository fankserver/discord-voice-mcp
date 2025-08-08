# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Discord Voice MCP Server - A high-performance Discord voice transcription server with MCP (Model Context Protocol) integration, written in Go. This is a complete rewrite from Node.js achieving 99.5% Docker image size reduction (11MB vs 2.35GB).

### Key Technologies
- **Go 1.24** - Primary language with standard project layout
- **MCP Go SDK** (v0.2.0) - Official Model Context Protocol SDK (experimental)
- **discordgo** - Discord API wrapper
- **gopus** - Opus audio codec (requires CGO)
- **logrus** - Structured logging

## Architecture

### Project Structure
```
cmd/discord-voice-mcp/    # Main application entry point
internal/                 # Private application code
├── audio/               # Audio processing pipeline (Opus decoding, PCM buffering)
├── bot/                 # Discord bot management (voice channel handling)
├── mcp/                 # MCP server using official SDK
└── session/             # Transcript session management
pkg/transcriber/         # Public transcriber interface (Whisper/Google/Mock)
```

### Key Components & Responsibilities

1. **MCP Server (`internal/mcp/server.go`)**
   - Uses official MCP Go SDK with typed tool handlers
   - Tools: join_voice_channel, leave_voice_channel, get_transcript, list_sessions, export_session, get_bot_status
   - Returns structured `CallToolResultFor[T]` responses

2. **Audio Pipeline (`internal/audio/processor.go`)**
   - Decodes Opus packets to PCM using gopus
   - Buffers audio per user (SSRC-based identification)
   - Triggers transcription at 2-second buffer threshold
   - Constants: 48kHz sample rate, 2 channels, 960 frame size

3. **Discord Bot (`internal/bot/bot.go`)**
   - Manages Discord voice connections
   - Handles voice state updates
   - Commands: !join, !leave, !status

4. **Session Manager (`internal/session/manager.go`)**
   - Thread-safe transcript storage
   - UUID-based session identification
   - JSON export functionality

## Common Development Commands

### Building & Running
```bash
# Build optimized binary (12MB)
make build

# Run with MCP mode
./discord-voice-mcp -mcp -token "YOUR_TOKEN"

# Run tests
make test

# Format code
make fmt

# Lint (requires golangci-lint)
make lint
```

### Docker Operations
```bash
# Build normal image (199MB with ffmpeg)
docker build -t discord-voice-mcp:latest .

# Build minimal image (11MB, no ffmpeg)
docker build -f Dockerfile.minimal -t discord-voice-mcp:minimal .

# Compare sizes
make size-compare

# Cross-platform builds
make build-all
```

### Testing Specific Components
```bash
# Test single package
go test -v ./internal/audio

# Test with race detection
go test -race ./...

# Test with coverage
go test -coverprofile=coverage.txt ./...

# Update test fixtures (for dialect tests)
env UPDATE_EXPECT=1 go test
```

## Critical Implementation Details

### CGO Requirements
The gopus library requires CGO for Opus codec support. Static builds must use:
```bash
CGO_ENABLED=1 go build -a -tags netgo -ldflags '-w -s -extldflags "-static"'
```

### MCP SDK Considerations
- SDK is v0.2.0 and marked as experimental/unstable
- Tool handlers must match signature: `func(context.Context, *ServerSession, *CallToolParamsFor[In]) (*CallToolResultFor[Out], error)`
- Use generics for type safety: `mcp.AddTool[InputType, OutputType](...)`

### Audio Processing Flow
1. Discord sends Opus packets via `VoiceConnection.OpusRecv` channel
2. Processor decodes to PCM (48kHz, stereo)
3. PCM buffered per user until 2 seconds accumulated
4. Transcriber called (currently Mock, Whisper/Google pending)
5. Transcript added to session with timestamp

### Error Handling Patterns
- Safe type assertions to prevent panics (check `ok` return)
- No busy-wait loops (removed `default` cases in select statements)
- Nil checks for Discord guild state
- Structured logging with logrus for debugging

## Environment Variables
```bash
DISCORD_TOKEN=             # Required: Bot token
DISCORD_CHANNEL_ID=        # Optional: Auto-join channel
DISCORD_GUILD_ID=          # Optional: Guild for auto-join
LOG_LEVEL=                 # debug, info, warn, error (default: info)
```

## Docker Build Optimization
Both Dockerfiles use:
- Multi-stage builds (builder not in final image)
- Alpine Linux base for small size
- Static binary compilation with CGO
- hadolint ignore directives for unpinned packages (DL3018)

Normal image includes ffmpeg for audio processing.
Minimal image uses scratch base with only binary and CA certificates.

## GitHub Actions Workflows
- **CI**: Tests on Go 1.23/1.24, linting, security scanning
- **Docker Build**: Multi-arch builds (amd64/arm64) for normal and minimal variants
- **Release**: Publishes Docker images and platform binaries on tag

Images published to:
- GitHub Container Registry: `ghcr.io/fankserver/discord-voice-mcp`
- Docker Hub (optional): Requires DOCKERHUB_USERNAME and DOCKERHUB_TOKEN secrets

## Known Issues & Limitations
- MCP SDK is experimental - expect breaking changes
- Opus library shows compilation warnings (harmless, from upstream)
- Transcriber implementations (Whisper/Google) not yet complete
- Audio currently uses mock transcription

## Performance Characteristics
- Binary size: 12MB (static with all dependencies)
- Memory usage: ~10MB idle
- Startup time: <100ms
- Docker minimal: 12.4MB
- Docker with ffmpeg: 201MB