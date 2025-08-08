# Discord Voice Transcription MCP Server for Claude Code

## 🎯 Concept Overview

A Model Context Protocol (MCP) server that bridges Discord voice channels with Claude Code, enabling voice-based development workflows. Speak your ideas in Discord, and they appear as context in Claude Code for refinement and implementation.

## 🏗️ Architecture

```
┌─────────────────┐     ┌──────────────┐     ┌─────────────┐
│  Discord Voice  │────▶│  Discord Bot │────▶│ MCP Server  │
│    Channel      │     │   (Node.js)  │     │  (Node.js)  │
└─────────────────┘     └──────────────┘     └─────────────┘
                               │                     │
                               ▼                     ▼
                        ┌──────────────┐     ┌─────────────┐
                        │ Whisper API  │     │ Claude Code │
                        │(Transcription)│     │   Desktop   │
                        └──────────────┘     └─────────────┘
```

## 📁 Project Structure

```
discord-voice-mcp/
├── src/
│   ├── mcp-server.js      # MCP server implementation
│   ├── discord-bot.js     # Discord bot with voice capture
│   ├── transcription.js   # Speech-to-text service
│   ├── session-manager.js # Manage transcription sessions
│   └── utils/
│       ├── audio.js       # Audio processing utilities
│       └── config.js      # Configuration management
├── .env
├── package.json
├── mcp-config.json        # MCP server configuration
└── README.md
```

## 💻 MCP Server Implementation

### Main MCP Server (mcp-server.js)

```javascript
#!/usr/bin/env node

import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { 
  CallToolRequestSchema, 
  ListResourcesRequestSchema,
  ReadResourceRequestSchema 
} from "@modelcontextprotocol/sdk/types.js";
import { DiscordVoiceBot } from './discord-bot.js';
import { SessionManager } from './session-manager.js';
import { z } from 'zod';

class DiscordVoiceMCP {
  constructor() {
    this.bot = new DiscordVoiceBot();
    this.sessionManager = new SessionManager();
    this.server = new Server(
      {
        name: "discord-voice-transcription",
        version: "1.0.0"
      },
      {
        capabilities: {
          resources: {},
          tools: {}
        }
      }
    );

    this.setupHandlers();
  }

  setupHandlers() {
    // Tool handler
    this.server.setRequestHandler(CallToolRequestSchema, async (request) => {
      const { name, arguments: args } = request.params;

      switch (name) {
        case "start_voice_session":
          return await this.startVoiceSession(args);
        
        case "stop_voice_session":
          return await this.stopVoiceSession(args);
        
        case "get_transcript":
          return await this.getTranscript(args);
        
        case "clear_transcript":
          return await this.clearTranscript(args);
        
        case "join_voice_channel":
          return await this.joinVoiceChannel(args);
        
        case "leave_voice_channel":
          return await this.leaveVoiceChannel(args);
        
        case "list_active_sessions":
          return await this.listActiveSessions();
        
        case "set_transcription_mode":
          return await this.setTranscriptionMode(args);

        default:
          throw new Error(`Unknown tool: ${name}`);
      }
    });

    // Resource handler for transcripts
    this.server.setRequestHandler(ListResourcesRequestSchema, async () => {
      const sessions = this.sessionManager.getAllSessions();
      return {
        resources: sessions.map(session => ({
          uri: `transcript://${session.id}`,
          name: `Transcript: ${session.name}`,
          description: `Voice transcript from ${session.startTime}`,
          mimeType: "text/plain"
        }))
      };
    });

    // Read resource handler
    this.server.setRequestHandler(ReadResourceRequestSchema, async (request) => {
      const { uri } = request.params;
      if (uri.startsWith("transcript://")) {
        const sessionId = uri.replace("transcript://", "");
        const transcript = this.sessionManager.getTranscript(sessionId);
        return {
          contents: [{
            uri,
            mimeType: "text/plain",
            text: transcript
          }]
        };
      }
      throw new Error(`Unknown resource: ${uri}`);
    });

    // Define available tools
    this.server.setRequestHandler(ListToolsRequestSchema, async () => {
      return {
        tools: [
          {
            name: "start_voice_session",
            description: "Start a new voice transcription session",
            inputSchema: {
              type: "object",
              properties: {
                sessionName: {
                  type: "string",
                  description: "Name for this transcription session"
                },
                channelId: {
                  type: "string",
                  description: "Discord voice channel ID"
                }
              },
              required: ["sessionName"]
            }
          },
          {
            name: "stop_voice_session",
            description: "Stop the current voice transcription session",
            inputSchema: {
              type: "object",
              properties: {
                sessionId: {
                  type: "string",
                  description: "Session ID to stop"
                }
              }
            }
          },
          {
            name: "get_transcript",
            description: "Get the current transcript from the active session",
            inputSchema: {
              type: "object",
              properties: {
                sessionId: {
                  type: "string",
                  description: "Session ID to get transcript from"
                },
                lastNMinutes: {
                  type: "number",
                  description: "Get transcript from last N minutes only"
                }
              }
            }
          },
          {
            name: "clear_transcript",
            description: "Clear the transcript buffer for a session",
            inputSchema: {
              type: "object",
              properties: {
                sessionId: {
                  type: "string",
                  description: "Session ID to clear"
                }
              }
            }
          },
          {
            name: "join_voice_channel",
            description: "Join a Discord voice channel",
            inputSchema: {
              type: "object",
              properties: {
                channelId: {
                  type: "string",
                  description: "Discord voice channel ID"
                },
                guildId: {
                  type: "string",
                  description: "Discord guild/server ID"
                }
              },
              required: ["channelId", "guildId"]
            }
          },
          {
            name: "set_transcription_mode",
            description: "Set transcription mode (continuous/push-to-talk)",
            inputSchema: {
              type: "object",
              properties: {
                mode: {
                  type: "string",
                  enum: ["continuous", "push-to-talk", "voice-activity"],
                  description: "Transcription mode"
                }
              },
              required: ["mode"]
            }
          }
        ]
      };
    });
  }

  async startVoiceSession(args) {
    const session = await this.sessionManager.createSession(args.sessionName);
    
    // Set up real-time transcription callback
    this.bot.onTranscription((userId, username, text) => {
      this.sessionManager.addTranscript(session.id, {
        userId,
        username,
        text,
        timestamp: new Date().toISOString()
      });
    });

    return {
      content: [
        {
          type: "text",
          text: `Started voice session: ${session.id}\nName: ${args.sessionName}`
        }
      ]
    };
  }

  async stopVoiceSession(args) {
    const transcript = this.sessionManager.getTranscript(args.sessionId);
    this.sessionManager.endSession(args.sessionId);
    
    return {
      content: [
        {
          type: "text",
          text: `Session stopped. Final transcript length: ${transcript.length} characters`
        }
      ]
    };
  }

  async getTranscript(args) {
    const transcript = this.sessionManager.getTranscript(
      args.sessionId, 
      args.lastNMinutes
    );
    
    return {
      content: [
        {
          type: "text",
          text: transcript
        }
      ]
    };
  }

  async joinVoiceChannel(args) {
    await this.bot.joinChannel(args.channelId, args.guildId);
    return {
      content: [
        {
          type: "text",
          text: `Joined voice channel ${args.channelId}`
        }
      ]
    };
  }

  async run() {
    // Initialize Discord bot
    await this.bot.initialize();
    
    // Start MCP server
    const transport = new StdioServerTransport();
    await this.server.connect(transport);
    
    console.error("Discord Voice MCP Server running");
  }
}

