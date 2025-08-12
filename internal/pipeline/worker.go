package pipeline

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
	"github.com/sirupsen/logrus"
)

var (
	// ErrQueueFull is returned when the queue is at capacity
	ErrQueueFull = errors.New("queue is full")

	// ErrQueueStopped is returned when the queue has been stopped
	ErrQueueStopped = errors.New("queue has been stopped")

	// ErrProcessTimeout is returned when processing exceeds timeout
	ErrProcessTimeout = errors.New("processing timeout exceeded")
)

// Worker processes audio segments from the queue
type Worker struct {
	id          int
	queue       *TranscriptionQueue
	transcriber transcriber.Transcriber
	config      QueueConfig
	logger      *logrus.Entry
}

// NewWorker creates a new worker
func NewWorker(id int, queue *TranscriptionQueue, trans transcriber.Transcriber, config QueueConfig) *Worker {
	return &Worker{
		id:          id,
		queue:       queue,
		transcriber: trans,
		config:      config,
		logger: logrus.WithFields(logrus.Fields{
			"worker_id": id,
		}),
	}
}

// Run starts the worker processing loop
func (w *Worker) Run(ctx context.Context) {
	w.logger.Info("Worker started")
	defer w.logger.Info("Worker stopped")

	for {
		// Check if we should stop
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Try to get a segment from queues (priority order)
		segment := w.getNextSegment(ctx)
		if segment == nil {
			// Context was cancelled or timeout
			continue
		}

		// Process the segment
		atomic.AddInt32(&w.queue.metrics.ActiveWorkers, 1)
		w.processSegment(segment)
		atomic.AddInt32(&w.queue.metrics.ActiveWorkers, -1)
	}
}

// getNextSegment retrieves the next segment from priority queues
func (w *Worker) getNextSegment(ctx context.Context) *AudioSegment {
	select {
	// Priority 1: Urgent queue
	case segment := <-w.queue.urgentQueue:
		return segment

	// Priority 2: High queue
	case segment := <-w.queue.highQueue:
		return segment

	// Priority 3: Normal queue
	case segment := <-w.queue.normalQueue:
		return segment

	// Check for context cancellation
	case <-ctx.Done():
		return nil

	// Timeout to prevent blocking forever
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// processSegment handles the transcription of a single segment
func (w *Worker) processSegment(segment *AudioSegment) {
	startTime := time.Now()

	w.logger.WithFields(logrus.Fields{
		"segment_id": segment.ID,
		"user":       segment.Username,
		"duration":   segment.Duration,
		"priority":   segment.Priority,
	}).Debug("Processing segment")

	// Notify start
	if segment.OnStart != nil {
		segment.OnStart()
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), w.config.ProcessTimeout)
	defer cancel()

	// Process with retries
	var lastError error
	for attempt := 0; attempt < w.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-time.After(w.config.RetryDelay):
			case <-ctx.Done():
				lastError = ErrProcessTimeout
				break
			}
		}

		// Check if transcriber is ready
		if !w.transcriber.IsReady() {
			w.logger.Warn("Transcriber not ready, waiting...")
			time.Sleep(time.Second)
			continue
		}

		// Perform transcription
		result, err := w.transcribeWithTimeout(ctx, segment)
		if err == nil {
			// Success
			processTime := time.Since(startTime)
			w.queue.updateMetricsAfterProcess(processTime, true)

			w.logger.WithFields(logrus.Fields{
				"segment_id":   segment.ID,
				"process_time": processTime,
				"text_length":  len(result.Text),
				"confidence":   result.Confidence,
			}).Info("Segment transcribed successfully")

			// Notify completion
			if segment.OnComplete != nil {
				segment.OnComplete(result.Text)
			}
			return
		}

		// Handle error
		lastError = err
		w.logger.WithError(err).WithFields(logrus.Fields{
			"segment_id": segment.ID,
			"attempt":    attempt + 1,
		}).Warn("Transcription failed, retrying...")
	}

	// All retries failed
	processTime := time.Since(startTime)
	w.queue.updateMetricsAfterProcess(processTime, false)

	w.logger.WithError(lastError).WithFields(logrus.Fields{
		"segment_id":   segment.ID,
		"process_time": processTime,
	}).Error("Segment transcription failed after all retries")

	// Notify error
	if segment.OnError != nil {
		segment.OnError(lastError)
	}
}

// transcribeWithTimeout performs transcription with timeout
func (w *Worker) transcribeWithTimeout(ctx context.Context, segment *AudioSegment) (*transcriber.TranscriptResult, error) {
	// Channel for result
	resultChan := make(chan *transcriber.TranscriptResult, 1)
	errorChan := make(chan error, 1)

	// Run transcription in goroutine
	go func() {
		opts := transcriber.TranscriptionOptions{
			PreviousContext: segment.Context,
			Language:        "auto",
		}

		result, err := w.transcriber.TranscribeWithContext(segment.Audio, opts)
		if err != nil {
			errorChan <- err
		} else {
			resultChan <- result
		}
	}()

	// Wait for result or timeout
	select {
	case result := <-resultChan:
		return result, nil

	case err := <-errorChan:
		return nil, err

	case <-ctx.Done():
		return nil, ErrProcessTimeout
	}
}

// GetStatus returns the worker's current status
func (w *Worker) GetStatus() WorkerStatus {
	return WorkerStatus{
		ID:       w.id,
		IsActive: atomic.LoadInt32(&w.queue.metrics.ActiveWorkers) > 0,
		IsReady:  w.transcriber.IsReady(),
	}
}

// WorkerStatus represents the status of a worker
type WorkerStatus struct {
	ID       int
	IsActive bool
	IsReady  bool
}
