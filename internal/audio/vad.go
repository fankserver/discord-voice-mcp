package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"

	webrtcvad "github.com/baabaaox/go-webrtcvad"
	"github.com/sirupsen/logrus"
)

const (
	// Maximum buffer sizes to prevent unbounded growth
	maxMonoBufferSize       = 48000  // 1 second at 48kHz mono
	maxDownsampleBufferSize = 16000  // 1 second at 16kHz
	maxFrameBufferSize      = 32000  // 1 second worth of bytes
	
	// WebRTC VAD frame sizes at 16kHz
	frameSize10ms = 160  // 10ms at 16kHz
	frameSize20ms = 320  // 20ms at 16kHz
	frameSize30ms = 480  // 30ms at 16kHz
	
	// Audio parameters
	discordSampleRate = 48000
	vadSampleRate     = 16000
	downsampleRatio   = 3  // 48kHz / 16kHz
)

// Buffer pools for efficient memory reuse
var (
	// Pool for temporary mono conversion buffers
	tempMonoPool = sync.Pool{
		New: func() interface{} {
			return &[]int16{}
		},
	}
	
	// Pool for temporary downsample buffers
	tempDownsamplePool = sync.Pool{
		New: func() interface{} {
			return &[]int16{}
		},
	}
	
	// Pool for temporary byte buffers
	tempBytePool = sync.Pool{
		New: func() interface{} {
			return &[]byte{}
		},
	}
)

// getBuffer gets a buffer from pool and ensures it has the required capacity
func getBuffer[T any](pool *sync.Pool, size int) *[]T {
	buf := pool.Get().(*[]T)
	if cap(*buf) < size {
		*buf = make([]T, size)
	} else {
		*buf = (*buf)[:size]
	}
	return buf
}

// VoiceActivityDetector uses Google's WebRTC VAD implementation
type VoiceActivityDetector struct {
	vad                   webrtcvad.VadInst
	mode                  int // 0-3, higher is more aggressive
	speechFramesRequired  int
	silenceFramesRequired int
	speechCount           int
	silenceCount          int
	isSpeaking            bool
	
	// Frame buffer for incomplete frames
	frameBuffer      []int16 // Buffer for incomplete frames at 16kHz
	frameBufferPos   int     // Current position in frame buffer
	
	// Anti-aliasing filter (improved design)
	filterCoeffs []float64
	filterDelay  []float64 // Filter delay line for continuous processing
	
	mu sync.Mutex // Protect concurrent access
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
		speechFramesRequired:  config.SpeechFramesRequired,
		silenceFramesRequired: config.SilenceFramesRequired,
		isSpeaking:            false,
		frameBuffer:           make([]int16, 0, frameSize30ms),
		frameBufferPos:        0,
		// Use improved 64-tap filter for better anti-aliasing
		filterCoeffs:          generateImprovedLowPassCoeffs(8000, discordSampleRate, 64),
	}
	
	// Initialize filter delay line
	vad.filterDelay = make([]float64, len(vad.filterCoeffs))
	
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
		"mode":            vad.mode,
		"speech_frames":   vad.speechFramesRequired,
		"silence_frames":  vad.silenceFramesRequired,
		"filter_taps":     len(vad.filterCoeffs),
	}).Info("WebRTC VAD initialized with improved design")
	
	return vad
}

