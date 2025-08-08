# Discord Voice MCP - Docker Deployment

Run the Discord Voice MCP Server in a Docker container with one-command setup!

## 🚀 Quick Start

### One-Command Setup

```bash
# Make script executable
chmod +x docker-setup.sh

# Interactive setup (recommended for first time)
./docker-setup.sh --interactive

# Or quick setup with arguments
./docker-setup.sh -p vosk -t YOUR_BOT_TOKEN -c CLIENT_ID -g GUILD_ID -a run
```

## 📦 Docker Setup Script Options

### Basic Usage

```bash
./docker-setup.sh [OPTIONS]
```

### Available Options

| Option | Description | Default |
|--------|-------------|---------|
| `-h, --help` | Show help message | - |
| `-p, --provider TYPE` | Set provider (vosk/whisper/google) | vosk |
| `-v, --vosk-model SIZE` | Vosk model (small/medium/large) | small |
| `-w, --whisper-model MODEL` | Whisper model (tiny.en/base.en/small.en/medium.en/large) | base.en |
| `-t, --token TOKEN` | Discord bot token | - |
| `-c, --client-id ID` | Discord client ID | - |
| `-g, --guild-id ID` | Discord guild ID | - |
| `-a, --action ACTION` | Action to perform | setup |
| `--cpu-limit CPUS` | CPU limit (e.g., 2) | 2 |
| `--memory-limit MEM` | Memory limit (e.g., 2G) | 2G |
| `--interactive` | Run interactive setup | - |
| `--dev` | Development mode with hot reload | - |

### Available Actions

| Action | Description |
|--------|-------------|
| `setup` | Interactive setup and configuration |
| `build` | Build Docker image only |
| `run` | Run the container (builds if needed) |
| `stop` | Stop the running container |
| `logs` | Show container logs |
| `clean` | Remove container and images |
| `shell` | Open shell in running container |

## 🎯 Usage Examples

### 1. Interactive Setup (Recommended)

```bash
./docker-setup.sh --interactive
```

This will guide you through:
- Choosing transcription provider
- Selecting model size
- Configuring Discord credentials
- Setting resource limits

### 2. Quick Setup with Vosk

```bash
./docker-setup.sh \
  -p vosk \
  -v small \
  -t "YOUR_DISCORD_TOKEN" \
  -c "YOUR_CLIENT_ID" \
  -g "YOUR_GUILD_ID" \
  -a run
```

### 3. Setup with Whisper

```bash
./docker-setup.sh \
  -p whisper \
  -w base.en \
  -t "YOUR_DISCORD_TOKEN" \
  -c "YOUR_CLIENT_ID" \
  -a run
```

### 4. Setup with Google Cloud

```bash
# First, place your service account key
mkdir -p credentials
cp /path/to/google-key.json credentials/google-cloud-key.json

# Then run
./docker-setup.sh \
  -p google \
  -t "YOUR_DISCORD_TOKEN" \
  -c "YOUR_CLIENT_ID" \
  -a run
```

### 5. Development Mode

```bash
./docker-setup.sh --dev --interactive
```

Features:
- Hot reload on code changes
- Debug port exposed (9229)
- Source code mounted as volume

### 6. Container Management

```bash
# View logs
./docker-setup.sh -a logs

# Stop container
./docker-setup.sh -a stop

# Open shell in container
./docker-setup.sh -a shell

# Clean up everything
./docker-setup.sh -a clean
```

## 🐳 Docker Compose Usage

### Manual Docker Compose

```bash
# Create .env file
cp .env.example .env
# Edit .env with your credentials

# Build and run
docker-compose up -d

# View logs
docker-compose logs -f

# Stop
docker-compose down
```

### Custom Build Arguments

```bash
# Build with specific models
docker-compose build \
  --build-arg PROVIDER=whisper \
  --build-arg WHISPER_MODEL=small.en

# Run with custom resource limits
CPU_LIMIT=4 MEMORY_LIMIT=4G docker-compose up -d
```

## 🔧 Claude Desktop Integration

After running the container, the script automatically generates `claude_desktop_docker_config.json`:

```json
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
```

