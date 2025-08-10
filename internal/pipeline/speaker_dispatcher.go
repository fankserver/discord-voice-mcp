package pipeline

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
	"github.com/sirupsen/logrus"
)

// SpeakerAwareDispatcher manages per-speaker transcription queues for optimal Discord multi-speaker processing
type SpeakerAwareDispatcher struct {
	// Per-speaker queues maintain order for each user while allowing parallel processing
	speakerQueues map[string]*SpeakerQueue
	queuesMu      sync.RWMutex
	
	// Shared transcriber pool
	transcriber transcriber.Transcriber
	
	// Global worker pool for processing all speakers
	workers    []*SpeakerWorker
	workerWg   sync.WaitGroup
	
	// Configuration
	config     SpeakerDispatcherConfig
	
	// Metrics
	metrics    *DispatcherMetrics
	
	// Control
	ctx        context.Context
	cancel     context.CancelFunc
	
	// Round-robin scheduling for fairness
	lastServedQueue int
	scheduleMu      sync.Mutex
}

// SpeakerDispatcherConfig holds configuration for the dispatcher
type SpeakerDispatcherConfig struct {
	WorkerCount           int           // Global workers across all speakers
	MaxQueueSize          int           // Max segments per speaker queue
	ProcessTimeout        time.Duration // Per-segment timeout
	SpeakerIdleTimeout    time.Duration // Cleanup idle speaker queues
	MaxActiveSpeakers     int           // Limit concurrent speaker processing
	PriorityBoostDuration time.Duration // How long to boost priority for rapid responses
}

// DefaultSpeakerDispatcherConfig returns optimized config for Discord multi-speaker
func DefaultSpeakerDispatcherConfig() SpeakerDispatcherConfig {
	return SpeakerDispatcherConfig{
		WorkerCount:           4,                    // More workers for multi-speaker
		MaxQueueSize:          50,                   // Per-speaker queue size
		ProcessTimeout:        20 * time.Second,    // Reasonable timeout for Discord
		SpeakerIdleTimeout:    2 * time.Minute,     // Cleanup after 2min silence
		MaxActiveSpeakers:     8,                    // Up to 8 concurrent speakers
		PriorityBoostDuration: 5 * time.Second,     // Boost responses within 5s
	}
}

// SpeakerQueue holds segments for a specific speaker
type SpeakerQueue struct {
	userID           string
	username         string
	segments         chan *AudioSegment
	lastActivity     time.Time
	isProcessing     bool
	processingMu     sync.Mutex
	segmentsQueued   int64
	segmentsComplete int64
}

// DispatcherMetrics tracks multi-speaker processing performance
type DispatcherMetrics struct {
	ActiveSpeakers     int32
	TotalSpeakers      int64
	SegmentsDispatched int64
	SegmentsCompleted  int64
	SegmentsDropped    int64
	AverageLatency     int64 // milliseconds
	ConcurrentPeak     int32 // Peak concurrent speakers
}

// NewSpeakerAwareDispatcher creates a new speaker-aware dispatcher
func NewSpeakerAwareDispatcher(trans transcriber.Transcriber, config SpeakerDispatcherConfig) *SpeakerAwareDispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	
	d := &SpeakerAwareDispatcher{
		speakerQueues: make(map[string]*SpeakerQueue),
		transcriber:   trans,
		workers:       make([]*SpeakerWorker, 0, config.WorkerCount),
		config:        config,
		metrics:       &DispatcherMetrics{},
		ctx:           ctx,
		cancel:        cancel,
	}
	
	// Start workers
	for i := 0; i < config.WorkerCount; i++ {
		worker := &SpeakerWorker{
			id:         i,
			dispatcher: d,
			transcriber: trans,
		}
		d.workers = append(d.workers, worker)
		
		d.workerWg.Add(1)
		go func(w *SpeakerWorker) {
			defer d.workerWg.Done()
			w.run(ctx)
		}(worker)
	}
	
	// Start cleanup routine
	go d.cleanupIdleSpeakers()
	
	logrus.WithFields(logrus.Fields{
		"workers":         config.WorkerCount,
		"max_speakers":    config.MaxActiveSpeakers,
		"queue_size":      config.MaxQueueSize,
	}).Info("Speaker-aware dispatcher initialized for multi-speaker Discord")
	
	return d
}

