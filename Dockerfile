# Build stage
FROM golang:1.24-alpine3.21 AS builder

# Build arguments for target platform
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

# Install build dependencies
# hadolint ignore=DL3018
RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary with CGO
RUN CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -a -tags netgo -ldflags '-w -s -extldflags "-static"' \
    -o discord-voice-mcp ./cmd/discord-voice-mcp

# Final stage - using alpine for ffmpeg support
FROM alpine:3.20

# Install only ffmpeg (needed for audio processing)
# hadolint ignore=DL3018
RUN apk add --no-cache ffmpeg

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/discord-voice-mcp .

# Create non-root user
RUN adduser -D -u 1000 mcp
USER mcp

# Note: No ports exposed as this uses stdin/stdout for MCP protocol

# Audio processing configuration (defaults)
ENV AUDIO_BUFFER_DURATION_SEC=2 \
    AUDIO_SILENCE_TIMEOUT_MS=1500 \
    AUDIO_MIN_BUFFER_MS=100

# Run the binary
CMD ["./discord-voice-mcp"]

# Expected image size: ~50MB (vs 2.35GB for Node.js version!)
# Binary size: ~15MB
# Alpine + ffmpeg: ~35MB