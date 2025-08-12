package pipeline

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// AudioSegment represents a segment of audio to be transcribed
type AudioSegment struct {
	ID          string
	UserID      string
	Username    string
	SSRC        uint32
	Audio       []byte
	Duration    time.Duration
	Context     string
	Priority    int
	Reason      string
	SubmittedAt time.Time

	// Callbacks for progress tracking
	OnStart    func()
	OnProgress func(partial string)
	OnComplete func(final string)
	OnError    func(error)
}

// TranscriptionQueue manages the async processing queue
type TranscriptionQueue struct {
	// Channels for different priorities
	urgentQueue chan *AudioSegment
	highQueue   chan *AudioSegment
	normalQueue chan *AudioSegment

	// Worker management
	workers  []*Worker
	workerWg sync.WaitGroup

	// Metrics
	metrics *QueueMetrics

	// Control
	ctx    context.Context
	cancel context.CancelFunc

	// Configuration
	config QueueConfig
}

// QueueConfig holds queue configuration
type QueueConfig struct {
	WorkerCount    int
	QueueSize      int
	MaxRetries     int
	RetryDelay     time.Duration
	ProcessTimeout time.Duration
}

// DefaultQueueConfig returns default configuration
func DefaultQueueConfig() QueueConfig {
	return QueueConfig{
		WorkerCount:    2,
		QueueSize:      100,
		MaxRetries:     3,
		RetryDelay:     time.Second,
		ProcessTimeout: 30 * time.Second,
	}
}

// QueueMetrics tracks queue performance
type QueueMetrics struct {
	SegmentsQueued     int64
	SegmentsProcessed  int64
	SegmentsFailed     int64
	TotalProcessTime   int64 // in milliseconds
	AverageProcessTime int64 // in milliseconds
	CurrentQueueDepth  int32
	ActiveWorkers      int32
}

// NewTranscriptionQueue creates a new transcription queue
func NewTranscriptionQueue(config QueueConfig) *TranscriptionQueue {
	ctx, cancel := context.WithCancel(context.Background())

	return &TranscriptionQueue{
		urgentQueue: make(chan *AudioSegment, config.QueueSize/4),
		highQueue:   make(chan *AudioSegment, config.QueueSize/4),
		normalQueue: make(chan *AudioSegment, config.QueueSize/2),
		workers:     make([]*Worker, 0, config.WorkerCount),
		metrics:     &QueueMetrics{},
		ctx:         ctx,
		cancel:      cancel,
		config:      config,
	}
}

// Start begins processing with worker pool
func (q *TranscriptionQueue) Start(trans transcriber.Transcriber) {
	// Create and start workers
	for i := 0; i < q.config.WorkerCount; i++ {
		worker := NewWorker(i, q, trans, q.config)
		q.workers = append(q.workers, worker)

		q.workerWg.Add(1)
		go func(w *Worker) {
			defer q.workerWg.Done()
			w.Run(q.ctx)
		}(worker)
	}

	logrus.WithField("workers", q.config.WorkerCount).Info("Transcription queue started")
}

// Stop gracefully shuts down the queue
func (q *TranscriptionQueue) Stop() {
	logrus.Info("Stopping transcription queue...")

	// Cancel context to stop workers
	q.cancel()

	// Wait for workers to finish
	q.workerWg.Wait()

	// Close channels
	close(q.urgentQueue)
	close(q.highQueue)
	close(q.normalQueue)

	logrus.Info("Transcription queue stopped")
}

// Submit adds a segment to the appropriate priority queue
func (q *TranscriptionQueue) Submit(segment *AudioSegment) error {
	// Assign ID if not set
	if segment.ID == "" {
		segment.ID = uuid.New().String()
	}

	// Update metrics
	atomic.AddInt64(&q.metrics.SegmentsQueued, 1)
	atomic.AddInt32(&q.metrics.CurrentQueueDepth, 1)

	// Route to appropriate queue based on priority
	var targetQueue chan *AudioSegment
	switch segment.Priority {
	case 2: // Urgent
		targetQueue = q.urgentQueue
	case 1: // High
		targetQueue = q.highQueue
	default: // Normal
		targetQueue = q.normalQueue
	}

	// Non-blocking send with timeout
	select {
	case targetQueue <- segment:
		logrus.WithFields(logrus.Fields{
			"segment_id": segment.ID,
			"user":       segment.Username,
			"duration":   segment.Duration,
			"priority":   segment.Priority,
		}).Debug("Segment queued for transcription")
		return nil

	case <-time.After(100 * time.Millisecond):
		// Queue is full
		atomic.AddInt32(&q.metrics.CurrentQueueDepth, -1)
		atomic.AddInt64(&q.metrics.SegmentsFailed, 1)

		logrus.WithFields(logrus.Fields{
			"segment_id": segment.ID,
			"user":       segment.Username,
		}).Warn("Queue full, segment rejected")

		if segment.OnError != nil {
			segment.OnError(ErrQueueFull)
		}
		return ErrQueueFull

	case <-q.ctx.Done():
		return ErrQueueStopped
	}
}

// GetMetrics returns current queue metrics
func (q *TranscriptionQueue) GetMetrics() QueueMetrics {
	metrics := QueueMetrics{
		SegmentsQueued:    atomic.LoadInt64(&q.metrics.SegmentsQueued),
		SegmentsProcessed: atomic.LoadInt64(&q.metrics.SegmentsProcessed),
		SegmentsFailed:    atomic.LoadInt64(&q.metrics.SegmentsFailed),
		TotalProcessTime:  atomic.LoadInt64(&q.metrics.TotalProcessTime),
		CurrentQueueDepth: atomic.LoadInt32(&q.metrics.CurrentQueueDepth),
		ActiveWorkers:     atomic.LoadInt32(&q.metrics.ActiveWorkers),
	}

	// Calculate average process time
	if metrics.SegmentsProcessed > 0 {
		metrics.AverageProcessTime = metrics.TotalProcessTime / metrics.SegmentsProcessed
	}

	return metrics
}

// GetQueueDepth returns the current queue depth across all priorities
func (q *TranscriptionQueue) GetQueueDepth() int {
	return len(q.urgentQueue) + len(q.highQueue) + len(q.normalQueue)
}

// updateMetricsAfterProcess updates metrics after processing a segment
func (q *TranscriptionQueue) updateMetricsAfterProcess(processTime time.Duration, success bool) {
	atomic.AddInt32(&q.metrics.CurrentQueueDepth, -1)

	if success {
		atomic.AddInt64(&q.metrics.SegmentsProcessed, 1)
		atomic.AddInt64(&q.metrics.TotalProcessTime, int64(processTime.Milliseconds()))
	} else {
		atomic.AddInt64(&q.metrics.SegmentsFailed, 1)
	}
}
