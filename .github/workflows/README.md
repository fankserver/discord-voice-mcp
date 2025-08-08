# GitHub Actions Workflows

This repository uses GitHub Actions for CI/CD with the following workflows:

## Workflows

### 1. CI (`ci.yml`)
Runs on every push and pull request to ensure code quality:
- **Tests**: Runs Go tests with race detection on Go 1.23 and 1.24
- **Coverage**: Uploads test coverage to Codecov
- **Linting**: Runs golangci-lint for code quality
- **Security**: Scans with Trivy and gosec for vulnerabilities
- **Docker Lint**: Validates Dockerfiles with Hadolint
- **Format Check**: Ensures code is properly formatted

### 2. Docker Build (`docker-build.yml`)
Builds and pushes multi-architecture Docker images:
- **Platforms**: `linux/amd64` and `linux/arm64`
- **Images**: 
  - Normal image with ffmpeg support
  - Minimal image (scratch-based, ~12MB)
- **Registries**: 
  - GitHub Container Registry (ghcr.io)
  - Docker Hub (optional, requires secrets)
- **Tags**:
  - Branch names (e.g., `main`, `feat/golang-rewrite`)
  - PR numbers (e.g., `pr-123`)
  - Version tags (e.g., `v1.0.0`)
  - SHA commits

### 3. Release (`release.yml`)
Triggered when a GitHub Release is published:
- Builds and pushes production Docker images
- Creates pre-built binaries for multiple platforms:
  - Linux (amd64, arm64)
  - macOS (amd64, arm64)
  - Windows (amd64)
- Uploads binaries as release assets

## Docker Images

### Available Tags

#### Normal Image (with ffmpeg)
- `latest` - Latest stable release
- `v1.0.0` - Specific version
- `main` - Latest from main branch
- `sha-abc123` - Specific commit

#### Minimal Image (without ffmpeg)
- `minimal` - Latest minimal stable release
- `v1.0.0-minimal` - Specific minimal version
- `main-minimal` - Latest minimal from main branch
- `sha-abc123-minimal` - Specific minimal commit

### Image Sizes
- **Normal**: ~200MB (includes Alpine Linux + ffmpeg)
- **Minimal**: ~12MB (scratch-based, static binary only)

## Setup

### Required Secrets
Configure these in your repository settings:

1. **For Docker Hub (optional)**:
   - `DOCKERHUB_USERNAME`: Your Docker Hub username
   - `DOCKERHUB_TOKEN`: Docker Hub access token

2. **Automatic (provided by GitHub)**:
   - `GITHUB_TOKEN`: Automatically provided for GitHub Container Registry

### Triggering Workflows

#### Manual Trigger
You can manually trigger workflows from the Actions tab using "workflow_dispatch"

#### Automatic Triggers
- **Push to main/feat branches**: Triggers CI and Docker builds
- **Pull requests**: Triggers CI checks
- **Git tags**: Triggers release builds
- **GitHub Release**: Triggers full release workflow

## Usage Examples

### Pull Docker Images

```bash
# GitHub Container Registry
docker pull ghcr.io/fankserver/discord-voice-mcp:latest
docker pull ghcr.io/fankserver/discord-voice-mcp:minimal

# Docker Hub (if configured)
docker pull fankserver/discord-voice-mcp:latest
docker pull fankserver/discord-voice-mcp:minimal
```

### Multi-arch Support

Images automatically work on both amd64 and arm64:

```bash
# On Apple Silicon Mac (arm64)
docker run ghcr.io/fankserver/discord-voice-mcp:latest

# On Intel/AMD (amd64)
docker run ghcr.io/fankserver/discord-voice-mcp:latest
```

Docker automatically pulls the correct architecture.

## Binary Releases

Pre-built binaries are available from the [Releases](../../releases) page:

- `discord-voice-mcp-linux-amd64.tar.gz`
- `discord-voice-mcp-linux-arm64.tar.gz`
- `discord-voice-mcp-darwin-amd64.tar.gz` (macOS Intel)
- `discord-voice-mcp-darwin-arm64.tar.gz` (macOS Apple Silicon)
- `discord-voice-mcp-windows-amd64.zip`

## Development

### Running CI Locally

Use [act](https://github.com/nektos/act) to run workflows locally:

```bash
# Run CI workflow
act -W .github/workflows/ci.yml

# Run with specific event
act pull_request -W .github/workflows/ci.yml
```

### Building Multi-arch Locally

```bash
# Set up buildx
docker buildx create --use

# Build for multiple platforms
docker buildx build --platform linux/amd64,linux/arm64 -t myimage:latest .
```