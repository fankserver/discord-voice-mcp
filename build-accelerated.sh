#!/bin/bash
# Build script for Discord Voice MCP with various acceleration backends
# Default: Builds with CUDA, ROCm, Vulkan, and OpenBLAS support (all backends)

set -e

# Default values
ACCELERATION="all"
IMAGE_TAG="discord-voice-mcp:whisper"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --cuda|--nvidia)
            ACCELERATION="cuda"
            IMAGE_TAG="discord-voice-mcp:cuda"
            shift
            ;;
        --rocm|--amd)
            ACCELERATION="rocm"
            IMAGE_TAG="discord-voice-mcp:rocm"
            shift
            ;;
        --vulkan)
            ACCELERATION="vulkan"
            IMAGE_TAG="discord-voice-mcp:vulkan"
            shift
            ;;
        --sycl|--intel)
            ACCELERATION="sycl"
            IMAGE_TAG="discord-voice-mcp:sycl"
            shift
            ;;
        --openblas|--cpu-only)
            ACCELERATION="cpu-only"
            IMAGE_TAG="discord-voice-mcp:cpu-only"
            shift
            ;;
        --all|--default)
            ACCELERATION="all"
            IMAGE_TAG="discord-voice-mcp:whisper"
            shift
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --cuda, --nvidia    Build with NVIDIA CUDA support"
            echo "  --rocm, --amd       Build with AMD ROCm support"
            echo "  --vulkan            Build with Vulkan support"
            echo "  --sycl, --intel     Build with Intel SYCL support"
            echo "  --cpu-only          Build with OpenBLAS CPU acceleration only"
            echo "  --all, --default    Build with all acceleration backends (default)"
            echo "  --help              Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0 --cuda           # Build for NVIDIA GPUs"
            echo "  $0 --rocm           # Build for AMD GPUs"
            echo "  $0 --cpu-only       # Build with CPU optimization only"
            echo "  $0                  # Build with all backends (default)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

echo "Building Discord Voice MCP with $ACCELERATION acceleration..."
echo "Target image: $IMAGE_TAG"

# Set build arguments based on acceleration type
case $ACCELERATION in
    cuda)
        BUILD_ARGS="--build-arg GGML_CUDA=ON --build-arg GGML_OPENBLAS=ON"
        # For CUDA, we need the nvidia base image
        echo "Note: For CUDA support, you need NVIDIA Docker runtime"
        ;;
    rocm)
        BUILD_ARGS="--build-arg GGML_ROCM=ON --build-arg GGML_OPENBLAS=ON"
        echo "Note: For ROCm support, you need AMD GPU drivers"
        ;;
    vulkan)
        BUILD_ARGS="--build-arg GGML_VULKAN=ON --build-arg GGML_OPENBLAS=ON"
        ;;
    sycl)
        BUILD_ARGS="--build-arg GGML_SYCL=ON --build-arg GGML_OPENBLAS=ON"
        ;;
    all)
        # Default configuration - all major backends
        BUILD_ARGS=""  # Uses Dockerfile defaults (CUDA=ON, ROCm=ON, Vulkan=ON, OpenBLAS=ON)
        echo "Building with all GPU backends (CUDA, ROCm, Vulkan) + OpenBLAS"
        ;;
    cpu-only)
        # CPU-only build - disable all GPU backends
        BUILD_ARGS="--build-arg GGML_CUDA=OFF --build-arg GGML_ROCM=OFF --build-arg GGML_VULKAN=OFF --build-arg GGML_OPENBLAS=ON"
        ;;
    *)
        # Default: All backends
        BUILD_ARGS=""
        ;;
esac

# Build the Docker image
docker build \
    -f Dockerfile.whisper \
    -t "$IMAGE_TAG" \
    $BUILD_ARGS \
    .

echo ""
echo "Build complete! Image tagged as: $IMAGE_TAG"
echo ""
echo "To run the container:"
echo "  docker run -e DISCORD_TOKEN=YOUR_TOKEN $IMAGE_TAG"
echo ""
if [[ $ACCELERATION == "cuda" ]]; then
    echo "For CUDA acceleration, run with:"
    echo "  docker run --gpus all -e DISCORD_TOKEN=YOUR_TOKEN $IMAGE_TAG"
elif [[ $ACCELERATION == "rocm" ]]; then
    echo "For ROCm acceleration, run with:"
    echo "  docker run --device=/dev/kfd --device=/dev/dri -e DISCORD_TOKEN=YOUR_TOKEN $IMAGE_TAG"
fi