// Start the server
const mcp = new DiscordVoiceMCP();
mcp.run().catch(console.error);
```

### Discord Bot Integration (discord-bot.js)

```javascript
import { Client, GatewayIntentBits } from 'discord.js';
import { 
  joinVoiceChannel, 
  EndBehaviorType,
  VoiceConnectionStatus,
  entersState
} from '@discordjs/voice';
import { VoiceTranscriber } from './transcription.js';
import EventEmitter from 'events';

export class DiscordVoiceBot extends EventEmitter {
  constructor() {
    super();
    this.client = new Client({
      intents: [
        GatewayIntentBits.Guilds,
        GatewayIntentBits.GuildVoiceStates,
        GatewayIntentBits.GuildMessages
      ]
    });
    
    this.connections = new Map();
    this.transcribers = new Map();
  }

  async initialize() {
    await this.client.login(process.env.DISCORD_TOKEN);
    
    this.client.once('ready', () => {
      console.error(`Discord bot logged in as ${this.client.user.tag}`);
    });
  }

  async joinChannel(channelId, guildId) {
    const guild = this.client.guilds.cache.get(guildId);
    if (!guild) throw new Error('Guild not found');
    
    const channel = guild.channels.cache.get(channelId);
    if (!channel) throw new Error('Channel not found');
    
    const connection = joinVoiceChannel({
      channelId: channel.id,
      guildId: guild.id,
      adapterCreator: guild.voiceAdapterCreator,
      selfDeaf: false
    });

    await entersState(connection, VoiceConnectionStatus.Ready, 30_000);
    
    // Set up transcriber
    const transcriber = new VoiceTranscriber(connection);
    this.transcribers.set(guildId, transcriber);
    
    // Forward transcriptions to MCP
    transcriber.on('transcription', (data) => {
      this.emit('transcription', data.userId, data.username, data.text);
    });
    
    this.connections.set(guildId, connection);
    return connection;
  }