// ProcessAudio processes audio samples and returns voice activity detection result
// This replaces DetectVoiceActivity with a cleaner API
func (vad *VoiceActivityDetector) ProcessAudio(samples []int16) bool {
	vad.mu.Lock()
	defer vad.mu.Unlock()
	
	// Handle empty input as silence (cleaner than nil check)
	if len(samples) == 0 {
		vad.updateState(false)
		return vad.isSpeaking
	}
	
	// Prevent unbounded growth - reject oversized buffers
	if len(samples) > maxMonoBufferSize*2 { // stereo limit
		logrus.Warn("Audio buffer too large, truncating to prevent memory issues")
		samples = samples[:maxMonoBufferSize*2]
	}
	
	// Get temporary buffers from pools
	monoLength := len(samples) / 2
	monoBuf := getBuffer[int16](&tempMonoPool, monoLength)
	defer tempMonoPool.Put(monoBuf)
	
	// Convert stereo to mono
	vad.stereoToMono(samples, *monoBuf)
	
	// Downsample with anti-aliasing filter
	downsampledLength := monoLength / downsampleRatio
	downsampledBuf := getBuffer[int16](&tempDownsamplePool, downsampledLength)
	defer tempDownsamplePool.Put(downsampledBuf)
	
	vad.downsampleWithImprovedFilter(*monoBuf, *downsampledBuf)
	
	// Process all audio, not just first 20ms
	return vad.processDownsampledAudio(*downsampledBuf)
}

// processDownsampledAudio processes the entire downsampled buffer
func (vad *VoiceActivityDetector) processDownsampledAudio(samples []int16) bool {
	// Add samples to frame buffer
	vad.frameBuffer = append(vad.frameBuffer, samples...)
	
	// Prevent unbounded growth of frame buffer
	if len(vad.frameBuffer) > maxDownsampleBufferSize {
		logrus.Warn("Frame buffer overflow, discarding old frames")
		// Keep only the most recent data
		vad.frameBuffer = vad.frameBuffer[len(vad.frameBuffer)-maxDownsampleBufferSize:]
	}
	
	// Process complete frames (20ms at 16kHz = 320 samples)
	for len(vad.frameBuffer) >= frameSize20ms {
		// Get frame data
		frameData := vad.frameBuffer[:frameSize20ms]
		
		// Get temporary byte buffer from pool
		byteBuf := getBuffer[byte](&tempBytePool, frameSize20ms*2)
		defer tempBytePool.Put(byteBuf)
		
		// Convert to bytes for WebRTC VAD
		for i, sample := range frameData {
			binary.LittleEndian.PutUint16((*byteBuf)[i*2:], uint16(sample))
		}
		
		// Process with WebRTC VAD
		isVoice, err := webrtcvad.Process(vad.vad, vadSampleRate, *byteBuf, frameSize20ms)
		if err != nil {
			logrus.WithError(err).Debug("WebRTC VAD process error")
			isVoice = false
		}
		
		// Update state
		vad.updateState(isVoice)
		
		// Remove processed frame from buffer
		vad.frameBuffer = vad.frameBuffer[frameSize20ms:]
	}
	
	return vad.isSpeaking
}

// stereoToMono converts stereo samples to mono by averaging channels
func (vad *VoiceActivityDetector) stereoToMono(stereo []int16, mono []int16) {
	for i := 0; i < len(mono); i++ {
		left := int32(stereo[i*2])
		right := int32(stereo[i*2+1])
		mono[i] = int16((left + right) / 2)
	}
}

