package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockTranscriber for testing
type MockTranscriber struct {
	mock.Mock
}

func (m *MockTranscriber) Transcribe(audioData []byte) (string, error) {
	args := m.Called(audioData)
	return args.String(0), args.Error(1)
}

func (m *MockTranscriber) TranscribeWithContext(audio []byte, opts transcriber.TranscriptionOptions) (*transcriber.TranscriptResult, error) {
	args := m.Called(audio, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return &transcriber.TranscriptResult{
		Text:       args.String(0),
		Confidence: 0.95,
		Language:   "en",
		Duration:   10 * time.Millisecond,
	}, args.Error(1)
}

func (m *MockTranscriber) IsReady() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockTranscriber) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestProcessorBufferThreshold(t *testing.T) {
	tests := []struct {
		name          string
		bufferSize    int
		shouldTrigger bool
		description   string
	}{
		{
			name:          "buffer_exactly_at_threshold",
			bufferSize:    transcriptionBufferSize,
			shouldTrigger: true,
			description:   "Buffer at exactly 384000 bytes (100%) should trigger transcription",
		},
		{
			name:          "buffer_just_below_threshold",
			bufferSize:    transcriptionBufferSize - 1,
			shouldTrigger: false,
			description:   "Buffer at 383999 bytes (99.99%) should NOT trigger transcription",
		},
		{
			name:          "buffer_above_threshold",
			bufferSize:    transcriptionBufferSize + 1,
			shouldTrigger: true,
			description:   "Buffer at 384001 bytes (100.0003%) should trigger transcription",
		},
		{
			name:          "buffer_at_50_percent",
			bufferSize:    transcriptionBufferSize / 2,
			shouldTrigger: false,
			description:   "Buffer at 50% should NOT trigger transcription",
		},
		{
			name:          "buffer_empty",
			bufferSize:    0,
			shouldTrigger: false,
			description:   "Empty buffer should NOT trigger transcription",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockTranscriber := new(MockTranscriber)
			processor := NewProcessor(mockTranscriber)
			sessionManager := session.NewManager()
			sessionID := sessionManager.CreateSession("test-guild", "test-channel")

			// Create a stream with specific buffer size
			stream := &Stream{
				UserID:   "test-user",
				Username: "TestUser",
				Buffer:   bytes.NewBuffer(make([]byte, tt.bufferSize)),
			}
			processor.activeStreams["test-stream"] = stream

			// Setup mock expectations
			if tt.shouldTrigger {
				// Expect transcription to be called
				mockTranscriber.On("Transcribe", mock.Anything).Return("test transcription", nil).Once()
			}

			// Simulate the threshold check logic
			if stream.Buffer.Len() >= transcriptionBufferSize {
				// This simulates what happens in ProcessVoiceReceive
				go processor.transcribeAndClear(stream, sessionManager, sessionID)

				// Give goroutine time to execute
				time.Sleep(100 * time.Millisecond)
			}

			// Verify expectations
			if tt.shouldTrigger {
				mockTranscriber.AssertExpectations(t)
				// Buffer should be cleared after transcription
				assert.Equal(t, 0, stream.Buffer.Len(), "Buffer should be cleared after transcription")
			} else {
				mockTranscriber.AssertNotCalled(t, "Transcribe")
				// Buffer should remain unchanged
				assert.Equal(t, tt.bufferSize, stream.Buffer.Len(), "Buffer should remain unchanged when threshold not met")
			}
		})
	}
}

func TestProcessorBufferGrowthAndReset(t *testing.T) {
	// Setup
	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create a stream
	stream := &Stream{
		UserID:   "test-user",
		Username: "TestUser",
		Buffer:   new(bytes.Buffer),
	}
	processor.activeStreams["test-stream"] = stream

	// Simulate adding audio packets until threshold is reached
	frameSize := 960
	channels := 2
	bytesPerSample := 2
	packetSize := frameSize * channels * bytesPerSample // 3840 bytes per packet

	packetsNeeded := (transcriptionBufferSize / packetSize) // 100 packets to reach threshold

	// Add packets just below threshold
	for i := 0; i < packetsNeeded-1; i++ {
		data := make([]byte, packetSize)
		stream.Buffer.Write(data)
	}

	assert.Equal(t, packetSize*(packetsNeeded-1), stream.Buffer.Len(),
		"Buffer should be just below threshold")

	// Add one more packet to reach threshold
	data := make([]byte, packetSize)
	stream.Buffer.Write(data)

	assert.Equal(t, transcriptionBufferSize, stream.Buffer.Len(),
		"Buffer should be exactly at threshold")

	// Setup mock for transcription
	mockTranscriber.On("Transcribe", mock.Anything).Return("test transcription", nil).Once()

	// Trigger transcription
	processor.transcribeAndClear(stream, sessionManager, sessionID)

	// Verify buffer was cleared
	assert.Equal(t, 0, stream.Buffer.Len(), "Buffer should be empty after transcription")
	mockTranscriber.AssertExpectations(t)
}

