.PHONY: build run test clean docker-build docker-run size-compare

# Build the Go binary
build:
	go build -ldflags="-w -s" -o discord-voice-mcp ./cmd/discord-voice-mcp

# Run the application
run:
	go run ./cmd/discord-voice-mcp

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -f discord-voice-mcp
	rm -f discord-voice-mcp.exe
	docker rmi discord-voice-mcp:go 2>/dev/null || true

# Build Docker image (Go version)
docker-build:
	docker build -f Dockerfile -t discord-voice-mcp:go .

# Run Docker container (Go version)
docker-run:
	docker run --rm \
		-e DISCORD_TOKEN="${DISCORD_TOKEN}" \
		-e DISCORD_CLIENT_ID="${DISCORD_CLIENT_ID}" \
		discord-voice-mcp:go

# Compare sizes between Node.js and Go versions
size-compare:
	@echo "=== Binary/Image Size Comparison ==="
	@echo "Go binary size:"
	@ls -lh discord-voice-mcp 2>/dev/null || echo "  Not built yet. Run 'make build' first"
	@echo ""
	@echo "Docker images:"
	@docker images | grep discord-voice-mcp | awk '{printf "  %-20s %s\n", $$2, $$7}'
	@echo ""
	@echo "Memory usage (if running):"
	@docker stats --no-stream discord-voice-mcp 2>/dev/null || echo "  No containers running"

# Download dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Build for all platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -o dist/discord-voice-mcp-linux-amd64
	GOOS=linux GOARCH=arm64 go build -o dist/discord-voice-mcp-linux-arm64
	GOOS=darwin GOARCH=amd64 go build -o dist/discord-voice-mcp-darwin-amd64
	GOOS=darwin GOARCH=arm64 go build -o dist/discord-voice-mcp-darwin-arm64
	GOOS=windows GOARCH=amd64 go build -o dist/discord-voice-mcp-windows-amd64.exe
	@echo "Built binaries for all platforms in dist/"
	@ls -lh dist/

# Quick benchmark
bench:
	@echo "=== Performance Comparison ==="
	@echo "Go version startup:"
	@time -p ./discord-voice-mcp -version 2>/dev/null || echo "Not built"
	@echo ""
	@echo "Node.js version startup:"
	@time -p node src/mcp-server.js -version 2>/dev/null || echo "Not available"

help:
	@echo "Discord Voice MCP - Go Version"
	@echo ""
	@echo "Available targets:"
	@echo "  make build          - Build the Go binary"
	@echo "  make run            - Run the application"
	@echo "  make test           - Run tests"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make docker-run     - Run Docker container"
	@echo "  make size-compare   - Compare sizes with Node.js version"
	@echo "  make build-all      - Build for all platforms"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make help           - Show this help"