// downsampleWithImprovedFilter applies improved anti-aliasing and downsamples
func (vad *VoiceActivityDetector) downsampleWithImprovedFilter(input []int16, output []int16) {
	filterLen := len(vad.filterCoeffs)
	
	// Process with proper convolution
	for i := 0; i < len(output); i++ {
		srcIdx := i * downsampleRatio
		
		// Convolution with filter
		var sum float64
		for j := 0; j < filterLen && srcIdx+j < len(input); j++ {
			sum += float64(input[srcIdx+j]) * vad.filterCoeffs[j]
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

// Reset resets the VAD state and frame buffer
func (vad *VoiceActivityDetector) Reset() {
	vad.mu.Lock()
	defer vad.mu.Unlock()
	
	vad.speechCount = 0
	vad.silenceCount = 0
	vad.isSpeaking = false
	vad.frameBuffer = vad.frameBuffer[:0] // Reset but keep capacity
	
	// Clear filter delay line
	for i := range vad.filterDelay {
		vad.filterDelay[i] = 0
	}
}

// IsSpeaking returns the current speaking state (thread-safe)
func (vad *VoiceActivityDetector) IsSpeaking() bool {
	vad.mu.Lock()
	defer vad.mu.Unlock()
	return vad.isSpeaking
}

// SetMode sets the WebRTC VAD aggressiveness mode (0-3)
func (vad *VoiceActivityDetector) SetMode(mode int) error {
	if mode < 0 || mode > 3 {
		return fmt.Errorf("invalid mode %d: must be 0-3", mode)
	}
	
	vad.mu.Lock()
	defer vad.mu.Unlock()
	
	vad.mode = mode
	return webrtcvad.SetMode(vad.vad, mode)
}

// Free releases WebRTC VAD resources
func (vad *VoiceActivityDetector) Free() {
	vad.mu.Lock()
	defer vad.mu.Unlock()
	
	if vad.vad != nil {
		webrtcvad.Free(vad.vad)
		vad.vad = nil
	}
}

// generateImprovedLowPassCoeffs generates better filter coefficients
func generateImprovedLowPassCoeffs(cutoffFreq, sampleRate float64, numTaps int) []float64 {
	if numTaps%2 == 0 {
		numTaps++ // Ensure odd number of taps
	}
	
	coeffs := make([]float64, numTaps)
	
	// Normalized cutoff frequency
	wc := 2.0 * math.Pi * cutoffFreq / sampleRate
	halfLen := numTaps / 2
	
	// Generate windowed-sinc filter with Kaiser window for better stopband
	beta := 8.0 // Kaiser window parameter (higher = better stopband, wider transition)
	
	for i := 0; i < numTaps; i++ {
		n := i - halfLen
		
		// Sinc function
		if n == 0 {
			coeffs[i] = wc / math.Pi
		} else {
			coeffs[i] = math.Sin(wc*float64(n)) / (math.Pi * float64(n))
		}
		
		// Apply Kaiser window
		coeffs[i] *= kaiserWindow(i, numTaps, beta)
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

// kaiserWindow computes Kaiser window coefficient
func kaiserWindow(n, N int, beta float64) float64 {
	alpha := float64(N-1) / 2.0
	num := modifiedBesselI0(beta * math.Sqrt(1.0-math.Pow((float64(n)-alpha)/alpha, 2)))
	den := modifiedBesselI0(beta)
	return num / den
}

// modifiedBesselI0 computes modified Bessel function of first kind, order 0
func modifiedBesselI0(x float64) float64 {
	sum := 1.0
	term := 1.0
	
	for k := 1; k < 50; k++ {
		term *= (x / (2.0 * float64(k))) * (x / (2.0 * float64(k)))
		sum += term
		if term < 1e-10*sum {
			break
		}
	}
	
	return sum
}

// Backward compatibility methods

// DetectVoiceActivity is deprecated, use ProcessAudio instead
// Kept for backward compatibility with existing code
func (vad *VoiceActivityDetector) DetectVoiceActivity(samples []int16) bool {
	return vad.ProcessAudio(samples)
}

// GetNoiseLevel returns 0 as WebRTC VAD doesn't provide noise level estimation
func (vad *VoiceActivityDetector) GetNoiseLevel() float64 {
	return 0.0 // WebRTC VAD doesn't expose noise level
}

// Deprecated internal methods for backward compatibility

// convertToMonoInPlace is deprecated, use stereoToMono
func (vad *VoiceActivityDetector) convertToMonoInPlace(stereo []int16, mono []int16) {
	vad.stereoToMono(stereo, mono)
}

// downsampleWithFilter is deprecated, use downsampleWithImprovedFilter
func (vad *VoiceActivityDetector) downsampleWithFilter(input []int16, output []int16) {
	vad.downsampleWithImprovedFilter(input, output)
}

// generateLowPassCoeffs is deprecated, use generateImprovedLowPassCoeffs
func generateLowPassCoeffs(cutoffFreq, sampleRate float64) []float64 {
	return generateImprovedLowPassCoeffs(cutoffFreq, sampleRate, 64)
}