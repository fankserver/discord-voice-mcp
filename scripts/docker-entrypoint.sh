#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}Discord Voice MCP Server - Initializing...${NC}"

# Function to download Vosk model
download_vosk_model() {
    local model_size=$1
    local model_dir="/app/models/vosk-model"
    
    if [ -d "$model_dir" ] && [ "$(ls -A $model_dir)" ]; then
        echo -e "${GREEN}✓ Vosk model already exists${NC}"
        return 0
    fi
    
    echo -e "${YELLOW}Downloading Vosk model: $model_size${NC}"
    
    case $model_size in
        small)
            MODEL_URL="https://alphacephei.com/vosk/models/vosk-model-small-en-us-0.15.zip"
            ;;
        medium)
            MODEL_URL="https://alphacephei.com/vosk/models/vosk-model-en-us-0.22-lgraph.zip"
            ;;
        large)
            MODEL_URL="https://alphacephei.com/vosk/models/vosk-model-en-us-0.22.zip"
            ;;
        *)
            MODEL_URL="https://alphacephei.com/vosk/models/vosk-model-small-en-us-0.15.zip"
            ;;
    esac
    
    cd /app/models
    wget -q --show-progress -O model.zip "$MODEL_URL"
    unzip -q model.zip
    rm model.zip
    
    # Rename to standard name
    mv vosk-model-* vosk-model
    
    echo -e "${GREEN}✓ Vosk model downloaded successfully${NC}"
}

# Function to download Whisper model
download_whisper_model() {
    local model_name=$1
    local model_file="/app/models/whisper-model.bin"
    
    if [ -f "$model_file" ]; then
        echo -e "${GREEN}✓ Whisper model already exists${NC}"
        return 0
    fi
    
    echo -e "${YELLOW}Downloading Whisper model: $model_name${NC}"
    
    cd /app/whisper.cpp
    
    # Use the download script from whisper.cpp
    if [ -f "download-ggml-model.sh" ]; then
        bash download-ggml-model.sh "$model_name"
        mv "models/ggml-${model_name}.bin" "/app/models/whisper-model.bin"
    else
        # Fallback to direct download
        case $model_name in
            tiny.en)
                MODEL_URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.en.bin"
                ;;
            base.en)
                MODEL_URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin"
                ;;
            small.en)
                MODEL_URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.en.bin"
                ;;
            medium.en)
                MODEL_URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.en.bin"
                ;;
            large-v3)
                MODEL_URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin"
                ;;
            *)
                MODEL_URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin"
                ;;
        esac
        
        wget -q --show-progress -O "/app/models/whisper-model.bin" "$MODEL_URL"
    fi
    
    echo -e "${GREEN}✓ Whisper model downloaded successfully${NC}"
}

# Check if models should be auto-downloaded
if [ "$AUTO_DOWNLOAD_MODELS" = "true" ]; then
    case $TRANSCRIPTION_PROVIDER in
        vosk)
            download_vosk_model "$VOSK_MODEL_SIZE"
            ;;
        whisper)
            download_whisper_model "$WHISPER_MODEL_NAME"
            ;;
        google)
            echo -e "${BLUE}Using Google Cloud Speech - no local models needed${NC}"
            if [ ! -f "$GOOGLE_APPLICATION_CREDENTIALS" ]; then
                echo -e "${YELLOW}⚠ Warning: Google credentials file not found at $GOOGLE_APPLICATION_CREDENTIALS${NC}"
            fi
            ;;
        *)
            echo -e "${YELLOW}Unknown provider: $TRANSCRIPTION_PROVIDER, defaulting to Vosk${NC}"
            download_vosk_model "small"
            ;;
    esac
else
    echo -e "${BLUE}Auto-download disabled. Assuming models are pre-installed.${NC}"
fi

# Create required directories if they don't exist
mkdir -p /app/sessions /app/exports /app/logs /app/temp

echo -e "${GREEN}✓ Initialization complete${NC}"
echo -e "${BLUE}Starting MCP server with provider: $TRANSCRIPTION_PROVIDER${NC}"

# Execute the main command
exec "$@"