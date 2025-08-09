# Docker Multi-Stage Build Caching Strategy

## How Multi-Stage Caching Works

Yes, multi-stage builds are **highly cacheable**! Docker caches each stage independently, and the cache works efficiently when:

1. **Base images don't change** - Using specific tags (e.g., `ubuntu:22.04` not `ubuntu:latest`)
2. **Earlier layers remain unchanged** - Copying go.mod/go.sum before source code
3. **Build commands are deterministic** - Same input = same output

## Current Caching Strategy

### 1. Go Builder Stage (Highly Cacheable)
```dockerfile
FROM golang:1.24-alpine3.21 AS go-builder
COPY go.mod go.sum ./       # Cached until dependencies change
RUN go mod download         # Cached until go.mod changes
COPY . .                    # Only invalidated when source changes
RUN go build ...            # Only rebuilds when source changes
```

**Cache efficiency**: Excellent - Only rebuilds when Go code or dependencies change.

### 2. Whisper AMD64 (Instant)
```dockerfile
FROM ghcr.io/ggml-org/whisper.cpp:main-cuda AS whisper-amd64
```

**Cache efficiency**: Perfect - Just pulls a prebuilt image, instant caching.

### 3. Whisper ARM64 (Good with Trade-offs)
```dockerfile
FROM ubuntu:22.04 AS whisper-arm64
RUN apt-get install build-essential ...  # Cached after first build
RUN git clone ...                         # Not efficiently cached
RUN cmake ... && make                     # Rebuilds each time
```

**Cache efficiency**: Moderate - The `git clone` breaks caching, but this only affects ARM64 builds.

### 4. Final Stage (Well Cached)
```dockerfile
FROM base-${TARGETARCH} AS final
RUN apt-get install ffmpeg ...     # Cached after first install
COPY --from=go-builder ...         # Only invalidated if Go binary changes
COPY --from=whisper-source ...     # Only invalidated if whisper changes
```

**Cache efficiency**: Good - Most layers are stable and rarely change.

## Cache Performance by Platform

| Platform | Initial Build | Subsequent Builds | After Code Change |
|----------|--------------|-------------------|-------------------|
| **AMD64** | ~5 minutes | ~30 seconds | ~1-2 minutes |
| **ARM64** | ~15 minutes | ~10 minutes* | ~10-12 minutes |

*ARM64 rebuilds whisper.cpp each time due to `git clone`

## Optimization Trade-offs

### Current Approach Benefits
✅ **AMD64 gets CUDA support** - Full GPU acceleration
✅ **Simple and maintainable** - Clear separation of concerns
✅ **AMD64 builds are fast** - Uses prebuilt image
✅ **Works on all platforms** - No failures

### Current Approach Drawbacks
❌ **ARM64 rebuilds whisper** - `git clone` breaks cache
❌ **Two different paths** - More complex than single approach

## Potential Optimizations

### Option 1: Pin Whisper Version (Recommended)
```dockerfile
ARG WHISPER_VERSION=v1.5.4
RUN wget https://github.com/ggml-org/whisper.cpp/archive/refs/tags/${WHISPER_VERSION}.tar.gz && \
    tar xzf ${WHISPER_VERSION}.tar.gz
```
**Benefit**: Better caching for ARM64
**Cost**: Need to manually update version

### Option 2: Build Everything from Source
```dockerfile
# Same build process for both architectures
FROM ubuntu:22.04 AS whisper-builder
# Build with CUDA for AMD64, NEON for ARM64
```
**Benefit**: Consistent caching behavior
**Cost**: No GPU support without complex CUDA setup

### Option 3: Use BuildKit Cache Mounts
```dockerfile
RUN --mount=type=cache,target=/var/cache/apt \
    --mount=type=cache,target=/var/lib/apt \
    apt-get update && apt-get install ...
```
**Benefit**: Faster apt operations
**Cost**: Requires BuildKit

## Recommendations

1. **Keep current approach** - The benefits of CUDA support outweigh caching issues
2. **Consider pinning versions** - For production, pin whisper.cpp to specific release
3. **Use BuildKit** - Enable with `DOCKER_BUILDKIT=1` for better caching
4. **Separate CI builds** - Build AMD64 and ARM64 in parallel jobs

## Build Commands for Optimal Caching

```bash
# Enable BuildKit for better caching
export DOCKER_BUILDKIT=1

# Build with cache
docker buildx build \
  --cache-from type=registry,ref=ghcr.io/fankserver/discord-voice-mcp:cache \
  --cache-to type=registry,ref=ghcr.io/fankserver/discord-voice-mcp:cache,mode=max \
  -f Dockerfile.whisper \
  -t discord-voice-mcp:whisper .

# Build specific architecture (faster)
docker buildx build --platform linux/amd64 ...  # Skip ARM64 build
docker buildx build --platform linux/arm64 ...  # Skip AMD64 build
```

## Conclusion

The current multi-stage approach **is cacheable** and provides the best balance between:
- **Performance** (CUDA for AMD64)
- **Compatibility** (works on ARM64)
- **Build speed** (fast for AMD64, acceptable for ARM64)

The ARM64 caching could be improved, but the trade-off for getting CUDA support on AMD64 is worth it.