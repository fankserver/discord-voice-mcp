import { v4 as uuidv4 } from 'uuid';
import fs from 'fs';
import path from 'path';
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

export class SessionManager {
  constructor() {
    this.sessions = new Map();
    this.sessionDir = './sessions';
    
    // Create sessions directory if it doesn't exist
    if (!fs.existsSync(this.sessionDir)) {
      fs.mkdirSync(this.sessionDir, { recursive: true });
    }
    
    // Load existing sessions from disk
    this.loadSessions();
  }

  createSession(name) {
    const session = {
      id: uuidv4(),
      name: name || `Session ${new Date().toISOString()}`,
      startTime: new Date().toISOString(),
      endTime: null,
      transcripts: [],
      active: true,
      metadata: {
        provider: null,
        wordCount: 0,
        speakerCount: 0
      }
    };
    
    this.sessions.set(session.id, session);
    this.saveSession(session);
    
    logger.info(`Created session: ${session.id} - ${session.name}`);
    
    return session;
  }

  addTranscript(sessionId, transcript) {
    const session = this.sessions.get(sessionId);
    if (!session) {
      logger.warn(`Session not found: ${sessionId}`);
      return;
    }
    
    if (!session.active) {
      logger.warn(`Cannot add transcript to inactive session: ${sessionId}`);
      return;
    }
    
    // Add transcript entry
    session.transcripts.push({
      ...transcript,
      timestamp: transcript.timestamp || new Date().toISOString()
    });
    
    // Update metadata
    if (transcript.text) {
      const words = transcript.text.split(' ').filter(w => w.length > 0);
      session.metadata.wordCount += words.length;
    }
    
    // Track unique speakers
    const speakers = new Set(session.transcripts.map(t => t.userId));
    session.metadata.speakerCount = speakers.size;
    
    // Save to disk periodically (every 10 transcripts)
    if (session.transcripts.length % 10 === 0) {
      this.saveSession(session);
    }
    
    logger.debug(`Added transcript to session ${sessionId}: ${transcript.text?.substring(0, 50)}...`);
  }

  getTranscript(sessionId, lastNMinutes) {
    const session = this.sessions.get(sessionId);
    if (!session) {
      logger.warn(`Session not found: ${sessionId}`);
      return '';
    }
    
    let transcripts = session.transcripts;
    
    // Filter by time if requested
    if (lastNMinutes) {
      const cutoffTime = new Date(Date.now() - lastNMinutes * 60000);
      transcripts = transcripts.filter(t => 
        new Date(t.timestamp) > cutoffTime
      );
    }
    
    // Format transcripts
    return transcripts
      .map(t => {
        const time = new Date(t.timestamp).toLocaleTimeString();
        return `[${time}] ${t.username}: ${t.text}`;
      })
      .join('\n');
  }

  getSession(sessionId) {
    return this.sessions.get(sessionId);
  }

  clearTranscript(sessionId) {
    const session = this.sessions.get(sessionId);
    if (session) {
      session.transcripts = [];
      session.metadata.wordCount = 0;
      this.saveSession(session);
      logger.info(`Cleared transcript for session: ${sessionId}`);
    }
  }

  endSession(sessionId) {
    const session = this.sessions.get(sessionId);
    if (session) {
      session.active = false;
      session.endTime = new Date().toISOString();
      this.saveSession(session);
      logger.info(`Ended session: ${sessionId}`);
    }
  }

  getAllSessions() {
    return Array.from(this.sessions.values())
      .sort((a, b) => new Date(b.startTime) - new Date(a.startTime));
  }

  getActiveSessions() {
    return Array.from(this.sessions.values())
      .filter(s => s.active);
  }

  saveSession(session) {
    const filePath = path.join(this.sessionDir, `${session.id}.json`);
    try {
      fs.writeFileSync(filePath, JSON.stringify(session, null, 2));
      logger.debug(`Saved session to disk: ${session.id}`);
    } catch (error) {
      logger.error(`Failed to save session ${session.id}: ${error.message}`);
    }
  }

  loadSessions() {
    try {
      const files = fs.readdirSync(this.sessionDir);
      
      for (const file of files) {
        if (!file.endsWith('.json')) continue;
        
        const filePath = path.join(this.sessionDir, file);
        const data = fs.readFileSync(filePath, 'utf8');
        const session = JSON.parse(data);
        
        // Mark old sessions as inactive
        if (session.active && !session.endTime) {
          const startTime = new Date(session.startTime);
          const hoursSinceStart = (Date.now() - startTime) / (1000 * 60 * 60);
          
          // Auto-end sessions older than 24 hours
          if (hoursSinceStart > 24) {
            session.active = false;
            session.endTime = new Date(startTime.getTime() + 24 * 60 * 60 * 1000).toISOString();
          }
        }
        
        this.sessions.set(session.id, session);
      }
      
      logger.info(`Loaded ${this.sessions.size} sessions from disk`);
    } catch (error) {
      logger.error(`Failed to load sessions: ${error.message}`);
    }
  }

  exportSession(sessionId, format = 'txt') {
    const session = this.sessions.get(sessionId);
    if (!session) {
      throw new Error(`Session not found: ${sessionId}`);
    }
    
    const exportDir = './exports';
    if (!fs.existsSync(exportDir)) {
      fs.mkdirSync(exportDir, { recursive: true });
    }
    
    const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
    const filename = `${session.name.replace(/[^a-z0-9]/gi, '_')}_${timestamp}`;
    
    let content;
    let extension;
    
    switch (format) {
      case 'json':
        content = JSON.stringify(session, null, 2);
        extension = 'json';
        break;
      
      case 'markdown':
        content = this.formatAsMarkdown(session);
        extension = 'md';
        break;
      
      case 'txt':
      default:
        content = this.getTranscript(sessionId);
        extension = 'txt';
        break;
    }
    
    const filePath = path.join(exportDir, `${filename}.${extension}`);
    fs.writeFileSync(filePath, content);
    
    logger.info(`Exported session ${sessionId} to ${filePath}`);
    
    return filePath;
  }

  formatAsMarkdown(session) {
    const duration = session.endTime 
      ? new Date(session.endTime) - new Date(session.startTime)
      : Date.now() - new Date(session.startTime);
    const minutes = Math.floor(duration / 60000);
    
    let markdown = `# ${session.name}\n\n`;
    markdown += `**Session ID:** ${session.id}\n`;
    markdown += `**Started:** ${session.startTime}\n`;
    markdown += `**Duration:** ${minutes} minutes\n`;
    markdown += `**Word Count:** ${session.metadata.wordCount}\n`;
    markdown += `**Speakers:** ${session.metadata.speakerCount}\n\n`;
    markdown += `## Transcript\n\n`;
    
    for (const t of session.transcripts) {
      const time = new Date(t.timestamp).toLocaleTimeString();
      markdown += `**[${time}] ${t.username}:**\n${t.text}\n\n`;
    }
    
    return markdown;
  }

  cleanup() {
    // Save all active sessions before cleanup
    for (const session of this.sessions.values()) {
      if (session.active) {
        this.saveSession(session);
      }
    }
  }
}