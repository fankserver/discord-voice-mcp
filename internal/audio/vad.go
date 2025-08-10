package audio

import (
	"encoding/binary"
	"fmt"

	webrtcvad "github.com/baabaaox/go-webrtcvad"
	"github.com/sirupsen/logrus"
)

// VoiceActivityDetector uses Google's WebRTC VAD implementation
type VoiceActivityDetector struct {
	vad                  webrtcvad.VadInst
	mode                 int // 0-3, higher is more aggressive
	frameSize            int // Samples per frame (must be 160, 320, or 480 for 16kHz)
	sampleRate           int // Must be 8000, 16000, 32000, or 48000
	speechFramesRequired int
	silenceFramesRequired int
	speechCount          int
	silenceCount         int
	isSpeaking           bool
	
	// Resampling buffer for 48kHz -> 16kHz conversion
	resampleBuffer []int16
}

// VADConfig holds configuration for Voice Activity Detector
type VADConfig struct {
	EnergyThreshold       float64 // Not used by WebRTC VAD
	SpeechFramesRequired  int
	SilenceFramesRequired int
}

// NewVoiceActivityDetector creates a new WebRTC-based VAD instance
func NewVoiceActivityDetector() *VoiceActivityDetector {
	return NewVoiceActivityDetectorWithConfig(VADConfig{})
}

// NewVoiceActivityDetectorWithConfig creates a new WebRTC VAD with custom configuration
func NewVoiceActivityDetectorWithConfig(config VADConfig) *VoiceActivityDetector {
	// Apply defaults if not specified
	if config.SpeechFramesRequired <= 0 {
		config.SpeechFramesRequired = 3
	}
	if config.SilenceFramesRequired <= 0 {
		config.SilenceFramesRequired = 15
	}
	
	vad := &VoiceActivityDetector{
		vad:                   webrtcvad.Create(),
		mode:                  2, // Moderate aggressiveness (0-3)
		frameSize:             320, // 20ms at 16kHz
		sampleRate:            16000, // WebRTC VAD works best at 16kHz
		speechFramesRequired:  config.SpeechFramesRequired,
		silenceFramesRequired: config.SilenceFramesRequired,
		isSpeaking:            false,
		resampleBuffer:        make([]int16, 0, 960), // Pre-allocate for resampling
	}
	
	// Initialize WebRTC VAD
	if err := webrtcvad.Init(vad.vad); err != nil {
		logrus.WithError(err).Error("Failed to initialize WebRTC VAD")
		return nil
	}
	
	// Set mode (0-3, higher is more aggressive in filtering out non-speech)
	if err := webrtcvad.SetMode(vad.vad, vad.mode); err != nil {
		logrus.WithError(err).Error("Failed to set WebRTC VAD mode")
		return nil
	}
	
	logrus.WithFields(logrus.Fields{
		"mode":                 vad.mode,
		"sample_rate":          vad.sampleRate,
		"frame_size":           vad.frameSize,
		"speech_frames":        vad.speechFramesRequired,
		"silence_frames":       vad.silenceFramesRequired,
	}).Info("WebRTC VAD initialized")
	
	return vad
}

// DetectVoiceActivity analyzes PCM audio samples to detect voice
func (vad *VoiceActivityDetector) DetectVoiceActivity(pcmData []byte) bool {
	// Handle nil or empty data as silence
	if pcmData == nil || len(pcmData) == 0 {
		vad.updateState(false)
		return vad.isSpeaking
	}
	
	// Convert byte array to int16 samples
	samples := bytesToInt16(pcmData)
	if len(samples) == 0 {
		vad.updateState(false)
		return vad.isSpeaking
	}
	
	// Discord provides 48kHz stereo, but WebRTC VAD needs 16kHz mono
	// Downsample from 48kHz stereo to 16kHz mono
	monoSamples := vad.convertToMono(samples)
	downsampledSamples := vad.downsample48to16(monoSamples)
	
	// WebRTC VAD requires exactly 10, 20, or 30ms of audio at 16kHz
	// 20ms at 16kHz = 320 samples
	if len(downsampledSamples) < vad.frameSize {
		// Not enough samples for a full frame
		vad.updateState(false)
		return vad.isSpeaking
	}
	
	// Process the frame with WebRTC VAD
	// Take only the required frame size
	frameData := downsampledSamples[:vad.frameSize]
	
	// WebRTC VAD expects []byte in little-endian format
	frameBytes := make([]byte, len(frameData)*2)
	for i, sample := range frameData {
		binary.LittleEndian.PutUint16(frameBytes[i*2:], uint16(sample))
	}
	
	// Process with WebRTC VAD
	isVoice, err := webrtcvad.Process(vad.vad, vad.sampleRate, frameBytes, len(frameData))
	if err != nil {
		logrus.WithError(err).Debug("WebRTC VAD process error")
		vad.updateState(false)
		return vad.isSpeaking
	}
	
	// Update state with hysteresis
	vad.updateState(isVoice)
	
	logrus.WithFields(logrus.Fields{
		"is_voice":      isVoice,
		"is_speaking":   vad.isSpeaking,
		"speech_count":  vad.speechCount,
		"silence_count": vad.silenceCount,
	}).Debug("WebRTC VAD analysis")
	
	return vad.isSpeaking
}

// convertToMono converts stereo samples to mono by averaging channels
func (vad *VoiceActivityDetector) convertToMono(stereoSamples []int16) []int16 {
	// Assuming stereo input (2 channels)
	monoSamples := make([]int16, len(stereoSamples)/2)
	for i := 0; i < len(monoSamples); i++ {
		left := stereoSamples[i*2]
		right := stereoSamples[i*2+1]
		// Average the two channels
		monoSamples[i] = (left + right) / 2
	}
	return monoSamples
}

// downsample48to16 downsamples from 48kHz to 16kHz (3:1 ratio)
func (vad *VoiceActivityDetector) downsample48to16(samples48k []int16) []int16 {
	// Simple downsampling by taking every 3rd sample
	// For better quality, consider using a low-pass filter before downsampling
	downsampled := make([]int16, len(samples48k)/3)
	for i := 0; i < len(downsampled); i++ {
		downsampled[i] = samples48k[i*3]
	}
	return downsampled
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

// Reset resets the VAD state
func (vad *VoiceActivityDetector) Reset() {
	vad.speechCount = 0
	vad.silenceCount = 0
	vad.isSpeaking = false
}

// IsSpeaking returns the current speaking state
func (vad *VoiceActivityDetector) IsSpeaking() bool {
	return vad.isSpeaking
}

// GetNoiseLevel returns 0 as WebRTC VAD doesn't provide noise level estimation
func (vad *VoiceActivityDetector) GetNoiseLevel() float64 {
	return 0.0 // WebRTC VAD doesn't expose noise level
}

// SetMode sets the WebRTC VAD aggressiveness mode (0-3)
func (vad *VoiceActivityDetector) SetMode(mode int) error {
	if mode < 0 || mode > 3 {
		return fmt.Errorf("invalid mode %d: must be 0-3", mode)
	}
	vad.mode = mode
	return webrtcvad.SetMode(vad.vad, mode)
}

// Free releases WebRTC VAD resources
func (vad *VoiceActivityDetector) Free() {
	if vad.vad != nil {
		webrtcvad.Free(vad.vad)
		vad.vad = nil
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