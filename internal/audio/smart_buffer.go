package audio

import (
	"bytes"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// BufferState represents the state of a buffer
type BufferState int

const (
	BufferStateActive BufferState = iota
	BufferStateProcessing
	BufferStateIdle
)

// AudioBuffer holds PCM audio data with metadata
type AudioBuffer struct {
	data           *bytes.Buffer
	firstWriteTime time.Time
	lastWriteTime  time.Time
	lastSpeechTime time.Time
	totalSamples   int
	sampleRate     int
	channels       int
	bytesPerSample int
}

// NewAudioBuffer creates a new audio buffer
func NewAudioBuffer(sampleRate, channels int) *AudioBuffer {
	return &AudioBuffer{
		data:           new(bytes.Buffer),
		sampleRate:     sampleRate,
		channels:       channels,
		bytesPerSample: 2, // 16-bit audio
	}
}

// Append adds PCM data to the buffer
func (b *AudioBuffer) Append(pcm []byte, isSpeech bool) {
	if b.firstWriteTime.IsZero() {
		b.firstWriteTime = time.Now()
	}
	b.lastWriteTime = time.Now()
	
	if isSpeech {
		b.lastSpeechTime = time.Now()
	}
	
	b.data.Write(pcm)
	b.totalSamples += len(pcm) / (b.channels * b.bytesPerSample)
}

// Duration returns the duration of audio in the buffer
func (b *AudioBuffer) Duration() time.Duration {
	if b.sampleRate == 0 {
		return 0
	}
	seconds := float64(b.totalSamples) / float64(b.sampleRate)
	return time.Duration(seconds * float64(time.Second))
}

// GetPCM returns the PCM data
func (b *AudioBuffer) GetPCM() []byte {
	return b.data.Bytes()
}

// LastSpeechTime returns when speech was last detected
func (b *AudioBuffer) LastSpeechTime() time.Time {
	return b.lastSpeechTime
}

// SilenceDuration returns how long since last speech
func (b *AudioBuffer) SilenceDuration() time.Duration {
	if b.lastSpeechTime.IsZero() {
		return 0
	}
	return time.Since(b.lastSpeechTime)
}

// Size returns the buffer size in bytes
func (b *AudioBuffer) Size() int {
	return b.data.Len()
}

// Reset clears the buffer
func (b *AudioBuffer) Reset() {
	b.data.Reset()
	b.firstWriteTime = time.Time{}
	b.lastWriteTime = time.Time{}
	b.lastSpeechTime = time.Time{}
	b.totalSamples = 0
}

// SmartUserBuffer implements dual-buffer system for non-blocking audio processing
type SmartUserBuffer struct {
	// User identification
	userID    string
	username  string
	ssrc      uint32
	sessionID string // Added for event correlation
	
	// Dual buffer system
	activeBuffer     *AudioBuffer
	processingBuffer *AudioBuffer
	
	// VAD for intelligent segmentation
	vad *IntelligentVAD
	
	// State tracking
	lastTranscript     string
	lastTranscriptTime time.Time
	isProcessing       bool
	
	// Configuration
	config BufferConfig
	
	// Metrics
	metrics *BufferMetrics
	
	// Thread safety
	mu sync.Mutex
	
	// Output channel for segments
	outputChan chan<- *AudioSegment
	
	// Callback for transcription completion
	onTranscriptionComplete func(sessionID, userID, username, text string) error
}

// BufferConfig holds configuration for smart buffer
type BufferConfig struct {
	SampleRate         int
	Channels           int
	TargetDuration     time.Duration // Ideal buffer size (3 seconds)
	MaxDuration        time.Duration // Force transcribe at this size (10 seconds)
	MinSpeechDuration  time.Duration // Minimum speech before transcribing (500ms)
	ContextExpiration  time.Duration // How long to keep context (30 seconds)
}

// DefaultBufferConfig returns default configuration optimized for multi-speaker Discord conversations
func DefaultBufferConfig() BufferConfig {
	// Multi-speaker Discord conversations need ultra-responsive processing
	// to handle rapid exchanges without waiting for global silence
	return BufferConfig{
		SampleRate:        48000,
		Channels:          2,
		TargetDuration:    1500 * time.Millisecond, // 1.5s for rapid exchanges
		MaxDuration:       3 * time.Second,          // 3s max to prevent long waits
		MinSpeechDuration: 300 * time.Millisecond,  // 300ms min for quick responses
		ContextExpiration: 15 * time.Second,         // Shorter context for active discussions
	}
}

// BufferMetrics tracks buffer performance
type BufferMetrics struct {
	SegmentsCreated   int
	BytesProcessed    int64
	TotalAudioTime    time.Duration
	AverageBufferSize time.Duration
	DroppedSegments   int
}

// NewSmartUserBuffer creates a new smart buffer for a user
func NewSmartUserBuffer(userID, username string, ssrc uint32, outputChan chan<- *AudioSegment, config BufferConfig) *SmartUserBuffer {
	return NewSmartUserBufferWithCallback(userID, username, ssrc, outputChan, config, nil)
}

// NewSmartUserBufferWithCallback creates a new smart buffer for a user with transcription callback
func NewSmartUserBufferWithCallback(userID, username string, ssrc uint32, outputChan chan<- *AudioSegment, config BufferConfig, onTranscriptionComplete func(sessionID, userID, username, text string) error) *SmartUserBuffer {
	return &SmartUserBuffer{
		userID:                  userID,
		username:                username,
		ssrc:                    ssrc,
		activeBuffer:            NewAudioBuffer(config.SampleRate, config.Channels),
		vad:                     NewIntelligentVAD(NewIntelligentVADConfig()),
		config:                  config,
		metrics:                 &BufferMetrics{},
		outputChan:              outputChan,
		onTranscriptionComplete: onTranscriptionComplete,
	}
}

// SetSessionID sets the session ID for event correlation
func (b *SmartUserBuffer) SetSessionID(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sessionID = sessionID
}

// ProcessAudio handles incoming audio with ultra-responsive multi-speaker processing
func (b *SmartUserBuffer) ProcessAudio(pcm []byte, isSpeech bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	// Always append to active buffer
	b.activeBuffer.Append(pcm, isSpeech)
	b.metrics.BytesProcessed += int64(len(pcm))
	
	// Multi-speaker Discord optimization: ultra-responsive processing
	if !b.isProcessing {
		// Standard VAD decision
		decision := b.vad.ShouldTranscribe(b.activeBuffer)
		
		// Or rapid conversational response (within 5s of last transcript)
		if !decision.Should && time.Since(b.lastTranscriptTime) < 5*time.Second {
			if b.activeBuffer.Duration() > 500*time.Millisecond && b.activeBuffer.SilenceDuration() > 300*time.Millisecond {
				decision = TranscribeDecision{
					Should:   true,
					Priority: PriorityHigh,
					Reason:   "Conversational response detected",
				}
			}
		}
		
		// Or ultra-short processing for very active conversations
		if !decision.Should && b.activeBuffer.Duration() > 800*time.Millisecond {
			if b.activeBuffer.SilenceDuration() > 200*time.Millisecond {
				decision = TranscribeDecision{
					Should:   true,
					Priority: PriorityNormal,
					Reason:   "Ultra-short segment for rapid conversation",
				}
			}
		}
		
		if decision.Should {
			b.triggerTranscription(decision)
		}
	}
}

// triggerTranscription swaps buffers and sends segment for processing
func (b *SmartUserBuffer) triggerTranscription(decision TranscribeDecision) {
	// Don't transcribe tiny buffers
	if b.activeBuffer.Duration() < b.config.MinSpeechDuration {
		logrus.WithFields(logrus.Fields{
			"user":     b.username,
			"duration": b.activeBuffer.Duration(),
			"min":      b.config.MinSpeechDuration,
		}).Debug("Buffer too small, skipping transcription")
		return
	}
	
	// Swap buffers - instant, non-blocking
	b.processingBuffer = b.activeBuffer
	b.activeBuffer = NewAudioBuffer(b.config.SampleRate, b.config.Channels)
	b.isProcessing = true
	
	// Get context if not expired
	var context string
	if time.Since(b.lastTranscriptTime) < b.config.ContextExpiration && b.lastTranscript != "" {
		context = b.lastTranscript
		logrus.WithFields(logrus.Fields{
			"user":          b.username,
			"context_age":   time.Since(b.lastTranscriptTime),
			"context_chars": len(context),
		}).Debug("Using previous transcript as context")
	}
	
	// Create segment for processing
	segment := &AudioSegment{
		ID:          uuid.New().String(),
		SessionID:   b.sessionID,
		UserID:      b.userID,
		Username:    b.username,
		SSRC:        b.ssrc,
		Audio:       b.processingBuffer.GetPCM(),
		Duration:    b.processingBuffer.Duration(),
		Context:     context,
		Priority:    decision.Priority,
		Reason:      decision.Reason,
		SubmittedAt: time.Now(),
		OnComplete: func(text string) {
			b.mu.Lock()
			b.lastTranscript = text
			b.lastTranscriptTime = time.Now()
			b.isProcessing = false
			sessionID := b.sessionID
			b.mu.Unlock()
			
			logrus.WithFields(logrus.Fields{
				"user":       b.username,
				"length":     len(text),
				"session_id": sessionID,
				"text":       text,
			}).Debug("Transcription completed in buffer")
			
			// Call session manager callback if available
			if b.onTranscriptionComplete != nil && text != "" {
				err := b.onTranscriptionComplete(sessionID, b.userID, b.username, text)
				if err != nil {
					logrus.WithError(err).WithFields(logrus.Fields{
						"user":       b.username,
						"session_id": sessionID,
						"text":       text,
					}).Error("Failed to add transcript to session")
				} else {
					logrus.WithFields(logrus.Fields{
						"user":       b.username,
						"session_id": sessionID,
						"text":       text,
					}).Info("Transcript successfully added to session")
				}
			} else {
				logrus.WithFields(logrus.Fields{
					"user":                b.username,
					"has_callback":        b.onTranscriptionComplete != nil,
					"text_empty":          text == "",
					"session_id":          sessionID,
				}).Warn("Transcript not added to session - missing callback or empty text")
			}
		},
		OnError: func(err error) {
			b.mu.Lock()
			b.isProcessing = false
			b.mu.Unlock()
			
			logrus.WithError(err).WithField("user", b.username).Error("Transcription failed")
		},
	}
	
	// Update metrics
	b.metrics.SegmentsCreated++
	b.metrics.TotalAudioTime += segment.Duration
	
	// Non-blocking send to output channel
	select {
	case b.outputChan <- segment:
		logrus.WithFields(logrus.Fields{
			"user":     b.username,
			"duration": segment.Duration,
			"priority": segment.Priority,
			"reason":   segment.Reason,
		}).Info("Audio segment queued for transcription")
	default:
		// Queue full, log and drop
		b.metrics.DroppedSegments++
		logrus.WithFields(logrus.Fields{
			"user":     b.username,
			"duration": segment.Duration,
		}).Warn("Transcription queue full, segment dropped")
		b.isProcessing = false
	}
}

// GetMetrics returns buffer metrics
func (b *SmartUserBuffer) GetMetrics() BufferMetrics {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	metrics := *b.metrics
	if b.metrics.SegmentsCreated > 0 {
		metrics.AverageBufferSize = b.metrics.TotalAudioTime / time.Duration(b.metrics.SegmentsCreated)
	}
	return metrics
}

// GetStatus returns current buffer status
func (b *SmartUserBuffer) GetStatus() BufferStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	return BufferStatus{
		UserID:          b.userID,
		Username:        b.username,
		BufferDuration:  b.activeBuffer.Duration(),
		IsProcessing:    b.isProcessing,
		HasContext:      b.lastTranscript != "",
		ContextAge:      time.Since(b.lastTranscriptTime),
		SegmentsCreated: b.metrics.SegmentsCreated,
		DroppedSegments: b.metrics.DroppedSegments,
	}
}

// BufferStatus represents the current state of a buffer
type BufferStatus struct {
	UserID          string
	Username        string
	BufferDuration  time.Duration
	IsProcessing    bool
	HasContext      bool
	ContextAge      time.Duration
	SegmentsCreated int
	DroppedSegments int
}



// Reset clears the buffer state
func (b *SmartUserBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	b.activeBuffer.Reset()
	if b.processingBuffer != nil {
		b.processingBuffer.Reset()
	}
	b.lastTranscript = ""
	b.lastTranscriptTime = time.Time{}
	b.isProcessing = false
}