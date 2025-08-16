# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Discord Voice MCP Server - A high-performance Discord voice transcription server with MCP (Model Context Protocol) integration, written in Go.

### Key Technologies
- **Go 1.24** - Primary language with standard project layout
- **MCP Go SDK** (v0.2.0) - Official Model Context Protocol SDK (experimental)
- **discordgo** (v0.28.1) - Discord API wrapper
- **gopus** - Opus audio codec (requires CGO)
- **logrus** (v1.9.3) - Structured logging
- **uuid** (v1.6.0) - UUID generation for sessions
- **godotenv** (v1.5.1) - Environment variable loading

### Transcription Options
- **faster-whisper** - 4x faster than OpenAI Whisper, prebuilt wheels, GPU support (CUDA/ROCm)
- **whisper.cpp** - Official implementation with GPU acceleration (CUDA/Vulkan/Metal)
- **Mock transcriber** - Development/testing without actual transcription

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
   - Tools: join_my_voice_channel, follow_me, join_specific_channel, leave_voice_channel, get_transcript, list_sessions, export_session, get_bot_status
   - User-centric tools for joining configured user's channel and auto-following
   - Returns structured `CallToolResultFor[T]` responses

2. **Audio Pipeline (`internal/audio/processor.go`)**
   - Decodes Opus packets to PCM using gopus
   - Buffers audio per user (SSRC-based identification)
   - Triggers transcription at 2-second buffer threshold
   - Constants: 48kHz sample rate, 2 channels, 960 frame size

3. **Discord Bot (`internal/bot/bot.go`)**
   - Manages Discord voice connections
   - Handles voice state updates and speaking events
   - Auto-follow functionality for configured users
   - Uses SimpleSSRCManager for deterministic SSRC-to-user mapping
   
4. **SimpleSSRCManager (`internal/bot/simple_ssrc_manager.go`)**
   - Deterministic SSRC-to-user mapping using ONLY VoiceSpeakingUpdate events
   - Thread-safe with RWMutex protection
   - Returns "Unknown-XXXXX" for unmapped SSRCs (no guessing)
   - Clears all mappings on channel switch
   - Known limitation: Users already speaking when bot joins need to toggle mic once

5. **Session Manager (`internal/session/manager.go`)**
   - Thread-safe transcript storage
   - UUID-based session identification
   - JSON export functionality

## Common Development Commands

### Building & Running
```bash
# Build optimized binary
go build -o discord-voice-mcp ./cmd/discord-voice-mcp

# Run (always MCP mode)
./discord-voice-mcp -token "YOUR_TOKEN"

# Run with specific transcriber
./discord-voice-mcp -token "YOUR_TOKEN" -transcriber whisper -whisper-model "path/to/model"

# Run tests
go test ./...

# Format code
go fmt ./...

# Test specific package
go test -v ./internal/audio
```

### Docker Operations

#### Fast Deployment Options (Under 5 minutes)
```bash
# Fastest deployment: FasterWhisper with GPU acceleration
docker build -f Dockerfile.faster-whisper -t discord-voice-mcp:faster-whisper .
docker run --gpus all -e DISCORD_TOKEN="YOUR_TOKEN" discord-voice-mcp:faster-whisper

# AMD GPU via ROCm (7x performance improvement)
docker build -f Dockerfile.rocm -t discord-voice-mcp:rocm .
docker run --device=/dev/kfd --device=/dev/dri -e DISCORD_TOKEN="YOUR_TOKEN" discord-voice-mcp:rocm

# ARM64 Jetson support
docker build -f Dockerfile.jetson -t discord-voice-mcp:jetson .
docker run --gpus all -e DISCORD_TOKEN="YOUR_TOKEN" discord-voice-mcp:jetson

# Docker Compose for multiple variants
docker-compose up discord-voice-mcp-faster  # FasterWhisper
docker-compose up discord-voice-mcp-rocm    # AMD GPU
docker-compose up discord-voice-mcp-cuda    # NVIDIA CUDA
```

