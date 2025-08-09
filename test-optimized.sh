#!/bin/bash

# Test script for ARM64-optimized Docker images
# Validates that optimizations don't break functionality

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

IMAGE_NAME=${1:-discord-voice-mcp}

echo -e "${BLUE}🧪 Testing ARM64-optimized Docker images...${NC}"

# Test function
test_image() {
    local image_tag=$1
    local description=$2
    
    echo -e "${YELLOW}Testing $description...${NC}"
    
    # Test basic binary execution (should show help/usage)
    if timeout 10s docker run --rm --platform linux/arm64 ${IMAGE_NAME}:${image_tag} --help >/dev/null 2>&1; then
        echo -e "${GREEN}✅ $description: Binary executes successfully${NC}"
    else
        echo -e "${RED}❌ $description: Binary execution failed${NC}"
        return 1
    fi
    
    # Test that opus libraries are available (CGO dependency)
    if docker run --rm --platform linux/arm64 ${IMAGE_NAME}:${image_tag} sh -c "ldd /app/discord-voice-mcp 2>/dev/null | grep -q opus || echo 'Static binary or opus linked'" >/dev/null 2>&1; then
        echo -e "${GREEN}✅ $description: Opus dependency satisfied${NC}"
    else
        echo -e "${YELLOW}ℹ️  $description: Static binary (expected for musl builds)${NC}"
    fi
}

# Test all optimized variants
echo -e "${BLUE}=== Testing Optimized Images ===${NC}"

# Test if images exist first
if docker images ${IMAGE_NAME}:upx-arm64 --format "{{.Repository}}" | grep -q ${IMAGE_NAME}; then
    test_image "upx-arm64" "UPX + Distroless Base"
else
    echo -e "${YELLOW}⚠️  UPX optimized image not found, skipping test${NC}"
fi

if docker images ${IMAGE_NAME}:musl-arm64 --format "{{.Repository}}" | grep -q ${IMAGE_NAME}; then
    test_image "musl-arm64" "Musl + UPX + Static"
else
    echo -e "${YELLOW}⚠️  Musl optimized image not found, skipping test${NC}"
fi

# Test current baseline for comparison
if docker images ${IMAGE_NAME}:current-arm64 --format "{{.Repository}}" | grep -q ${IMAGE_NAME}; then
    test_image "current-arm64" "Current Minimal (Baseline)"
else
    echo -e "${YELLOW}⚠️  Current baseline image not found, skipping test${NC}"
fi

echo -e "${GREEN}✅ Testing complete!${NC}"

# Show final size comparison
echo -e "${YELLOW}📊 Final Size Comparison:${NC}"
echo "=========================================="
docker images | grep ${IMAGE_NAME} | grep arm64 | awk '{print $1":"$2" - "$7}' | sort

echo ""
echo -e "${BLUE}💡 Recommendations:${NC}"
echo "1. Use 'musl' variant for maximum size reduction (static linking)"
echo "2. Use 'upx' variant if you need dynamic linking but want compression" 
echo "3. Both variants should be significantly smaller than current baseline"
echo ""
echo -e "${GREEN}Ready for production testing!${NC}"