Add this to your Claude Desktop configuration file:
- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`
- Linux: `~/.config/Claude/claude_desktop_config.json`

## 📊 Model Sizes and Performance

### Vosk Models

| Model | Size | Accuracy | Speed | RAM Usage |
|-------|------|----------|-------|-----------|
| Small | 40 MB | Good | Fast | ~200 MB |
| Medium | 128 MB | Better | Fast | ~400 MB |
| Large | 1.8 GB | Best | Moderate | ~2 GB |

### Whisper Models

| Model | Size | Accuracy | Speed | RAM Usage |
|-------|------|----------|-------|-----------|
| tiny.en | 39 MB | Fair | Very Fast | ~390 MB |
| base.en | 74 MB | Good | Fast | ~500 MB |
| small.en | 244 MB | Better | Moderate | ~1 GB |
| medium.en | 769 MB | Great | Slow | ~2.5 GB |
| large-v3 | 1.5 GB | Best | Very Slow | ~4 GB |

## 🔍 Troubleshooting

### Container won't start

```bash
# Check logs
docker-compose logs discord-voice-mcp

# Verify .env file exists
cat .env

# Check Docker resources
docker system df
```

### No audio transcription

```bash
# Check bot permissions in Discord
# Ensure bot is not self-deafened
# Verify provider is initialized
./docker-setup.sh -a logs | grep "provider initialized"
```

### High CPU/Memory usage

```bash
# Adjust limits in .env
CPU_LIMIT=1
MEMORY_LIMIT=1G

# Restart container
docker-compose restart
```

### Permission errors

```bash
# Fix volume permissions
sudo chown -R 1000:1000 sessions/ exports/ logs/
```

## 🛠️ Advanced Configuration

### Custom Models

Place custom models in `./custom-models/` and mount them:

```yaml
volumes:
  - ./custom-models:/app/custom-models:ro
```

### Multi-Container Setup

Run multiple instances with different providers:

```bash
# Vosk instance
COMPOSE_PROJECT_NAME=vosk docker-compose up -d

# Whisper instance
COMPOSE_PROJECT_NAME=whisper PROVIDER=whisper docker-compose up -d
```

### Resource Monitoring

```bash
# Real-time stats
docker stats discord-voice-mcp

# Resource usage over time
docker-compose logs | grep "Memory usage"
```

## 🔐 Security Best Practices

1. **Never commit .env file**
   ```bash
   echo ".env" >> .gitignore
   ```

2. **Use Docker secrets for production**
   ```yaml
   secrets:
     discord_token:
       external: true
   ```

3. **Run as non-root user** (already configured)

4. **Limit network access**
   ```yaml
   networks:
     - internal
   ```

5. **Regular updates**
   ```bash
   docker-compose pull
   docker-compose build --no-cache
   ```

## 📝 Environment Variables

Complete list of environment variables:

```env
# Required
DISCORD_TOKEN=             # Discord bot token
DISCORD_CLIENT_ID=         # Discord application client ID

# Optional
DISCORD_GUILD_ID=          # Specific guild ID
PROVIDER=vosk              # vosk|whisper|google
VOSK_MODEL=small           # small|medium|large
WHISPER_MODEL=base.en      # Model name
LOG_LEVEL=info             # debug|info|warn|error
WS_PORT=3001              # WebSocket port
CPU_LIMIT=2               # CPU cores limit
MEMORY_LIMIT=2G           # Memory limit
```

## 🚦 Health Checks

The container includes health checks:

```bash
# Check health status
docker inspect discord-voice-mcp --format='{{.State.Health.Status}}'

# View health check logs
docker inspect discord-voice-mcp --format='{{json .State.Health}}' | jq
```

## 📦 Backup and Restore

### Backup sessions

```bash
# Backup
docker run --rm -v discord_sessions:/data -v $(pwd):/backup alpine tar czf /backup/sessions-backup.tar.gz -C /data .

# Restore
docker run --rm -v discord_sessions:/data -v $(pwd):/backup alpine tar xzf /backup/sessions-backup.tar.gz -C /data
```

## 🎉 Success!

Once running, you can:
1. Join a Discord voice channel
2. Ask Claude: "Start a voice transcription session"
3. Speak in Discord
4. Ask Claude: "Show me the transcript"

The Docker container handles all dependencies and model downloads automatically!