// DispatchSegment routes a segment to the appropriate speaker queue
func (d *SpeakerAwareDispatcher) DispatchSegment(segment *AudioSegment) error {
	// Get or create speaker queue
	queue := d.getOrCreateSpeakerQueue(segment.UserID, segment.Username)
	if queue == nil {
		atomic.AddInt64(&d.metrics.SegmentsDropped, 1)
		return ErrQueueFull
	}
	
	// Apply priority boost for rapid conversational responses
	if time.Since(segment.SubmittedAt) < d.config.PriorityBoostDuration {
		segment.Priority = max(segment.Priority, 1) // Boost to high priority
	}
	
	// Non-blocking dispatch to speaker queue
	select {
	case queue.segments <- segment:
		atomic.AddInt64(&queue.segmentsQueued, 1)
		atomic.AddInt64(&d.metrics.SegmentsDispatched, 1)
		queue.lastActivity = time.Now()
		
		logrus.WithFields(logrus.Fields{
			"user":       segment.Username,
			"segment_id": segment.ID,
			"priority":   segment.Priority,
			"reason":     segment.Reason,
		}).Debug("Segment dispatched to speaker queue")
		
		return nil
		
	default:
		// Speaker queue is full - drop the segment
		atomic.AddInt64(&d.metrics.SegmentsDropped, 1)
		logrus.WithFields(logrus.Fields{
			"user":       segment.Username,
			"segment_id": segment.ID,
		}).Warn("Speaker queue full, segment dropped")
		
		return ErrQueueFull
	}
}

// getOrCreateSpeakerQueue gets or creates a queue for a speaker
func (d *SpeakerAwareDispatcher) getOrCreateSpeakerQueue(userID, username string) *SpeakerQueue {
	d.queuesMu.RLock()
	queue, exists := d.speakerQueues[userID]
	d.queuesMu.RUnlock()
	
	if exists {
		return queue
	}
	
	// Check if we've hit the max active speakers limit
	if int(atomic.LoadInt32(&d.metrics.ActiveSpeakers)) >= d.config.MaxActiveSpeakers {
		logrus.WithField("user", username).Warn("Max active speakers reached, rejecting new speaker")
		return nil
	}
	
	// Create new speaker queue
	d.queuesMu.Lock()
	defer d.queuesMu.Unlock()
	
	// Double-check after acquiring write lock
	if queue, exists = d.speakerQueues[userID]; exists {
		return queue
	}
	
	queue = &SpeakerQueue{
		userID:       userID,
		username:     username,
		segments:     make(chan *AudioSegment, d.config.MaxQueueSize),
		lastActivity: time.Now(),
	}
	
	d.speakerQueues[userID] = queue
	atomic.AddInt32(&d.metrics.ActiveSpeakers, 1)
	atomic.AddInt64(&d.metrics.TotalSpeakers, 1)
	
	// Update peak if needed
	current := atomic.LoadInt32(&d.metrics.ActiveSpeakers)
	for {
		peak := atomic.LoadInt32(&d.metrics.ConcurrentPeak)
		if current <= peak || atomic.CompareAndSwapInt32(&d.metrics.ConcurrentPeak, peak, current) {
			break
		}
	}
	
	logrus.WithFields(logrus.Fields{
		"user":            username,
		"active_speakers": current,
	}).Info("New speaker queue created")
	
	return queue
}

// getNextWork returns the next segment to process using round-robin scheduling
func (d *SpeakerAwareDispatcher) getNextWork(ctx context.Context) *AudioSegment {
	d.queuesMu.RLock()
	defer d.queuesMu.RUnlock()
	
	if len(d.speakerQueues) == 0 {
		return nil
	}
	
	// Round-robin through speaker queues for fairness
	d.scheduleMu.Lock()
	startIndex := d.lastServedQueue
	d.scheduleMu.Unlock()
	
	queueSlice := make([]*SpeakerQueue, 0, len(d.speakerQueues))
	for _, queue := range d.speakerQueues {
		queueSlice = append(queueSlice, queue)
	}
	
	// Try each queue starting from last served position
	for i := 0; i < len(queueSlice); i++ {
		index := (startIndex + i) % len(queueSlice)
		queue := queueSlice[index]
		
		// Skip if this speaker is already being processed (maintain order)
		queue.processingMu.Lock()
		if queue.isProcessing {
			queue.processingMu.Unlock()
			continue
		}
		queue.processingMu.Unlock()
		
		// Try to get segment non-blocking
		select {
		case segment := <-queue.segments:
			// Mark this queue as being processed
			queue.processingMu.Lock()
			queue.isProcessing = true
			queue.processingMu.Unlock()
			
			// Update last served index
			d.scheduleMu.Lock()
			d.lastServedQueue = (index + 1) % len(queueSlice)
			d.scheduleMu.Unlock()
			
			return segment
			
		default:
			// Queue is empty, continue to next
			continue
		}
	}
	
	return nil
}

// markSpeakerComplete marks a speaker as no longer processing
func (d *SpeakerAwareDispatcher) markSpeakerComplete(userID string) {
	d.queuesMu.RLock()
	queue, exists := d.speakerQueues[userID]
	d.queuesMu.RUnlock()
	
	if exists {
		queue.processingMu.Lock()
		queue.isProcessing = false
		atomic.AddInt64(&queue.segmentsComplete, 1)
		atomic.AddInt64(&d.metrics.SegmentsCompleted, 1)
		queue.processingMu.Unlock()
	}
}

