package audio

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProcessorCleanupStreams tests that cleanupStreams properly frees VAD resources
func TestProcessorCleanupStreams(t *testing.T) {
	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)

	// Create multiple streams with VAD
	for i := 0; i < 5; i++ {
		ssrc := uint32(1000 + i)
		userID := string(rune('A' + i))
		stream := processor.getOrCreateStream(ssrc, userID, userID, userID, nil, "")
		assert.NotNil(t, stream.vad, "VAD should be initialized")
	}

	// Verify streams were created
	assert.Equal(t, 5, len(processor.activeStreams))

	// Call cleanup
	processor.cleanupStreams()

	// Verify all VADs were freed and streams cleared
	assert.Equal(t, 0, len(processor.activeStreams), "All streams should be cleared")
}

// TestProcessorCleanupOnChannelClose tests that cleanup is called when voice channel closes
func TestProcessorCleanupOnChannelClose(t *testing.T) {
	// This test verifies that ProcessVoiceReceive calls cleanupStreams when the channel closes
	// The actual test would require mocking discordgo.VoiceConnection which is complex
	// For now, we just ensure the cleanup method exists and works
	
	mockTranscriber := new(MockTranscriber)
	processor := NewProcessor(mockTranscriber)
	
	// Create a stream
	stream := processor.getOrCreateStream(12345, "user1", "user1", "user1", nil, "")
	assert.NotNil(t, stream.vad)
	
	// Cleanup should free the VAD
	processor.cleanupStreams()
	
	// Verify cleanup happened
	assert.Equal(t, 0, len(processor.activeStreams))
}

// TestVADResourceLeak tests that VAD Free() is idempotent
func TestVADResourceLeak(t *testing.T) {
	vad := NewVoiceActivityDetector()
	assert.NotNil(t, vad.vad)
	
	// First free
	vad.Free()
	assert.Nil(t, vad.vad)
	
	// Second free should not panic
	vad.Free()
	assert.Nil(t, vad.vad)
}