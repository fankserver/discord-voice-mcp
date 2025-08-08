import fs from 'fs';
import { spawn } from 'child_process';
import vosk from 'vosk';
import speech from '@google-cloud/speech';
import winston from 'winston';
import path from 'path';

const logger = winston.createLogger({
  level: 'info',
  format: winston.format.json(),
  transports: [
    new winston.transports.Console({
      stderrLevels: ['error', 'warn', 'info', 'debug']
    })
  ]
});

// Base provider interface
class TranscriptionProvider {
  async initialize() {}
  async transcribe(audioBuffer) {}
  async transcribeStream(audioStream) {}
  async cleanup() {}
  async getStatus() { return { ready: false }; }
}

// Vosk Provider - Free, Offline, Lightweight
class VoskProvider extends TranscriptionProvider {
  constructor(modelPath) {
    super();
    this.modelPath = modelPath || './models/vosk-model-en-us-0.22';
    this.model = null;
    this.recognizer = null;
  }

  async initialize() {
    try {
      if (!fs.existsSync(this.modelPath)) {
        throw new Error(`Vosk model not found at ${this.modelPath}. Run 'npm run download-models' to download.`);
      }
      
      vosk.setLogLevel(0);
      this.model = new vosk.Model(this.modelPath);
      this.recognizer = new vosk.Recognizer({
        model: this.model,
        sampleRate: 48000
      });
      
      logger.info('Vosk provider initialized successfully');
    } catch (error) {
      logger.error(`Failed to initialize Vosk: ${error.message}`);
      throw error;
    }
  }

  async transcribe(audioBuffer) {
    if (!this.recognizer) {
      await this.initialize();
    }

    // Process PCM audio buffer in chunks
    const chunkSize = 4000;
    for (let i = 0; i < audioBuffer.length; i += chunkSize) {
      const chunk = audioBuffer.slice(i, Math.min(i + chunkSize, audioBuffer.length));
      this.recognizer.acceptWaveform(chunk);
    }

    const result = this.recognizer.finalResult();
    const parsed = JSON.parse(result);
    
    return parsed.text || '';
  }

  async transcribeStream(audioStream) {
    if (!this.recognizer) {
      await this.initialize();
    }

    return new Promise((resolve, reject) => {
      const transcripts = [];
      let timeout;

      const resetTimeout = () => {
        if (timeout) clearTimeout(timeout);
        timeout = setTimeout(() => {
          audioStream.destroy();
          const finalResult = JSON.parse(this.recognizer.finalResult());
          if (finalResult.text) {
            transcripts.push(finalResult.text);
          }
          resolve(transcripts.join(' '));
        }, 2000); // 2 second silence timeout
      };

      audioStream.on('data', (chunk) => {
        resetTimeout();
        
        if (this.recognizer.acceptWaveform(chunk)) {
          const result = JSON.parse(this.recognizer.result());
          if (result.text) {
            transcripts.push(result.text);
            logger.debug(`Vosk transcript: ${result.text}`);
          }
        } else {
          // Partial result available
          const partial = JSON.parse(this.recognizer.partialResult());
          if (partial.partial) {
            logger.debug(`Vosk partial: ${partial.partial}`);
          }
        }
      });

      audioStream.on('end', () => {
        if (timeout) clearTimeout(timeout);
        const finalResult = JSON.parse(this.recognizer.finalResult());
        if (finalResult.text) {
          transcripts.push(finalResult.text);
        }
        resolve(transcripts.join(' '));
      });

      audioStream.on('error', reject);
    });
  }

  async cleanup() {
    if (this.recognizer) {
      this.recognizer.free();
      this.recognizer = null;
    }
    if (this.model) {
      this.model.free();
      this.model = null;
    }
  }

  async getStatus() {
    return {
      ready: this.recognizer !== null,
      modelPath: this.modelPath,
      modelExists: fs.existsSync(this.modelPath)
    };
  }
}

// Whisper.cpp Provider - Free, Offline, High Accuracy
class WhisperCppProvider extends TranscriptionProvider {
  constructor(modelPath, executablePath) {
    super();
    this.modelPath = modelPath || './models/ggml-base.en.bin';
    this.executablePath = executablePath || './whisper.cpp/main';
    this.tempDir = './temp';
  }

  async initialize() {
    // Check if whisper.cpp is installed
    if (!fs.existsSync(this.executablePath)) {
      throw new Error(`Whisper.cpp not found at ${this.executablePath}. Run setup script to install.`);
    }
    
    if (!fs.existsSync(this.modelPath)) {
      throw new Error(`Whisper model not found at ${this.modelPath}. Run 'npm run download-models' to download.`);
    }
    
    // Create temp directory if it doesn't exist
    if (!fs.existsSync(this.tempDir)) {
      fs.mkdirSync(this.tempDir, { recursive: true });
    }
    
    logger.info('Whisper.cpp provider initialized successfully');
  }

