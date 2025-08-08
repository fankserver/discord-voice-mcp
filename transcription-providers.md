# Alternative Speech-to-Text Providers for Discord Voice MCP

## 🎯 Provider Comparison

| Provider | Cost | Real-time | Self-hosted | Accuracy | Setup Difficulty |
|----------|------|-----------|-------------|----------|------------------|
| **Whisper.cpp** | Free | ✅ | ✅ | High | Medium |
| **Vosk** | Free | ✅ | ✅ | Good | Easy |
| **Deepgram** | $200 free credits | ✅ | ❌ | Excellent | Easy |
| **Azure Speech** | Free tier (5h/month) | ✅ | ❌ | Excellent | Medium |
| **Google Cloud** | Free tier ($300) | ✅ | ❌ | Excellent | Medium |
| **Faster-Whisper** | Free | ✅ | ✅ | High | Medium |
| **SpeechRecognition** | Free | ❌ | ✅ | Good | Easy |

## 📦 Multi-Provider Transcription Service

### transcription.js - Flexible Provider System

```javascript
import fs from 'fs';
import { spawn } from 'child_process';
import WebSocket from 'ws';
import axios from 'axios';
import vosk from 'vosk';
import { Deepgram } from '@deepgram/sdk';
import speech from '@google-cloud/speech';

// Provider interface
class TranscriptionProvider {
  async initialize() {}
  async transcribe(audioBuffer) {}
  async transcribeStream(audioStream) {}
  async cleanup() {}
}

// 1. Whisper.cpp Provider (Self-hosted, Free)
class WhisperCppProvider extends TranscriptionProvider {
  constructor(modelPath = './models/ggml-base.en.bin') {
    super();
    this.modelPath = modelPath;
    this.whisperPath = './whisper.cpp/main'; // Path to compiled whisper.cpp
  }

  async transcribe(audioBuffer) {
    // Save audio to temp file
    const tempFile = `/tmp/audio_${Date.now()}.wav`;
    fs.writeFileSync(tempFile, audioBuffer);

    return new Promise((resolve, reject) => {
      const whisper = spawn(this.whisperPath, [
        '-m', this.modelPath,
        '-f', tempFile,
        '--no-timestamps',
        '--output-json'
      ]);

      let output = '';
      whisper.stdout.on('data', (data) => {
        output += data.toString();
      });

      whisper.on('close', (code) => {
        fs.unlinkSync(tempFile);
        if (code === 0) {
          try {
            const result = JSON.parse(output);
            resolve(result.text);
          } catch {
            resolve(output.trim());
          }
        } else {
          reject(new Error(`Whisper process exited with code ${code}`));
        }
      });
    });
  }

  async transcribeStream(audioStream) {
    // For real-time, use whisper.cpp stream mode
    const whisper = spawn(this.whisperPath, [
      '-m', this.modelPath,
      '--stream',
      '--step', '3000',  // Process every 3 seconds
      '--length', '10000' // 10 second chunks
    ]);

    audioStream.pipe(whisper.stdin);

    return new Promise((resolve) => {
      const transcripts = [];
      
      whisper.stdout.on('data', (data) => {
        const text = data.toString().trim();
        if (text) {
          transcripts.push(text);
        }
      });

      whisper.on('close', () => {
        resolve(transcripts.join(' '));
      });
    });
  }
}

// 2. Vosk Provider (Self-hosted, Free, Lightweight)
class VoskProvider extends TranscriptionProvider {
  constructor(modelPath = './models/vosk-model-en-us-0.22') {
    super();
    this.modelPath = modelPath;
    this.recognizer = null;
  }

  async initialize() {
    vosk.setLogLevel(0);
    const model = new vosk.Model(this.modelPath);
    this.recognizer = new vosk.Recognizer({
      model: model,
      sampleRate: 48000
    });
  }

  async transcribe(audioBuffer) {
    if (!this.recognizer) {
      await this.initialize();
    }

    // Process PCM audio buffer
    const chunkSize = 4000;
    for (let i = 0; i < audioBuffer.length; i += chunkSize) {
      const chunk = audioBuffer.slice(i, Math.min(i + chunkSize, audioBuffer.length));
      this.recognizer.acceptWaveform(chunk);
    }

    const result = this.recognizer.finalResult();
    return JSON.parse(result).text;
  }

  async transcribeStream(audioStream) {
    if (!this.recognizer) {
      await this.initialize();
    }

    return new Promise((resolve) => {
      const transcripts = [];

      audioStream.on('data', (chunk) => {
        if (this.recognizer.acceptWaveform(chunk)) {
          const result = JSON.parse(this.recognizer.result());
          if (result.text) {
            transcripts.push(result.text);
          }
        }
      });

      audioStream.on('end', () => {
        const finalResult = JSON.parse(this.recognizer.finalResult());
        if (finalResult.text) {
          transcripts.push(finalResult.text);
        }
        resolve(transcripts.join(' '));
      });
    });
  }

  async cleanup() {
    if (this.recognizer) {
      this.recognizer.free();
    }
  }
}

// 3. Deepgram Provider (Cloud, Free credits, Real-time)
class DeepgramProvider extends TranscriptionProvider {
  constructor(apiKey) {
    super();
    this.deepgram = new Deepgram(apiKey);
  }

  async transcribeStream(audioStream) {
    const deepgramLive = this.deepgram.transcription.live({
      model: 'nova-2',
      language: 'en',
      smart_format: true,
      interim_results: true,
      utterance_end_ms: 1000,
      vad_events: true
    });

    return new Promise((resolve, reject) => {
      const transcripts = [];

      deepgramLive.on('transcriptReceived', (message) => {
        const data = JSON.parse(message);
        if (data.channel?.alternatives?.[0]?.transcript) {
          transcripts.push(data.channel.alternatives[0].transcript);
        }
      });

      deepgramLive.on('error', reject);
      
      deepgramLive.on('close', () => {
        resolve(transcripts.join(' '));
      });

      // Pipe audio to Deepgram
      audioStream.on('data', (chunk) => {
        deepgramLive.send(chunk);
      });

      audioStream.on('end', () => {
        deepgramLive.finish();
      });
    });
  }
}

// 4. Faster-Whisper Provider (Python bridge, Self-hosted)
class FasterWhisperProvider extends TranscriptionProvider {
  constructor() {
    super();
    this.pythonScript = `
