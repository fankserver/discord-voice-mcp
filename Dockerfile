# Discord Voice MCP Server
FROM node:24-slim

# Install system dependencies
RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    build-essential \
    git \
    wget \
    ffmpeg \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy package files and install dependencies
COPY package*.json ./
RUN npm ci --only=production

# Copy application code
COPY src/ ./src/
COPY scripts/docker-entrypoint.sh ./

# Make entrypoint executable
RUN chmod +x docker-entrypoint.sh

# Create necessary directories
RUN mkdir -p models sessions exports logs whisper.cpp

# Build whisper.cpp (without models - they'll be downloaded at runtime)
RUN git clone https://github.com/ggerganov/whisper.cpp.git /tmp/whisper.cpp && \
    cd /tmp/whisper.cpp && \
    make && \
    cp /tmp/whisper.cpp/main /app/whisper.cpp/main && \
    cp -r /tmp/whisper.cpp/models/*.sh /app/whisper.cpp/ && \
    rm -rf /tmp/whisper.cpp

# Set default environment variables
ENV NODE_ENV=production \
    TRANSCRIPTION_PROVIDER=whisper \
    WHISPER_MODEL_NAME=base.en \
    WHISPER_MODEL_PATH=/app/models/whisper-model.bin \
    WHISPER_EXECUTABLE=/app/whisper.cpp/main \
    PORT=3000 \
    AUTO_DOWNLOAD_MODELS=true

# Create non-root user
RUN useradd -m -u 1000 mcp && \
    chown -R mcp:mcp /app

USER mcp

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD pgrep -f "node.*mcp-server.js" > /dev/null || exit 1

# Expose port (optional, for debugging)
EXPOSE 3000

# Use entrypoint script to download models on first run
ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["node", "src/mcp-server.js"]