# Discord Voice Transcription Bot - Complete Research & Design

## 📋 Core Architecture

### Technology Stack
- **Discord.js v14** + **@discordjs/voice** for Discord interaction
- **prism-media** for audio decoding (Opus to PCM)
- **Speech-to-Text Options:**
  - OpenAI Whisper API (best accuracy, multilingual)
  - Google Cloud Speech-to-Text V2 (real-time streaming)
  - Free alternatives: WitAI, Vosk

## 🎙️ Voice Channel Audio Capture

### Key Implementation Details

```javascript
// Join voice channel with audio reception
const connection = joinVoiceChannel({
    channelId: voiceChannel.id,
    guildId: guild.id,
    adapterCreator: guild.voiceAdapterCreator,
    selfDeaf: false // Required for receiving audio
});

// Subscribe to user audio stream
const opusStream = connection.receiver.subscribe(userId, {
    end: {
        behavior: EndBehaviorType.AfterSilence,
        duration: 100
    }
});

// Decode Opus to PCM
const decoder = new prism.opus.Decoder({
    frameSize: 960,
    channels: 2,
    rate: 48000
});
```

## 🔊 Audio Processing Pipeline

1. **Capture**: Opus packets from Discord (48kHz, stereo)
2. **Decode**: Convert to PCM (s16le format)
3. **Buffer**: Accumulate audio chunks for transcription
4. **Transcribe**: Send to STT API
5. **Output**: Display transcription in text channel

## 🤖 Speech-to-Text API Comparison

| Service | Pros | Cons | Cost |
|---------|------|------|------|
| **OpenAI Whisper** | Best accuracy, 50+ languages, simple API | 25MB file limit, not real-time streaming | $0.006/minute |
| **Google Cloud STT V2** | Real-time streaming, speaker diarization | Complex setup, requires GCP account | $0.016/minute |
| **WitAI** | Free, 120+ languages | Lower accuracy, rate limits | Free |
| **Vosk** | Offline, privacy-focused | Requires model download, CPU intensive | Free |

## 🏗️ Project Structure

```
discord-transcriber/
├── src/
│   ├── index.js           # Main bot entry
│   ├── commands/          # Slash commands
│   │   ├── join.js        # Join voice channel
│   │   ├── leave.js       # Leave voice channel
│   │   └── transcribe.js  # Start/stop transcription
│   ├── services/
│   │   ├── audioCapture.js    # Voice recording logic
│   │   ├── transcription.js   # STT API integration
│   │   └── storage.js         # Save transcripts
│   └── utils/
│       └── config.js      # Configuration management
├── .env                   # Environment variables
├── .gitignore
├── package.json
└── README.md
```

## 🔐 Security & Configuration

### Environment Variables (.env file)

```env
DISCORD_TOKEN=your_bot_token_here
CLIENT_ID=your_client_id
GUILD_ID=your_guild_id

# Speech-to-Text API Keys
OPENAI_API_KEY=sk-...
GOOGLE_APPLICATION_CREDENTIALS=./path/to/service-account.json
WITAI_TOKEN=your_witai_token

# Bot Settings
TRANSCRIPTION_SERVICE=whisper # whisper|google|witai|vosk
OUTPUT_CHANNEL_ID=123456789
SAVE_AUDIO=false
```

### Security Best Practices

1. **Never commit tokens to Git** - Use .gitignore for .env files
2. **Token regeneration** - Regenerate tokens if compromised
3. **Environment-specific configs** - Different tokens for dev/prod
4. **Secure token storage** - Use environment variables, not hardcoded values

## 💻 Complete Implementation Example

### Main Bot File (index.js)

