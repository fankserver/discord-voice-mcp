#!/usr/bin/env node

import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { 
  CallToolRequestSchema, 
  ListResourcesRequestSchema,
  ReadResourceRequestSchema,
  ListToolsRequestSchema 
} from "@modelcontextprotocol/sdk/types.js";
import { DiscordVoiceBot } from './discord-bot.js';
import { SessionManager } from './session-manager.js';
import { TranscriptionService } from './services/transcription.js';
import dotenv from 'dotenv';
import winston from 'winston';

dotenv.config();

// Configure logger to write to stderr
const logger = winston.createLogger({
  level: 'info',
  format: winston.format.json(),
  transports: [
    new winston.transports.Console({
      stderrLevels: ['error', 'warn', 'info', 'debug']
    })
  ]
});

class DiscordVoiceMCP {
  constructor() {
    this.bot = null;
    this.sessionManager = new SessionManager();
    this.transcriptionService = null;
    this.currentProvider = process.env.TRANSCRIPTION_PROVIDER || 'vosk';
    
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

  async initialize() {
    // Initialize transcription service
    this.transcriptionService = new TranscriptionService({
      provider: this.currentProvider,
      voskModel: process.env.VOSK_MODEL_PATH || './models/vosk-model-en-us-0.22',
      whisperModel: process.env.WHISPER_MODEL_PATH || './models/ggml-base.en.bin',
      whisperExecutable: process.env.WHISPER_EXECUTABLE || './whisper.cpp/main',
      googleCredentials: process.env.GOOGLE_APPLICATION_CREDENTIALS
    });

    await this.transcriptionService.initialize();
    
    // Initialize Discord bot
    this.bot = new DiscordVoiceBot(this.transcriptionService);
    await this.bot.initialize();
    
    // Set up event handlers
    this.bot.on('transcription', (data) => {
      if (data.sessionId) {
        this.sessionManager.addTranscript(data.sessionId, {
          userId: data.userId,
          username: data.username,
          text: data.text,
          timestamp: new Date().toISOString()
        });
      }
    });
  }

  setupHandlers() {
    // Tool handler
    this.server.setRequestHandler(CallToolRequestSchema, async (request) => {
      const { name, arguments: args } = request.params;

      try {
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
          
          case "switch_provider":
            return await this.switchProvider(args);
          
          case "get_provider_status":
            return await this.getProviderStatus();

          default:
            throw new Error(`Unknown tool: ${name}`);
        }
      } catch (error) {
        logger.error(`Tool error: ${error.message}`);
        return {
          content: [
            {
              type: "text",
              text: `Error: ${error.message}`
            }
          ]
        };
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
            description: "Start a new voice transcription session in the current voice channel",
            inputSchema: {
              type: "object",
              properties: {
                sessionName: {
                  type: "string",
                  description: "Name for this transcription session"
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
                  description: "Session ID to stop (optional, stops current if not provided)"
                }
              }
            }
          },
          {
            name: "get_transcript",
            description: "Get the transcript from a session",
            inputSchema: {
              type: "object",
              properties: {
                sessionId: {
                  type: "string",
                  description: "Session ID (optional, uses current if not provided)"
                },
                lastNMinutes: {
                  type: "number",
                  description: "Get transcript from last N minutes only"
                },
                format: {
                  type: "string",
                  enum: ["text", "markdown", "json"],
                  description: "Output format (default: text)"
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
            name: "leave_voice_channel",
            description: "Leave the current voice channel",
            inputSchema: {
              type: "object",
              properties: {
                guildId: {
                  type: "string",
                  description: "Discord guild/server ID"
                }
              }
            }
          },
          {
            name: "list_active_sessions",
            description: "List all active and past transcription sessions",
            inputSchema: {
              type: "object",
              properties: {}
            }
          },
          {
            name: "switch_provider",
            description: "Switch transcription provider (vosk, whisper, google)",
            inputSchema: {
              type: "object",
              properties: {
                provider: {
                  type: "string",
                  enum: ["vosk", "whisper", "google"],
                  description: "Transcription provider to use"
                }
              },
              required: ["provider"]
            }
          },
          {
            name: "get_provider_status",
            description: "Get current transcription provider status and capabilities",
            inputSchema: {
              type: "object",
              properties: {}
            }
          }
        ]
      };
    });
  }

  async startVoiceSession(args) {
    const session = this.sessionManager.createSession(args.sessionName);
    
    // Link session to bot
    if (this.bot.currentConnection) {
      this.bot.setCurrentSession(session.id);
    }

    logger.info(`Started voice session: ${session.id}`);
    
    return {
      content: [
        {
          type: "text",
          text: `✅ Started voice session: "${args.sessionName}"\nSession ID: ${session.id}\nProvider: ${this.currentProvider}\n\nNow transcribing voice channel audio...`
        }
      ]
    };
  }

  async stopVoiceSession(args) {
    const sessionId = args.sessionId || this.bot.currentSessionId;
    if (!sessionId) {
      throw new Error("No active session to stop");
    }
    
    const transcript = this.sessionManager.getTranscript(sessionId);
    const wordCount = transcript.split(' ').filter(w => w.length > 0).length;
    this.sessionManager.endSession(sessionId);
    
    if (this.bot.currentSessionId === sessionId) {
      this.bot.setCurrentSession(null);
    }
    
    logger.info(`Stopped voice session: ${sessionId}`);
    
    return {
      content: [
        {
          type: "text",
          text: `⏹️ Session stopped.\nTranscript length: ${transcript.length} characters\nWord count: ${wordCount}\n\nUse 'get_transcript' to retrieve the full transcript.`
        }
      ]
    };
  }

  async getTranscript(args) {
    const sessionId = args.sessionId || this.bot.currentSessionId;
    if (!sessionId) {
      throw new Error("No session specified and no active session");
    }
    
    const transcript = this.sessionManager.getTranscript(
      sessionId, 
      args.lastNMinutes
    );
    
    if (!transcript) {
      return {
        content: [
          {
            type: "text",
            text: "No transcript available for this session yet."
          }
        ]
      };
    }
    
    // Format based on request
    let formattedTranscript = transcript;
    if (args.format === 'markdown') {
      formattedTranscript = `## Transcript\n\n${transcript}`;
    } else if (args.format === 'json') {
      const session = this.sessionManager.getSession(sessionId);
      formattedTranscript = JSON.stringify(session.transcripts, null, 2);
    }
    
    return {
      content: [
        {
          type: "text",
          text: formattedTranscript
        }
      ]
    };
  }

  async clearTranscript(args) {
    if (!args.sessionId) {
      throw new Error("Session ID required");
    }
    
    this.sessionManager.clearTranscript(args.sessionId);
    logger.info(`Cleared transcript for session: ${args.sessionId}`);
    
    return {
      content: [
        {
          type: "text",
          text: `🗑️ Transcript cleared for session ${args.sessionId}`
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
          text: `🔊 Joined voice channel ${args.channelId}\nReady to transcribe audio using ${this.currentProvider}`
        }
      ]
    };
  }

  async leaveVoiceChannel(args) {
    const guildId = args.guildId || this.bot.currentGuildId;
    if (!guildId) {
      throw new Error("No guild ID specified and not in a voice channel");
    }
    
    await this.bot.leaveChannel(guildId);
    
    return {
      content: [
        {
          type: "text",
          text: `👋 Left voice channel`
        }
      ]
    };
  }

  async listActiveSessions() {
    const sessions = this.sessionManager.getAllSessions();
    
    if (sessions.length === 0) {
      return {
        content: [
          {
            type: "text",
            text: "No transcription sessions found."
          }
        ]
      };
    }
    
    const sessionList = sessions.map(s => {
      const status = s.active ? "🔴 Active" : "⚫ Ended";
      const duration = s.endTime 
        ? new Date(s.endTime) - new Date(s.startTime)
        : Date.now() - new Date(s.startTime);
      const minutes = Math.floor(duration / 60000);
      
      return `${status} ${s.name} (${s.id.slice(0, 8)}...)\n  Started: ${s.startTime}\n  Duration: ${minutes} minutes\n  Transcripts: ${s.transcripts.length}`;
    }).join('\n\n');
    
    return {
      content: [
        {
          type: "text",
          text: `📝 Transcription Sessions:\n\n${sessionList}`
        }
      ]
    };
  }

  async switchProvider(args) {
    const oldProvider = this.currentProvider;
    this.currentProvider = args.provider;
    
    // Reinitialize transcription service with new provider
    await this.transcriptionService.switchProvider(args.provider);
    
    logger.info(`Switched transcription provider from ${oldProvider} to ${args.provider}`);
    
    return {
      content: [
        {
          type: "text",
          text: `🔄 Switched transcription provider:\n${oldProvider} → ${args.provider}\n\nNew transcriptions will use ${args.provider}`
        }
      ]
    };
  }

  async getProviderStatus() {
    const status = await this.transcriptionService.getStatus();
    
    return {
      content: [
        {
          type: "text",
          text: `📊 Transcription Provider Status:\n\nCurrent: ${this.currentProvider}\nStatus: ${status.ready ? '✅ Ready' : '❌ Not Ready'}\n\nAvailable Providers:\n${status.available.map(p => `  • ${p}`).join('\n')}\n\nCapabilities:\n${status.capabilities.map(c => `  • ${c}`).join('\n')}`
        }
      ]
    };
  }

  async run() {
    try {
      // Initialize services
      await this.initialize();
      
      // Start MCP server
      const transport = new StdioServerTransport();
      await this.server.connect(transport);
      
      logger.info("Discord Voice MCP Server running");
      logger.info(`Default provider: ${this.currentProvider}`);
    } catch (error) {
      logger.error(`Failed to start MCP server: ${error.message}`);
      process.exit(1);
    }
  }
}

// Start the server
const mcp = new DiscordVoiceMCP();
mcp.run().catch(error => {
  logger.error(`Fatal error: ${error.message}`);
  process.exit(1);
});