  async leaveChannel(guildId) {
    const connection = this.connections.get(guildId);
    if (connection) {
      connection.destroy();
      this.connections.delete(guildId);
      this.transcribers.delete(guildId);
    }
  }

  onTranscription(callback) {
    this.on('transcription', callback);
  }
}
```

### Session Manager (session-manager.js)

```javascript
import { v4 as uuidv4 } from 'uuid';

export class SessionManager {
  constructor() {
    this.sessions = new Map();
  }

  createSession(name) {
    const session = {
      id: uuidv4(),
      name: name,
      startTime: new Date().toISOString(),
      transcripts: [],
      active: true
    };
    
    this.sessions.set(session.id, session);
    return session;
  }

  addTranscript(sessionId, transcript) {
    const session = this.sessions.get(sessionId);
    if (session && session.active) {
      session.transcripts.push(transcript);
    }
  }

  getTranscript(sessionId, lastNMinutes) {
    const session = this.sessions.get(sessionId);
    if (!session) return '';
    
    let transcripts = session.transcripts;
    
    if (lastNMinutes) {
      const cutoffTime = new Date(Date.now() - lastNMinutes * 60000);
      transcripts = transcripts.filter(t => 
        new Date(t.timestamp) > cutoffTime
      );
    }
    
    return transcripts
      .map(t => `[${t.timestamp}] ${t.username}: ${t.text}`)
      .join('\n');
  }

  clearTranscript(sessionId) {
    const session = this.sessions.get(sessionId);
    if (session) {
      session.transcripts = [];
    }
  }

  endSession(sessionId) {
    const session = this.sessions.get(sessionId);
    if (session) {
      session.active = false;
      session.endTime = new Date().toISOString();
    }
  }

  getAllSessions() {
    return Array.from(this.sessions.values());
  }
}
```

## 🔧 Claude Desktop Configuration

### claude_desktop_config.json

```json
{
  "mcpServers": {
    "discord-voice": {
      "command": "node",
      "args": [
        "/absolute/path/to/discord-voice-mcp/src/mcp-server.js"
      ],
      "env": {
        "DISCORD_TOKEN": "your-discord-bot-token",
        "OPENAI_API_KEY": "your-openai-api-key"
      }
    }
  }
}
```

## 🚀 Setup Instructions

### 1. Install Dependencies

```bash
# Create project directory
mkdir discord-voice-mcp && cd discord-voice-mcp

# Initialize package.json
npm init -y

# Install dependencies
npm install \
  @modelcontextprotocol/sdk \
  discord.js \
  @discordjs/voice \
  @discordjs/opus \
  prism-media \
  openai \
  dotenv \
  zod \
  uuid

# Install dev dependencies
npm install --save-dev \
  @types/node \
  nodemon
```

### 2. Package.json Configuration

```json
{
  "name": "discord-voice-mcp",
  "version": "1.0.0",
  "type": "module",
  "main": "src/mcp-server.js",
  "bin": {
    "discord-voice-mcp": "./src/mcp-server.js"
  },
  "scripts": {
    "start": "node src/mcp-server.js",
    "dev": "nodemon src/mcp-server.js",
    "test-bot": "node src/discord-bot.js"
  },
  "dependencies": {
    "@modelcontextprotocol/sdk": "^1.0.0",
    "discord.js": "^14.14.1",
    "@discordjs/voice": "^0.16.1",
    "@discordjs/opus": "^0.9.0",
    "prism-media": "^1.3.5",
    "openai": "^4.28.0",
    "dotenv": "^16.4.1",
    "zod": "^3.22.4",
    "uuid": "^9.0.1"
  }
}
```

### 3. Environment Configuration (.env)

```env
# Discord Configuration
DISCORD_TOKEN=your_discord_bot_token
DISCORD_CLIENT_ID=your_client_id
DISCORD_GUILD_ID=your_guild_id

# Transcription Service
OPENAI_API_KEY=your_openai_api_key
TRANSCRIPTION_SERVICE=whisper

# MCP Configuration
MCP_SERVER_NAME=discord-voice
MCP_SERVER_PORT=3000
```

### 4. Discord Bot Setup

1. Create Discord Application at https://discord.com/developers/applications
2. Create Bot and copy token
3. Set Intents: Guild, Voice States, Message Content
4. Generate invite URL with permissions:
   - Connect
   - Speak
   - Read Messages
   - Send Messages

### 5. Claude Desktop Integration

1. Locate Claude Desktop config:
   - macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
   - Windows: `%APPDATA%\Claude\claude_desktop_config.json`
   - Linux: `~/.config/Claude/claude_desktop_config.json`

2. Add MCP server configuration
3. Restart Claude Desktop

## 📝 Usage Examples in Claude Code

### Starting a Voice Session

```
You: Start a voice transcription session for our brainstorming meeting

