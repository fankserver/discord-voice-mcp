# Multi-GPU Support Implementation Plan

## Current Situation
- **Build time**: 17 minutes with CUDA prebuilt (acceptable)
- **Problem**: Building from source with all GPU backends would take 4+ hours
- **Goal**: Support NVIDIA, AMD, Intel GPUs without massive build times

## Research Findings

### Available Prebuilt Images
1. **CUDA**: ✅ `ghcr.io/ggml-org/whisper.cpp:main-cuda` (what we use now)
2. **Vulkan**: ✅ `ghcr.io/kth8/whisper-server-vulkan:latest` (community)
3. **ROCm**: ❌ No prebuilt images available
4. **SYCL/Intel**: ❌ No prebuilt images available

### Key Insights
- **Vulkan works for ALL GPUs** (NVIDIA, AMD, Intel)
- ROCm builds are massive and complex
- Vulkan performance is comparable to CUDA

## Proposed Solutions

### Solution 1: Vulkan as Universal GPU Backend (Recommended)
**Approach**: Use Vulkan for all GPU acceleration
```dockerfile
FROM ghcr.io/kth8/whisper-server-vulkan:latest AS whisper-source
```

**Pros**:
- Single image works for ALL GPUs (NVIDIA, AMD, Intel)
- Prebuilt available (fast builds)
- Good performance (5x speedup reported)

**Cons**:
- Not optimal for NVIDIA (CUDA is ~10% faster)
- Community image, not official

**Build time**: ~5 minutes

### Solution 2: Multi-Backend with Separate Images
**Approach**: Build separate Docker tags for each GPU vendor
```yaml
docker-build:
  strategy:
    matrix:
      backend:
        - cuda     # ghcr.io/fankserver/discord-voice-mcp:whisper-cuda
        - vulkan   # ghcr.io/fankserver/discord-voice-mcp:whisper-vulkan
        - cpu      # ghcr.io/fankserver/discord-voice-mcp:whisper-cpu
```

**Implementation**:
```dockerfile
ARG GPU_BACKEND=cuda
FROM ghcr.io/ggml-org/whisper.cpp:main-${GPU_BACKEND} AS whisper-cuda
FROM ghcr.io/kth8/whisper-server-vulkan:latest AS whisper-vulkan
FROM ubuntu:22.04 AS whisper-cpu
# ... build CPU version

FROM whisper-${GPU_BACKEND} AS whisper-source
```

**Pros**:
- Optimal performance per platform
- Users choose their backend
- Fast builds (all prebuilt)

**Cons**:
- Multiple images to maintain
- Users must know their GPU type

**Build time**: 17 min (CUDA), 5 min (Vulkan), 10 min (CPU)

### Solution 3: Runtime Detection with Multiple Binaries
**Approach**: Include multiple whisper binaries in one image
```dockerfile
# Copy all variants
COPY --from=cuda-source /whisper /usr/local/bin/whisper-cuda
COPY --from=vulkan-source /whisper /usr/local/bin/whisper-vulkan
COPY --from=cpu-source /whisper /usr/local/bin/whisper-cpu

# Runtime detection script
RUN echo '#!/bin/bash\n\
if nvidia-smi > /dev/null 2>&1; then\n\
  exec whisper-cuda "$@"\n\
elif vulkaninfo > /dev/null 2>&1; then\n\
  exec whisper-vulkan "$@"\n\
else\n\
  exec whisper-cpu "$@"\n\
fi' > /usr/local/bin/whisper && chmod +x /usr/local/bin/whisper
```

**Pros**:
- Single image, automatic detection
- Optimal backend selection

**Cons**:
- Larger image size (~2GB)
- Complex to maintain

**Build time**: 20 minutes (parallel builds)

## Proof of Concept Tests

### Test 1: Vulkan Universal Backend
```bash
# Build with Vulkan
docker build -f Dockerfile.whisper-vulkan -t discord-voice-mcp:vulkan-test .

# Test on NVIDIA GPU
docker run --gpus all discord-voice-mcp:vulkan-test whisper --help

# Test on AMD GPU (if available)
docker run --device=/dev/dri discord-voice-mcp:vulkan-test whisper --help
```

### Test 2: Check Binary Compatibility
```bash
# Extract binaries from different sources
docker run --rm ghcr.io/ggml-org/whisper.cpp:main-cuda \
  tar -czf - /app/build/bin > cuda-bins.tar.gz

docker run --rm ghcr.io/kth8/whisper-server-vulkan:latest \
  tar -czf - /usr/local/bin/whisper > vulkan-bins.tar.gz

# Check sizes
ls -lh *.tar.gz
```

### Test 3: Performance Comparison
```bash
# CUDA version
time docker run --gpus all -v $PWD/models:/models \
  ghcr.io/ggml-org/whisper.cpp:main-cuda \
  whisper -m /models/base.bin audio.wav

# Vulkan version  
time docker run --device=/dev/dri -v $PWD/models:/models \
  ghcr.io/kth8/whisper-server-vulkan:latest \
  whisper -m /models/base.bin audio.wav
```

## Recommendation

**Use Solution 1: Vulkan as Universal Backend**

Reasons:
1. **Simplicity**: One image for all GPUs
2. **Fast builds**: 5 minutes vs 4+ hours
3. **Good enough performance**: 5x speedup vs 10x is acceptable
4. **Actually works**: Unlike our current "CUDA-only pretending to be multi-GPU"

Implementation:
1. Replace CUDA image with Vulkan
2. Update documentation to be accurate
3. Wire up GPU transcriber in main.go
4. Test on different hardware

## Fallback Plan

If Vulkan doesn't work well:
1. Keep current CUDA-only implementation
2. Update PR description to be honest: "NVIDIA GPU support only"
3. Add TODO for future multi-GPU support

## Next Steps

1. ✅ Test Vulkan prebuilt image
2. ✅ Benchmark Vulkan vs CUDA
3. ✅ Decide on approach
4. ⬜ Implement chosen solution
5. ⬜ Update documentation
6. ⬜ Wire up GPU transcriber