  async transcribe(audioBuffer) {
    // Save audio to temp WAV file
    const tempFile = path.join(this.tempDir, `audio_${Date.now()}.wav`);
    
    // Add WAV header to PCM data
    const wavBuffer = this.addWavHeader(audioBuffer);
    fs.writeFileSync(tempFile, wavBuffer);

    try {
      return await this.runWhisper(tempFile);
    } finally {
      // Clean up temp file
      if (fs.existsSync(tempFile)) {
        fs.unlinkSync(tempFile);
      }
    }
  }

  async transcribeStream(audioStream) {
    const chunks = [];
    
    return new Promise((resolve, reject) => {
      audioStream.on('data', (chunk) => {
        chunks.push(chunk);
      });

      audioStream.on('end', async () => {
        const audioBuffer = Buffer.concat(chunks);
        try {
          const text = await this.transcribe(audioBuffer);
          resolve(text);
        } catch (error) {
          reject(error);
        }
      });

      audioStream.on('error', reject);
    });
  }

  async runWhisper(audioFile) {
    return new Promise((resolve, reject) => {
      const args = [
        '-m', this.modelPath,
        '-f', audioFile,
        '--no-timestamps',
        '--language', 'en',
        '--threads', '4'
      ];

      const whisper = spawn(this.executablePath, args);
      
      let output = '';
      let errorOutput = '';

      whisper.stdout.on('data', (data) => {
        output += data.toString();
      });

      whisper.stderr.on('data', (data) => {
        errorOutput += data.toString();
      });

      whisper.on('close', (code) => {
        if (code === 0) {
          // Clean the output - remove metadata and extract only transcript
          const lines = output.split('\n');
          const transcript = lines
            .filter(line => !line.startsWith('[') && line.trim().length > 0)
            .join(' ')
            .trim();
          
          logger.debug(`Whisper transcript: ${transcript}`);
          resolve(transcript);
        } else {
          logger.error(`Whisper error: ${errorOutput}`);
          reject(new Error(`Whisper process exited with code ${code}`));
        }
      });
    });
  }

  addWavHeader(pcmBuffer) {
    const header = Buffer.alloc(44);
    
    // RIFF header
    header.write('RIFF', 0);
    header.writeUInt32LE(pcmBuffer.length + 36, 4);
    header.write('WAVE', 8);
    
    // fmt subchunk
    header.write('fmt ', 12);
    header.writeUInt32LE(16, 16); // Subchunk size
    header.writeUInt16LE(1, 20); // Audio format (1 = PCM)
    header.writeUInt16LE(2, 22); // Number of channels
    header.writeUInt32LE(48000, 24); // Sample rate
    header.writeUInt32LE(48000 * 2 * 2, 28); // Byte rate
    header.writeUInt16LE(4, 32); // Block align
    header.writeUInt16LE(16, 34); // Bits per sample
    
    // data subchunk
    header.write('data', 36);
    header.writeUInt32LE(pcmBuffer.length, 40);
    
    return Buffer.concat([header, pcmBuffer]);
  }

  async cleanup() {
    // Clean up any remaining temp files
    if (fs.existsSync(this.tempDir)) {
      const files = fs.readdirSync(this.tempDir);
      files.forEach(file => {
        if (file.startsWith('audio_')) {
          fs.unlinkSync(path.join(this.tempDir, file));
        }
      });
    }
  }

  async getStatus() {
    return {
      ready: fs.existsSync(this.executablePath) && fs.existsSync(this.modelPath),
      executablePath: this.executablePath,
      modelPath: this.modelPath,
      executableExists: fs.existsSync(this.executablePath),
      modelExists: fs.existsSync(this.modelPath)
    };
  }
}

// Google Cloud Speech Provider - Cloud-based, High Accuracy
class GoogleCloudProvider extends TranscriptionProvider {
  constructor(credentialsPath) {
    super();
    this.credentialsPath = credentialsPath;
    this.client = null;
  }

  async initialize() {
    if (!this.credentialsPath || !fs.existsSync(this.credentialsPath)) {
      throw new Error(`Google Cloud credentials not found at ${this.credentialsPath}`);
    }

    // Set credentials environment variable
    process.env.GOOGLE_APPLICATION_CREDENTIALS = this.credentialsPath;
    
    this.client = new speech.SpeechClient();
    logger.info('Google Cloud Speech provider initialized successfully');
  }

  async transcribe(audioBuffer) {
    if (!this.client) {
      await this.initialize();
    }

    const request = {
      audio: {
        content: audioBuffer.toString('base64')
      },
      config: {
        encoding: 'LINEAR16',
        sampleRateHertz: 48000,
        languageCode: 'en-US',
        enableAutomaticPunctuation: true,
        model: 'latest_long'
      }
    };

    try {
      const [response] = await this.client.recognize(request);
      const transcription = response.results
        .map(result => result.alternatives[0].transcript)
        .join(' ');
      
      logger.debug(`Google Cloud transcript: ${transcription}`);
      return transcription;
    } catch (error) {
      logger.error(`Google Cloud Speech error: ${error.message}`);
      throw error;
    }
  }