// cleanupIdleSpeakers removes idle speaker queues to free memory
func (d *SpeakerAwareDispatcher) cleanupIdleSpeakers() {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.performCleanup()
		}
	}
}

// performCleanup removes idle speaker queues
func (d *SpeakerAwareDispatcher) performCleanup() {
	d.queuesMu.Lock()
	defer d.queuesMu.Unlock()
	
	now := time.Now()
	for userID, queue := range d.speakerQueues {
		// Remove if idle for too long and no pending segments
		if now.Sub(queue.lastActivity) > d.config.SpeakerIdleTimeout && len(queue.segments) == 0 {
			close(queue.segments)
			delete(d.speakerQueues, userID)
			atomic.AddInt32(&d.metrics.ActiveSpeakers, -1)
			
			logrus.WithFields(logrus.Fields{
				"user":         queue.username,
				"idle_time":    now.Sub(queue.lastActivity),
				"queued":       atomic.LoadInt64(&queue.segmentsQueued),
				"completed":    atomic.LoadInt64(&queue.segmentsComplete),
			}).Info("Cleaned up idle speaker queue")
		}
	}
}

// Stop gracefully shuts down the dispatcher
func (d *SpeakerAwareDispatcher) Stop() {
	logrus.Info("Stopping speaker-aware dispatcher...")
	
	// Cancel context
	d.cancel()
	
	// Wait for workers to finish
	d.workerWg.Wait()
	
	// Close all speaker queues
	d.queuesMu.Lock()
	for _, queue := range d.speakerQueues {
		close(queue.segments)
	}
	d.queuesMu.Unlock()
	
	logrus.Info("Speaker-aware dispatcher stopped")
}

// GetMetrics returns current dispatcher metrics
func (d *SpeakerAwareDispatcher) GetMetrics() DispatcherMetrics {
	return DispatcherMetrics{
		ActiveSpeakers:     atomic.LoadInt32(&d.metrics.ActiveSpeakers),
		TotalSpeakers:      atomic.LoadInt64(&d.metrics.TotalSpeakers),
		SegmentsDispatched: atomic.LoadInt64(&d.metrics.SegmentsDispatched),
		SegmentsCompleted:  atomic.LoadInt64(&d.metrics.SegmentsCompleted),
		SegmentsDropped:    atomic.LoadInt64(&d.metrics.SegmentsDropped),
		ConcurrentPeak:     atomic.LoadInt32(&d.metrics.ConcurrentPeak),
	}
}

// SpeakerWorker processes segments from the speaker-aware dispatcher
type SpeakerWorker struct {
	id         int
	dispatcher *SpeakerAwareDispatcher
	transcriber transcriber.Transcriber
}

// run starts the worker processing loop
func (w *SpeakerWorker) run(ctx context.Context) {
	logger := logrus.WithField("speaker_worker", w.id)
	logger.Info("Speaker worker started")
	defer logger.Info("Speaker worker stopped")
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		// Get next segment using fair scheduling
		segment := w.dispatcher.getNextWork(ctx)
		if segment == nil {
			// No work available, brief pause
			time.Sleep(10 * time.Millisecond)
			continue
		}
		
		// Process the segment
		w.processSegment(segment)
		
		// Mark speaker as complete
		w.dispatcher.markSpeakerComplete(segment.UserID)
	}
}

// processSegment handles transcription of a single segment
func (w *SpeakerWorker) processSegment(segment *AudioSegment) {
	startTime := time.Now()
	
	logger := logrus.WithFields(logrus.Fields{
		"worker":     w.id,
		"segment_id": segment.ID,
		"user":       segment.Username,
		"duration":   segment.Duration,
		"priority":   segment.Priority,
	})
	
	// Notify start
	if segment.OnStart != nil {
		segment.OnStart()
	}
	
	// Transcribe with context
	options := transcriber.TranscriptionOptions{
		PreviousContext: segment.Context,
	}
	
	result, err := w.transcriber.TranscribeWithContext(segment.Audio, options)
	if err != nil {
		logger.WithError(err).Error("Transcription failed")
		if segment.OnError != nil {
			segment.OnError(err)
		}
		return
	}
	
	// Calculate processing time
	processingTime := time.Since(startTime)
	
	logger.WithFields(logrus.Fields{
		"text_length":    len(result.Text),
		"processing_ms":  processingTime.Milliseconds(),
	}).Info("Segment transcribed successfully")
	
	// Update latency metrics (simple moving average)
	currentLatency := atomic.LoadInt64(&w.dispatcher.metrics.AverageLatency)
	newLatency := (currentLatency + processingTime.Milliseconds()) / 2
	atomic.StoreInt64(&w.dispatcher.metrics.AverageLatency, newLatency)
	
	// Notify completion
	if segment.OnComplete != nil {
		segment.OnComplete(result.Text)
	}
}

// Helper function for max
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}