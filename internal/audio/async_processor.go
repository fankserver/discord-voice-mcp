package audio

import (
	"encoding/binary"
	"math"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/fankserver/discord-voice-mcp/internal/feedback"
	"github.com/fankserver/discord-voice-mcp/internal/pipeline"
	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
	"github.com/sirupsen/logrus"
	"layeh.com/gopus"
)

const (
	// Audio configuration constants
	defaultSampleRate = 48000
	defaultChannels   = 2

	// Worker and queue configuration
	defaultWorkerCount     = 2
	defaultQueueSize       = 100
	defaultEventBufferSize = 1000
	perSpeakerQueueRatio   = 4 // Divisor for per-speaker queue size

	// Event publishing intervals
	bufferingEventPacketInterval = 50 // Publish buffering status every N packets

	// Packet size thresholds
	comfortNoisePacketMaxSize = 3 // Packets <= 3 bytes are comfort noise

	// Audio data calculations
	bytesPerSample = 2 // 16-bit audio = 2 bytes per sample
)

// UserResolver interface for resolving SSRC to user information
type UserResolver interface {
	GetUserBySSRC(ssrc uint32) (userID, username, nickname string)
	RegisterAudioPacket(ssrc uint32, packetSize int) // Track incoming audio for intelligent mapping
}