  async transcribeStream(audioStream) {
    if (!this.client) {
      await this.initialize();
    }

    const request = {
      config: {
        encoding: 'LINEAR16',
        sampleRateHertz: 48000,
        languageCode: 'en-US',
        enableAutomaticPunctuation: true,
        model: 'latest_long'
      },
      interimResults: true
    };

    const recognizeStream = this.client
      .streamingRecognize(request)
      .on('error', (error) => {
        logger.error(`Google Cloud streaming error: ${error.message}`);
      })
      .on('data', (data) => {
        if (data.results[0] && data.results[0].alternatives[0]) {
          const transcript = data.results[0].alternatives[0].transcript;
          const isFinal = data.results[0].isFinal;
          
          if (isFinal) {
            logger.debug(`Google Cloud final transcript: ${transcript}`);
          } else {
            logger.debug(`Google Cloud interim: ${transcript}`);
          }
        }
      });

    // Pipe audio stream to Google Cloud
    audioStream.pipe(recognizeStream);

    return new Promise((resolve, reject) => {
      const transcripts = [];
      
      recognizeStream.on('data', (data) => {
        if (data.results[0] && data.results[0].isFinal) {
          transcripts.push(data.results[0].alternatives[0].transcript);
        }
      });

      recognizeStream.on('end', () => {
        resolve(transcripts.join(' '));
      });

      recognizeStream.on('error', reject);
    });
  }

  async cleanup() {
    this.client = null;
  }

  async getStatus() {
    return {
      ready: this.client !== null,
      credentialsPath: this.credentialsPath,
      credentialsExist: fs.existsSync(this.credentialsPath || '')
    };
  }
}

// Main Transcription Service
export class TranscriptionService {
  constructor(config) {
    this.config = config;
    this.provider = null;
    this.currentProviderName = config.provider || 'vosk';
  }

  async initialize() {
    await this.initializeProvider(this.currentProviderName);
  }

  async initializeProvider(providerName) {
    // Clean up existing provider
    if (this.provider) {
      await this.provider.cleanup();
    }

    switch (providerName) {
      case 'vosk':
        this.provider = new VoskProvider(this.config.voskModel);
        break;
      
      case 'whisper':
        this.provider = new WhisperCppProvider(
          this.config.whisperModel,
          this.config.whisperExecutable
        );
        break;
      
      case 'google':
        this.provider = new GoogleCloudProvider(this.config.googleCredentials);
        break;
      
      default:
        logger.warn(`Unknown provider ${providerName}, defaulting to Vosk`);
        this.provider = new VoskProvider(this.config.voskModel);
        providerName = 'vosk';
    }

    try {
      await this.provider.initialize();
      this.currentProviderName = providerName;
      logger.info(`Transcription provider set to: ${providerName}`);
    } catch (error) {
      logger.error(`Failed to initialize ${providerName}: ${error.message}`);
      
      // Fallback to Vosk if other providers fail
      if (providerName !== 'vosk') {
        logger.info('Falling back to Vosk provider');
        this.provider = new VoskProvider(this.config.voskModel);
        await this.provider.initialize();
        this.currentProviderName = 'vosk';
      } else {
        throw error;
      }
    }
  }

  async switchProvider(providerName) {
    await this.initializeProvider(providerName);
  }

  async transcribe(audioBuffer) {
    if (!this.provider) {
      throw new Error('Transcription provider not initialized');
    }
    return await this.provider.transcribe(audioBuffer);
  }

  async transcribeStream(audioStream) {
    if (!this.provider) {
      throw new Error('Transcription provider not initialized');
    }
    return await this.provider.transcribeStream(audioStream);
  }

  async cleanup() {
    if (this.provider) {
      await this.provider.cleanup();
    }
  }

  async getStatus() {
    const providerStatus = this.provider ? await this.provider.getStatus() : { ready: false };
    
    return {
      current: this.currentProviderName,
      ready: providerStatus.ready,
      available: ['vosk', 'whisper', 'google'],
      capabilities: this.getProviderCapabilities(this.currentProviderName),
      ...providerStatus
    };
  }

  getProviderCapabilities(provider) {
    const capabilities = {
      'vosk': [
        'Offline operation',
        'Real-time streaming',
        'Low resource usage',
        'Multiple language support'
      ],
      'whisper': [
        'Offline operation',
        'High accuracy',
        'Multilingual (100+ languages)',
        'Punctuation and formatting'
      ],
      'google': [
        'Cloud-based processing',
        'Real-time streaming',
        'Speaker diarization',
        'Word-level timestamps'
      ]
    };
    
    return capabilities[provider] || [];
  }
}