package feedback

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// EventType represents the type of event
type EventType string

const (
	// Transcription events
	EventTranscriptionStarted   EventType = "transcription.started"
	EventTranscriptionProgress  EventType = "transcription.progress"
	EventTranscriptionCompleted EventType = "transcription.completed"
	EventTranscriptionFailed    EventType = "transcription.failed"
	
	// Audio events
	EventAudioBuffering  EventType = "audio.buffering"
	EventAudioSegmented  EventType = "audio.segmented"
	EventSpeakerStarted  EventType = "audio.speaker.started"
	EventSpeakerStopped  EventType = "audio.speaker.stopped"
	
	// System events
	EventQueueDepthChanged EventType = "queue.depth.changed"
	EventWorkerStatusChanged EventType = "worker.status.changed"
	EventSessionCreated EventType = "session.created"
	EventSessionEnded EventType = "session.ended"
)

// Event represents a system event
type Event struct {
	Type      EventType
	Timestamp time.Time
	SessionID string
	Data      interface{}
}

// TranscriptionStartedData contains data for transcription started events
type TranscriptionStartedData struct {
	SegmentID  string
	UserID     string
	Username   string
	Duration   time.Duration
	QueueDepth int
	Priority   int
}

// TranscriptionProgressData contains data for transcription progress events
type TranscriptionProgressData struct {
	SegmentID    string
	UserID       string
	PartialText  string
	ProcessTime  time.Duration
}

// TranscriptionCompletedData contains data for transcription completed events
type TranscriptionCompletedData struct {
	SegmentID    string
	UserID       string
	Username     string
	Text         string
	Confidence   float32
	ProcessTime  time.Duration
	AudioDuration time.Duration
}

// AudioBufferingData contains data for audio buffering events
type AudioBufferingData struct {
	UserID         string
	Username       string
	BufferDuration time.Duration
	BufferSize     int
	IsSpeaking     bool
}

// QueueDepthData contains data for queue depth change events
type QueueDepthData struct {
	TotalDepth    int
	UrgentDepth   int
	HighDepth     int
	NormalDepth   int
	ActiveWorkers int
}

// EventHandler is a function that handles events
type EventHandler func(event Event)

// EventBus manages event distribution
type EventBus struct {
	mu           sync.RWMutex
	handlers     map[EventType][]EventHandler
	allHandlers  []EventHandler
	buffer       chan Event
	stopCh       chan struct{}
	wg           sync.WaitGroup
	metrics      *EventMetrics
}

// EventMetrics tracks event statistics
type EventMetrics struct {
	EventsPublished map[EventType]int64
	EventsDelivered int64
	EventsDropped   int64
	mu              sync.Mutex
}

// NewEventBus creates a new event bus
func NewEventBus(bufferSize int) *EventBus {
	eb := &EventBus{
		handlers: make(map[EventType][]EventHandler),
		buffer:   make(chan Event, bufferSize),
		stopCh:   make(chan struct{}),
		metrics: &EventMetrics{
			EventsPublished: make(map[EventType]int64),
		},
	}
	
	// Start event processor
	eb.wg.Add(1)
	go eb.processEvents()
	
	return eb
}

// Subscribe registers a handler for specific event types
func (eb *EventBus) Subscribe(eventType EventType, handler EventHandler) func() {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	
	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
	
	// Return unsubscribe function
	return func() {
		eb.Unsubscribe(eventType, handler)
	}
}

// SubscribeAll registers a handler for all events
func (eb *EventBus) SubscribeAll(handler EventHandler) func() {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	
	eb.allHandlers = append(eb.allHandlers, handler)
	
	// Return unsubscribe function
	return func() {
		eb.UnsubscribeAll(handler)
	}
}

// Unsubscribe removes a handler for a specific event type
func (eb *EventBus) Unsubscribe(eventType EventType, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	
	handlers := eb.handlers[eventType]
	for i, h := range handlers {
		// Compare function pointers
		if &h == &handler {
			eb.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			break
		}
	}
}

// UnsubscribeAll removes a handler from all events
func (eb *EventBus) UnsubscribeAll(handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	
	for i, h := range eb.allHandlers {
		if &h == &handler {
			eb.allHandlers = append(eb.allHandlers[:i], eb.allHandlers[i+1:]...)
			break
		}
	}
}

