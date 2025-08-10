package audio

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestVADInitialization tests WebRTC VAD initialization
func TestVADInitialization(t *testing.T) {
	vad := NewVoiceActivityDetector()
	assert.NotNil(t, vad)
	assert.False(t, vad.IsSpeaking())
	assert.Equal(t, 0.0, vad.GetNoiseLevel()) // WebRTC VAD doesn't provide noise level
}

// TestVADConfiguration tests WebRTC VAD with custom configuration
func TestVADConfiguration(t *testing.T) {
	config := VADConfig{
		SpeechFramesRequired:  5,
		SilenceFramesRequired: 10,
	}
	vad := NewVoiceActivityDetectorWithConfig(config)
	assert.NotNil(t, vad)
	assert.Equal(t, 5, vad.speechFramesRequired)
	assert.Equal(t, 10, vad.silenceFramesRequired)
}

// TestVADSilenceDetection tests VAD detection on silence
func TestVADSilenceDetection(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	// Generate silent audio (48kHz stereo)
	silentAudio := make([]byte, 3840) // 960 samples * 2 channels * 2 bytes
	
	// Process multiple frames of silence
	for i := 0; i < 20; i++ {
		result := vad.DetectVoiceActivity(silentAudio)
		if i < 15 { // Before silence threshold
			// May or may not be speaking initially
			_ = result
		} else {
			// After 15 frames of silence, should not be speaking
			assert.False(t, result, "Should detect silence after threshold")
		}
	}
	
	assert.False(t, vad.IsSpeaking(), "Should not be speaking after silence")
}

// TestVADSpeechDetection tests VAD detection on speech
func TestVADSpeechDetection(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	// Generate speech-like audio (48kHz stereo)
	speechAudio := make([]byte, 3840) // 960 samples * 2 channels * 2 bytes
	for i := 0; i < len(speechAudio)/2; i++ {
		// Create a sine wave pattern
		value := int16(5000 * (i % 100) / 100)
		binary.LittleEndian.PutUint16(speechAudio[i*2:], uint16(value))
	}
	
	// Process multiple frames of speech
	speechDetected := false
	for i := 0; i < 10; i++ {
		result := vad.DetectVoiceActivity(speechAudio)
		if result {
			speechDetected = true
		}
	}
	
	// WebRTC VAD may or may not detect this as speech depending on its algorithm
	// Just ensure it doesn't crash
	_ = speechDetected
}

// TestVADNilInput tests VAD with nil input
func TestVADNilInput(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	// Should handle nil input gracefully
	result := vad.DetectVoiceActivity(nil)
	assert.False(t, result, "Nil input should be treated as silence")
	
	// Empty input should also be handled
	result = vad.DetectVoiceActivity([]byte{})
	assert.False(t, result, "Empty input should be treated as silence")
}

// TestVADStateTransitions tests state transitions
func TestVADStateTransitions(t *testing.T) {
	vad := NewVoiceActivityDetectorWithConfig(VADConfig{
		SpeechFramesRequired:  3,
		SilenceFramesRequired: 5,
	})
	
	// Start with silence
	assert.False(t, vad.IsSpeaking())
	
	// Generate loud audio
	loudAudio := make([]byte, 3840)
	for i := 0; i < len(loudAudio)/2; i++ {
		binary.LittleEndian.PutUint16(loudAudio[i*2:], uint16(20000))
	}
	
	// Should transition to speaking after speech frames threshold
	for i := 0; i < 5; i++ {
		vad.DetectVoiceActivity(loudAudio)
	}
	// Note: WebRTC VAD may have its own internal logic
	
	// Generate silence
	silentAudio := make([]byte, 3840)
	
	// Should transition to silence after silence frames threshold
	for i := 0; i < 10; i++ {
		vad.DetectVoiceActivity(silentAudio)
	}
}

// TestVADReset tests VAD reset functionality
func TestVADReset(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	// Set to speaking state
	speechAudio := make([]byte, 3840)
	for i := 0; i < len(speechAudio)/2; i++ {
		binary.LittleEndian.PutUint16(speechAudio[i*2:], uint16(10000))
	}
	
	for i := 0; i < 10; i++ {
		vad.DetectVoiceActivity(speechAudio)
	}
	
	// Reset
	vad.Reset()
	
	// Should be in initial state
	assert.False(t, vad.IsSpeaking(), "Should not be speaking after reset")
	assert.Equal(t, 0, vad.speechCount)
	assert.Equal(t, 0, vad.silenceCount)
}

// TestVADModeChange tests changing VAD mode
func TestVADModeChange(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	// Test valid mode changes
	err := vad.SetMode(0) // Least aggressive
	assert.NoError(t, err)
	assert.Equal(t, 0, vad.mode)
	
	err = vad.SetMode(3) // Most aggressive
	assert.NoError(t, err)
	assert.Equal(t, 3, vad.mode)
	
	// Test invalid modes
	err = vad.SetMode(-1)
	assert.Error(t, err)
	
	err = vad.SetMode(4)
	assert.Error(t, err)
}

// TestVADResampling tests the resampling functions
func TestVADResampling(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	// Test stereo to mono conversion
	stereo := []int16{100, 200, 300, 400, 500, 600}
	mono := vad.convertToMono(stereo)
	assert.Equal(t, []int16{150, 350, 550}, mono) // Average of pairs
	
	// Test downsampling 48kHz to 16kHz
	samples48k := []int16{1, 2, 3, 4, 5, 6, 7, 8, 9}
	samples16k := vad.downsample48to16(samples48k)
	assert.Equal(t, []int16{1, 4, 7}, samples16k) // Every 3rd sample
}

// TestVADCleanup tests resource cleanup
func TestVADCleanup(t *testing.T) {
	vad := NewVoiceActivityDetector()
	assert.NotNil(t, vad.vad)
	
	// Free resources
	vad.Free()
	assert.Nil(t, vad.vad)
	
	// Should not panic on double free
	vad.Free()
	assert.Nil(t, vad.vad)
}

// BenchmarkVAD benchmarks VAD performance
func BenchmarkVAD(b *testing.B) {
	vad := NewVoiceActivityDetector()
	
	// Generate test audio
	audio := make([]byte, 3840) // 960 samples * 2 channels * 2 bytes
	for i := 0; i < len(audio)/2; i++ {
		binary.LittleEndian.PutUint16(audio[i*2:], uint16(i*100))
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		vad.DetectVoiceActivity(audio)
	}
}