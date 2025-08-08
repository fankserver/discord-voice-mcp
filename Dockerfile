# Build stage
FROM golang:1.24-alpine3.20 AS builder

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
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
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

# Run the binary
CMD ["./discord-voice-mcp"]

# Expected image size: ~50MB (vs 2.35GB for Node.js version!)
# Binary size: ~15MB
# Alpine + ffmpeg: ~35MB