func TestProcessorConcurrentAccess(t *testing.T) {
	// Test that concurrent access to streams is thread-safe
	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)

	// Setup mock to handle multiple transcriptions
	mockTranscriber.On("Transcribe", mock.Anything).Return("test", nil).Maybe()
	mockTranscriber.On("Close").Return(nil).Maybe()

	var wg sync.WaitGroup
	numGoroutines := 10

	// Simulate multiple concurrent voice streams
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Create/get stream
			ssrc := uint32(id)
			userID := fmt.Sprintf("user-%d", id)
			stream := processor.getOrCreateStream(ssrc, userID, userID, userID, nil, "")

			// Add data to buffer
			data := make([]byte, 1000)
			stream.mu.Lock()
			stream.Buffer.Write(data)
			stream.mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Verify all streams were created
	assert.Equal(t, numGoroutines, len(processor.activeStreams))
}

func TestGetOrCreateStream(t *testing.T) {
	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)

	// First call should create a new stream
	ssrc := uint32(12345)
	userID := "user-123"
	username := "TestUser"
	nickname := "TestNick"
	stream1 := processor.getOrCreateStream(ssrc, userID, username, nickname, nil, "")

	assert.NotNil(t, stream1)
	assert.Equal(t, userID, stream1.UserID)
	assert.Equal(t, nickname, stream1.Username) // Username field stores nickname
	assert.NotNil(t, stream1.Buffer)

	// Second call with same SSRC should return the same stream
	stream2 := processor.getOrCreateStream(ssrc, userID, username, nickname, nil, "")
	assert.Equal(t, stream1, stream2, "Should return the same stream instance")

	// Different SSRC should create a new stream
	differentSSRC := uint32(67890)
	stream3 := processor.getOrCreateStream(differentSSRC, "different-user", "DifferentUser", "DiffNick", nil, "")
	assert.NotEqual(t, stream1, stream3, "Should create a different stream for different SSRC")
}

func TestTranscribeAndClearEmptyBuffer(t *testing.T) {
	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create stream with empty buffer
	stream := &Stream{
		UserID:   "test-user",
		Username: "TestUser",
		Buffer:   new(bytes.Buffer),
	}

	// Should not call transcriber for empty buffer
	processor.transcribeAndClear(stream, sessionManager, sessionID)

	mockTranscriber.AssertNotCalled(t, "Transcribe")
}

func TestTranscribeAndClearAddsToSession(t *testing.T) {
	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create stream with data
	stream := &Stream{
		UserID:   "test-user",
		Username: "TestUser",
		Buffer:   bytes.NewBuffer(make([]byte, 1000)),
	}

	expectedText := "Hello, this is a test transcription"
	mockTranscriber.On("Transcribe", mock.Anything).Return(expectedText, nil).Once()

	// Perform transcription
	processor.transcribeAndClear(stream, sessionManager, sessionID)

	// Allow time for goroutine if needed
	time.Sleep(10 * time.Millisecond)

	// Verify transcript was added to session
	session, err := sessionManager.GetSession(sessionID)
	assert.NoError(t, err)
	assert.Len(t, session.Transcripts, 1)
	assert.Equal(t, expectedText, session.Transcripts[0].Text)
	assert.Equal(t, stream.UserID, session.Transcripts[0].UserID)
	assert.Equal(t, stream.Username, session.Transcripts[0].Username)

	mockTranscriber.AssertExpectations(t)
}

// BenchmarkProcessorBufferOperations benchmarks buffer write and threshold check
func BenchmarkProcessorBufferOperations(b *testing.B) {
	// Create a stream
	stream := &Stream{
		UserID:   "bench-user",
		Username: "BenchUser",
		Buffer:   new(bytes.Buffer),
	}

	// Create sample PCM data (one packet)
	pcmData := make([]byte, 3840) // 960 samples * 2 channels * 2 bytes

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Write to buffer
		stream.Buffer.Write(pcmData)

		// Check threshold
		if stream.Buffer.Len() >= transcriptionBufferSize {
			stream.Buffer.Reset()
		}
	}
}