import sys
import json
from faster_whisper import WhisperModel

model = WhisperModel("base", device="cpu", compute_type="int8")

# Read audio from stdin
audio_data = sys.stdin.buffer.read()

# Transcribe
segments, info = model.transcribe(audio_data, beam_size=5)

# Output transcription
text = " ".join([segment.text for segment in segments])
print(json.dumps({"text": text}))
    `;
  }

  async transcribe(audioBuffer) {
    return new Promise((resolve, reject) => {
      const python = spawn('python3', ['-c', this.pythonScript]);
      
      let output = '';
      python.stdout.on('data', (data) => {
        output += data.toString();
      });

      python.stderr.on('data', (data) => {
        console.error('Python error:', data.toString());
      });

      python.on('close', (code) => {
        if (code === 0) {
          try {
            const result = JSON.parse(output);
            resolve(result.text);
          } catch (e) {
            reject(e);
          }
        } else {
          reject(new Error(`Python process exited with code ${code}`));
        }
      });

      // Send audio to Python process
      python.stdin.write(audioBuffer);
      python.stdin.end();
    });
  }
}

// 5. Azure Speech Provider (Cloud, Free tier available)
class AzureSpeechProvider extends TranscriptionProvider {
  constructor(subscriptionKey, region) {
    super();
    this.subscriptionKey = subscriptionKey;
    this.region = region;
    this.endpoint = `wss://${region}.stt.speech.microsoft.com/speech/recognition/conversation/cognitiveservices/v1`;
  }

  async transcribeStream(audioStream) {
    const ws = new WebSocket(this.endpoint, {
      headers: {
        'Ocp-Apim-Subscription-Key': this.subscriptionKey,
        'Content-Type': 'audio/wav; codec=audio/pcm; samplerate=48000'
      }
    });

    return new Promise((resolve, reject) => {
      const transcripts = [];

      ws.on('open', () => {
        // Send audio format header
        const header = {
          context: {
            system: {
              name: 'discord-voice-mcp',
              version: '1.0.0'
            }
          },
          audio: {
            format: 'pcm',
            sampleRate: 48000,
            channels: 2,
            bitsPerSample: 16
          }
        };
        ws.send(JSON.stringify(header));
      });

      ws.on('message', (data) => {
        const result = JSON.parse(data);
        if (result.DisplayText) {
          transcripts.push(result.DisplayText);
        }
      });

      ws.on('error', reject);
      ws.on('close', () => resolve(transcripts.join(' ')));

      // Stream audio to Azure
      audioStream.on('data', (chunk) => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(chunk);
        }
      });

      audioStream.on('end', () => {
        ws.close();
      });
    });
  }
}