#### Traditional Build Options (Optimized with ccache)
```bash
# Build normal image (~50MB with ffmpeg, ccache optimized)
docker build -t discord-voice-mcp:latest .

# Build NVIDIA CUDA optimized image
docker build -f Dockerfile.whisper-cuda -t discord-voice-mcp:cuda .

# Run with environment variables
docker run -e DISCORD_TOKEN="YOUR_TOKEN" -e DISCORD_USER_ID="USER_ID" discord-voice-mcp:latest
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
3. PCM buffered per user with intelligent multi-tier triggers:
   - Ultra-responsive: 300ms speech + 400ms silence (rapid exchanges)
   - Target duration: 1.5 seconds (optimal for conversations)
   - Max duration: 3 seconds (prevents long waits)
4. Speaker-aware dispatcher routes to per-user queues
5. Parallel transcription with context preservation
6. Transcript added to session with timestamp

### Audio Configuration (Ultra-Responsive Defaults)
System defaults are optimized for Discord multi-speaker conversations.
Override via environment variables if needed:
- `VAD_MIN_SPEECH_MS`: Minimum speech duration (default: 300ms)
- `VAD_SENTENCE_END_SILENCE_MS`: Sentence boundary detection (default: 400ms)
- `VAD_TARGET_DURATION_MS`: Target segment duration (default: 1500ms)
- `VAD_MAX_SEGMENT_DURATION_S`: Maximum segment duration (default: 3s)

### SSRC Mapping Approach
- **Deterministic only** - Uses ONLY VoiceSpeakingUpdate events from Discord
- **No guessing** - Never attempts to deduce mappings from audio patterns
- **Discord API limitation** - VoiceSpeakingUpdate only fires when user starts speaking AFTER bot joins
- **User experience** - Shows "Unknown-XXXXX" for unmapped users (they need to toggle mic once)
- **Clean implementation** - SimpleSSRCManager replaces complex confidence-based system (removed 900+ lines)

### Error Handling Patterns
- Safe type assertions to prevent panics (check `ok` return)
- No busy-wait loops (removed `default` cases in select statements)
- Nil checks for Discord guild state
- Structured logging with logrus for debugging

## Environment Variables

### Core Configuration
```bash
DISCORD_TOKEN=             # Required: Bot token
DISCORD_USER_ID=           # Optional: User ID for "my channel" and follow features
TRANSCRIBER_TYPE=          # Optional: mock, whisper, faster-whisper, google (default: mock)
LOG_LEVEL=                 # debug, info, warn, error (default: info)

# Audio processing configuration (ultra-responsive defaults)
VAD_MIN_SPEECH_MS=300          # Minimum speech before transcription (default: 300ms)
VAD_SENTENCE_END_SILENCE_MS=400 # Silence for sentence boundaries (default: 400ms)
VAD_TARGET_DURATION_MS=1500     # Target segment duration (default: 1500ms)
VAD_MAX_SEGMENT_DURATION_S=3    # Maximum segment duration (default: 3s)
VAD_ENERGY_DROP_RATIO=0.20      # Energy drop sensitivity (default: 0.20)
VAD_MIN_ENERGY_LEVEL=70         # Minimum energy threshold (default: 70)
```

### Whisper.cpp Configuration
```bash
WHISPER_MODEL_PATH=        # Required: Path to whisper model file
WHISPER_USE_GPU=           # true/false: Enable GPU acceleration (default: true in Docker)
WHISPER_GPU_TYPE=          # cuda, vulkan, metal: GPU backend type
WHISPER_LANGUAGE=          # Language code (default: auto)
WHISPER_THREADS=           # Thread count (default: CPU cores)
WHISPER_BEAM_SIZE=         # 1 (fast) to 5 (accurate), default: 1
```

### FasterWhisper Configuration (Fastest Deployment)
```bash
FASTER_WHISPER_MODEL=      # Model name: tiny.en, base.en, small.en, medium.en, large-v3
FASTER_WHISPER_DEVICE=     # auto, cpu, cuda, rocm (default: auto)
FASTER_WHISPER_COMPUTE_TYPE= # float16, int8_float16, int8 (default: float16)
FASTER_WHISPER_LANGUAGE=   # Language code (default: auto)
FASTER_WHISPER_BEAM_SIZE=  # 1-5, default: 1 for speed
```

## Docker Build Optimization

### Fast Deployment Images (Under 5 minutes)
- **Dockerfile.faster-whisper**: FasterWhisper with GPU support (~2GB, 4x faster transcription)
- **Dockerfile.rocm**: AMD GPU via ROCm prebuilt images (7x performance improvement)
- **Dockerfile.jetson**: ARM64 Jetson with TensorRT optimization
- **Dockerfile.whisper-cuda**: NVIDIA CUDA maximum performance

### Traditional Build Images (ccache optimized)
- **Dockerfile**: Alpine base with ffmpeg (~50MB, ccache enabled)
- **Dockerfile.whisper-cuda**: NVIDIA CUDA maximum performance (ccache enabled)

All images feature:
- Multi-stage builds with ccache for 80-90% faster rebuilds
- BuildKit cache mounts for persistent compilation cache
- Non-root user for security
- GPU acceleration support

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
- Google transcriber is stub implementation (returns "not implemented")
- Whisper transcriber fully implemented but requires whisper.cpp binary
- Default uses mock transcription for development
- **Discord API limitation**: Users already speaking when bot joins need to toggle mic once for identification
  - This is a fundamental Discord API constraint - VoiceSpeakingUpdate only fires on state change
  - Bot displays "Unknown-XXXXX" for unmapped users until they toggle mic

## Performance Characteristics
- Binary size: ~15MB (static with all dependencies)
- Memory usage: ~10MB idle
- Startup time: <100ms
- Docker minimal: ~12MB
- Docker with ffmpeg: ~50MB