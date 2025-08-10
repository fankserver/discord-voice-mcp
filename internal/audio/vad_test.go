package audio

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewVoiceActivityDetector tests VAD creation with defaults
func TestNewVoiceActivityDetector(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	assert.NotNil(t, vad)
	assert.Equal(t, 0.01, vad.energyThreshold)
	assert.Equal(t, 3, vad.speechFramesRequired)
	assert.Equal(t, 15, vad.silenceFramesRequired)
	assert.False(t, vad.isSpeaking)
}

// TestNewVoiceActivityDetectorWithConfig tests VAD creation with custom config
func TestNewVoiceActivityDetectorWithConfig(t *testing.T) {
	config := VADConfig{
		EnergyThreshold:       0.02,
		SpeechFramesRequired:  5,
		SilenceFramesRequired: 20,
	}
	
	vad := NewVoiceActivityDetectorWithConfig(config)
	
	assert.NotNil(t, vad)
	assert.Equal(t, 0.02, vad.energyThreshold)
	assert.Equal(t, 5, vad.speechFramesRequired)
	assert.Equal(t, 20, vad.silenceFramesRequired)
}

// TestVADConfigDefaults tests that zero values get replaced with defaults
func TestVADConfigDefaults(t *testing.T) {
	vad := NewVoiceActivityDetectorWithConfig(VADConfig{})
	
	assert.Equal(t, 0.01, vad.energyThreshold)
	assert.Equal(t, 3, vad.speechFramesRequired)
	assert.Equal(t, 15, vad.silenceFramesRequired)
}

// TestDetectVoiceActivityEmpty tests VAD with empty data
func TestDetectVoiceActivityEmpty(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	result := vad.DetectVoiceActivity([]byte{})
	assert.False(t, result)
	assert.False(t, vad.isSpeaking)
}

// TestDetectVoiceActivitySilence tests VAD with silence
func TestDetectVoiceActivitySilence(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	// Create silent audio (all zeros)
	silentAudio := make([]byte, 1920) // 960 samples * 2 bytes
	
	// Process multiple frames of silence
	for i := 0; i < 20; i++ {
		result := vad.DetectVoiceActivity(silentAudio)
		assert.False(t, result, "Should not detect voice in silence")
	}
	
	assert.False(t, vad.isSpeaking)
}

// TestDetectVoiceActivityLoudNoise tests VAD with loud noise (high energy, high ZCR)
func TestDetectVoiceActivityLoudNoise(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	// Create high-frequency noise (rapid zero crossings)
	noiseAudio := make([]byte, 1920)
	for i := 0; i < len(noiseAudio)/2; i++ {
		// Alternate between positive and negative values (high ZCR)
		var sample int16
		if i%2 == 0 {
			sample = 5000
		} else {
			sample = -5000
		}
		binary.LittleEndian.PutUint16(noiseAudio[i*2:], uint16(sample))
	}
	
	// Process the noise
	result := vad.DetectVoiceActivity(noiseAudio)
	
	// Should not detect as voice due to high ZCR
	assert.False(t, result, "Should not detect high-frequency noise as voice")
}

// TestDetectVoiceActivitySpeech tests VAD with simulated speech
func TestDetectVoiceActivitySpeech(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	// Create simulated speech (moderate energy, moderate ZCR)
	speechAudio := make([]byte, 1920)
	for i := 0; i < len(speechAudio)/2; i++ {
		// Simulate a low-frequency wave (speech-like)
		angle := float64(i) * 2.0 * math.Pi / 50.0 // ~960Hz at 48kHz
		sample := int16(10000 * math.Sin(angle))
		binary.LittleEndian.PutUint16(speechAudio[i*2:], uint16(sample))
	}
	
	// Process multiple frames to trigger speech detection
	var detectedSpeech bool
	for i := 0; i < 10; i++ {
		result := vad.DetectVoiceActivity(speechAudio)
		if result {
			detectedSpeech = true
			break
		}
	}
	
	assert.True(t, detectedSpeech, "Should detect simulated speech")
}