// Main Transcription Service with Provider Selection
export class TranscriptionService {
  constructor(config) {
    this.config = config;
    this.provider = null;
    this.initializeProvider();
  }

  initializeProvider() {
    const providerType = this.config.provider || 'vosk';
    
    switch (providerType) {
      case 'whisper.cpp':
        this.provider = new WhisperCppProvider(this.config.whisperModel);
        break;
      
      case 'vosk':
        this.provider = new VoskProvider(this.config.voskModel);
        break;
      
      case 'deepgram':
        if (!this.config.deepgramApiKey) {
          throw new Error('Deepgram API key required');
        }
        this.provider = new DeepgramProvider(this.config.deepgramApiKey);
        break;
      
      case 'faster-whisper':
        this.provider = new FasterWhisperProvider();
        break;
      
      case 'azure':
        if (!this.config.azureKey || !this.config.azureRegion) {
          throw new Error('Azure credentials required');
        }
        this.provider = new AzureSpeechProvider(
          this.config.azureKey,
          this.config.azureRegion
        );
        break;
      
      default:
        // Default to Vosk as it's free and works offline
        this.provider = new VoskProvider();
    }
  }

  async transcribe(audioBuffer) {
    return await this.provider.transcribe(audioBuffer);
  }

  async transcribeStream(audioStream) {
    return await this.provider.transcribeStream(audioStream);
  }

  async cleanup() {
    if (this.provider.cleanup) {
      await this.provider.cleanup();
    }
  }
}
```

## 🔧 Environment Configuration

### .env file with multiple providers

```env
# Discord Configuration
DISCORD_TOKEN=your_discord_bot_token
DISCORD_CLIENT_ID=your_client_id
DISCORD_GUILD_ID=your_guild_id

# Transcription Provider Selection
# Options: whisper.cpp, vosk, deepgram, faster-whisper, azure, google
TRANSCRIPTION_PROVIDER=vosk

# Vosk Configuration (Free, Offline)
VOSK_MODEL_PATH=./models/vosk-model-en-us-0.22

# Whisper.cpp Configuration (Free, Offline)
WHISPER_MODEL_PATH=./models/ggml-base.en.bin
WHISPER_EXECUTABLE=./whisper.cpp/main

# Deepgram Configuration (Free credits available)
DEEPGRAM_API_KEY=your_deepgram_api_key

# Azure Speech Configuration (Free tier: 5 hours/month)
AZURE_SPEECH_KEY=your_azure_key
AZURE_SPEECH_REGION=your_region

# Google Cloud Speech (Free tier: $300 credits)
GOOGLE_APPLICATION_CREDENTIALS=./google-credentials.json
```

## 📥 Installation Scripts

### install-whisper-cpp.sh

```bash
#!/bin/bash
# Install Whisper.cpp
git clone https://github.com/ggerganov/whisper.cpp.git
cd whisper.cpp
make

# Download base English model
bash ./models/download-ggml-model.sh base.en

