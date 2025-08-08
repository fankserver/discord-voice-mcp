import { Client, GatewayIntentBits } from 'discord.js';
import { 
  joinVoiceChannel, 
  EndBehaviorType,
  VoiceConnectionStatus,
  entersState
} from '@discordjs/voice';
import prism from 'prism-media';
import EventEmitter from 'events';
import winston from 'winston';

const logger = winston.createLogger({
  level: 'info',
  format: winston.format.json(),
  transports: [
    new winston.transports.Console({
      stderrLevels: ['error', 'warn', 'info', 'debug']
    })
  ]
});

export class DiscordVoiceBot extends EventEmitter {
  constructor(transcriptionService) {
    super();
    this.transcriptionService = transcriptionService;
    this.client = null;
    this.connections = new Map(); // guildId -> connection
    this.activeRecordings = new Map(); // userId -> recording state
    this.guildSessions = new Map(); // guildId -> sessionId
    // Remove single connection tracking in favor of per-guild tracking
  }

  async initialize() {
    this.client = new Client({
      intents: [
        GatewayIntentBits.Guilds,
        GatewayIntentBits.GuildVoiceStates,
        GatewayIntentBits.GuildMessages,
        GatewayIntentBits.MessageContent
      ]
    });
    
    await this.client.login(process.env.DISCORD_TOKEN);
    
    return new Promise((resolve) => {
      this.client.once('ready', () => {
        logger.info(`Discord bot logged in as ${this.client.user.tag}`);
        resolve();
      });
    });
  }

  async joinChannel(channelId, guildId) {
    const guild = this.client.guilds.cache.get(guildId);
    if (!guild) {
      throw new Error('Guild not found');
    }
    
    const channel = guild.channels.cache.get(channelId);
    if (!channel) {
      throw new Error('Channel not found');
    }
    
    if (!channel.isVoiceBased()) {
      throw new Error('Channel is not a voice channel');
    }
    
    // Leave existing connection in this guild if any
    if (this.connections.has(guildId)) {
      const oldConnection = this.connections.get(guildId);
      oldConnection.destroy();
      this.connections.delete(guildId);
    }
    
    const connection = joinVoiceChannel({
      channelId: channel.id,
      guildId: guild.id,
      adapterCreator: guild.voiceAdapterCreator,
      selfDeaf: false // Important: must be false to receive audio
    });

    await entersState(connection, VoiceConnectionStatus.Ready, 30_000);
    
    this.connections.set(guildId, connection);
    
    // Set up voice receiver
    this.setupVoiceReceiver(connection, guild);
    
    logger.info(`Joined voice channel: ${channel.name} in ${guild.name}`);
    
    return connection;
  }

  setupVoiceReceiver(connection, guild) {
    const receiver = connection.receiver;
    
    // Listen for speaking events
    receiver.speaking.on('start', (userId) => {
      // Don't record the bot itself
      if (userId === this.client.user.id) return;
      
      // Check if we're already recording this user
      if (this.activeRecordings.has(userId)) return;
      
      const member = guild.members.cache.get(userId);
      if (!member) return;
      
      logger.debug(`User started speaking: ${member.displayName}`);
      
      // Start recording this user with guild context
      this.startRecording(receiver, userId, member.displayName, guild.id);
    });
  }

  startRecording(receiver, userId, username, guildId) {
    // Subscribe to user's audio stream
    const opusStream = receiver.subscribe(userId, {
      end: {
        behavior: EndBehaviorType.AfterSilence,
        duration: 1000 // End after 1 second of silence
      }
    });
    
    // Mark as actively recording
    this.activeRecordings.set(userId, true);
    
    // Create Opus decoder
    const decoder = new prism.opus.Decoder({
      frameSize: 960,
      channels: 2,
      rate: 48000
    });
    
    // Collect audio chunks
    const chunks = [];
    
    decoder.on('data', (chunk) => {
      chunks.push(chunk);
    });
    
    decoder.on('end', async () => {
      // Remove from active recordings
      this.activeRecordings.delete(userId);
      
      if (chunks.length === 0) {
        logger.debug('No audio data collected');
        return;
      }
      
      // Combine chunks into single buffer
      const audioBuffer = Buffer.concat(chunks);
      
      logger.debug(`Collected ${audioBuffer.length} bytes of audio from ${username}`);
      
      // Transcribe the audio
      try {
        const transcript = await this.transcriptionService.transcribe(audioBuffer);
        
        if (transcript && transcript.trim().length > 0) {
          logger.info(`Transcription from ${username}: ${transcript}`);
          
          // Emit transcription event with guild context
          this.emit('transcription', {
            userId,
            username,
            text: transcript,
            sessionId: this.guildSessions.get(guildId),
            guildId
          });
        }
      } catch (error) {
        logger.error(`Transcription error: ${error.message}`);
      }
    });
    
    // Error handling
    decoder.on('error', (error) => {
      logger.error(`Decoder error: ${error.message}`);
      this.activeRecordings.delete(userId);
    });
    
    opusStream.on('error', (error) => {
      logger.error(`Opus stream error: ${error.message}`);
      this.activeRecordings.delete(userId);
    });
    
    // Pipe opus stream through decoder
    opusStream.pipe(decoder);
  }

  async leaveChannel(guildId) {
    const connection = this.connections.get(guildId);
    if (connection) {
      connection.destroy();
      this.connections.delete(guildId);
      this.guildSessions.delete(guildId);
      
      logger.info(`Left voice channel in guild ${guildId}`);
    }
  }

  setSessionForGuild(guildId, sessionId) {
    if (sessionId) {
      this.guildSessions.set(guildId, sessionId);
      logger.info(`Session ${sessionId} set for guild ${guildId}`);
    } else {
      this.guildSessions.delete(guildId);
      logger.info(`Session cleared for guild ${guildId}`);
    }
  }

  getSessionForGuild(guildId) {
    return this.guildSessions.get(guildId);
  }

  getCurrentConnection(guildId) {
    return this.connections.get(guildId);
  }

  async cleanup() {
    // Leave all voice channels
    for (const [guildId, connection] of this.connections) {
      connection.destroy();
    }
    this.connections.clear();
    
    // Logout from Discord
    if (this.client) {
      await this.client.destroy();
    }
  }
}