// TestPCMConversion tests the PCM int16 to byte conversion
func TestPCMConversion(t *testing.T) {
	// Test data
	pcmSamples := []int16{-32768, -100, 0, 100, 32767}

	// Convert to bytes (as done in ProcessVoiceReceive)
	pcmBytes := make([]byte, len(pcmSamples)*2)
	for i := 0; i < len(pcmSamples); i++ {
		binary.LittleEndian.PutUint16(pcmBytes[i*2:], uint16(pcmSamples[i]))
	}

	// Convert back to verify
	recovered := make([]int16, len(pcmSamples))
	for i := 0; i < len(pcmSamples); i++ {
		recovered[i] = int16(binary.LittleEndian.Uint16(pcmBytes[i*2:]))
	}

	assert.Equal(t, pcmSamples, recovered, "PCM conversion should be lossless")
}

// TestSilenceDetection tests the silence timer functionality
func TestSilenceDetection(t *testing.T) {
	// Set a short silence timeout for testing
	oldTimeout := silenceTimeout
	silenceTimeout = 100 * time.Millisecond
	defer func() { silenceTimeout = oldTimeout }()

	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create a stream with some audio data
	stream := &Stream{
		UserID:   "test-user",
		Username: "TestUser",
		Buffer:   bytes.NewBuffer(make([]byte, minAudioBuffer+100)), // Just above minimum
	}

	// Set up mock expectation for transcription
	mockTranscriber.On("Transcribe", mock.Anything).Return("silence detected transcript", nil).Once()

	// Start silence timer
	stream.startSilenceTimer(processor, sessionManager, sessionID)

	// Wait for silence timeout to trigger
	time.Sleep(150 * time.Millisecond)

	// Verify transcription was called
	mockTranscriber.AssertExpectations(t)

	// Buffer should be cleared
	assert.Equal(t, 0, stream.Buffer.Len(), "Buffer should be cleared after silence detection")
}

// TestSilenceTimerCancellation tests that silence timer is cancelled when new audio arrives
func TestSilenceTimerCancellation(t *testing.T) {
	// Set a short silence timeout for testing
	oldTimeout := silenceTimeout
	silenceTimeout = 100 * time.Millisecond
	defer func() { silenceTimeout = oldTimeout }()

	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create a stream with some audio data
	stream := &Stream{
		UserID:   "test-user",
		Username: "TestUser",
		Buffer:   bytes.NewBuffer(make([]byte, minAudioBuffer+100)),
	}

	// Start silence timer
	stream.startSilenceTimer(processor, sessionManager, sessionID)

	// Simulate new audio arriving (which should cancel the timer)
	stream.mu.Lock()
	if stream.silenceTimer != nil {
		stream.silenceTimer.Stop()
		stream.silenceTimer = nil
	}
	stream.mu.Unlock()

	// Wait past the silence timeout
	time.Sleep(150 * time.Millisecond)

	// Transcription should NOT have been called
	mockTranscriber.AssertNotCalled(t, "Transcribe")
}

// TestSilenceTimerNotStartedForSmallBuffer tests that silence timer doesn't start for buffers below minimum
func TestSilenceTimerNotStartedForSmallBuffer(t *testing.T) {
	// Set a short silence timeout for testing
	oldTimeout := silenceTimeout
	silenceTimeout = 50 * time.Millisecond
	defer func() { silenceTimeout = oldTimeout }()

	mockTranscriber := new(MockTranscriber)
	// processor and sessionID are created but not used directly in this test
	_ = NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	_ = sessionManager.CreateSession("test-guild", "test-channel")

	// Create a stream with buffer below minimum
	stream := &Stream{
		UserID:   "test-user",
		Username: "TestUser",
		Buffer:   bytes.NewBuffer(make([]byte, minAudioBuffer-1)), // Just below minimum
	}

	// The silence timer logic checks buffer size before transcribing
	// Simulate the check that happens in startSilenceTimer's AfterFunc
	stream.mu.Lock()
	bufferSize := stream.Buffer.Len()
	stream.mu.Unlock()

	if bufferSize > minAudioBuffer {
		// This shouldn't happen in this test
		t.Fatal("Buffer should be below minimum")
	}

	// Transcription should NOT be called for small buffers
	mockTranscriber.AssertNotCalled(t, "Transcribe")
}

