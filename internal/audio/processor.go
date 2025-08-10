package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
	"github.com/sirupsen/logrus"
	"layeh.com/gopus"
)

// UserResolver interface for resolving SSRC to user information
type UserResolver interface {
	GetUserBySSRC(ssrc uint32) (userID, username, nickname string)
}

const (
	// Audio configuration (these are fixed by Discord)
	sampleRate = 48000
	channels   = 2
	frameSize  = 960 // 20ms @ 48kHz

	// Default values (can be overridden by environment variables)
	// Note: For better transcription accuracy, especially with non-English languages,
	// consider increasing buffer duration to 5-10 seconds to maintain sentence context
	defaultBufferDurationSec    = 3    // 3 seconds provides good balance with prompt context
	defaultSilenceTimeoutMs     = 1500 // 1.5 seconds for natural speech pauses
	defaultMinAudioMs           = 100  // Default minimum audio in milliseconds
	defaultOverlapMs            = 0    // Disabled - audio overlap not needed with prompt context
	defaultContextExpirationSec = 10   // Clear context after 10 seconds of no activity
	
	// VAD defaults
	defaultVADEnergyThreshold   = 0.01  // Energy threshold for voice detection
	defaultVADSpeechFrames      = 3     // Frames needed to confirm speech (60ms)
	defaultVADSilenceFrames     = 15    // Frames needed to confirm silence (300ms)
)

// Configurable variables (set from environment or defaults)
var (
	// transcriptionBufferSize is the buffer size that triggers transcription
	transcriptionBufferSize int

	// silenceTimeout is the duration of silence that triggers transcription
	silenceTimeout time.Duration

	// minAudioBuffer is the minimum audio bytes before considering transcription
	minAudioBuffer int

	// contextExpiration is how long to keep context from previous transcripts
	contextExpiration time.Duration
	
	// VAD configuration
	vadEnergyThreshold   float64
	vadSpeechFrames      int
	vadSilenceFrames     int
)

func init() {
	// Initialize configuration from environment variables or use defaults

	// Buffer duration in seconds (AUDIO_BUFFER_DURATION_SEC)
	bufferDuration := defaultBufferDurationSec
	if val := os.Getenv("AUDIO_BUFFER_DURATION_SEC"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			bufferDuration = parsed
		}
	}
	transcriptionBufferSize = sampleRate * channels * 2 * bufferDuration // samples * channels * bytes per sample * seconds

	// Silence timeout in milliseconds (AUDIO_SILENCE_TIMEOUT_MS)
	silenceMs := defaultSilenceTimeoutMs
	if val := os.Getenv("AUDIO_SILENCE_TIMEOUT_MS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			silenceMs = parsed
		}
	}
	silenceTimeout = time.Duration(silenceMs) * time.Millisecond

	// Minimum audio in milliseconds (AUDIO_MIN_BUFFER_MS)
	minAudioMs := defaultMinAudioMs
	if val := os.Getenv("AUDIO_MIN_BUFFER_MS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			minAudioMs = parsed
		}
	}
	// Calculate minimum buffer size: (samples/sec * channels * bytes/sample * ms) / 1000
	minAudioBuffer = (sampleRate * channels * 2 * minAudioMs) / 1000

	// Get overlap duration
	overlapMs := defaultOverlapMs
	if val := os.Getenv("AUDIO_OVERLAP_MS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed >= 0 {
			overlapMs = parsed
		}
	}

	// Context expiration in seconds (AUDIO_CONTEXT_EXPIRATION_SEC)
	contextExpirationSec := defaultContextExpirationSec
	if val := os.Getenv("AUDIO_CONTEXT_EXPIRATION_SEC"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			contextExpirationSec = parsed
		}
	}
	contextExpiration = time.Duration(contextExpirationSec) * time.Second

	// VAD Energy threshold (VAD_ENERGY_THRESHOLD)
	vadEnergyThreshold = defaultVADEnergyThreshold
	if val := os.Getenv("VAD_ENERGY_THRESHOLD"); val != "" {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil && parsed > 0 {
			vadEnergyThreshold = parsed
		}
	}
	
	// VAD Speech frames required (VAD_SPEECH_FRAMES)
	vadSpeechFrames = defaultVADSpeechFrames
	if val := os.Getenv("VAD_SPEECH_FRAMES"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			vadSpeechFrames = parsed
		}
	}
	
	// VAD Silence frames required (VAD_SILENCE_FRAMES)
	vadSilenceFrames = defaultVADSilenceFrames
	if val := os.Getenv("VAD_SILENCE_FRAMES"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			vadSilenceFrames = parsed
		}
	}

	// Log configuration
	logrus.WithFields(logrus.Fields{
		"buffer_duration_sec":    bufferDuration,
		"buffer_size_bytes":      transcriptionBufferSize,
		"silence_timeout_ms":     silenceMs,
		"min_audio_ms":           minAudioMs,
		"min_audio_bytes":        minAudioBuffer,
		"overlap_ms":             overlapMs,
		"context_expiration_sec": contextExpirationSec,
		"vad_energy_threshold":   vadEnergyThreshold,
		"vad_speech_frames":      vadSpeechFrames,
		"vad_silence_frames":     vadSilenceFrames,
	}).Info("Audio processor configuration loaded")
}