Claude: I'll start a voice transcription session for you.

[Uses tool: start_voice_session with sessionName: "brainstorming_meeting"]

Voice session started successfully! I've begun transcribing audio from the Discord voice channel. 
Speak your ideas and I'll capture them as context for our development work.
```

### Getting Transcripts

```
You: Show me what we've discussed in the last 5 minutes

Claude: [Uses tool: get_transcript with lastNMinutes: 5]

Here's the transcript from the last 5 minutes:

[2025-01-08 10:23:45] You: So I'm thinking we need a real-time data pipeline
[2025-01-08 10:23:52] You: It should handle WebSocket connections
[2025-01-08 10:24:03] You: And process events using a queue system
[2025-01-08 10:24:15] You: Maybe Redis for the queue?
```

### Using Transcripts as Context

```
You: Based on our voice discussion, create a technical specification

Claude: Based on your voice discussion about the real-time data pipeline, here's a technical specification:

## Real-Time Data Pipeline Specification

### Architecture Overview
- WebSocket server for client connections
- Redis queue for event processing
- Worker processes for parallel processing
[...]
```

## 🎯 Advanced Features

### 1. Real-Time Streaming

```javascript
// Add WebSocket support for real-time transcript streaming
import { WebSocketServer } from 'ws';

class RealtimeTranscriptServer {
  constructor(port = 8080) {
    this.wss = new WebSocketServer({ port });
    this.clients = new Set();
    
    this.wss.on('connection', (ws) => {
      this.clients.add(ws);
      ws.on('close', () => this.clients.delete(ws));
    });
  }
  
  broadcast(transcript) {
    const message = JSON.stringify({
      type: 'transcript',
      data: transcript
    });
    
    this.clients.forEach(client => {
      if (client.readyState === WebSocket.OPEN) {
        client.send(message);
      }
    });
  }
}
```

### 2. Multi-User Support

```javascript
// Track individual speakers
class SpeakerTracker {
  constructor() {
    this.speakers = new Map();
  }
  
  addSpeaker(userId, userInfo) {
    this.speakers.set(userId, {
      ...userInfo,
      startTime: Date.now(),
      wordCount: 0,
      lastSpoke: Date.now()
    });
  }
  
  updateSpeaker(userId, transcript) {
    const speaker = this.speakers.get(userId);
    if (speaker) {
      speaker.wordCount += transcript.split(' ').length;
      speaker.lastSpoke = Date.now();
    }
  }
  
  getSpeakerStats() {
    return Array.from(this.speakers.values());
  }
}
```

### 3. Contextual Commands

```javascript
// Voice commands within Discord
const VOICE_COMMANDS = {
  'clear context': () => sessionManager.clearTranscript(),
  'save checkpoint': () => sessionManager.saveCheckpoint(),
  'summarize': () => sessionManager.generateSummary(),
  'new topic': (topic) => sessionManager.startNewTopic(topic)
};

// Process transcripts for commands
function processVoiceCommand(transcript) {
  for (const [command, handler] of Object.entries(VOICE_COMMANDS)) {
    if (transcript.toLowerCase().includes(command)) {
      handler();
      return true;
    }
  }
  return false;
}
```

## 🔐 Security Considerations

1. **Token Management**: Never expose Discord or API tokens
2. **User Privacy**: Inform users about recording
3. **Data Storage**: Implement data retention policies
4. **Access Control**: Limit who can start/stop sessions
5. **Rate Limiting**: Prevent API abuse

## 🐛 Troubleshooting

### Common Issues

1. **MCP Server Not Connecting**
   - Check Claude Desktop config path
   - Verify absolute paths in configuration
   - Check stderr output in console

2. **No Audio Received**
   - Ensure bot has proper permissions
   - Check selfDeaf is set to false
   - Verify opus codec installation

3. **Transcription Errors**
   - Check API key validity
   - Monitor rate limits
   - Verify audio format compatibility

## 📚 Resources

- [Model Context Protocol Docs](https://modelcontextprotocol.io)
- [Discord.js Voice Guide](https://discordjs.guide/voice/)
- [OpenAI Whisper API](https://platform.openai.com/docs/guides/speech-to-text)
- [MCP TypeScript SDK](https://github.com/modelcontextprotocol/typescript-sdk)

## 🎉 Benefits

1. **Voice-First Development**: Brainstorm ideas naturally through conversation
2. **Context Preservation**: All discussions become searchable context
3. **Team Collaboration**: Multiple developers can contribute via voice
4. **Idea Refinement**: Claude helps structure and improve voiced concepts
5. **Documentation**: Automatic meeting notes and technical discussions

---

*This design enables seamless voice-based development workflows where you can speak your ideas in Discord and have Claude Code help refine and implement them in real-time.*