// TestVADHysteresis tests the hysteresis behavior
func TestVADHysteresis(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	// Create speech audio
	speechAudio := make([]byte, 1920)
	for i := 0; i < len(speechAudio)/2; i++ {
		sample := int16(8000) // Constant moderate volume
		binary.LittleEndian.PutUint16(speechAudio[i*2:], uint16(sample))
	}
	
	// Start with speech - need multiple frames to trigger
	for i := 0; i < vad.speechFramesRequired; i++ {
		vad.DetectVoiceActivity(speechAudio)
	}
	assert.True(t, vad.isSpeaking, "Should be speaking after required frames")
	
	// Now send silence - should need multiple frames to stop
	silentAudio := make([]byte, 1920)
	for i := 0; i < vad.silenceFramesRequired-1; i++ {
		result := vad.DetectVoiceActivity(silentAudio)
		assert.True(t, result, "Should still be speaking during silence hysteresis")
	}
	
	// One more silence frame should trigger transition
	result := vad.DetectVoiceActivity(silentAudio)
	assert.False(t, result, "Should stop speaking after required silence frames")
}

// TestVADNoiseAdaptation tests background noise adaptation
func TestVADNoiseAdaptation(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	// Start with low background noise
	lowNoise := make([]byte, 1920)
	for i := 0; i < len(lowNoise)/2; i++ {
		sample := int16(100) // Very low amplitude
		binary.LittleEndian.PutUint16(lowNoise[i*2:], uint16(sample))
	}
	
	// Process multiple frames to establish noise floor
	initialNoiseLevel := vad.backgroundNoiseLevel
	for i := 0; i < 50; i++ {
		vad.DetectVoiceActivity(lowNoise)
	}
	
	// Noise level should have adapted
	assert.NotEqual(t, initialNoiseLevel, vad.backgroundNoiseLevel, "Noise level should adapt")
	assert.Greater(t, vad.backgroundNoiseLevel, 0.0, "Noise level should be positive")
}

// TestCalculateRMS tests RMS calculation
func TestCalculateRMS(t *testing.T) {
	tests := []struct {
		name     string
		samples  []int16
		expected float64
		epsilon  float64
	}{
		{
			name:     "all_zeros",
			samples:  []int16{0, 0, 0, 0},
			expected: 0.0,
			epsilon:  0.001,
		},
		{
			name:     "constant_positive",
			samples:  []int16{1000, 1000, 1000, 1000},
			expected: 1000.0 / 32768.0,
			epsilon:  0.001,
		},
		{
			name:     "constant_negative",
			samples:  []int16{-1000, -1000, -1000, -1000},
			expected: 1000.0 / 32768.0,
			epsilon:  0.001,
		},
		{
			name:     "alternating",
			samples:  []int16{1000, -1000, 1000, -1000},
			expected: 1000.0 / 32768.0,
			epsilon:  0.001,
		},
		{
			name:     "max_amplitude",
			samples:  []int16{32767, 32767},
			expected: 32767.0 / 32768.0,
			epsilon:  0.001,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rms := calculateRMS(tt.samples)
			assert.InDelta(t, tt.expected, rms, tt.epsilon)
		})
	}
}

// TestCalculateZeroCrossingRate tests ZCR calculation
func TestCalculateZeroCrossingRate(t *testing.T) {
	tests := []struct {
		name     string
		samples  []int16
		expected float64
	}{
		{
			name:     "no_crossings_positive",
			samples:  []int16{100, 200, 300, 400},
			expected: 0.0,
		},
		{
			name:     "no_crossings_negative",
			samples:  []int16{-100, -200, -300, -400},
			expected: 0.0,
		},
		{
			name:     "all_crossings",
			samples:  []int16{100, -100, 100, -100},
			expected: 1.0, // 3 crossings / 3 intervals = 1.0
		},
		{
			name:     "one_crossing",
			samples:  []int16{100, 100, -100, -100},
			expected: 1.0 / 3.0, // 1 crossing / 3 intervals
		},
		{
			name:     "with_zero",
			samples:  []int16{100, 0, -100},
			expected: 1.0 / 2.0, // 1 crossing / 2 intervals
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zcr := calculateZeroCrossingRate(tt.samples)
			assert.InDelta(t, tt.expected, zcr, 0.01)
		})
	}
}

