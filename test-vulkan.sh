#!/bin/bash
# Test Vulkan-based whisper.cpp image

echo "Testing Vulkan image availability..."
docker manifest inspect ghcr.io/kth8/whisper-server-vulkan:latest 2>&1 | head -20

echo -e "\n\nChecking official whisper.cpp images..."
for tag in main main-cuda main-vulkan main-rocm; do
    echo -n "Testing ghcr.io/ggml-org/whisper.cpp:$tag ... "
    if docker manifest inspect ghcr.io/ggml-org/whisper.cpp:$tag >/dev/null 2>&1; then
        echo "EXISTS"
    else
        echo "NOT FOUND"
    fi
done

echo -e "\n\nChecking llama.cpp images (same org, might have patterns)..."
for tag in full-vulkan light-vulkan server-vulkan full-rocm light-rocm server-rocm; do
    echo -n "Testing ghcr.io/ggml-org/llama.cpp:$tag ... "
    if docker manifest inspect ghcr.io/ggml-org/llama.cpp:$tag >/dev/null 2>&1; then
        echo "EXISTS"
    else
        echo "NOT FOUND"
    fi
done