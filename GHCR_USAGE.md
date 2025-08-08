# Using Discord Voice MCP from GitHub Container Registry

## 🚀 Quick Start with Pre-built Images

No need to build! Pull and run directly from GitHub Container Registry:

```bash
# Pull the latest image (lightweight, downloads models on first run)
docker pull ghcr.io/yourusername/discord-voice-mcp:latest

# Run with your configuration
docker run -d \
  --name discord-voice-mcp \
  -e DISCORD_TOKEN="your-token" \
  -e DISCORD_CLIENT_ID="your-client-id" \
  -e TRANSCRIPTION_PROVIDER="vosk" \
  -e VOSK_MODEL_SIZE="small" \
  -v $(pwd)/sessions:/app/sessions \
  -v $(pwd)/models:/app/models \
  ghcr.io/yourusername/discord-voice-mcp:latest
```

## 📦 Available Images

All images use **Node.js 24** and support **linux/amd64** and **linux/arm64** platforms.

### Production Image
```bash
# Latest stable (downloads models at runtime)
ghcr.io/yourusername/discord-voice-mcp:latest

# Specific version
ghcr.io/yourusername/discord-voice-mcp:v1.0.0

# Branch builds
ghcr.io/yourusername/discord-voice-mcp:main
```

### Development Image
```bash
# Development image with hot reload
ghcr.io/yourusername/discord-voice-mcp:dev
```

## 🎯 Model Configuration

Models are downloaded automatically on first run based on environment variables:

### Vosk Configuration
```bash
docker run -d \
  -e TRANSCRIPTION_PROVIDER="vosk" \
  -e VOSK_MODEL_SIZE="small" \  # small, medium, or large
  -v $(pwd)/models:/app/models \
  ghcr.io/yourusername/discord-voice-mcp:latest
```

### Whisper Configuration
```bash
docker run -d \
  -e TRANSCRIPTION_PROVIDER="whisper" \
  -e WHISPER_MODEL_NAME="base.en" \  # tiny.en, base.en, small.en, medium.en, large-v3
  -v $(pwd)/models:/app/models \
  ghcr.io/yourusername/discord-voice-mcp:latest
```

### Google Cloud Configuration
```bash
docker run -d \
  -e TRANSCRIPTION_PROVIDER="google" \
  -v $(pwd)/credentials:/app/credentials:ro \
  -e GOOGLE_APPLICATION_CREDENTIALS="/app/credentials/google-key.json" \
  ghcr.io/yourusername/discord-voice-mcp:latest
```

## 🐳 Docker Compose with GHCR

### docker-compose.yml
```yaml
version: '3.8'

services:
  discord-voice-mcp:
    image: ghcr.io/yourusername/discord-voice-mcp:latest
    container_name: discord-voice-mcp
    restart: unless-stopped
    
    environment:
      # Discord Configuration
      DISCORD_TOKEN: ${DISCORD_TOKEN}
      DISCORD_CLIENT_ID: ${DISCORD_CLIENT_ID}
      
      # Provider Configuration
      TRANSCRIPTION_PROVIDER: ${PROVIDER:-vosk}
      VOSK_MODEL_SIZE: ${VOSK_MODEL:-small}
      WHISPER_MODEL_NAME: ${WHISPER_MODEL:-base.en}
      AUTO_DOWNLOAD_MODELS: "true"
    
    volumes:
      # Persist models to avoid re-downloading
      - ./models:/app/models
      - ./sessions:/app/sessions
      - ./exports:/app/exports
      - ./credentials:/app/credentials:ro
```

### .env file
```env
DISCORD_TOKEN=your_discord_token
DISCORD_CLIENT_ID=your_client_id
PROVIDER=vosk
VOSK_MODEL=small
```

### Run with Docker Compose
```bash
# Pull latest image
docker-compose pull

# Start container
docker-compose up -d

# View logs
docker-compose logs -f
```

## 💾 Model Persistence

Models are downloaded once and cached. Mount the models directory to persist them:

```bash
# First run - downloads models
docker run -d \
  -v $(pwd)/models:/app/models \
  -e TRANSCRIPTION_PROVIDER="vosk" \
  -e VOSK_MODEL_SIZE="large" \
  ghcr.io/yourusername/discord-voice-mcp:latest

# Subsequent runs - uses cached models
docker run -d \
  -v $(pwd)/models:/app/models \
  -e AUTO_DOWNLOAD_MODELS="false" \
  ghcr.io/yourusername/discord-voice-mcp:latest
```

