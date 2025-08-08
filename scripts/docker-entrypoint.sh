#!/bin/bash
set -e

echo "Starting Discord Voice MCP Server..."

# Download models if enabled and not present
if [ "$AUTO_DOWNLOAD_MODELS" = "true" ]; then
    if [ "$TRANSCRIPTION_PROVIDER" = "whisper" ] && [ ! -f "$WHISPER_MODEL_PATH" ]; then
        echo "Downloading Whisper model: $WHISPER_MODEL_NAME"
        cd /app/whisper.cpp
        
        # Use whisper.cpp download script if available
        if [ -f "download-ggml-model.sh" ]; then
            ./download-ggml-model.sh "$WHISPER_MODEL_NAME"
            mv "models/ggml-${WHISPER_MODEL_NAME}.bin" "$WHISPER_MODEL_PATH"
        else
            # Direct download fallback
            case $WHISPER_MODEL_NAME in
                tiny*) URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.en.bin" ;;
                base*) URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin" ;;
                small*) URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.en.bin" ;;
                medium*) URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.en.bin" ;;
                large*) URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin" ;;
                *) URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin" ;;
            esac
            wget -q --show-progress -O "$WHISPER_MODEL_PATH" "$URL"
        fi
        echo "Whisper model ready"
        
    elif [ "$TRANSCRIPTION_PROVIDER" = "vosk" ] && [ ! -d "/app/models/vosk-model" ]; then
        echo "Downloading Vosk model: ${VOSK_MODEL_SIZE:-small}"
        cd /app/models
        
        case "${VOSK_MODEL_SIZE:-small}" in
            small)
                wget -q https://alphacephei.com/vosk/models/vosk-model-small-en-us-0.15.zip
                unzip -q vosk-model-small-en-us-0.15.zip && rm vosk-model-small-en-us-0.15.zip
                mv vosk-model-small-en-us-0.15 vosk-model
                ;;
            large)
                wget -q https://alphacephei.com/vosk/models/vosk-model-en-us-0.22.zip
                unzip -q vosk-model-en-us-0.22.zip && rm vosk-model-en-us-0.22.zip
                mv vosk-model-en-us-0.22 vosk-model
                ;;
            *)
                wget -q https://alphacephei.com/vosk/models/vosk-model-en-us-0.22-lgraph.zip
                unzip -q vosk-model-en-us-0.22-lgraph.zip && rm vosk-model-en-us-0.22-lgraph.zip
                mv vosk-model-en-us-0.22-lgraph vosk-model
                ;;
        esac
        echo "Vosk model ready"
    fi
fi

# Verify required environment variables
if [ -z "$DISCORD_TOKEN" ]; then
    echo "Error: DISCORD_TOKEN is required"
    exit 1
fi

if [ -z "$DISCORD_CLIENT_ID" ]; then
    echo "Error: DISCORD_CLIENT_ID is required"
    exit 1
fi

echo "Provider: $TRANSCRIPTION_PROVIDER"

# Execute the main command
exec "$@"