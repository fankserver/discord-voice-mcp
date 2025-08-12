package audio

import (
	"math"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

// Priority levels for transcription
type Priority int

const (
	PriorityNormal Priority = iota
	PriorityHigh
	PriorityUrgent
)

func (p Priority) String() string {
	switch p {
	case PriorityHigh:
		return "high"
	case PriorityUrgent:
		return "urgent"
	default:
		return "normal"
	}
}

// TranscribeDecision represents the VAD's decision on whether to transcribe
type TranscribeDecision struct {
	Should   bool
	Priority Priority
	Reason   string
}

// IntelligentVADConfig holds configuration for intelligent VAD
type IntelligentVADConfig struct {
	// Timing thresholds
	MinSpeechDuration  time.Duration // Minimum speech before considering transcription
	MaxSilenceInSpeech time.Duration // Maximum silence allowed mid-speech
	SentenceEndSilence time.Duration // Silence duration to detect sentence end
	MaxSegmentDuration time.Duration // Maximum duration before forced transcription
	TargetDuration     time.Duration // Ideal segment duration

	// Energy thresholds
	EnergyDropRatio float64 // Ratio of energy drop to detect pause (0.4 = 40% drop)
	MinEnergyLevel  float64 // Minimum energy to consider as speech
}

// Helper function to parse environment variable duration in milliseconds
func parseEnvDurationMs(envVar string, defaultMs int) time.Duration {
	if value := os.Getenv(envVar); value != "" {
		if ms, err := strconv.Atoi(value); err == nil {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return time.Duration(defaultMs) * time.Millisecond
}

// Helper function to parse environment variable duration in seconds
func parseEnvDurationSec(envVar string, defaultSec int) time.Duration {
	if value := os.Getenv(envVar); value != "" {
		if s, err := strconv.Atoi(value); err == nil {
			return time.Duration(s) * time.Second
		}
	}
	return time.Duration(defaultSec) * time.Second
}

// Helper function to parse environment variable float
func parseEnvFloat(envVar string, defaultValue float64) float64 {
	if value := os.Getenv(envVar); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

// NewIntelligentVADConfig returns ultra-responsive configuration optimized for Discord multi-speaker
func NewIntelligentVADConfig() IntelligentVADConfig {
	// Default to ultra-responsive settings optimized for Discord multi-speaker conversations
	// These settings work well for both single and multi-speaker scenarios
	return IntelligentVADConfig{
		MinSpeechDuration:  parseEnvDurationMs("VAD_MIN_SPEECH_MS", 300),            // 0.3s min speech for quick response
		MaxSilenceInSpeech: parseEnvDurationMs("VAD_MAX_SILENCE_IN_SPEECH_MS", 200), // 0.2s max pause for tight detection
		SentenceEndSilence: parseEnvDurationMs("VAD_SENTENCE_END_SILENCE_MS", 400),  // 0.4s silence for sentence boundaries
		MaxSegmentDuration: parseEnvDurationSec("VAD_MAX_SEGMENT_DURATION_S", 3),    // 3s max to prevent long waits
		TargetDuration:     parseEnvDurationMs("VAD_TARGET_DURATION_MS", 1500),      // 1.5s target for rapid exchanges
		EnergyDropRatio:    parseEnvFloat("VAD_ENERGY_DROP_RATIO", 0.20),            // 20% drop for sensitive detection
		MinEnergyLevel:     parseEnvFloat("VAD_MIN_ENERGY_LEVEL", 70.0),             // Lower threshold for Discord voice
	}
}

// IntelligentVAD provides smart voice activity detection with natural pause detection
type IntelligentVAD struct {
	config IntelligentVADConfig

	// Energy tracking
	energyHistory   []float64
	maxHistorySize  int
	lastEnergyLevel float64
	avgEnergyLevel  float64

	// State tracking
	consecutiveSilenceFrames int
	consecutiveSpeechFrames  int
	inSpeechSegment          bool
	segmentStartTime         time.Time
}

// NewIntelligentVAD creates a new intelligent VAD
func NewIntelligentVAD(config IntelligentVADConfig) *IntelligentVAD {
	return &IntelligentVAD{
		config:         config,
		energyHistory:  make([]float64, 0, 100),
		maxHistorySize: 100,
	}
}

// ShouldTranscribe determines if the buffer should be transcribed
func (v *IntelligentVAD) ShouldTranscribe(buffer *AudioBuffer) TranscribeDecision {
	duration := buffer.Duration()
	silenceDuration := buffer.SilenceDuration()

	// Priority 1: Maximum duration reached
	if duration >= v.config.MaxSegmentDuration {
		return TranscribeDecision{
			Should:   true,
			Priority: PriorityUrgent,
			Reason:   "Maximum segment duration reached",
		}
	}

	// Priority 2: Natural sentence ending detected
	if v.detectSentenceEnd(buffer, silenceDuration) {
		return TranscribeDecision{
			Should:   true,
			Priority: PriorityHigh,
			Reason:   "Natural pause detected (sentence end)",
		}
	}

	// Priority 3: Target duration reached with silence
	if duration >= v.config.TargetDuration && silenceDuration > v.config.MaxSilenceInSpeech {
		return TranscribeDecision{
			Should:   true,
			Priority: PriorityNormal,
			Reason:   "Target duration reached with pause",
		}
	}

	// Priority 4: Long silence after speech
	if duration >= v.config.MinSpeechDuration && silenceDuration > v.config.SentenceEndSilence*2 {
		return TranscribeDecision{
			Should:   true,
			Priority: PriorityHigh,
			Reason:   "Extended silence detected",
		}
	}

	// Don't transcribe yet
	return TranscribeDecision{
		Should: false,
		Reason: "Continuing to buffer",
	}
}

// detectSentenceEnd checks for natural sentence boundaries
func (v *IntelligentVAD) detectSentenceEnd(buffer *AudioBuffer, silenceDuration time.Duration) bool {
	// Need minimum speech duration
	if buffer.Duration() < v.config.MinSpeechDuration {
		return false
	}

	// Check for sentence-ending pause
	if silenceDuration >= v.config.SentenceEndSilence {
		// Additional heuristics could go here:
		// - Check if energy dropped significantly
		// - Look for pitch patterns indicating sentence end
		// - Consider duration patterns

		logrus.WithFields(logrus.Fields{
			"silence_duration": silenceDuration,
			"buffer_duration":  buffer.Duration(),
		}).Debug("Sentence end detected")

		return true
	}

	return false
}

// ProcessAudioFrame processes a single frame of audio for VAD
func (v *IntelligentVAD) ProcessAudioFrame(pcm []int16) bool {
	// Calculate frame energy
	energy := v.calculateEnergy(pcm)

	// Update energy history
	v.updateEnergyHistory(energy)

	// Determine if this frame is speech
	isSpeech := v.isFrameSpeech(energy)

	// Update consecutive counters
	if isSpeech {
		v.consecutiveSpeechFrames++
		v.consecutiveSilenceFrames = 0
	} else {
		v.consecutiveSilenceFrames++
		v.consecutiveSpeechFrames = 0
	}

	// Update segment state
	if !v.inSpeechSegment && v.consecutiveSpeechFrames >= 3 {
		v.inSpeechSegment = true
		v.segmentStartTime = time.Now()
		logrus.Debug("Speech segment started")
	} else if v.inSpeechSegment && v.consecutiveSilenceFrames >= 15 {
		v.inSpeechSegment = false
		logrus.Debug("Speech segment ended")
	}

	return isSpeech
}

// calculateEnergy calculates the energy of an audio frame
func (v *IntelligentVAD) calculateEnergy(pcm []int16) float64 {
	if len(pcm) == 0 {
		return 0
	}

	var sum float64
	for _, sample := range pcm {
		val := float64(sample)
		sum += val * val
	}

	// RMS energy
	energy := math.Sqrt(sum / float64(len(pcm)))

	return energy
}

// updateEnergyHistory maintains a rolling window of energy levels
func (v *IntelligentVAD) updateEnergyHistory(energy float64) {
	v.energyHistory = append(v.energyHistory, energy)

	// Maintain max history size
	if len(v.energyHistory) > v.maxHistorySize {
		v.energyHistory = v.energyHistory[1:]
	}

	// Update average energy
	if len(v.energyHistory) > 0 {
		var sum float64
		for _, e := range v.energyHistory {
			sum += e
		}
		v.avgEnergyLevel = sum / float64(len(v.energyHistory))
	}

	v.lastEnergyLevel = energy
}

// isFrameSpeech determines if a frame contains speech based on energy
func (v *IntelligentVAD) isFrameSpeech(energy float64) bool {
	// Absolute threshold
	if energy < v.config.MinEnergyLevel {
		return false
	}

	// Dynamic threshold based on average
	if v.avgEnergyLevel > 0 {
		// Speech should be significantly above average noise floor
		threshold := v.avgEnergyLevel * 1.5
		return energy > threshold
	}

	// Fallback to simple threshold
	return energy > v.config.MinEnergyLevel*2
}

// DetectEnergyDrop checks if there's been a significant energy drop
func (v *IntelligentVAD) DetectEnergyDrop() bool {
	if len(v.energyHistory) < 10 {
		return false
	}

	// Compare recent energy to previous energy
	recentStart := len(v.energyHistory) - 5
	recentEnergy := v.calculateAverage(v.energyHistory[recentStart:])

	previousEnd := recentStart
	if previousEnd < 5 {
		return false
	}
	previousEnergy := v.calculateAverage(v.energyHistory[previousEnd-5 : previousEnd])

	// Check for significant drop
	if previousEnergy > 0 {
		dropRatio := (previousEnergy - recentEnergy) / previousEnergy
		return dropRatio > v.config.EnergyDropRatio
	}

	return false
}

// calculateAverage calculates the average of a slice of values
func (v *IntelligentVAD) calculateAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// Reset resets the VAD state
func (v *IntelligentVAD) Reset() {
	v.energyHistory = v.energyHistory[:0]
	v.lastEnergyLevel = 0
	v.avgEnergyLevel = 0
	v.consecutiveSilenceFrames = 0
	v.consecutiveSpeechFrames = 0
	v.inSpeechSegment = false
	v.segmentStartTime = time.Time{}
}

// GetState returns the current VAD state for debugging
func (v *IntelligentVAD) GetState() VADState {
	return VADState{
		InSpeechSegment:          v.inSpeechSegment,
		ConsecutiveSpeechFrames:  v.consecutiveSpeechFrames,
		ConsecutiveSilenceFrames: v.consecutiveSilenceFrames,
		AverageEnergyLevel:       v.avgEnergyLevel,
		LastEnergyLevel:          v.lastEnergyLevel,
		SegmentDuration:          time.Since(v.segmentStartTime),
	}
}

// VADState represents the current state of the VAD
type VADState struct {
	InSpeechSegment          bool
	ConsecutiveSpeechFrames  int
	ConsecutiveSilenceFrames int
	AverageEnergyLevel       float64
	LastEnergyLevel          float64
	SegmentDuration          time.Duration
}
