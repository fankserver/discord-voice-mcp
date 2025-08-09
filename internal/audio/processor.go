package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
	"github.com/sirupsen/logrus"
	"layeh.com/gopus"
)

const (
	// Audio configuration
	sampleRate = 48000
	channels   = 2
	frameSize  = 960 // 20ms @ 48kHz

	// Buffer configuration
	transcriptionBufferSize = sampleRate * channels * 2 * 2 // 2 seconds of audio (samples * channels * bytes per sample * seconds)
	
	// Silence detection
	silenceTimeout = 1500 * time.Millisecond // Trigger transcription after 1.5 seconds of silence
	minAudioBuffer = 4800                     // Minimum audio bytes before considering transcription (100ms of audio)
)

// Processor handles audio capture and transcription
type Processor struct {
	mu            sync.Mutex
	transcriber   transcriber.Transcriber
	activeStreams map[string]*Stream
}

// Stream represents an active audio stream from a user
type Stream struct {
	UserID       string
	Username     string
	Buffer       *bytes.Buffer
	mu           sync.Mutex
	silenceTimer *time.Timer     // Timer for detecting silence
	lastAudioTime time.Time      // Last time we received real audio (not silence)
	isTranscribing bool          // Prevent concurrent transcriptions
}

// NewProcessor creates a new audio processor
func NewProcessor(t transcriber.Transcriber) *Processor {
	return &Processor{
		transcriber:   t,
		activeStreams: make(map[string]*Stream),
	}
}

// ProcessVoiceReceive handles incoming voice packets
func (p *Processor) ProcessVoiceReceive(vc *discordgo.VoiceConnection, sessionManager *session.Manager, activeSessionID string) {
	// Create opus decoder
	decoder, err := gopus.NewDecoder(sampleRate, channels)
	if err != nil {
		logrus.WithError(err).Error("Error creating opus decoder")
		return
	}

	logrus.Info("Started processing voice receive")

	packetCount := 0
	// Process incoming audio
	for packet := range vc.OpusRecv {
		packetCount++
		if packetCount % 100 == 0 {
			logrus.WithField("packets_received", packetCount).Debug("Voice packets received")
		}
		
		// Log packet details
		isSilence := len(packet.Opus) <= 3
		logrus.WithFields(logrus.Fields{
			"ssrc":       packet.SSRC,
			"opus_len":   len(packet.Opus),
			"timestamp":  packet.Timestamp,
			"packet_num": packetCount,
			"is_silence": isSilence,
		}).Debug("Received voice packet")
		
		// Get or create stream for user (using SSRC as ID)
		stream := p.getOrCreateStream(packet.SSRC, fmt.Sprintf("%d", packet.SSRC), sessionManager, activeSessionID)

		// Handle silence packets
		if isSilence {
			// Silence packet detected - start/continue silence timer
			stream.mu.Lock()
			bufferSize := stream.Buffer.Len()
			stream.mu.Unlock()
			
			if bufferSize > minAudioBuffer {
				// We have audio in the buffer, start silence timer if not already running
				stream.startSilenceTimer(p, sessionManager, activeSessionID)
			}
			continue
		}
		
		// Decode opus to PCM (real audio)
		pcm, err := decoder.Decode(packet.Opus, frameSize, false)
		if err != nil {
			logrus.WithError(err).Debug("Error decoding opus")
			continue
		}
		
		logrus.WithFields(logrus.Fields{
			"pcm_samples": len(pcm),
			"ssrc":        packet.SSRC,
		}).Debug("Decoded opus to PCM")

		// Convert PCM to bytes
		pcmBytes := make([]byte, len(pcm)*2) // samples * 2 bytes per sample
		for i := 0; i < len(pcm); i++ {
			// Safe conversion from int16 to uint16 for binary encoding
			// #nosec G115 - This is safe as we're reinterpreting the bits, not converting the value
			binary.LittleEndian.PutUint16(pcmBytes[i*2:], uint16(pcm[i]))
		}

		// Add to buffer and update audio timing
		stream.mu.Lock()
		stream.Buffer.Write(pcmBytes)
		bufferSize := stream.Buffer.Len()
		stream.lastAudioTime = time.Now()
		
		// Cancel silence timer since we got real audio
		if stream.silenceTimer != nil {
			stream.silenceTimer.Stop()
			stream.silenceTimer = nil
		}
		stream.mu.Unlock()
		
		logrus.WithFields(logrus.Fields{
			"buffer_size":   bufferSize,
			"threshold":     transcriptionBufferSize,
			"percent_full":  float64(bufferSize) / float64(transcriptionBufferSize) * 100,
			"user":          stream.UserID,
		}).Debug("Audio buffer status")

		// If buffer is large enough, transcribe immediately
		if bufferSize >= transcriptionBufferSize {
			logrus.WithFields(logrus.Fields{
				"buffer_size": bufferSize,
				"user":        stream.UserID,
			}).Info("Buffer threshold reached, triggering transcription")
			go p.transcribeAndClear(stream, sessionManager, activeSessionID)
		}
	}

	logrus.Info("Voice receive channel closed")
}

