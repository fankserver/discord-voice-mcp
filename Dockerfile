# Build stage
FROM golang:1.24-alpine3.21 AS builder

# Install build dependencies
# hadolint ignore=DL3018
RUN apk add --no-cache git gcc musl-dev pkgconfig opus-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary with CGO
# Docker buildx automatically handles cross-compilation via --platform flag
# Using dynamic linking as static opus lib not available for all architectures
RUN CGO_ENABLED=1 go build -ldflags '-w -s' \
    -o discord-voice-mcp ./cmd/discord-voice-mcp

# Final stage
FROM alpine:3.22

# Install opus runtime library (required for dynamic linking)
# hadolint ignore=DL3018
RUN apk add --no-cache opus

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/discord-voice-mcp .

# Create non-root user
RUN adduser -D -u 1000 mcp
USER mcp

# Note: No ports exposed as this uses stdin/stdout for MCP protocol

# Run the binary
CMD ["./discord-voice-mcp"]

# Expected image size: ~15-20MB
# Binary size: ~15MB
# Alpine + opus: ~5MB