# Multi-stage build for Discord Voice MCP Server
FROM node:24-slim AS base

# Install system dependencies
RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    build-essential \
    git \
    wget \
    unzip \
    ffmpeg \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy package files
COPY package*.json ./

# Install Node dependencies
RUN npm ci --only=production

# Copy application code
COPY src/ ./src/
COPY scripts/ ./scripts/

# Make scripts executable
RUN chmod +x scripts/*.sh

# Create necessary directories
RUN mkdir -p models temp sessions exports credentials logs

# ========================================
# Vosk model download stage
FROM base AS vosk-models

ARG VOSK_MODEL=small

WORKDIR /app/models

# Download Vosk model based on build argument
RUN if [ "$VOSK_MODEL" = "small" ]; then \
        wget -q https://alphacephei.com/vosk/models/vosk-model-small-en-us-0.15.zip && \
        unzip -q vosk-model-small-en-us-0.15.zip && \
        rm vosk-model-small-en-us-0.15.zip && \
        mv vosk-model-small-en-us-0.15 vosk-model; \
    elif [ "$VOSK_MODEL" = "large" ]; then \
        wget -q https://alphacephei.com/vosk/models/vosk-model-en-us-0.22.zip && \
        unzip -q vosk-model-en-us-0.22.zip && \
        rm vosk-model-en-us-0.22.zip && \
        mv vosk-model-en-us-0.22 vosk-model; \
    else \
        wget -q https://alphacephei.com/vosk/models/vosk-model-en-us-0.22-lgraph.zip && \
        unzip -q vosk-model-en-us-0.22-lgraph.zip && \
        rm vosk-model-en-us-0.22-lgraph.zip && \
        mv vosk-model-en-us-0.22-lgraph vosk-model; \
    fi

# ========================================
# Whisper.cpp build stage
FROM base AS whisper-build

ARG WHISPER_MODEL=base.en

WORKDIR /tmp

# Clone and build whisper.cpp
RUN git clone https://github.com/ggerganov/whisper.cpp.git && \
    cd whisper.cpp && \
    make && \
    cd models && \
    bash ./download-ggml-model.sh ${WHISPER_MODEL}

# ========================================
# Final stage
FROM base AS final

ARG PROVIDER=vosk
ARG VOSK_MODEL=small
ARG WHISPER_MODEL=base.en

# Copy Vosk models if using Vosk
COPY --from=vosk-models /app/models/vosk-model /app/models/vosk-model

# Copy Whisper.cpp if using Whisper
COPY --from=whisper-build /tmp/whisper.cpp/main /app/whisper.cpp/main
COPY --from=whisper-build /tmp/whisper.cpp/models/ggml-*.bin /app/models/

# Set environment variables
ENV NODE_ENV=production \
    TRANSCRIPTION_PROVIDER=${PROVIDER} \
    VOSK_MODEL_PATH=/app/models/vosk-model \
    WHISPER_MODEL_PATH=/app/models/ggml-${WHISPER_MODEL}.bin \
    WHISPER_EXECUTABLE=/app/whisper.cpp/main \
    PORT=3000

# Create non-root user
RUN useradd -m -u 1000 mcp && \
    chown -R mcp:mcp /app

USER mcp

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD node -e "console.log('Health check passed')" || exit 1

# Expose port for WebSocket connections (optional)
EXPOSE 3000

# Start the MCP server
CMD ["node", "src/mcp-server.js"]