func (p *Processor) getOrCreateStream(ssrc uint32, userID string, sessionManager *session.Manager, sessionID string) *Stream {
	p.mu.Lock()
	defer p.mu.Unlock()

	streamID := fmt.Sprintf("%d", ssrc)
	if stream, exists := p.activeStreams[streamID]; exists {
		logrus.WithFields(logrus.Fields{
			"ssrc":   ssrc,
			"stream_id": streamID,
		}).Debug("Using existing stream")
		return stream
	}

	stream := &Stream{
		UserID:   userID,
		Username: userID, // In production, resolve username
		Buffer:   new(bytes.Buffer),
		lastAudioTime: time.Now(),
	}
	p.activeStreams[streamID] = stream
	logrus.WithFields(logrus.Fields{
		"ssrc":      ssrc,
		"stream_id": streamID,
		"user_id":   userID,
	}).Info("Created new audio stream")
	return stream
}

// startSilenceTimer starts or resets the silence detection timer
func (s *Stream) startSilenceTimer(processor *Processor, sessionManager *session.Manager, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// If timer already exists, don't create a new one
	if s.silenceTimer != nil {
		return
	}
	
	// Create timer that triggers after silence timeout
	s.silenceTimer = time.AfterFunc(silenceTimeout, func() {
		s.mu.Lock()
		bufferSize := s.Buffer.Len()
		s.mu.Unlock()
		
		if bufferSize > minAudioBuffer {
			logrus.WithFields(logrus.Fields{
				"buffer_size": bufferSize,
				"user":        s.UserID,
				"silence_duration": silenceTimeout,
			}).Info("Silence detected, triggering transcription")
			processor.transcribeAndClear(s, sessionManager, sessionID)
		}
		
		// Clear the timer reference
		s.mu.Lock()
		s.silenceTimer = nil
		s.mu.Unlock()
	})
}

func (p *Processor) transcribeAndClear(stream *Stream, sessionManager *session.Manager, sessionID string) {
	stream.mu.Lock()
	// Prevent concurrent transcriptions
	if stream.isTranscribing {
		stream.mu.Unlock()
		logrus.Debug("Transcription already in progress, skipping")
		return
	}
	stream.isTranscribing = true
	
	// Cancel any pending silence timer
	if stream.silenceTimer != nil {
		stream.silenceTimer.Stop()
		stream.silenceTimer = nil
	}
	
	audioData := stream.Buffer.Bytes()
	stream.Buffer.Reset()
	stream.mu.Unlock()

	// Always clear the transcribing flag when done
	defer func() {
		stream.mu.Lock()
		stream.isTranscribing = false
		stream.mu.Unlock()
	}()

	if len(audioData) == 0 {
		logrus.Debug("No audio data to transcribe")
		return
	}
	
	logrus.WithFields(logrus.Fields{
		"audio_bytes": len(audioData),
		"user":        stream.UserID,
		"session":     sessionID,
	}).Info("Starting transcription")

	// Transcribe audio
	text, err := p.transcriber.Transcribe(audioData)
	if err != nil {
		logrus.WithError(err).Error("Error transcribing audio")
		return
	}
	
	logrus.WithFields(logrus.Fields{
		"text_length": len(text),
		"user":        stream.UserID,
	}).Debug("Transcription completed")

	if text != "" {
		// Add to session
		err = sessionManager.AddTranscript(sessionID, stream.UserID, stream.Username, text)
		if err != nil {
			logrus.WithError(err).Error("Error adding transcript")
		} else {
			logrus.WithFields(logrus.Fields{
				"user":       stream.Username,
				"transcript": text,
			}).Info("Transcript added")
		}
	}
}
