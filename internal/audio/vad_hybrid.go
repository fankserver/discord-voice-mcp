package audio

import (
	"math"
	"sync"

	"github.com/sirupsen/logrus"
)

// HybridVAD combines energy detection at 48kHz with WebRTC VAD at 16kHz
// This addresses the limitation of WebRTC VAD losing high-frequency information
type HybridVAD struct {
	webrtcVAD *VoiceActivityDetector // Existing WebRTC VAD
	
	// Energy detection at full 48kHz
	energyThreshold    float64
	energyWindowSize   int
	energyHistory      []float64
	historyIndex       int
	
	// High-frequency detection (8-24kHz range)
	highFreqThreshold  float64
	highFreqRatio      float64 // Ratio of high to low frequency energy
	
	// Combined decision making
	energyWeight       float64 // Weight for energy detection (0-1)
	webrtcWeight       float64 // Weight for WebRTC VAD (0-1)
	
	// State
	isSpeaking         bool
	confidence         float64
	consecutiveFrames  int
	
	mu sync.Mutex
}

// HybridVADConfig configuration for hybrid VAD
type HybridVADConfig struct {
	EnergyThreshold   float64 // RMS energy threshold
	HighFreqThreshold float64 // High frequency energy threshold
	EnergyWeight      float64 // Weight for energy detection (0-1)
	WebRTCWeight      float64 // Weight for WebRTC VAD (0-1)
	WindowSize        int     // Energy window size in frames
}

// NewHybridVAD creates a VAD that works better with Discord's 48kHz audio
func NewHybridVAD(config HybridVADConfig) *HybridVAD {
	// Apply defaults
	if config.EnergyThreshold <= 0 {
		config.EnergyThreshold = 0.01 // Normalized RMS threshold
	}
	if config.HighFreqThreshold <= 0 {
		config.HighFreqThreshold = 0.005
	}
	if config.EnergyWeight <= 0 {
		config.EnergyWeight = 0.3
	}
	if config.WebRTCWeight <= 0 {
		config.WebRTCWeight = 0.7
	}
	if config.WindowSize <= 0 {
		config.WindowSize = 10 // 10 frames = 200ms at 20ms/frame
	}
	
	return &HybridVAD{
		webrtcVAD:         NewVoiceActivityDetector(),
		energyThreshold:   config.EnergyThreshold,
		highFreqThreshold: config.HighFreqThreshold,
		energyWeight:      config.EnergyWeight,
		webrtcWeight:      config.WebRTCWeight,
		energyWindowSize:  config.WindowSize,
		energyHistory:     make([]float64, config.WindowSize),
		historyIndex:      0,
	}
}

// ProcessAudio processes audio at full 48kHz quality
func (vad *HybridVAD) ProcessAudio(samples []int16) bool {
	vad.mu.Lock()
	defer vad.mu.Unlock()
	
	if len(samples) == 0 {
		vad.updateState(false, 0)
		return vad.isSpeaking
	}
	
	// 1. Energy detection at full 48kHz (no downsampling)
	energyScore := vad.detectEnergyAt48kHz(samples)
	
	// 2. High-frequency analysis (8-24kHz range)
	// This captures information that WebRTC VAD misses
	highFreqScore := vad.analyzeHighFrequencies(samples)
	
	// 3. WebRTC VAD at 16kHz (existing implementation)
	webrtcScore := 0.0
	if vad.webrtcVAD.ProcessAudio(samples) {
		webrtcScore = 1.0
	}
	
	// 4. Combine scores with weights
	combinedScore := (energyScore * vad.energyWeight) +
	                (highFreqScore * 0.2) + // High freq gets 20% weight
	                (webrtcScore * vad.webrtcWeight)
	
	// 5. Make decision with hysteresis
	vad.updateState(combinedScore > 0.5, combinedScore)
	
	logrus.WithFields(logrus.Fields{
		"energy_score":   energyScore,
		"highfreq_score": highFreqScore,
		"webrtc_score":   webrtcScore,
		"combined_score": combinedScore,
		"is_speaking":    vad.isSpeaking,
	}).Debug("Hybrid VAD analysis")
	
	return vad.isSpeaking
}