// TestBytesToInt16 tests PCM byte to int16 conversion
func TestBytesToInt16(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []int16
	}{
		{
			name:     "empty",
			input:    []byte{},
			expected: []int16{},
		},
		{
			name:     "odd_length_returns_nil",
			input:    []byte{1, 2, 3},
			expected: nil,
		},
		{
			name:     "positive_values",
			input:    []byte{0x00, 0x01, 0x00, 0x02}, // Little-endian 256, 512
			expected: []int16{256, 512},
		},
		{
			name:     "negative_values",
			input:    []byte{0xFF, 0xFF, 0xFE, 0xFF}, // Little-endian -1, -2
			expected: []int16{-1, -2},
		},
		{
			name:     "mixed_values",
			input:    []byte{0x00, 0x01, 0xFF, 0xFF}, // Little-endian 256, -1
			expected: []int16{256, -1},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bytesToInt16(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestVADReset tests the reset functionality
func TestVADReset(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	// Set up some state
	vad.speechCount = 5
	vad.silenceCount = 10
	vad.isSpeaking = true
	vad.backgroundNoiseLevel = 0.05
	vad.adaptiveThreshold = 0.1
	
	// Reset
	vad.Reset()
	
	// Verify reset state
	assert.Equal(t, 0, vad.speechCount)
	assert.Equal(t, 0, vad.silenceCount)
	assert.False(t, vad.isSpeaking)
	assert.Equal(t, 0.001, vad.backgroundNoiseLevel)
	assert.Equal(t, vad.energyThreshold, vad.adaptiveThreshold)
}

// TestVADGetters tests the getter methods
func TestVADGetters(t *testing.T) {
	vad := NewVoiceActivityDetector()
	
	// Test initial state
	assert.False(t, vad.IsSpeaking())
	assert.Equal(t, 0.001, vad.GetNoiseLevel())
	
	// Modify state
	vad.isSpeaking = true
	vad.backgroundNoiseLevel = 0.05
	
	// Test modified state
	assert.True(t, vad.IsSpeaking())
	assert.Equal(t, 0.05, vad.GetNoiseLevel())
}

// TestClassifyFrame tests frame classification logic
func TestClassifyFrame(t *testing.T) {
	vad := NewVoiceActivityDetector()
	vad.adaptiveThreshold = 0.01
	vad.backgroundNoiseLevel = 0.001
	
	tests := []struct {
		name     string
		energy   float64
		zcr      float64
		expected bool
	}{
		{
			name:     "below_threshold",
			energy:   0.005,
			zcr:      0.1,
			expected: false,
		},
		{
			name:     "above_threshold_low_zcr",
			energy:   0.02,
			zcr:      0.1,
			expected: true,
		},
		{
			name:     "above_threshold_high_zcr",
			energy:   0.02,
			zcr:      0.6, // > zcThreshold * 2
			expected: false,
		},
		{
			name:     "just_above_noise",
			energy:   0.0015, // < backgroundNoiseLevel * 2
			zcr:      0.1,
			expected: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := vad.classifyFrame(tt.energy, tt.zcr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// BenchmarkDetectVoiceActivity benchmarks VAD processing
func BenchmarkDetectVoiceActivity(b *testing.B) {
	vad := NewVoiceActivityDetector()
	
	// Create test audio data (960 samples * 2 bytes)
	audio := make([]byte, 1920)
	for i := 0; i < len(audio)/2; i++ {
		sample := int16(5000)
		binary.LittleEndian.PutUint16(audio[i*2:], uint16(sample))
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = vad.DetectVoiceActivity(audio)
	}
}

// BenchmarkCalculateRMS benchmarks RMS calculation
func BenchmarkCalculateRMS(b *testing.B) {
	samples := make([]int16, 960)
	for i := range samples {
		samples[i] = int16(i * 10)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = calculateRMS(samples)
	}
}

// BenchmarkCalculateZeroCrossingRate benchmarks ZCR calculation
func BenchmarkCalculateZeroCrossingRate(b *testing.B) {
	samples := make([]int16, 960)
	for i := range samples {
		if i%2 == 0 {
			samples[i] = 1000
		} else {
			samples[i] = -1000
		}
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = calculateZeroCrossingRate(samples)
	}
}

// TestVADConcurrentUse tests thread safety
func TestVADConcurrentUse(t *testing.T) {
	vad := NewVoiceActivityDetector()
	audio := make([]byte, 1920)
	
	// Fill with some data
	for i := 0; i < len(audio)/2; i++ {
		binary.LittleEndian.PutUint16(audio[i*2:], uint16(1000))
	}
	
	// Run multiple goroutines
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = vad.DetectVoiceActivity(audio)
			}
			done <- true
		}()
	}
	
	// Wait for all to complete
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Should complete without panic
	assert.NotNil(t, vad)
}