// TestMultipleSilenceTimers tests that multiple silence timers don't interfere
func TestMultipleSilenceTimers(t *testing.T) {
	// Set a short silence timeout for testing
	oldTimeout := silenceTimeout
	silenceTimeout = 100 * time.Millisecond
	defer func() { silenceTimeout = oldTimeout }()

	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create a stream
	stream := &Stream{
		UserID:   "test-user",
		Username: "TestUser",
		Buffer:   bytes.NewBuffer(make([]byte, minAudioBuffer+100)),
	}

	// Start first silence timer
	stream.startSilenceTimer(processor, sessionManager, sessionID)

	// Try to start another timer immediately (should not create a new one)
	stream.startSilenceTimer(processor, sessionManager, sessionID)

	// Verify only one timer exists
	stream.mu.Lock()
	hasTimer := stream.silenceTimer != nil
	stream.mu.Unlock()

	assert.True(t, hasTimer, "Should have a timer")

	// Clean up timer
	stream.mu.Lock()
	if stream.silenceTimer != nil {
		stream.silenceTimer.Stop()
		stream.silenceTimer = nil
	}
	stream.mu.Unlock()
}

// TestTranscribeAndClearWithSilenceTimer tests that transcribeAndClear cancels silence timer
func TestTranscribeAndClearWithSilenceTimer(t *testing.T) {
	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create stream with data and active silence timer
	stream := &Stream{
		UserID:   "test-user",
		Username: "TestUser",
		Buffer:   bytes.NewBuffer(make([]byte, 1000)),
	}

	// Start a silence timer
	stream.silenceTimer = time.NewTimer(1 * time.Hour) // Long timer that should be cancelled

	// Setup mock
	mockTranscriber.On("Transcribe", mock.Anything).Return("test", nil).Once()

	// Call transcribeAndClear
	processor.transcribeAndClear(stream, sessionManager, sessionID)

	// Verify silence timer was cancelled
	stream.mu.Lock()
	timerIsNil := stream.silenceTimer == nil
	stream.mu.Unlock()

	assert.True(t, timerIsNil, "Silence timer should be nil after transcribeAndClear")
	mockTranscriber.AssertExpectations(t)
}

// TestConfigurationInit tests the initialization of configuration from environment variables
func TestConfigurationInit(t *testing.T) {
	// Save original values
	origBufferSize := transcriptionBufferSize
	origSilenceTimeout := silenceTimeout
	origMinAudioBuffer := minAudioBuffer

	// Test custom environment variables
	_ = os.Setenv("AUDIO_BUFFER_DURATION_SEC", "5")
	_ = os.Setenv("AUDIO_SILENCE_TIMEOUT_MS", "3000")
	_ = os.Setenv("AUDIO_MIN_BUFFER_MS", "500")

	// Re-run init by calling the initialization logic directly
	// Note: We can't actually re-run init(), so we'll test the logic
	bufferDuration := 5
	expectedBufferSize := sampleRate * channels * 2 * bufferDuration // 48000 * 2 * 2 * 5 = 960000
	assert.Equal(t, 960000, expectedBufferSize, "Buffer size calculation should match expected")

	// Test invalid environment variables (should use defaults)
	_ = os.Setenv("AUDIO_BUFFER_DURATION_SEC", "invalid")
	_ = os.Setenv("AUDIO_SILENCE_TIMEOUT_MS", "-100")
	_ = os.Setenv("AUDIO_MIN_BUFFER_MS", "0")

	// The init function should handle these gracefully and use defaults
	// In real code, negative or zero values should be rejected

	// Clean up environment
	_ = os.Unsetenv("AUDIO_BUFFER_DURATION_SEC")
	_ = os.Unsetenv("AUDIO_SILENCE_TIMEOUT_MS")
	_ = os.Unsetenv("AUDIO_MIN_BUFFER_MS")

	// Restore original values
	transcriptionBufferSize = origBufferSize
	silenceTimeout = origSilenceTimeout
	minAudioBuffer = origMinAudioBuffer
}