```javascript
require('dotenv').config();
const { Client, GatewayIntentBits } = require('discord.js');
const { joinVoiceChannel, EndBehaviorType, VoiceConnectionStatus } = require('@discordjs/voice');
const prism = require('prism-media');
const OpenAI = require('openai');
const fs = require('fs');
const { pipeline } = require('stream');

const client = new Client({
    intents: [
        GatewayIntentBits.Guilds,
        GatewayIntentBits.GuildVoiceStates,
        GatewayIntentBits.GuildMessages,
        GatewayIntentBits.MessageContent
    ]
});

const openai = new OpenAI({ apiKey: process.env.OPENAI_API_KEY });
const activeConnections = new Map();

class VoiceTranscriber {
    constructor(connection, textChannel) {
        this.connection = connection;
        this.textChannel = textChannel;
        this.recordings = new Map();
    }

    async startRecording(userId, username) {
        const opusStream = this.connection.receiver.subscribe(userId, {
            end: {
                behavior: EndBehaviorType.AfterSilence,
                duration: 1000
            }
        });

        const decoder = new prism.opus.Decoder({
            frameSize: 960,
            channels: 2,
            rate: 48000
        });

        const chunks = [];
        
        decoder.on('data', chunk => {
            chunks.push(chunk);
        });

        decoder.on('end', async () => {
            const buffer = Buffer.concat(chunks);
            const tempFile = `./temp_${userId}_${Date.now()}.wav`;
            
            // Convert PCM to WAV
            await this.saveAsWav(buffer, tempFile);
            
            // Transcribe with Whisper
            try {
                const transcription = await openai.audio.transcriptions.create({
                    file: fs.createReadStream(tempFile),
                    model: "whisper-1"
                });
                
                // Send transcription to text channel
                await this.textChannel.send(`**${username}:** ${transcription.text}`);
            } catch (error) {
                console.error('Transcription error:', error);
            } finally {
                // Clean up temp file
                fs.unlinkSync(tempFile);
            }
        });

        pipeline(opusStream, decoder, (err) => {
            if (err) console.error('Pipeline error:', err);
        });
    }

    async saveAsWav(pcmBuffer, filepath) {
        // WAV header for 48kHz, 16-bit, stereo
        const header = Buffer.alloc(44);
        header.write('RIFF', 0);
        header.writeUInt32LE(pcmBuffer.length + 36, 4);
        header.write('WAVE', 8);
        header.write('fmt ', 12);
        header.writeUInt32LE(16, 16);
        header.writeUInt16LE(1, 20);
        header.writeUInt16LE(2, 22);
        header.writeUInt32LE(48000, 24);
        header.writeUInt32LE(48000 * 2 * 2, 28);
        header.writeUInt16LE(4, 32);
        header.writeUInt16LE(16, 34);
        header.write('data', 36);
        header.writeUInt32LE(pcmBuffer.length, 40);
        
        const wav = Buffer.concat([header, pcmBuffer]);
        fs.writeFileSync(filepath, wav);
    }
}

// Bot commands
client.on('messageCreate', async message => {
    if (!message.content.startsWith('!')) return;

    const args = message.content.slice(1).split(' ');
    const command = args[0].toLowerCase();

    if (command === 'join') {
        const voiceChannel = message.member.voice.channel;
        if (!voiceChannel) {
            return message.reply('You need to be in a voice channel!');
        }

        const connection = joinVoiceChannel({
            channelId: voiceChannel.id,
            guildId: message.guild.id,
            adapterCreator: message.guild.voiceAdapterCreator,
            selfDeaf: false
        });

        const transcriber = new VoiceTranscriber(connection, message.channel);
        activeConnections.set(message.guild.id, transcriber);

        // Listen for users speaking
        connection.receiver.speaking.on('start', userId => {
            const member = message.guild.members.cache.get(userId);
            if (member) {
                transcriber.startRecording(userId, member.displayName);
            }
        });

        message.reply('Joined voice channel and started transcribing!');
    }

    if (command === 'leave') {
        const transcriber = activeConnections.get(message.guild.id);
        if (transcriber) {
            transcriber.connection.destroy();
            activeConnections.delete(message.guild.id);
            message.reply('Left voice channel.');
        }
    }
});

client.once('ready', () => {
    console.log(`Logged in as ${client.user.tag}!`);
});

client.login(process.env.DISCORD_TOKEN);
```

## 🚀 Claude Code Integration Strategy

### Development Workflow

1. **Initial Setup**
   ```bash
   # Create project directory
   mkdir discord-transcriber && cd discord-transcriber
   
   # Initialize npm project
   npm init -y
   
   # Install dependencies
   npm install discord.js @discordjs/voice prism-media openai dotenv
   npm install --save-dev nodemon
   ```

2. **Package.json Scripts**
   ```json
   {
     "scripts": {
       "start": "node src/index.js",
       "dev": "nodemon src/index.js"
     }
   }
   ```

3. **Local Development**
   - Run bot locally: `npm run dev`
   - Test in personal Discord server
   - Monitor logs in Claude Code terminal

4. **Deployment Options**
   - **Local**: Run directly from Claude Code terminal
   - **VPS**: Deploy to cloud server (AWS, DigitalOcean)
   - **Heroku**: Free tier available
   - **Railway/Render**: Modern alternatives

## 📊 Advanced Features

### Real-time Streaming (Google Cloud)

```javascript
const speech = require('@google-cloud/speech');
const client = new speech.SpeechClient();

const request = {
    config: {
        encoding: 'LINEAR16',
        sampleRateHertz: 48000,
        languageCode: 'en-US',
    },
    interimResults: true,
};

const recognizeStream = client
    .streamingRecognize(request)
    .on('data', data => {
        const transcript = data.results[0].alternatives[0].transcript;
        // Send real-time updates
    });

// Pipe audio stream to recognizer
audioStream.pipe(recognizeStream);
```

### Multi-language Support

```javascript
// Detect language with Whisper
const transcription = await openai.audio.transcriptions.create({
    file: audioFile,
    model: "whisper-1",
    response_format: "verbose_json" // Includes language detection
});

console.log(`Detected language: ${transcription.language}`);
```

### Speaker Diarization (Google Cloud)

