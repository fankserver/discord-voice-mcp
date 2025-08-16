# Build stage with ccache optimization
FROM golang:1.24-alpine3.21 AS builder

# Install build dependencies including ccache
# hadolint ignore=DL3018
RUN apk add --no-cache git gcc musl-dev pkgconfig opus-dev ccache

# Set up ccache
ENV CCACHE_DIR=/ccache
ENV PATH="/usr/lib/ccache/bin:${PATH}"
RUN mkdir -p /ccache && chmod 777 /ccache

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary with CGO and ccache
# Docker buildx automatically handles cross-compilation via --platform flag
# Using dynamic linking as static opus lib not available for all architectures
RUN --mount=type=cache,target=/ccache \
    CGO_ENABLED=1 go build -ldflags '-w -s' \
    -o discord-voice-mcp ./cmd/discord-voice-mcp

# Final stage
FROM alpine:3.20

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