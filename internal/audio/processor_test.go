package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fankserver/discord-voice-mcp/internal/session"
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

func (m *MockTranscriber) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestProcessorBufferThreshold(t *testing.T) {
	tests := []struct {
		name           string
		bufferSize     int
		shouldTrigger  bool
		description    string
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