// detectEnergyAt48kHz performs energy detection without downsampling
func (vad *HybridVAD) detectEnergyAt48kHz(samples []int16) float64 {
	// Calculate RMS energy
	var sumSquares float64
	for _, sample := range samples {
		normalized := float64(sample) / 32768.0
		sumSquares += normalized * normalized
	}
	rms := math.Sqrt(sumSquares / float64(len(samples)))
	
	// Add to history window
	vad.energyHistory[vad.historyIndex] = rms
	vad.historyIndex = (vad.historyIndex + 1) % vad.energyWindowSize
	
	// Calculate average energy over window
	var avgEnergy float64
	for _, e := range vad.energyHistory {
		avgEnergy += e
	}
	avgEnergy /= float64(vad.energyWindowSize)
	
	// Compute score based on threshold
	if avgEnergy > vad.energyThreshold {
		// Normalize score between 0 and 1
		score := math.Min(1.0, avgEnergy / (vad.energyThreshold * 2))
		return score
	}
	
	return 0.0
}

// analyzeHighFrequencies analyzes 8-24kHz range that WebRTC VAD misses
func (vad *HybridVAD) analyzeHighFrequencies(samples []int16) float64 {
	// Simple high-frequency energy detection
	// In production, this should use FFT for proper frequency analysis
	
	// Calculate energy of high-frequency components using difference method
	// This is a simple approximation - proper implementation would use FFT
	var highFreqEnergy float64
	for i := 1; i < len(samples); i++ {
		diff := float64(samples[i] - samples[i-1])
		highFreqEnergy += diff * diff
	}
	highFreqEnergy = math.Sqrt(highFreqEnergy / float64(len(samples)-1))
	
	// Normalize
	highFreqEnergy /= 32768.0
	
	// Check if high frequency energy exceeds threshold
	if highFreqEnergy > vad.highFreqThreshold {
		score := math.Min(1.0, highFreqEnergy / (vad.highFreqThreshold * 2))
		return score
	}
	
	return 0.0
}

// updateState updates VAD state with hysteresis
func (vad *HybridVAD) updateState(isActive bool, confidence float64) {
	vad.confidence = confidence
	
	if isActive {
		vad.consecutiveFrames++
		if vad.consecutiveFrames >= 3 { // 60ms of speech
			vad.isSpeaking = true
		}
	} else {
		if vad.consecutiveFrames > 0 {
			vad.consecutiveFrames--
		}
		if vad.consecutiveFrames <= -10 { // 200ms of silence
			vad.isSpeaking = false
		}
	}
}

// IsSpeaking returns current speaking state
func (vad *HybridVAD) IsSpeaking() bool {
	vad.mu.Lock()
	defer vad.mu.Unlock()
	return vad.isSpeaking
}

// GetConfidence returns confidence score (0-1)
func (vad *HybridVAD) GetConfidence() float64 {
	vad.mu.Lock()
	defer vad.mu.Unlock()
	return vad.confidence
}

// Reset resets the VAD state
func (vad *HybridVAD) Reset() {
	vad.mu.Lock()
	defer vad.mu.Unlock()
	
	vad.isSpeaking = false
	vad.confidence = 0
	vad.consecutiveFrames = 0
	vad.historyIndex = 0
	for i := range vad.energyHistory {
		vad.energyHistory[i] = 0
	}
	
	if vad.webrtcVAD != nil {
		vad.webrtcVAD.Reset()
	}
}

// Free releases resources
func (vad *HybridVAD) Free() {
	vad.mu.Lock()
	defer vad.mu.Unlock()
	
	if vad.webrtcVAD != nil {
		vad.webrtcVAD.Free()
		vad.webrtcVAD = nil
	}
}

// DetectVoiceActivity for backward compatibility
func (vad *HybridVAD) DetectVoiceActivity(samples []int16) bool {
	return vad.ProcessAudio(samples)
}

// GetNoiseLevel returns average noise level
func (vad *HybridVAD) GetNoiseLevel() float64 {
	vad.mu.Lock()
	defer vad.mu.Unlock()
	
	// Return average of energy history as noise estimate
	var avg float64
	for _, e := range vad.energyHistory {
		avg += e
	}
	return avg / float64(len(vad.energyHistory))
}