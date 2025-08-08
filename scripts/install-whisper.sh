#!/bin/bash

echo "🎯 Installing Whisper.cpp"
echo "========================"
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Check for build tools
echo "Checking build tools..."
if ! command -v make &> /dev/null; then
    echo -e "${RED}❌ 'make' is required but not installed${NC}"
    echo "Please install build-essential (Ubuntu/Debian) or base-devel (Arch) or Xcode (macOS)"
    exit 1
fi

if ! command -v gcc &> /dev/null && ! command -v clang &> /dev/null; then
    echo -e "${RED}❌ C compiler (gcc or clang) is required${NC}"
    exit 1
fi
echo -e "${GREEN}✅ Build tools found${NC}"
echo ""

# Clone whisper.cpp if not exists
if [ ! -d "whisper.cpp" ]; then
    echo "Cloning whisper.cpp repository..."
    git clone https://github.com/ggerganov/whisper.cpp.git
    echo -e "${GREEN}✅ Repository cloned${NC}"
else
    echo "Updating whisper.cpp repository..."
    cd whisper.cpp
    git pull
    cd ..
    echo -e "${GREEN}✅ Repository updated${NC}"
fi
echo ""

# Build whisper.cpp
echo "Building whisper.cpp..."
cd whisper.cpp
make clean
make

if [ $? -ne 0 ]; then
    echo -e "${RED}❌ Build failed${NC}"
    exit 1
fi
echo -e "${GREEN}✅ Build successful${NC}"
cd ..
echo ""

# Download models
echo "Available Whisper models:"
echo "1) tiny.en (39 MB) - Fastest, lowest accuracy"
echo "2) base.en (74 MB) - Good balance (Recommended)"
echo "3) small.en (244 MB) - Better accuracy"
echo "4) medium.en (769 MB) - High accuracy"
echo "5) large-v3 (1.5 GB) - Best accuracy, multilingual"
echo ""
read -p "Select model to download (1-5): " MODEL_CHOICE

cd whisper.cpp/models

case $MODEL_CHOICE in
    1)
        MODEL_NAME="tiny.en"
        ;;
    2)
        MODEL_NAME="base.en"
        ;;
    3)
        MODEL_NAME="small.en"
        ;;
    4)
        MODEL_NAME="medium.en"
        ;;
    5)
        MODEL_NAME="large-v3"
        ;;
    *)
        echo "Invalid choice, defaulting to base.en"
        MODEL_NAME="base.en"
        ;;
esac

echo "Downloading $MODEL_NAME model..."
bash ./download-ggml-model.sh $MODEL_NAME

if [ $? -ne 0 ]; then
    echo -e "${RED}❌ Model download failed${NC}"
    exit 1
fi
echo -e "${GREEN}✅ Model downloaded${NC}"
cd ../..

# Create symlink in models directory
mkdir -p models
if [ ! -L "models/ggml-$MODEL_NAME.bin" ]; then
    ln -s "../whisper.cpp/models/ggml-$MODEL_NAME.bin" "models/ggml-$MODEL_NAME.bin"
fi

# Update .env with paths
if [ -f .env ]; then
    # Update WHISPER_MODEL_PATH
    if grep -q "WHISPER_MODEL_PATH" .env; then
        sed -i.bak "s|WHISPER_MODEL_PATH=.*|WHISPER_MODEL_PATH=./models/ggml-$MODEL_NAME.bin|" .env
    else
        echo "WHISPER_MODEL_PATH=./models/ggml-$MODEL_NAME.bin" >> .env
    fi
    
    # Update WHISPER_EXECUTABLE
    if grep -q "WHISPER_EXECUTABLE" .env; then
        sed -i.bak "s|WHISPER_EXECUTABLE=.*|WHISPER_EXECUTABLE=./whisper.cpp/main|" .env
    else
        echo "WHISPER_EXECUTABLE=./whisper.cpp/main" >> .env
    fi
    
    echo -e "${GREEN}✅ Updated .env file${NC}"
fi

# Test whisper.cpp
echo ""
echo "Testing whisper.cpp installation..."
./whisper.cpp/main -h > /dev/null 2>&1
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ Whisper.cpp is working!${NC}"
else
    echo -e "${YELLOW}⚠️  Whisper.cpp test failed, please check the installation${NC}"
fi

echo ""
echo -e "${GREEN}🎉 Whisper.cpp setup complete!${NC}"
echo "Model: ./models/ggml-$MODEL_NAME.bin"
echo "Executable: ./whisper.cpp/main"