// Processor handles audio capture and transcription
type Processor struct {
	mu            sync.Mutex
	transcriber   transcriber.Transcriber
	activeStreams map[string]*Stream
}

// Stream represents an active audio stream from a user
type Stream struct {
	UserID         string
	Username       string
	Buffer         *bytes.Buffer
	mu             sync.Mutex
	lastAudioTime  time.Time   // Last time we received real audio (not silence)
	isTranscribing bool        // Prevent concurrent transcriptions

	// Context preservation for better transcription accuracy
	lastTranscript     string    // Last successful transcript for context
	lastTranscriptTime time.Time // When the last transcript was created
	overlapBuffer      []byte    // Last 1 second of audio for overlap
	
	// Voice Activity Detection
	vad            *VoiceActivityDetector // VAD instance for this user
	decoderBuffer  []byte                 // Buffer for decoder output reuse
}

// NewProcessor creates a new audio processor
func NewProcessor(t transcriber.Transcriber) *Processor {
	return &Processor{
		transcriber:   t,
		activeStreams: make(map[string]*Stream),
	}
}

// ProcessVoiceReceive handles incoming voice packets
func (p *Processor) ProcessVoiceReceive(vc *discordgo.VoiceConnection, sessionManager *session.Manager, activeSessionID string, userResolver UserResolver) {
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
		if packetCount%100 == 0 {
			logrus.WithField("packets_received", packetCount).Debug("Voice packets received")
		}

		// Log packet details
		isSilencePacket := len(packet.Opus) <= 3
		logrus.WithFields(logrus.Fields{
			"ssrc":            packet.SSRC,
			"opus_len":        len(packet.Opus),
			"timestamp":       packet.Timestamp,
			"packet_num":      packetCount,
			"is_silence_pkt":  isSilencePacket,
		}).Debug("Received voice packet")

		// Get or create stream for user
		userID, username, nickname := userResolver.GetUserBySSRC(packet.SSRC)
		stream := p.getOrCreateStream(packet.SSRC, userID, username, nickname, sessionManager, activeSessionID)

		// Skip VAD for comfort noise packets - they're always silence
		if isSilencePacket {
			// Check if we need to trigger transcription due to silence
			stream.mu.Lock()
			wasSpeaking := stream.vad.IsSpeaking()
			stream.vad.DetectVoiceActivity(nil) // Process as silence
			isSpeaking := stream.vad.IsSpeaking()
			bufferSize := stream.Buffer.Len()
			stream.mu.Unlock()
			
			// Trigger transcription if we transitioned from speaking to silence
			if wasSpeaking && !isSpeaking && bufferSize > minAudioBuffer {
				logrus.WithFields(logrus.Fields{
					"buffer_size": bufferSize,
					"user":        stream.UserID,
				}).Info("VAD detected end of speech, triggering transcription")
				go p.transcribeAndClear(stream, sessionManager, activeSessionID)
			}
			continue
		}
		
		// Decode real audio packets
		var pcmBytes []byte
		{
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

			// Convert PCM to bytes (reuse buffer to reduce allocations)
			if len(stream.decoderBuffer) < len(pcm)*2 {
				stream.decoderBuffer = make([]byte, len(pcm)*2)
			}
			pcmBytes = stream.decoderBuffer[:len(pcm)*2]
			for i := 0; i < len(pcm); i++ {
				// Safe conversion from int16 to uint16 for binary encoding
				// #nosec G115 - This is safe as we're reinterpreting the bits, not converting the value
				binary.LittleEndian.PutUint16(pcmBytes[i*2:], uint16(pcm[i]))
			}
		}

		// Run VAD on the PCM data
		stream.mu.Lock()
		wasSpeaking := stream.vad.IsSpeaking()
		isSpeaking := stream.vad.DetectVoiceActivity(pcmBytes)
		
		// Smart buffering - only buffer when speaking
		if isSpeaking {
			// Voice detected - add to buffer
			stream.Buffer.Write(pcmBytes)
			stream.lastAudioTime = time.Now()
		}
		
		bufferSize := stream.Buffer.Len()
		stream.mu.Unlock()
		
		// Handle state transitions
		if wasSpeaking && !isSpeaking && bufferSize > minAudioBuffer {
			// Speech ended - trigger transcription
			logrus.WithFields(logrus.Fields{
				"buffer_size": bufferSize,
				"user":        stream.UserID,
			}).Info("VAD detected end of speech, triggering transcription")
			go p.transcribeAndClear(stream, sessionManager, activeSessionID)
		}

		logrus.WithFields(logrus.Fields{
			"buffer_size":  bufferSize,
			"threshold":    transcriptionBufferSize,
			"percent_full": float64(bufferSize) / float64(transcriptionBufferSize) * 100,
			"user":         stream.UserID,
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

func (p *Processor) getOrCreateStream(ssrc uint32, userID, username, nickname string, sessionManager *session.Manager, sessionID string) *Stream {
	p.mu.Lock()
	defer p.mu.Unlock()

	streamID := fmt.Sprintf("%d", ssrc)
	if stream, exists := p.activeStreams[streamID]; exists {
		// Update username/nickname in case they changed
		stream.Username = nickname // Use nickname as display name
		logrus.WithFields(logrus.Fields{
			"ssrc":      ssrc,
			"stream_id": streamID,
		}).Debug("Using existing stream")
		return stream
	}

	stream := &Stream{
		UserID:        userID,
		Username:      nickname, // Use nickname as display name
		Buffer:        new(bytes.Buffer),
		lastAudioTime: time.Now(),
		vad:           NewVoiceActivityDetectorWithConfig(VADConfig{
			EnergyThreshold:       vadEnergyThreshold,
			SpeechFramesRequired:  vadSpeechFrames,
			SilenceFramesRequired: vadSilenceFrames,
		}),
		decoderBuffer: make([]byte, frameSize*channels*2), // Pre-allocate decoder buffer
	}
	p.activeStreams[streamID] = stream
	logrus.WithFields(logrus.Fields{
		"ssrc":      ssrc,
		"stream_id": streamID,
		"user_id":   userID,
		"username":  username,
		"nickname":  nickname,
	}).Info("Created new audio stream")
	return stream
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
	audioData := stream.Buffer.Bytes()
	
	// Reset VAD state after transcription
	stream.vad.Reset()

	// Save audio for overlap to prevent word cutoffs
	// Get overlap duration from environment or use default
	overlapDurationMs := defaultOverlapMs
	if val := os.Getenv("AUDIO_OVERLAP_MS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed >= 0 {
			overlapDurationMs = parsed
		}
	}

	// Calculate overlap size in bytes
	// Note: 200ms is usually enough to capture word boundaries without causing duplicate transcriptions
	overlapSize := (sampleRate * channels * 2 * overlapDurationMs) / 1000

	// Skip overlap if disabled (overlapDurationMs = 0)
	if overlapDurationMs == 0 {
		stream.overlapBuffer = nil
	} else {
		// Determine the size of the overlap to copy
		copySize := overlapSize
		if len(audioData) < overlapSize {
			copySize = len(audioData)
		}

		// Reuse buffer if capacity is sufficient to avoid re-allocation
		if cap(stream.overlapBuffer) < copySize {
			stream.overlapBuffer = make([]byte, copySize)
		} else {
			stream.overlapBuffer = stream.overlapBuffer[:copySize]
		}
		copy(stream.overlapBuffer, audioData[len(audioData)-copySize:])
	}

	// Get context from previous transcript
	// Clear context if it's been too long since last transcription (conversation break)
	var lastTranscript string
	if time.Since(stream.lastTranscriptTime) < contextExpiration {
		lastTranscript = stream.lastTranscript
		logrus.WithFields(logrus.Fields{
			"time_since_last": time.Since(stream.lastTranscriptTime),
			"using_context":   true,
		}).Debug("Using previous transcript as context")
	} else if stream.lastTranscript != "" {
		// Clear expired context
		logrus.WithFields(logrus.Fields{
			"time_since_last": time.Since(stream.lastTranscriptTime),
			"context_cleared": true,
		}).Info("Context expired, starting fresh conversation")
		stream.lastTranscript = ""
	}

	stream.Buffer.Reset()
	stream.mu.Unlock()

	// Always clear the transcribing flag and remove pending when done
	defer func() {
		stream.mu.Lock()
		stream.isTranscribing = false
		stream.mu.Unlock()

		// Remove pending transcription (even if transcription failed)
		if err := sessionManager.RemovePendingTranscription(sessionID, stream.UserID); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"session_id": sessionID,
				"user_id":    stream.UserID,
			}).Warn("Failed to remove pending transcription")
		}
	}()

	if len(audioData) == 0 {
		logrus.Debug("No audio data to transcribe")
		return
	}

	// Calculate audio duration in seconds (48kHz, stereo, 16-bit)
	// bytes / (sample_rate * channels * bytes_per_sample)
	audioDuration := float64(len(audioData)) / (float64(sampleRate) * float64(channels) * 2.0) // 2 bytes per sample

	// Add pending transcription before starting
	err := sessionManager.AddPendingTranscription(sessionID, stream.UserID, stream.Username, audioDuration)
	if err != nil {
		logrus.WithError(err).Warn("Failed to add pending transcription")
	}

	logrus.WithFields(logrus.Fields{
		"audio_bytes":  len(audioData),
		"duration_sec": audioDuration,
		"user":         stream.UserID,
		"session":      sessionID,
		"has_context":  lastTranscript != "",
	}).Info("Starting transcription")

	// Transcribe audio with context for better accuracy
	text, err := transcriber.TranscribeWithContext(p.transcriber, audioData, transcriber.TranscribeOptions{
		PreviousTranscript: lastTranscript,
		OverlapAudio:       stream.overlapBuffer,
	})
	if err != nil {
		logrus.WithError(err).Error("Error transcribing audio")
		return
	}

	logrus.WithFields(logrus.Fields{
		"text_length": len(text),
		"user":        stream.UserID,
	}).Debug("Transcription completed")

	if text != "" {
		// Save transcript for context in next chunk
		stream.mu.Lock()
		stream.lastTranscript = text
		stream.lastTranscriptTime = time.Now()
		stream.mu.Unlock()

		// Add to session (this will also remove the pending transcription)
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
