package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"

	webrtcvad "github.com/baabaaox/go-webrtcvad"
	"github.com/sirupsen/logrus"
)

// Buffer pools to reduce allocations
var (
	monoBufferPool = sync.Pool{
		New: func() interface{} {
			return make([]int16, 480) // 960 stereo samples / 2
		},
	}
	downsampleBufferPool = sync.Pool{
		New: func() interface{} {
			return make([]int16, 320) // 960 / 3 for 48kHz -> 16kHz
		},
	}
	byteBufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 640) // 320 samples * 2 bytes
		},
	}
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
	
	// Reusable buffers to avoid allocations
	monoBuffer       []int16 // Buffer for mono conversion
	downsampleBuffer []int16 // Buffer for downsampled audio
	frameBytes       []byte  // Buffer for WebRTC VAD input
	
	// Low-pass filter coefficients for anti-aliasing
	filterCoeffs []float64
}

// VADConfig holds configuration for Voice Activity Detector
type VADConfig struct {
	Mode                  int // VAD aggressiveness mode (0-3)
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
	if config.Mode < 0 || config.Mode > 3 {
		config.Mode = 2 // Default to moderate aggressiveness
	}
	if config.SpeechFramesRequired <= 0 {
		config.SpeechFramesRequired = 3
	}
	if config.SilenceFramesRequired <= 0 {
		config.SilenceFramesRequired = 15
	}
	
	vad := &VoiceActivityDetector{
		vad:                   webrtcvad.Create(),
		mode:                  config.Mode,
		frameSize:             320, // 20ms at 16kHz
		sampleRate:            16000, // WebRTC VAD works best at 16kHz
		speechFramesRequired:  config.SpeechFramesRequired,
		silenceFramesRequired: config.SilenceFramesRequired,
		isSpeaking:            false,
		
		// Pre-allocate buffers
		monoBuffer:       make([]int16, 480),  // 960 stereo / 2
		downsampleBuffer: make([]int16, 320),  // 320 samples at 16kHz
		frameBytes:       make([]byte, 640),   // 320 * 2 bytes
		
		// Initialize filter coefficients for Butterworth low-pass at 8kHz
		filterCoeffs: generateLowPassCoeffs(8000, 48000),
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
// Now accepts int16 samples directly to avoid redundant conversions
func (vad *VoiceActivityDetector) DetectVoiceActivity(samples []int16) bool {
	// Handle nil or empty data as silence
	if samples == nil || len(samples) == 0 {
		vad.updateState(false)
		return vad.isSpeaking
	}
	
	// Discord provides 48kHz stereo, but WebRTC VAD needs 16kHz mono
	// Step 1: Convert stereo to mono (reuse buffer)
	monoLength := len(samples) / 2
	if monoLength > len(vad.monoBuffer) {
		vad.monoBuffer = make([]int16, monoLength)
	}
	vad.convertToMonoInPlace(samples, vad.monoBuffer[:monoLength])
	
	// Step 2: Apply anti-aliasing filter and downsample to 16kHz
	// This prevents aliasing that destroys high-frequency content
	downsampledLength := monoLength / 3
	if downsampledLength > len(vad.downsampleBuffer) {
		vad.downsampleBuffer = make([]int16, downsampledLength)
	}
	vad.downsampleWithFilter(vad.monoBuffer[:monoLength], vad.downsampleBuffer[:downsampledLength])
	
	// WebRTC VAD requires exactly 10, 20, or 30ms of audio at 16kHz
	// 20ms at 16kHz = 320 samples
	if downsampledLength < vad.frameSize {
		// Not enough samples for a full frame
		vad.updateState(false)
		return vad.isSpeaking
	}
	
	// WebRTC VAD expects []byte in little-endian format (reuse buffer)
	frameData := vad.downsampleBuffer[:vad.frameSize]
	for i, sample := range frameData {
		binary.LittleEndian.PutUint16(vad.frameBytes[i*2:], uint16(sample))
	}
	
	// Process with WebRTC VAD
	isVoice, err := webrtcvad.Process(vad.vad, vad.sampleRate, vad.frameBytes[:vad.frameSize*2], vad.frameSize)
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

// convertToMonoInPlace converts stereo samples to mono by averaging channels
// Uses pre-allocated buffer to avoid allocations
func (vad *VoiceActivityDetector) convertToMonoInPlace(stereoSamples []int16, output []int16) {
	for i := 0; i < len(output); i++ {
		left := int32(stereoSamples[i*2])
		right := int32(stereoSamples[i*2+1])
		// Use int32 to avoid overflow when averaging
		output[i] = int16((left + right) / 2)
	}
}

// downsampleWithFilter applies anti-aliasing filter and downsamples 48kHz to 16kHz
// This prevents aliasing that would destroy frequencies above 8kHz
func (vad *VoiceActivityDetector) downsampleWithFilter(input []int16, output []int16) {
	// Apply 4th order Butterworth low-pass filter at 8kHz before downsampling
	// This is a simple but effective anti-aliasing filter
	
	filterLen := len(vad.filterCoeffs)
	halfFilter := filterLen / 2
	
	for i := 0; i < len(output); i++ {
		srcIdx := i * 3 // 3:1 downsampling ratio
		
		// Apply filter centered at current sample
		var sum float64
		for j := 0; j < filterLen; j++ {
			sampleIdx := srcIdx + j - halfFilter
			if sampleIdx >= 0 && sampleIdx < len(input) {
				sum += float64(input[sampleIdx]) * vad.filterCoeffs[j]
			}
		}
		
		// Clamp to int16 range
		if sum > 32767 {
			output[i] = 32767
		} else if sum < -32768 {
			output[i] = -32768
		} else {
			output[i] = int16(sum)
		}
	}
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

// generateLowPassCoeffs generates filter coefficients for a low-pass filter
// Uses a windowed-sinc approach for anti-aliasing
func generateLowPassCoeffs(cutoffFreq, sampleRate float64) []float64 {
	// Use a reasonable filter length (must be odd)
	filterLen := 21
	coeffs := make([]float64, filterLen)
	
	// Normalized cutoff frequency
	wc := 2.0 * math.Pi * cutoffFreq / sampleRate
	halfLen := filterLen / 2
	
	// Generate sinc function with Hamming window
	for i := 0; i < filterLen; i++ {
		n := i - halfLen
		if n == 0 {
			coeffs[i] = wc / math.Pi
		} else {
			// Sinc function
			coeffs[i] = math.Sin(wc*float64(n)) / (math.Pi * float64(n))
		}
		
		// Apply Hamming window to reduce ringing
		window := 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(filterLen-1))
		coeffs[i] *= window
	}
	
	// Normalize coefficients
	sum := 0.0
	for _, c := range coeffs {
		sum += c
	}
	for i := range coeffs {
		coeffs[i] /= sum
	}
	
	return coeffs
}