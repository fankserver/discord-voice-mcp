# Discord Voice MCP Server - Makefile
# Includes ARM64 size optimization builds

.PHONY: help build build-all test lint fmt clean docker docker-minimal docker-optimized arm64-optimize test-optimized size-compare

# Default values
IMAGE_NAME ?= discord-voice-mcp
TAG ?= latest

# Colors for output
GREEN = \033[0;32m
BLUE = \033[0;34m
YELLOW = \033[1;33m
NC = \033[0m # No Color

help: ## Show this help message
	@echo "Discord Voice MCP Server - Build Commands"
	@echo "========================================"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "$(BLUE)%-20s$(NC) %s\n", $$1, $$2}'

# Go build commands
build: ## Build binary for current architecture
	@echo "$(GREEN)Building Go binary...$(NC)"
	go build -ldflags="-w -s" -o discord-voice-mcp ./cmd/discord-voice-mcp

build-all: ## Build binaries for all architectures
	@echo "$(GREEN)Building for all architectures...$(NC)"
	GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o discord-voice-mcp-linux-amd64 ./cmd/discord-voice-mcp
	GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o discord-voice-mcp-linux-arm64 ./cmd/discord-voice-mcp
	GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -o discord-voice-mcp-windows-amd64.exe ./cmd/discord-voice-mcp
	GOOS=darwin GOARCH=amd64 go build -ldflags="-w -s" -o discord-voice-mcp-darwin-amd64 ./cmd/discord-voice-mcp
	GOOS=darwin GOARCH=arm64 go build -ldflags="-w -s" -o discord-voice-mcp-darwin-arm64 ./cmd/discord-voice-mcp

# Test commands  
test: ## Run tests
	go test ./...

test-race: ## Run tests with race detection
	go test -race ./...

test-coverage: ## Run tests with coverage
	go test -coverprofile=coverage.txt ./...

# Code quality commands
lint: ## Run linter (requires golangci-lint)
	golangci-lint run

fmt: ## Format code
	go fmt ./...

# Docker build commands
docker: ## Build normal Docker image
	docker build -t $(IMAGE_NAME):$(TAG) .

docker-minimal: ## Build minimal Docker image  
	docker build -f Dockerfile.minimal -t $(IMAGE_NAME):minimal .

docker-whisper: ## Build whisper variant Docker image
	docker build -f Dockerfile.whisper -t $(IMAGE_NAME):whisper .

# ARM64 Optimization Commands (NEW)
arm64-optimize: ## Build ARM64-optimized Docker images (UPX + Distroless + Musl)
	@echo "$(YELLOW)🔧 Building ARM64-optimized images...$(NC)"
	./build-optimized.sh $(IMAGE_NAME) optimized

test-optimized: ## Test ARM64-optimized images functionality
	@echo "$(YELLOW)🧪 Testing optimized images...$(NC)" 
	./test-optimized.sh $(IMAGE_NAME)

size-compare: ## Compare sizes of all image variants
	@echo "$(BLUE)📊 Docker Image Size Comparison:$(NC)"
	@echo "=================================="
	@docker images | grep $(IMAGE_NAME) | sort | awk '{printf "%-40s %s\n", $$1":"$$2, $$7}'

# Multi-architecture builds
docker-buildx-setup: ## Set up buildx for multi-arch builds  
	docker buildx create --name multiarch --use --bootstrap

docker-multi: docker-buildx-setup ## Build multi-arch images with buildx
	docker buildx build --platform linux/amd64,linux/arm64 -t $(IMAGE_NAME):$(TAG) --push .

docker-multi-minimal: docker-buildx-setup ## Build multi-arch minimal images
	docker buildx build --platform linux/amd64,linux/arm64 -f Dockerfile.minimal -t $(IMAGE_NAME):minimal --push .

# Development commands
run: ## Run locally (requires DISCORD_TOKEN)
	./discord-voice-mcp -token "${DISCORD_TOKEN}"

run-docker: ## Run Docker container (requires DISCORD_TOKEN and DISCORD_USER_ID)
	docker run -i --rm \
		-e DISCORD_TOKEN="${DISCORD_TOKEN}" \
		-e DISCORD_USER_ID="${DISCORD_USER_ID}" \
		$(IMAGE_NAME):$(TAG)

# Cleanup commands
clean: ## Clean build artifacts
	rm -f discord-voice-mcp*
	docker system prune -f

clean-all: ## Clean everything including Docker images
	rm -f discord-voice-mcp*
	docker rmi $(IMAGE_NAME) || true
	docker system prune -af

# Quick development workflow
dev: fmt test build ## Format, test, and build (development workflow)

# Production optimization workflow  
optimize: arm64-optimize test-optimized size-compare ## Full ARM64 optimization pipeline

# Show current binary sizes
binary-sizes: build-all ## Show binary sizes for all architectures
	@echo "$(BLUE)📊 Go Binary Size Comparison:$(NC)"
	@echo "=============================="
	@ls -lh discord-voice-mcp-* 2>/dev/null | awk '{printf "%-30s %s\n", $$9, $$5}' || echo "No binaries found. Run 'make build-all' first."