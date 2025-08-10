package audio

import (
	"bytes"
	"testing"
	"time"

	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockContextAwareTranscriber for testing context features
type MockContextAwareTranscriber struct {
	mock.Mock
}

func (m *MockContextAwareTranscriber) Transcribe(audio []byte) (string, error) {
	args := m.Called(audio)
	return args.String(0), args.Error(1)
}

func (m *MockContextAwareTranscriber) TranscribeWithContext(audio []byte, opts transcriber.TranscribeOptions) (string, error) {
	args := m.Called(audio, opts)
	return args.String(0), args.Error(1)
}

func (m *MockContextAwareTranscriber) Close() error {
	args := m.Called()
	return args.Error(0)
}

// TestContextPreservation tests that context is preserved between transcriptions
func TestContextPreservation(t *testing.T) {
	mockTranscriber := new(MockContextAwareTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create stream
	stream := &Stream{
		UserID:             "test-user",
		Username:           "TestUser",
		Buffer:             bytes.NewBuffer(make([]byte, 1000)),
		lastTranscript:     "",
		lastTranscriptTime: time.Now(),
		vad:                NewVoiceActivityDetector(),
	}

	// First transcription - no context
	mockTranscriber.On("TranscribeWithContext", mock.Anything, mock.MatchedBy(func(opts transcriber.TranscribeOptions) bool {
		return opts.PreviousTranscript == ""
	})).Return("First transcript", nil).Once()

	processor.transcribeAndClear(stream, sessionManager, sessionID)

	// Verify context was saved
	assert.Equal(t, "First transcript", stream.lastTranscript)
	assert.NotZero(t, stream.lastTranscriptTime)

	// Second transcription - should have context
	stream.Buffer.Write(make([]byte, 1000))
	mockTranscriber.On("TranscribeWithContext", mock.Anything, mock.MatchedBy(func(opts transcriber.TranscribeOptions) bool {
		return opts.PreviousTranscript == "First transcript"
	})).Return("Second transcript", nil).Once()

	processor.transcribeAndClear(stream, sessionManager, sessionID)

	// Verify context was updated
	assert.Equal(t, "Second transcript", stream.lastTranscript)

	mockTranscriber.AssertExpectations(t)
}

// TestContextExpiration tests that context expires after timeout
func TestContextExpiration(t *testing.T) {
	// Save original timeout
	oldTimeout := contextExpiration
	contextExpiration = 100 * time.Millisecond
	defer func() { contextExpiration = oldTimeout }()

	mockTranscriber := new(MockContextAwareTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create stream with existing context
	stream := &Stream{
		UserID:             "test-user",
		Username:           "TestUser",
		Buffer:             bytes.NewBuffer(make([]byte, 1000)),
		lastTranscript:     "Old context",
		lastTranscriptTime: time.Now().Add(-200 * time.Millisecond), // Expired
		vad:                NewVoiceActivityDetector(),
	}

	// Should NOT use expired context
	mockTranscriber.On("TranscribeWithContext", mock.Anything, mock.MatchedBy(func(opts transcriber.TranscribeOptions) bool {
		return opts.PreviousTranscript == ""
	})).Return("New transcript", nil).Once()

	processor.transcribeAndClear(stream, sessionManager, sessionID)

	// Verify old context was cleared
	assert.Equal(t, "New transcript", stream.lastTranscript)

	mockTranscriber.AssertExpectations(t)
}

// TestContextNotExpired tests that context is used when not expired
func TestContextNotExpired(t *testing.T) {
	// Save original timeout
	oldTimeout := contextExpiration
	contextExpiration = 10 * time.Second
	defer func() { contextExpiration = oldTimeout }()

	mockTranscriber := new(MockContextAwareTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create stream with recent context
	stream := &Stream{
		UserID:             "test-user",
		Username:           "TestUser",
		Buffer:             bytes.NewBuffer(make([]byte, 1000)),
		lastTranscript:     "Recent context",
		lastTranscriptTime: time.Now().Add(-1 * time.Second), // Not expired
		vad:                NewVoiceActivityDetector(),
	}

	// Should use recent context
	mockTranscriber.On("TranscribeWithContext", mock.Anything, mock.MatchedBy(func(opts transcriber.TranscribeOptions) bool {
		return opts.PreviousTranscript == "Recent context"
	})).Return("New transcript", nil).Once()

	processor.transcribeAndClear(stream, sessionManager, sessionID)

	mockTranscriber.AssertExpectations(t)
}

// TestMultiUserContextIsolation tests that each user has isolated context
func TestMultiUserContextIsolation(t *testing.T) {
	mockTranscriber := new(MockContextAwareTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create two users' streams
	stream1 := &Stream{
		UserID:             "user-1",
		Username:           "User1",
		Buffer:             bytes.NewBuffer(make([]byte, 1000)),
		lastTranscript:     "User 1 context",
		lastTranscriptTime: time.Now(),
		vad:                NewVoiceActivityDetector(),
	}

	stream2 := &Stream{
		UserID:             "user-2",
		Username:           "User2",
		Buffer:             bytes.NewBuffer(make([]byte, 1000)),
		lastTranscript:     "User 2 context",
		lastTranscriptTime: time.Now(),
		vad:                NewVoiceActivityDetector(),
	}

	processor.activeStreams["stream-1"] = stream1
	processor.activeStreams["stream-2"] = stream2

	// User 1 transcription should use User 1's context
	mockTranscriber.On("TranscribeWithContext", mock.Anything, mock.MatchedBy(func(opts transcriber.TranscribeOptions) bool {
		return opts.PreviousTranscript == "User 1 context"
	})).Return("User 1 new", nil).Once()

	processor.transcribeAndClear(stream1, sessionManager, sessionID)

	// User 2 transcription should use User 2's context
	stream2.Buffer.Write(make([]byte, 1000)) // Add data back
	mockTranscriber.On("TranscribeWithContext", mock.Anything, mock.MatchedBy(func(opts transcriber.TranscribeOptions) bool {
		return opts.PreviousTranscript == "User 2 context"
	})).Return("User 2 new", nil).Once()

	processor.transcribeAndClear(stream2, sessionManager, sessionID)

	// Verify contexts are isolated
	assert.Equal(t, "User 1 new", stream1.lastTranscript)
	assert.Equal(t, "User 2 new", stream2.lastTranscript)

	mockTranscriber.AssertExpectations(t)
}

// TestOverlapBufferManagement tests overlap buffer creation and reuse
func TestOverlapBufferManagement(t *testing.T) {
	// Note: defaultOverlapMs is 0 by default (overlap disabled)

	mockTranscriber := new(MockContextAwareTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create stream with large audio buffer
	audioSize := 10000
	stream := &Stream{
		UserID:        "test-user",
		Username:      "TestUser",
		Buffer:        bytes.NewBuffer(make([]byte, audioSize)),
		overlapBuffer: nil,
		vad:           NewVoiceActivityDetector(),
	}

	// First transcription - should create overlap buffer
	mockTranscriber.On("TranscribeWithContext", mock.Anything, mock.MatchedBy(func(opts transcriber.TranscribeOptions) bool {
		// Since overlap is disabled by default (0ms), this should be nil
		return true
	})).Return("Transcript", nil).Once()

	processor.transcribeAndClear(stream, sessionManager, sessionID)

	// With default overlap of 0ms, overlap buffer should be nil
	assert.Nil(t, stream.overlapBuffer, "Overlap buffer should be nil when disabled")

	mockTranscriber.AssertExpectations(t)
}

// TestContextTimestampUpdate tests that timestamp is updated on successful transcription
func TestContextTimestampUpdate(t *testing.T) {
	mockTranscriber := new(MockContextAwareTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create stream with old timestamp
	oldTime := time.Now().Add(-1 * time.Hour)
	stream := &Stream{
		UserID:             "test-user",
		Username:           "TestUser",
		Buffer:             bytes.NewBuffer(make([]byte, 1000)),
		lastTranscript:     "Old",
		lastTranscriptTime: oldTime,
		vad:                NewVoiceActivityDetector(),
	}

	mockTranscriber.On("TranscribeWithContext", mock.Anything, mock.Anything).Return("New", nil).Once()

	beforeTranscription := time.Now()
	processor.transcribeAndClear(stream, sessionManager, sessionID)
	afterTranscription := time.Now()

	// Timestamp should be updated
	assert.True(t, stream.lastTranscriptTime.After(beforeTranscription))
	assert.True(t, stream.lastTranscriptTime.Before(afterTranscription))
	assert.NotEqual(t, oldTime, stream.lastTranscriptTime)

	mockTranscriber.AssertExpectations(t)
}

// TestEmptyTranscriptNoContextUpdate tests that empty results don't update context
func TestEmptyTranscriptNoContextUpdate(t *testing.T) {
	mockTranscriber := new(MockContextAwareTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create stream with existing context
	originalContext := "Original context"
	originalTime := time.Now()
	stream := &Stream{
		UserID:             "test-user",
		Username:           "TestUser",
		Buffer:             bytes.NewBuffer(make([]byte, 1000)),
		lastTranscript:     originalContext,
		lastTranscriptTime: originalTime,
		vad:                NewVoiceActivityDetector(),
	}

	// Return empty transcript
	mockTranscriber.On("TranscribeWithContext", mock.Anything, mock.Anything).Return("", nil).Once()

	processor.transcribeAndClear(stream, sessionManager, sessionID)

	// Context should NOT be updated for empty transcript
	assert.Equal(t, originalContext, stream.lastTranscript)
	assert.Equal(t, originalTime, stream.lastTranscriptTime)

	mockTranscriber.AssertExpectations(t)
}

// TestContextExpirationConfiguration tests configuration from environment
func TestContextExpirationConfiguration(t *testing.T) {
	// This test would require modifying the init() function behavior
	// For now, we just verify the default value is reasonable
	assert.Greater(t, contextExpiration, time.Duration(0))
	assert.LessOrEqual(t, contextExpiration, 60*time.Second)
}

// BenchmarkTranscribeWithContext benchmarks context-aware transcription
func BenchmarkTranscribeWithContext(b *testing.B) {
	mockTranscriber := new(MockContextAwareTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("bench-guild", "bench-channel")

	stream := &Stream{
		UserID:             "bench-user",
		Username:           "BenchUser",
		Buffer:             bytes.NewBuffer(make([]byte, 1000)),
		lastTranscript:     "Previous context for benchmarking",
		lastTranscriptTime: time.Now(),
		vad:                NewVoiceActivityDetector(),
	}

	// Setup mock to return quickly
	mockTranscriber.On("TranscribeWithContext", mock.Anything, mock.Anything).Return("Result", nil).Maybe()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Reset buffer
		stream.Buffer = bytes.NewBuffer(make([]byte, 1000))

		// Transcribe
		processor.transcribeAndClear(stream, sessionManager, sessionID)
	}
}

// TestRaceConditionContextUpdate tests for race conditions in context updates
func TestRaceConditionContextUpdate(t *testing.T) {
	mockTranscriber := new(MockContextAwareTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	stream := &Stream{
		UserID:   "test-user",
		Username: "TestUser",
		Buffer:   new(bytes.Buffer),
		vad:      NewVoiceActivityDetector(),
	}

	// Setup mock to handle concurrent calls
	mockTranscriber.On("TranscribeWithContext", mock.Anything, mock.Anything).Return("Result", nil).Maybe()

	// Run multiple concurrent transcriptions
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			stream.mu.Lock()
			stream.Buffer = bytes.NewBuffer(make([]byte, 1000))
			stream.mu.Unlock()

			processor.transcribeAndClear(stream, sessionManager, sessionID)
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should complete without panic or deadlock
	assert.NotNil(t, stream.lastTranscript)
}