// AsyncProcessor handles audio capture and transcription asynchronously
type AsyncProcessor struct {
	// Core components
	dispatcher  *pipeline.SpeakerAwareDispatcher
	transcriber transcriber.Transcriber
	eventBus    *feedback.EventBus

	// User buffers - one per SSRC
	buffers map[uint32]*SmartUserBuffer
	mu      sync.RWMutex

	// Audio segment channel
	segmentChan chan *AudioSegment

	// Configuration
	config ProcessorConfig

	// Metrics
	metrics *processorMetricsInternal

	// Control
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// ProcessorConfig holds configuration for the async processor
type ProcessorConfig struct {
	SampleRate      int
	Channels        int
	WorkerCount     int
	QueueSize       int
	EventBufferSize int
	BufferConfig    BufferConfig
}

// DefaultProcessorConfig returns default configuration
func DefaultProcessorConfig() ProcessorConfig {
	return ProcessorConfig{
		SampleRate:      defaultSampleRate,
		Channels:        defaultChannels,
		WorkerCount:     defaultWorkerCount,
		QueueSize:       defaultQueueSize,
		EventBufferSize: defaultEventBufferSize,
		BufferConfig:    DefaultBufferConfig(),
	}
}

// ProcessorMetrics tracks processor performance (public API)
type ProcessorMetrics struct {
	PacketsReceived  int64
	BytesProcessed   int64
	SegmentsCreated  int64
	ActiveBuffers    int
	TotalTranscripts int64
}

// processorMetricsInternal tracks processor performance with thread safety
type processorMetricsInternal struct {
	PacketsReceived  int64
	BytesProcessed   int64
	SegmentsCreated  int64
	ActiveBuffers    int
	TotalTranscripts int64
	mu               sync.Mutex
}

// NewAsyncProcessor creates a new async audio processor optimized for Discord multi-speaker
func NewAsyncProcessor(trans transcriber.Transcriber, config ProcessorConfig) *AsyncProcessor {
	p := &AsyncProcessor{
		transcriber: trans,
		buffers:     make(map[uint32]*SmartUserBuffer),
		segmentChan: make(chan *AudioSegment, config.QueueSize),
		config:      config,
		metrics:     &processorMetricsInternal{},
		stopCh:      make(chan struct{}),
		eventBus:    feedback.NewEventBus(config.EventBufferSize),
	}

	// Create speaker-aware dispatcher for optimal multi-speaker Discord processing
	dispatcherConfig := pipeline.DefaultSpeakerDispatcherConfig()
	dispatcherConfig.WorkerCount = config.WorkerCount
	dispatcherConfig.MaxQueueSize = config.QueueSize / perSpeakerQueueRatio // Per-speaker queue size
	p.dispatcher = pipeline.NewSpeakerAwareDispatcher(trans, dispatcherConfig)

	// Start segment router
	p.wg.Add(1)
	go p.routeSegments()

	logrus.WithFields(logrus.Fields{
		"workers":        config.WorkerCount,
		"max_speakers":   dispatcherConfig.MaxActiveSpeakers,
		"queue_per_user": dispatcherConfig.MaxQueueSize,
	}).Info("Async processor initialized with speaker-aware dispatcher")

	return p
}

// ProcessVoiceReceive handles incoming voice packets asynchronously
func (p *AsyncProcessor) ProcessVoiceReceive(vc *discordgo.VoiceConnection, sessionManager *session.Manager, activeSessionID string, userResolver UserResolver) {
	// Create opus decoder
	decoder, err := gopus.NewDecoder(p.config.SampleRate, p.config.Channels)
	if err != nil {
		logrus.WithError(err).Error("Error creating opus decoder")
		return
	}

	logrus.Info("Started async voice processing")

	// Publish session created event
	p.eventBus.Publish(feedback.Event{
		Type:      feedback.EventSessionCreated,
		SessionID: activeSessionID,
	})

	packetCount := 0

	// Process incoming audio
	for packet := range vc.OpusRecv {
		packetCount++
		p.metrics.mu.Lock()
		p.metrics.PacketsReceived++
		p.metrics.mu.Unlock()

		// Register packet with resolver for intelligent mapping
		userResolver.RegisterAudioPacket(packet.SSRC, len(packet.Opus))

		// Get user info
		userID, username, nickname := userResolver.GetUserBySSRC(packet.SSRC)

		// Get or create buffer for this user
		buffer := p.getOrCreateBuffer(packet.SSRC, userID, username, nickname, activeSessionID, sessionManager, userResolver)

		// Check if this is a comfort noise packet
		isSilence := len(packet.Opus) <= comfortNoisePacketMaxSize

		if isSilence {
			// Process as silence
			buffer.ProcessAudio(nil, false)
			continue
		}

		// Decode opus to PCM
		pcm, err := decoder.Decode(packet.Opus, frameSize, false)
		if err != nil {
			logrus.WithError(err).Debug("Error decoding opus")
			continue
		}

		// Convert PCM to bytes
		pcmBytes := make([]byte, len(pcm)*bytesPerSample)
		for i := 0; i < len(pcm); i++ {
			// #nosec G115 -- int16 to uint16 conversion is safe for audio samples
			binary.LittleEndian.PutUint16(pcmBytes[i*2:], uint16(pcm[i]))
		}

		// Process audio through smart buffer
		// The buffer will handle VAD and segmentation internally
		buffer.ProcessAudio(pcmBytes, true)

		// Publish buffering event periodically
		if packetCount%bufferingEventPacketInterval == 0 {
			status := buffer.GetStatus()
			p.eventBus.PublishAudioBuffering(activeSessionID, feedback.AudioBufferingData{
				UserID:         status.UserID,
				Username:       status.Username,
				BufferDuration: status.BufferDuration,
				BufferSize:     int(status.BufferDuration.Seconds() * float64(p.config.SampleRate*p.config.Channels*bytesPerSample)),
				IsSpeaking:     status.BufferDuration > 0,
			})
		}

		// Update metrics
		p.metrics.mu.Lock()
		p.metrics.BytesProcessed += int64(len(pcmBytes))
		p.metrics.mu.Unlock()
	}

	logrus.Info("Voice receive channel closed")

	// Publish session ended event
	p.eventBus.Publish(feedback.Event{
		Type:      feedback.EventSessionEnded,
		SessionID: activeSessionID,
	})
}

// getOrCreateBuffer gets or creates a buffer for a user
func (p *AsyncProcessor) getOrCreateBuffer(ssrc uint32, userID, username, nickname string, sessionID string, sessionManager *session.Manager, userResolver UserResolver) *SmartUserBuffer {
	p.mu.RLock()
	buffer, exists := p.buffers[ssrc]
	p.mu.RUnlock()

	if exists {
		return buffer
	}

	// Create new buffer
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if buffer, exists = p.buffers[ssrc]; exists {
		return buffer
	}

	// Use nickname as display name if available
	displayName := nickname
	if displayName == "" {
		displayName = username
	}

	// Create transcription completion callback
	onTranscriptionComplete := func(sessionID, userID, username, text string) error {
		return sessionManager.AddTranscript(sessionID, userID, username, text)
	}

	buffer = NewSmartUserBufferWithCallback(userID, displayName, ssrc, p.segmentChan, p.config.BufferConfig, onTranscriptionComplete)
	buffer.SetSessionID(sessionID)
	buffer.SetUserResolver(userResolver) // Set the resolver for dynamic username resolution
	p.buffers[ssrc] = buffer

	p.metrics.mu.Lock()
	p.metrics.ActiveBuffers = len(p.buffers)
	p.metrics.mu.Unlock()

	logrus.WithFields(logrus.Fields{
		"ssrc":     ssrc,
		"user_id":  userID,
		"username": displayName,
	}).Info("Created new audio buffer for user")

	// Publish speaker started event
	p.eventBus.Publish(feedback.Event{
		Type:      feedback.EventSpeakerStarted,
		SessionID: sessionID,
		Data: struct {
			UserID   string
			Username string
			SSRC     uint32
		}{
			UserID:   userID,
			Username: displayName,
			SSRC:     ssrc,
		},
	})

	return buffer
}

// routeSegments routes audio segments to the processing queue
func (p *AsyncProcessor) routeSegments() {
	defer p.wg.Done()

	for {
		select {
		case segment := <-p.segmentChan:
			// Convert to pipeline segment
			pipelineSegment := &pipeline.AudioSegment{
				ID:          segment.ID,
				UserID:      segment.UserID,
				Username:    segment.Username,
				SSRC:        segment.SSRC,
				Audio:       segment.Audio,
				Duration:    segment.Duration,
				Context:     segment.Context,
				Priority:    int(segment.Priority),
				Reason:      segment.Reason,
				SubmittedAt: segment.SubmittedAt,

				// Set up callbacks
				OnStart: func() {
					p.eventBus.PublishTranscriptionStarted(segment.SessionID, feedback.TranscriptionStartedData{
						SegmentID:  segment.ID,
						UserID:     segment.UserID,
						Username:   segment.Username,
						Duration:   segment.Duration,
						QueueDepth: int(p.dispatcher.GetMetrics().SegmentsDispatched - p.dispatcher.GetMetrics().SegmentsCompleted),
						Priority:   int(segment.Priority),
					})
				},

				OnComplete: func(text string) {
					// Call original callback
					if segment.OnComplete != nil {
						segment.OnComplete(text)
					}

					// Publish completion event
					p.eventBus.PublishTranscriptionCompleted(segment.SessionID, feedback.TranscriptionCompletedData{
						SegmentID:     segment.ID,
						UserID:        segment.UserID,
						Username:      segment.Username,
						Text:          text,
						AudioDuration: segment.Duration,
					})

					// Update metrics
					p.metrics.mu.Lock()
					p.metrics.TotalTranscripts++
					p.metrics.mu.Unlock()
				},

				OnError: func(err error) {
					// Call original callback
					if segment.OnError != nil {
						segment.OnError(err)
					}

					// Publish error event
					p.eventBus.Publish(feedback.Event{
						Type:      feedback.EventTranscriptionFailed,
						SessionID: segment.SessionID,
						Data: struct {
							SegmentID string
							UserID    string
							Error     string
						}{
							SegmentID: segment.ID,
							UserID:    segment.UserID,
							Error:     err.Error(),
						},
					})
				},
			}

			// Dispatch to speaker-aware dispatcher
			if err := p.dispatcher.DispatchSegment(pipelineSegment); err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"segment_id": segment.ID,
					"user":       segment.Username,
				}).Error("Failed to dispatch segment to speaker queue")
			}

			// Update metrics
			p.metrics.mu.Lock()
			p.metrics.SegmentsCreated++
			p.metrics.mu.Unlock()

			// Publish speaker queue metrics
			dispatcherMetrics := p.dispatcher.GetMetrics()
			p.eventBus.PublishQueueDepthChanged(feedback.QueueDepthData{
				TotalDepth:    int(dispatcherMetrics.SegmentsDispatched - dispatcherMetrics.SegmentsCompleted),
				ActiveWorkers: int(dispatcherMetrics.ActiveSpeakers),
			})

		case <-p.stopCh:
			return
		}
	}
}

