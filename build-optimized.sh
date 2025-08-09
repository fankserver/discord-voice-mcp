#!/bin/bash

# ARM64 Docker Image Size Optimization Build Script
# Based on research: separate builds perform better than buildx multi-arch

set -e

IMAGE_NAME=${1:-discord-voice-mcp}
TAG=${2:-optimized}

echo "🔧 Building ARM64-optimized Docker images..."
echo "Image: $IMAGE_NAME"
echo "Tag: $TAG"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to build and measure image size
build_and_measure() {
    local dockerfile=$1
    local platform=$2
    local tag_suffix=$3
    local description=$4
    
    echo -e "${BLUE}Building $description for $platform...${NC}"
    
    # Build the image
    docker build --platform=$platform -f $dockerfile -t ${IMAGE_NAME}:${tag_suffix}-$(echo $platform | cut -d'/' -f2) . --no-cache
    
    # Get image size
    local size=$(docker images ${IMAGE_NAME}:${tag_suffix}-$(echo $platform | cut -d'/' -f2) --format "{{.Size}}")
    echo -e "${GREEN}✅ $description ($platform): $size${NC}"
    
    return 0
}

echo -e "${YELLOW}🧪 Testing optimization approaches...${NC}"

# Test 1: UPX + Distroless Base
echo -e "${BLUE}=== Test 1: UPX + Distroless Base ===${NC}"
build_and_measure "Dockerfile.minimal-upx" "linux/amd64" "upx" "UPX + Distroless Base"
build_and_measure "Dockerfile.minimal-upx" "linux/arm64" "upx" "UPX + Distroless Base"

# Test 2: Musl + UPX + Distroless Static  
echo -e "${BLUE}=== Test 2: Musl + UPX + Distroless Static ===${NC}"
build_and_measure "Dockerfile.minimal-musl" "linux/amd64" "musl" "Musl + UPX + Static"
build_and_measure "Dockerfile.minimal-musl" "linux/arm64" "musl" "Musl + UPX + Static"

# Test 3: Current minimal for comparison
echo -e "${BLUE}=== Test 3: Current Minimal (Baseline) ===${NC}"
build_and_measure "Dockerfile.minimal" "linux/amd64" "current" "Current Minimal"
build_and_measure "Dockerfile.minimal" "linux/arm64" "current" "Current Minimal"

echo -e "${YELLOW}📊 Size Comparison Summary${NC}"
echo "==========================================="
docker images | grep $IMAGE_NAME | sort

echo -e "${YELLOW}🏆 Creating multi-arch manifests for best approach...${NC}"

# Determine best approach based on ARM64 size reduction
UPX_ARM64_SIZE=$(docker images ${IMAGE_NAME}:upx-arm64 --format "{{.Size}}")
MUSL_ARM64_SIZE=$(docker images ${IMAGE_NAME}:musl-arm64 --format "{{.Size}}")

echo -e "${GREEN}UPX approach ARM64 size: $UPX_ARM64_SIZE${NC}"
echo -e "${GREEN}Musl approach ARM64 size: $MUSL_ARM64_SIZE${NC}"

# Create manifest for the best performing approach
# (Script will create both, user can choose based on results)

echo -e "${BLUE}Creating UPX multi-arch manifest...${NC}"
docker manifest create ${IMAGE_NAME}:${TAG}-upx \
    ${IMAGE_NAME}:upx-amd64 \
    ${IMAGE_NAME}:upx-arm64
docker manifest push ${IMAGE_NAME}:${TAG}-upx

echo -e "${BLUE}Creating Musl multi-arch manifest...${NC}" 
docker manifest create ${IMAGE_NAME}:${TAG}-musl \
    ${IMAGE_NAME}:musl-amd64 \
    ${IMAGE_NAME}:musl-arm64
docker manifest push ${IMAGE_NAME}:${TAG}-musl

echo -e "${GREEN}✅ ARM64 optimization builds complete!${NC}"
echo ""
echo -e "${YELLOW}Usage:${NC}"
echo "# Test UPX optimized version:"
echo "docker run --platform linux/arm64 ${IMAGE_NAME}:${TAG}-upx"
echo ""
echo "# Test Musl optimized version:" 
echo "docker run --platform linux/arm64 ${IMAGE_NAME}:${TAG}-musl"
echo ""
echo -e "${BLUE}Size reduction achieved:${NC}"
echo "Check the output above to see ARM64 size improvements vs baseline!"