## 🔄 Switching Providers

Change providers without rebuilding:

```bash
# Stop current container
docker stop discord-voice-mcp

# Start with different provider
docker run -d \
  --name discord-voice-mcp \
  -v $(pwd)/models:/app/models \
  -e TRANSCRIPTION_PROVIDER="whisper" \
  -e WHISPER_MODEL_NAME="small.en" \
  ghcr.io/yourusername/discord-voice-mcp:latest
```

## 📊 Model Sizes and Download Times

| Provider | Model | Size | Download Time* | RAM Usage |
|----------|-------|------|----------------|-----------|
| **Vosk** |
| | small | 40 MB | ~30s | ~200 MB |
| | medium | 128 MB | ~1m | ~400 MB |
| | large | 1.8 GB | ~5m | ~2 GB |
| **Whisper** |
| | tiny.en | 39 MB | ~30s | ~390 MB |
| | base.en | 74 MB | ~45s | ~500 MB |
| | small.en | 244 MB | ~2m | ~1 GB |
| | medium.en | 769 MB | ~4m | ~2.5 GB |
| | large-v3 | 1.5 GB | ~7m | ~4 GB |

*Download times vary based on connection speed

## 🔐 Authentication for Private Registry

If the repository is private:

```bash
# Login to GitHub Container Registry
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin

# Pull image
docker pull ghcr.io/yourusername/discord-voice-mcp:latest
```

## 🚦 Health Checks

The container includes built-in health checks:

```bash
# Check container health
docker inspect discord-voice-mcp --format='{{.State.Health.Status}}'

# View detailed health info
docker inspect discord-voice-mcp --format='{{json .State.Health}}' | jq
```

## 🎯 Multi-Architecture Support

Images support multiple architectures:

```bash
# Automatically pulls correct architecture
docker pull ghcr.io/yourusername/discord-voice-mcp:latest

# Explicitly specify platform
docker pull --platform linux/arm64 ghcr.io/yourusername/discord-voice-mcp:latest
```

## 📝 Environment Variables Reference

| Variable | Description | Default | Options |
|----------|-------------|---------|---------|
| `TRANSCRIPTION_PROVIDER` | Speech-to-text provider | `vosk` | `vosk`, `whisper`, `google` |
| `VOSK_MODEL_SIZE` | Vosk model to download | `small` | `small`, `medium`, `large` |
| `WHISPER_MODEL_NAME` | Whisper model to download | `base.en` | `tiny.en`, `base.en`, `small.en`, `medium.en`, `large-v3` |
| `AUTO_DOWNLOAD_MODELS` | Auto-download models on startup | `true` | `true`, `false` |
| `DISCORD_TOKEN` | Discord bot token | - | Required |
| `DISCORD_CLIENT_ID` | Discord client ID | - | Required |
| `LOG_LEVEL` | Logging verbosity | `info` | `debug`, `info`, `warn`, `error` |

## 🔍 Troubleshooting

### Models not downloading
```bash
# Check logs for download progress
docker logs discord-voice-mcp

# Ensure volume has write permissions
chmod 777 ./models

# Manually trigger download
docker run -it --rm \
  -v $(pwd)/models:/app/models \
  ghcr.io/yourusername/discord-voice-mcp:latest \
  /app/scripts/docker-entrypoint.sh echo "Download complete"
```

### Container exits immediately
```bash
# Check logs for errors
docker logs discord-voice-mcp

# Run interactively for debugging
docker run -it --rm \
  -e DISCORD_TOKEN="your-token" \
  ghcr.io/yourusername/discord-voice-mcp:latest \
  /bin/bash
```

### High memory usage
```bash
# Limit container resources
docker run -d \
  --memory="1g" \
  --cpus="1.0" \
  -e VOSK_MODEL_SIZE="small" \
  ghcr.io/yourusername/discord-voice-mcp:latest
```

## 🎉 Benefits of Using GHCR Images

1. **No Build Required** - Pre-built images ready to use
2. **Multi-architecture** - Works on AMD64 and ARM64
3. **Automatic Updates** - CI/CD builds on every push
4. **Model Flexibility** - Download only the models you need
5. **Version Control** - Tagged releases for stability
6. **Lightweight Base** - ~200MB base image, models on-demand

## 📚 Additional Resources

- [GitHub Packages Documentation](https://docs.github.com/en/packages)
- [Docker Hub Mirror](https://hub.docker.com/r/yourusername/discord-voice-mcp) (if configured)
- [Source Code](https://github.com/yourusername/discord-voice-mcp)