echo "Whisper.cpp installed successfully!"
```

### install-vosk.sh

```bash
#!/bin/bash
# Install Vosk
npm install vosk

# Download English model
mkdir -p models
cd models
wget https://alphacephei.com/vosk/models/vosk-model-en-us-0.22.zip
unzip vosk-model-en-us-0.22.zip
rm vosk-model-en-us-0.22.zip

echo "Vosk installed successfully!"
```

### install-faster-whisper.sh

```bash
#!/bin/bash
# Install Faster-Whisper (requires Python)
pip install faster-whisper

echo "Faster-Whisper installed successfully!"
```

## 🚀 Quick Start Guide

### 1. Choose Your Provider

**For completely free, offline operation:**
- **Vosk**: Fastest setup, good accuracy, lightweight
- **Whisper.cpp**: Better accuracy, more resource intensive

**For best accuracy with some free tier:**
- **Deepgram**: $200 free credits, excellent real-time
- **Azure**: 5 hours free/month, enterprise-grade

### 2. Install Dependencies

```bash
# Core dependencies
npm install discord.js @discordjs/voice prism-media

# Provider-specific
npm install vosk              # For Vosk
npm install @deepgram/sdk     # For Deepgram
npm install @google-cloud/speech  # For Google
npm install @azure/cognitiveservices-speech-sdk  # For Azure
```

### 3. Download Models (for offline providers)

```bash
# Vosk model (50MB)
wget https://alphacephei.com/vosk/models/vosk-model-small-en-us-0.15.zip

# Whisper.cpp model (140MB for base)
./models/download-ggml-model.sh base.en
```

## 🎯 Provider Recommendations

### Best for Different Use Cases

**Privacy-focused / Offline:**
```javascript
// Use Vosk or Whisper.cpp
const transcription = new TranscriptionService({
  provider: 'vosk',
  voskModel: './models/vosk-model-en-us-0.22'
});
```

**Best accuracy / Real-time:**
```javascript
// Use Deepgram
const transcription = new TranscriptionService({
  provider: 'deepgram',
  deepgramApiKey: process.env.DEEPGRAM_API_KEY
});
```

**Resource-constrained (Raspberry Pi, etc):**
```javascript
// Use Vosk with small model
const transcription = new TranscriptionService({
  provider: 'vosk',
  voskModel: './models/vosk-model-small-en-us-0.15'
});
```

**Multi-language support:**
```javascript
// Use Whisper.cpp with multilingual model
const transcription = new TranscriptionService({
  provider: 'whisper.cpp',
  whisperModel: './models/ggml-base.bin' // Supports 100+ languages
});
```

## 📊 Performance Comparison

| Provider | Speed (RTF) | RAM Usage | CPU Usage | Accuracy |
|----------|------------|-----------|-----------|----------|
| Vosk (small) | 0.1x | ~50MB | Low | 85% |
| Vosk (large) | 0.3x | ~500MB | Medium | 92% |
| Whisper.cpp (base) | 0.5x | ~500MB | Medium | 95% |
| Whisper.cpp (large) | 2x | ~2GB | High | 98% |
| Deepgram | 0.05x | N/A | N/A | 97% |

*RTF = Real-Time Factor (lower is faster)*

## 🔌 Integration with MCP Server

Update your MCP server to use the multi-provider system:

```javascript
// In mcp-server.js
import { TranscriptionService } from './transcription.js';

class DiscordVoiceMCP {
  constructor() {
    this.transcriptionService = new TranscriptionService({
      provider: process.env.TRANSCRIPTION_PROVIDER || 'vosk',
      voskModel: process.env.VOSK_MODEL_PATH,
      whisperModel: process.env.WHISPER_MODEL_PATH,
      deepgramApiKey: process.env.DEEPGRAM_API_KEY,
      azureKey: process.env.AZURE_SPEECH_KEY,
      azureRegion: process.env.AZURE_SPEECH_REGION
    });
    
    // Rest of initialization...
  }
}
```

This gives you complete flexibility to choose the transcription provider that best fits your needs, budget, and privacy requirements!