// Publish sends an event to all subscribers
func (eb *EventBus) Publish(event Event) {
	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	// Update metrics
	eb.metrics.mu.Lock()
	eb.metrics.EventsPublished[event.Type]++
	eb.metrics.mu.Unlock()
	
	// Non-blocking send
	select {
	case eb.buffer <- event:
		// Event queued
	default:
		// Buffer full, drop event
		eb.metrics.mu.Lock()
		eb.metrics.EventsDropped++
		eb.metrics.mu.Unlock()
		
		logrus.WithFields(logrus.Fields{
			"event_type": event.Type,
			"session_id": event.SessionID,
		}).Warn("Event dropped, buffer full")
	}
}

// PublishAsync publishes an event asynchronously
func (eb *EventBus) PublishAsync(event Event) {
	go eb.Publish(event)
}

// processEvents handles event distribution to subscribers
func (eb *EventBus) processEvents() {
	defer eb.wg.Done()
	
	for {
		select {
		case event := <-eb.buffer:
			eb.deliverEvent(event)
			
		case <-eb.stopCh:
			// Process remaining events
			for len(eb.buffer) > 0 {
				select {
				case event := <-eb.buffer:
					eb.deliverEvent(event)
				default:
					return
				}
			}
			return
		}
	}
}

// deliverEvent sends an event to all relevant handlers
func (eb *EventBus) deliverEvent(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	
	// Deliver to specific handlers
	if handlers, ok := eb.handlers[event.Type]; ok {
		for _, handler := range handlers {
			// Call handler in goroutine to prevent blocking
			go func(h EventHandler) {
				defer func() {
					if r := recover(); r != nil {
						logrus.WithFields(logrus.Fields{
							"event_type": event.Type,
							"panic":      r,
						}).Error("Event handler panic")
					}
				}()
				
				h(event)
				
				eb.metrics.mu.Lock()
				eb.metrics.EventsDelivered++
				eb.metrics.mu.Unlock()
			}(handler)
		}
	}
	
	// Deliver to all-event handlers
	for _, handler := range eb.allHandlers {
		go func(h EventHandler) {
			defer func() {
				if r := recover(); r != nil {
					logrus.WithFields(logrus.Fields{
						"event_type": event.Type,
						"panic":      r,
					}).Error("Event handler panic")
				}
			}()
			
			h(event)
			
			eb.metrics.mu.Lock()
			eb.metrics.EventsDelivered++
			eb.metrics.mu.Unlock()
		}(handler)
	}
}

// Stop gracefully shuts down the event bus
func (eb *EventBus) Stop() {
	close(eb.stopCh)
	eb.wg.Wait()
	close(eb.buffer)
}

// GetMetrics returns event bus metrics
func (eb *EventBus) GetMetrics() EventMetrics {
	eb.metrics.mu.Lock()
	defer eb.metrics.mu.Unlock()
	
	// Create a copy
	metrics := EventMetrics{
		EventsPublished: make(map[EventType]int64),
		EventsDelivered: eb.metrics.EventsDelivered,
		EventsDropped:   eb.metrics.EventsDropped,
	}
	
	for k, v := range eb.metrics.EventsPublished {
		metrics.EventsPublished[k] = v
	}
	
	return metrics
}

// Helper functions for common event publishing

// PublishTranscriptionStarted publishes a transcription started event
func (eb *EventBus) PublishTranscriptionStarted(sessionID string, data TranscriptionStartedData) {
	eb.Publish(Event{
		Type:      EventTranscriptionStarted,
		SessionID: sessionID,
		Data:      data,
	})
}

// PublishTranscriptionCompleted publishes a transcription completed event
func (eb *EventBus) PublishTranscriptionCompleted(sessionID string, data TranscriptionCompletedData) {
	eb.Publish(Event{
		Type:      EventTranscriptionCompleted,
		SessionID: sessionID,
		Data:      data,
	})
}

// PublishAudioBuffering publishes an audio buffering event
func (eb *EventBus) PublishAudioBuffering(sessionID string, data AudioBufferingData) {
	eb.Publish(Event{
		Type:      EventAudioBuffering,
		SessionID: sessionID,
		Data:      data,
	})
}

// PublishQueueDepthChanged publishes a queue depth change event
func (eb *EventBus) PublishQueueDepthChanged(data QueueDepthData) {
	eb.Publish(Event{
		Type: EventQueueDepthChanged,
		Data: data,
	})
}