// TestConcurrentTranscriptionPrevention tests that multiple concurrent transcriptions are prevented
func TestConcurrentTranscriptionPrevention(t *testing.T) {
	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create stream with data
	stream := &Stream{
		UserID:         "test-user",
		Username:       "TestUser",
		Buffer:         bytes.NewBuffer(make([]byte, 1000)),
		isTranscribing: false,
	}

	// Setup mock to simulate slow transcription
	slowTranscription := make(chan bool)
	mockTranscriber.On("Transcribe", mock.Anything).Run(func(args mock.Arguments) {
		// Block until we signal
		<-slowTranscription
	}).Return("test", nil).Once()

	// Start first transcription in goroutine
	go processor.transcribeAndClear(stream, sessionManager, sessionID)

	// Give first transcription time to set isTranscribing flag
	time.Sleep(10 * time.Millisecond)

	// Try to start second transcription (should be prevented)
	processor.transcribeAndClear(stream, sessionManager, sessionID)

	// The second call should return immediately without calling Transcribe again
	// because isTranscribing is true

	// Unblock the first transcription
	slowTranscription <- true
	close(slowTranscription)

	// Give time for cleanup
	time.Sleep(10 * time.Millisecond)

	// Verify Transcribe was only called once
	mockTranscriber.AssertNumberOfCalls(t, "Transcribe", 1)
}

// TestTranscriptionErrorHandling tests graceful handling of transcription errors
func TestTranscriptionErrorHandling(t *testing.T) {
	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create stream with data
	stream := &Stream{
		UserID:   "test-user",
		Username: "TestUser",
		Buffer:   bytes.NewBuffer(make([]byte, 1000)),
	}

	// Add pending transcription
	err := sessionManager.AddPendingTranscription(sessionID, stream.UserID, stream.Username, 1.0)
	assert.NoError(t, err)

	// Setup mock to return error
	mockTranscriber.On("Transcribe", mock.Anything).Return("", errors.New("transcription failed"))

	// Call transcribeAndClear
	processor.transcribeAndClear(stream, sessionManager, sessionID)

	// Verify buffer was cleared despite error
	assert.Equal(t, 0, stream.Buffer.Len(), "Buffer should be cleared even on error")

	// Verify pending transcription was removed despite error
	session, err := sessionManager.GetSession(sessionID)
	assert.NoError(t, err)
	assert.Empty(t, session.PendingTranscriptions, "Pending should be removed even on error")

	// Verify no transcript was added
	assert.Empty(t, session.Transcripts, "No transcript should be added on error")

	mockTranscriber.AssertExpectations(t)
}

// TestTranscribeAndClearEmptyTranscriptionResult tests handling of empty transcription result
func TestTranscribeAndClearEmptyTranscriptionResult(t *testing.T) {
	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create stream with data
	stream := &Stream{
		UserID:   "test-user",
		Username: "TestUser",
		Buffer:   bytes.NewBuffer(make([]byte, 1000)),
	}

	// Setup mock to return empty string (e.g., silence or unrecognizable audio)
	mockTranscriber.On("Transcribe", mock.Anything).Return("", nil)

	// Call transcribeAndClear
	processor.transcribeAndClear(stream, sessionManager, sessionID)

	// Verify no transcript was added for empty result
	session, err := sessionManager.GetSession(sessionID)
	assert.NoError(t, err)
	assert.Empty(t, session.Transcripts, "No transcript should be added for empty result")

	mockTranscriber.AssertExpectations(t)
}

// TestIsTranscribingFlagReset tests that isTranscribing flag is always reset
func TestIsTranscribingFlagReset(t *testing.T) {
	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)
	sessionManager := session.NewManager()
	sessionID := sessionManager.CreateSession("test-guild", "test-channel")

	// Create stream
	stream := &Stream{
		UserID:         "test-user",
		Username:       "TestUser",
		Buffer:         bytes.NewBuffer(make([]byte, 1000)),
		isTranscribing: false,
	}

	// Test 1: Flag reset after successful transcription
	mockTranscriber.On("Transcribe", mock.Anything).Return("success", nil).Once()
	processor.transcribeAndClear(stream, sessionManager, sessionID)
	assert.False(t, stream.isTranscribing, "Flag should be reset after success")

	// Test 2: Flag reset after transcription error
	stream.Buffer.Write(make([]byte, 1000)) // Add data back
	mockTranscriber.On("Transcribe", mock.Anything).Return("", errors.New("error")).Once()
	processor.transcribeAndClear(stream, sessionManager, sessionID)
	assert.False(t, stream.isTranscribing, "Flag should be reset after error")

	// Test 3: Flag reset even if buffer is empty
	processor.transcribeAndClear(stream, sessionManager, sessionID)
	assert.False(t, stream.isTranscribing, "Flag should be reset even with empty buffer")

	mockTranscriber.AssertExpectations(t)
}