```javascript
const config = {
    encoding: 'LINEAR16',
    sampleRateHertz: 48000,
    languageCode: 'en-US',
    enableSpeakerDiarization: true,
    diarizationSpeakerCount: 2, // Expected number of speakers
};
```

## 🔧 Troubleshooting Guide

### Common Issues and Solutions

1. **Audio Quality Issues**
   - Ensure 48kHz sample rate matches Discord's output
   - Check Opus decoder settings (frameSize: 960)
   - Verify PCM conversion is 16-bit signed little-endian

2. **API Rate Limits**
   - Implement queuing system for transcription requests
   - Batch audio segments (30-60 seconds)
   - Add exponential backoff retry logic

3. **Memory Management**
   - Clean up temp files immediately after use
   - Limit recording duration (max 5 minutes per segment)
   - Use streaming instead of buffering when possible

4. **Bot Permissions**
   - Ensure bot has CONNECT and SPEAK permissions
   - Check voice channel user limit
   - Verify bot role hierarchy

## 📈 Performance Optimizations

### Audio Processing

- **Chunking Strategy**: Process 30-second segments for optimal API usage
- **Parallel Processing**: Use worker threads for multiple simultaneous transcriptions
- **Caching**: Store frequently transcribed phrases locally
- **Compression**: Convert to compressed formats before API calls

### Resource Management

```javascript
// Implement recording timeout
setTimeout(() => {
    if (recording) {
        recording.destroy();
        console.log('Recording timeout reached');
    }
}, 5 * 60 * 1000); // 5 minutes
```

## 🛠️ Additional Tools and Libraries

### Recommended NPM Packages

```json
{
  "dependencies": {
    "discord.js": "^14.14.1",
    "@discordjs/voice": "^0.16.1",
    "@discordjs/opus": "^0.9.0",
    "prism-media": "^1.3.5",
    "openai": "^4.28.0",
    "@google-cloud/speech": "^6.3.0",
    "dotenv": "^16.4.1",
    "winston": "^3.11.0", // Logging
    "node-cache": "^5.1.2" // Caching
  },
  "devDependencies": {
    "nodemon": "^3.0.3",
    "eslint": "^8.56.0"
  }
}
```

### FFmpeg Installation

```bash
# Ubuntu/Debian
sudo apt update && sudo apt install ffmpeg

# macOS
brew install ffmpeg

# Windows (using chocolatey)
choco install ffmpeg
```

## 📝 API Key Setup Guide

### OpenAI Whisper

1. Visit https://platform.openai.com/api-keys
2. Create new secret key
3. Add to .env: `OPENAI_API_KEY=sk-...`

### Google Cloud Speech-to-Text

1. Create GCP project at https://console.cloud.google.com
2. Enable Speech-to-Text API
3. Create service account and download JSON key
4. Set environment variable: `GOOGLE_APPLICATION_CREDENTIALS=./service-account.json`

### WitAI

1. Visit https://wit.ai
2. Create new app
3. Copy Server Access Token
4. Add to .env: `WITAI_TOKEN=...`

## 🎯 Use Cases

1. **Meeting Transcription**: Record and transcribe voice meetings
2. **Accessibility**: Help hearing-impaired users participate
3. **Moderation**: Real-time content monitoring
4. **Language Learning**: Transcribe and translate conversations
5. **Content Creation**: Generate podcast transcripts
6. **Command Bot**: Voice-controlled Discord bot

## 📚 Resources and References

### Official Documentation
- [Discord.js Guide](https://discordjs.guide/)
- [Discord.js Voice Documentation](https://discord.js.org/docs/packages/voice/main)
- [OpenAI Whisper API](https://platform.openai.com/docs/guides/speech-to-text)
- [Google Cloud Speech-to-Text](https://cloud.google.com/speech-to-text/docs)

### Community Resources
- [Discord.js Discord Server](https://discord.gg/djs)
- [GitHub Examples](https://github.com/discordjs/voice-examples)
- [Stack Overflow Discord.js Tag](https://stackoverflow.com/questions/tagged/discord.js)

### Related Projects
- [DiscordSpeechBot](https://github.com/inevolin/DiscordSpeechBot)
- [discord-transcriber](https://github.com/dtinth/discord-transcriber)
- [VoiceBot](https://github.com/moo-gn/VoiceBot)

## 🚦 Getting Started Checklist

- [ ] Create Discord application and bot
- [ ] Set up development environment
- [ ] Install Node.js dependencies
- [ ] Configure .env file with tokens
- [ ] Choose speech-to-text service
- [ ] Set up API credentials
- [ ] Test bot in private server
- [ ] Implement error handling
- [ ] Add logging system
- [ ] Deploy to production

## 📄 License and Legal Considerations

- Ensure compliance with Discord's Terms of Service
- Respect user privacy - inform users about recording
- Follow GDPR/privacy regulations for storing transcripts
- Check speech-to-text API usage terms
- Consider implementing opt-in/opt-out mechanisms

---

*Last updated: January 2025*
*Research compiled for Discord.js v14 and latest API versions*