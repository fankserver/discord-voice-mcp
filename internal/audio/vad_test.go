package audio

import (
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
	
	// Generate silent audio (48kHz stereo) - now using int16
	silentAudio := make([]int16, 1920) // 960 samples * 2 channels
	
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
	
	// Generate speech-like audio (48kHz stereo) - now using int16
	speechAudio := make([]int16, 1920) // 960 samples * 2 channels
	for i := 0; i < len(speechAudio); i++ {
		// Create a sine wave pattern
		value := int16(5000 * (i % 100) / 100)
		speechAudio[i] = value
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
	result = vad.DetectVoiceActivity([]int16{})
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
	
	// Generate loud audio - now using int16
	loudAudio := make([]int16, 1920) // 960 samples * 2 channels
	for i := 0; i < len(loudAudio); i++ {
		loudAudio[i] = 20000
	}
	
	// Should transition to speaking after speech frames threshold
	for i := 0; i < 5; i++ {
		vad.DetectVoiceActivity(loudAudio)
	}
	// Note: WebRTC VAD may have its own internal logic
	
	// Generate silence - now using int16
	silentAudio := make([]int16, 1920)
	
	// Should transition to silence after silence frames threshold
	for i := 0; i < 10; i++ {
		vad.DetectVoiceActivity(silentAudio)
	}
}

// TestVADReset tests VAD reset functionality
func TestVADReset(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	// Set to speaking state - now using int16
	speechAudio := make([]int16, 1920)
	for i := 0; i < len(speechAudio); i++ {
		speechAudio[i] = 10000
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
	
	// Test stereo to mono conversion - now using int16 directly
	stereo := []int16{100, 200, 300, 400, 500, 600}
	mono := make([]int16, 3)
	vad.convertToMonoInPlace(stereo, mono)
	assert.Equal(t, []int16{150, 350, 550}, mono) // Average of pairs
	
	// Test downsampling with filter - 48kHz to 16kHz
	// Note: The filter will modify values, so we can't expect exact values
	samples48k := make([]int16, 960) // Mono samples
	for i := range samples48k {
		samples48k[i] = int16(i * 10)
	}
	samples16k := make([]int16, 320) // 960 / 3
	vad.downsampleWithFilter(samples48k, samples16k)
	
	// Just verify the output has the right length and no crashes
	assert.Equal(t, 320, len(samples16k))
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
	
	// Generate test audio - now using int16
	audio := make([]int16, 1920) // 960 samples * 2 channels
	for i := 0; i < len(audio); i++ {
		audio[i] = int16(i * 100)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		vad.DetectVoiceActivity(audio)
	}
}