// GetEventBus returns the event bus for subscribing to events
func (p *AsyncProcessor) GetEventBus() *feedback.EventBus {
	return p.eventBus
}

// GetMetrics returns processor metrics
func (p *AsyncProcessor) GetMetrics() ProcessorMetrics {
	p.metrics.mu.Lock()
	defer p.metrics.mu.Unlock()

	// Create a new metrics struct to avoid copying the mutex
	metrics := ProcessorMetrics{
		PacketsReceived:  p.metrics.PacketsReceived,
		BytesProcessed:   p.metrics.BytesProcessed,
		SegmentsCreated:  p.metrics.SegmentsCreated,
		ActiveBuffers:    p.metrics.ActiveBuffers,
		TotalTranscripts: p.metrics.TotalTranscripts,
	}

	// Add current buffer count
	p.mu.RLock()
	metrics.ActiveBuffers = len(p.buffers)
	p.mu.RUnlock()

	return metrics
}

// GetQueueMetrics returns queue metrics
func (p *AsyncProcessor) GetQueueMetrics() pipeline.QueueMetrics {
	// Return dispatcher metrics in compatible format
	dispatcherMetrics := p.dispatcher.GetMetrics()
	return struct {
		SegmentsQueued     int64
		SegmentsProcessed  int64
		SegmentsFailed     int64
		TotalProcessTime   int64
		AverageProcessTime int64
		CurrentQueueDepth  int32
		ActiveWorkers      int32
	}{
		SegmentsQueued:     dispatcherMetrics.SegmentsDispatched,
		SegmentsProcessed:  dispatcherMetrics.SegmentsCompleted,
		SegmentsFailed:     dispatcherMetrics.SegmentsDropped,
		TotalProcessTime:   0, // Not tracked by dispatcher
		AverageProcessTime: dispatcherMetrics.AverageLatency,
		// #nosec G115 -- Queue depth calculation bounded by MaxInt32
		CurrentQueueDepth: int32(min(dispatcherMetrics.SegmentsDispatched-dispatcherMetrics.SegmentsCompleted, math.MaxInt32)),
		ActiveWorkers:     dispatcherMetrics.ActiveSpeakers,
	}
}

// GetBufferStatuses returns status of all active buffers
func (p *AsyncProcessor) GetBufferStatuses() []BufferStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	statuses := make([]BufferStatus, 0, len(p.buffers))
	for _, buffer := range p.buffers {
		statuses = append(statuses, buffer.GetStatus())
	}

	return statuses
}

// Stop gracefully shuts down the processor
func (p *AsyncProcessor) Stop() {
	logrus.Info("Stopping async processor...")

	// Stop accepting new segments
	close(p.stopCh)

	// Wait for segment router to finish
	p.wg.Wait()

	// Stop the queue
	p.dispatcher.Stop()

	// Stop event bus
	p.eventBus.Stop()

	// Clear buffers
	p.mu.Lock()
	for ssrc := range p.buffers {
		delete(p.buffers, ssrc)
	}
	p.mu.Unlock()

	logrus.Info("Async processor stopped")
}

// AudioSegment extends the base segment with session info
type AudioSegment struct {
	ID          string
	SessionID   string // Added for event correlation
	UserID      string
	Username    string
	SSRC        uint32
	Audio       []byte
	Duration    time.Duration
	Context     string
	Priority    Priority
	Reason      string
	SubmittedAt time.Time
	OnComplete  func(string)
	OnError     func(error)
}
