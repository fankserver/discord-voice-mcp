#!/bin/bash

echo "📦 Installing Vosk Speech Recognition"
echo "====================================="
echo ""

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Create models directory
mkdir -p models

echo "Available Vosk models:"
echo "1) vosk-model-small-en-us-0.15 (40 MB) - Fast, lower accuracy"
echo "2) vosk-model-en-us-0.22 (1.8 GB) - Balanced"
echo "3) vosk-model-en-us-0.22-lgraph (128 MB) - Good balance"
echo "4) vosk-model-en-us-0.42-gigaspeech (2.3 GB) - Best accuracy"
echo ""
read -p "Select model to download (1-4): " MODEL_CHOICE

case $MODEL_CHOICE in
    1)
        MODEL_NAME="vosk-model-small-en-us-0.15"
        MODEL_URL="https://alphacephei.com/vosk/models/vosk-model-small-en-us-0.15.zip"
        ;;
    2)
        MODEL_NAME="vosk-model-en-us-0.22"
        MODEL_URL="https://alphacephei.com/vosk/models/vosk-model-en-us-0.22.zip"
        ;;
    3)
        MODEL_NAME="vosk-model-en-us-0.22-lgraph"
        MODEL_URL="https://alphacephei.com/vosk/models/vosk-model-en-us-0.22-lgraph.zip"
        ;;
    4)
        MODEL_NAME="vosk-model-en-us-0.42-gigaspeech"
        MODEL_URL="https://alphacephei.com/vosk/models/vosk-model-en-us-0.42-gigaspeech.zip"
        ;;
    *)
        echo "Invalid choice, defaulting to small model"
        MODEL_NAME="vosk-model-small-en-us-0.15"
        MODEL_URL="https://alphacephei.com/vosk/models/vosk-model-small-en-us-0.15.zip"
        ;;
esac

cd models

# Check if model already exists
if [ -d "$MODEL_NAME" ]; then
    echo -e "${GREEN}✅ Model $MODEL_NAME already exists${NC}"
else
    echo "Downloading $MODEL_NAME..."
    wget -O "$MODEL_NAME.zip" "$MODEL_URL"
    
    echo "Extracting model..."
    unzip "$MODEL_NAME.zip"
    rm "$MODEL_NAME.zip"
    
    echo -e "${GREEN}✅ Model downloaded and extracted${NC}"
fi

cd ..

# Update .env with model path
if [ -f .env ]; then
    # Check if VOSK_MODEL_PATH exists in .env
    if grep -q "VOSK_MODEL_PATH" .env; then
        # Update existing path
        sed -i.bak "s|VOSK_MODEL_PATH=.*|VOSK_MODEL_PATH=./models/$MODEL_NAME|" .env
        echo -e "${GREEN}✅ Updated VOSK_MODEL_PATH in .env${NC}"
    else
        # Add new path
        echo "VOSK_MODEL_PATH=./models/$MODEL_NAME" >> .env
        echo -e "${GREEN}✅ Added VOSK_MODEL_PATH to .env${NC}"
    fi
fi

echo ""
echo -e "${GREEN}✅ Vosk setup complete!${NC}"
echo -e "Model installed at: ./models/$MODEL_NAME"