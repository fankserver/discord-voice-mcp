package audio

import (
	"encoding/binary"
	"math"

	"github.com/sirupsen/logrus"
)

// VoiceActivityDetector detects voice activity in audio samples
type VoiceActivityDetector struct {
	// Energy-based VAD parameters
	energyThreshold      float64 // Minimum energy threshold for voice
	adaptiveThreshold    float64 // Adaptive threshold based on background noise
	backgroundNoiseLevel float64 // Estimated background noise level
	
	// Zero-crossing rate for distinguishing voice from noise
	zcThreshold float64 // Zero-crossing rate threshold
	
	// Smoothing parameters
	smoothingFactor float64 // For exponential smoothing
	speechCount     int     // Consecutive frames detected as speech
	silenceCount    int     // Consecutive frames detected as silence
	
	// Thresholds for state transitions
	speechFramesRequired  int // Frames needed to transition to speech
	silenceFramesRequired int // Frames needed to transition to silence
	
	// Current state
	isSpeaking bool
}

// NewVoiceActivityDetector creates a new VAD instance
func NewVoiceActivityDetector() *VoiceActivityDetector {
	return NewVoiceActivityDetectorWithConfig(VADConfig{})
}

// VADConfig holds configuration for Voice Activity Detector
type VADConfig struct {
	EnergyThreshold       float64
	SpeechFramesRequired  int
	SilenceFramesRequired int
}

// NewVoiceActivityDetectorWithConfig creates a new VAD with custom configuration
func NewVoiceActivityDetectorWithConfig(config VADConfig) *VoiceActivityDetector {
	// Apply defaults if not specified
	if config.EnergyThreshold <= 0 {
		config.EnergyThreshold = 0.01
	}
	if config.SpeechFramesRequired <= 0 {
		config.SpeechFramesRequired = 3
	}
	if config.SilenceFramesRequired <= 0 {
		config.SilenceFramesRequired = 15
	}
	
	return &VoiceActivityDetector{
		energyThreshold:       config.EnergyThreshold,
		adaptiveThreshold:     config.EnergyThreshold,  // Start with base threshold
		backgroundNoiseLevel:  0.001,                   // Initial noise estimate
		zcThreshold:          0.25,                     // Zero-crossing rate threshold
		smoothingFactor:      0.1,                      // Exponential smoothing factor
		speechFramesRequired:  config.SpeechFramesRequired,
		silenceFramesRequired: config.SilenceFramesRequired,
		isSpeaking:           false,
	}
}

// DetectVoiceActivity analyzes PCM audio samples to detect voice
func (vad *VoiceActivityDetector) DetectVoiceActivity(pcmData []byte) bool {
	if len(pcmData) == 0 {
		return false
	}
	
	// Convert byte array to int16 samples
	samples := bytesToInt16(pcmData)
	if len(samples) == 0 {
		return false
	}
	
	// Calculate energy (RMS - Root Mean Square)
	energy := calculateRMS(samples)
	
	// Calculate zero-crossing rate
	zcr := calculateZeroCrossingRate(samples)
	
	// Update adaptive threshold based on background noise
	vad.updateNoiseEstimate(energy)
	
	// Determine if this frame contains voice
	isVoice := vad.classifyFrame(energy, zcr)
	
	// Update state with hysteresis to prevent rapid switching
	vad.updateState(isVoice)
	
	logrus.WithFields(logrus.Fields{
		"energy":            energy,
		"zcr":              zcr,
		"threshold":        vad.adaptiveThreshold,
		"noise_level":      vad.backgroundNoiseLevel,
		"is_voice":         isVoice,
		"is_speaking":      vad.isSpeaking,
		"speech_count":     vad.speechCount,
		"silence_count":    vad.silenceCount,
	}).Debug("VAD analysis")
	
	return vad.isSpeaking
}

// updateNoiseEstimate updates the background noise level estimate
func (vad *VoiceActivityDetector) updateNoiseEstimate(energy float64) {
	// Only update noise estimate during silence
	if !vad.isSpeaking && energy < vad.adaptiveThreshold*2 {
		// Exponential smoothing for noise level
		vad.backgroundNoiseLevel = vad.smoothingFactor*energy + 
			(1-vad.smoothingFactor)*vad.backgroundNoiseLevel
		
		// Update adaptive threshold (noise level + margin)
		vad.adaptiveThreshold = vad.backgroundNoiseLevel * 3.0
		
		// Ensure minimum threshold
		if vad.adaptiveThreshold < vad.energyThreshold {
			vad.adaptiveThreshold = vad.energyThreshold
		}
	}
}

// classifyFrame determines if a frame contains voice based on features
func (vad *VoiceActivityDetector) classifyFrame(energy, zcr float64) bool {
	// Energy-based detection
	if energy < vad.adaptiveThreshold {
		return false
	}
	
	// Zero-crossing rate helps distinguish voice from other sounds
	// Voice typically has ZCR in a certain range
	// Very high ZCR often indicates fricative sounds or noise
	if zcr > vad.zcThreshold*2 {
		// Very high ZCR, likely noise
		return false
	}
	
	// Additional check: energy should be significantly above noise
	if energy < vad.backgroundNoiseLevel*2 {
		return false
	}
	
	return true
}

// updateState updates the VAD state with hysteresis
func (vad *VoiceActivityDetector) updateState(isVoice bool) {
	if isVoice {
		vad.speechCount++
		vad.silenceCount = 0
		
		// Transition to speaking after enough speech frames
		if vad.speechCount >= vad.speechFramesRequired {
			vad.isSpeaking = true
		}
	} else {
		vad.silenceCount++
		vad.speechCount = 0
		
		// Transition to silence after enough silence frames
		if vad.silenceCount >= vad.silenceFramesRequired {
			vad.isSpeaking = false
		}
	}
}

// bytesToInt16 converts PCM byte array to int16 samples
func bytesToInt16(pcmData []byte) []int16 {
	if len(pcmData)%2 != 0 {
		// Invalid PCM data
		return nil
	}
	
	samples := make([]int16, len(pcmData)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(pcmData[i*2:]))
	}
	return samples
}

// calculateRMS calculates the Root Mean Square (energy) of audio samples
func calculateRMS(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}
	
	var sum float64
	for _, sample := range samples {
		// Normalize to [-1, 1] range
		normalized := float64(sample) / 32768.0
		sum += normalized * normalized
	}
	
	return math.Sqrt(sum / float64(len(samples)))
}

// calculateZeroCrossingRate calculates how often the signal crosses zero
func calculateZeroCrossingRate(samples []int16) float64 {
	if len(samples) < 2 {
		return 0
	}
	
	crossings := 0
	for i := 1; i < len(samples); i++ {
		// Check if sign changed
		if (samples[i-1] >= 0) != (samples[i] >= 0) {
			crossings++
		}
	}
	
	// Normalize by sample count
	return float64(crossings) / float64(len(samples)-1)
}

// Reset resets the VAD state
func (vad *VoiceActivityDetector) Reset() {
	vad.speechCount = 0
	vad.silenceCount = 0
	vad.isSpeaking = false
	vad.backgroundNoiseLevel = 0.001
	vad.adaptiveThreshold = vad.energyThreshold
}

// IsSpeaking returns the current speaking state
func (vad *VoiceActivityDetector) IsSpeaking() bool {
	return vad.isSpeaking
}

// GetNoiseLevel returns the estimated background noise level
func (vad *VoiceActivityDetector) GetNoiseLevel() float64 {
	return vad.backgroundNoiseLevel
}