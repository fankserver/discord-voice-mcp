package transcriber

import (
	"time"
)

// Transcriber is the unified interface for all transcription backends
type Transcriber interface {
	// Basic transcription without context
	Transcribe(audio []byte) (string, error)

	// Transcription with context for better accuracy
	TranscribeWithContext(audio []byte, opts TranscriptionOptions) (*TranscriptResult, error)

	// Check if the transcriber is ready to process
	IsReady() bool

	// Close releases resources
	Close() error
}

// TranscriptionOptions provides enhanced options for transcription
type TranscriptionOptions struct {
	// Previous transcript for context (improves accuracy)
	PreviousContext string

	// Language hint (e.g., "en", "es", "auto")
	Language string

	// Maximum number of alternative transcriptions
	MaxAlternatives int

	// Enable timestamp generation for words
	EnableTimestamps bool

	// Custom vocabulary or phrases for better recognition
	CustomVocabulary []string

	// Temperature for sampling (Whisper-specific, 0.0-1.0)
	Temperature float32

	// Overlap audio from previous chunk (prevents word cutoff)
	OverlapAudio []byte
}

// TranscriptResult contains the transcription result with metadata
type TranscriptResult struct {
	// Primary transcription text
	Text string

	// Confidence score (0.0-1.0)
	Confidence float32

	// Detected or specified language
	Language string

	// Processing duration
	Duration time.Duration

	// Alternative transcriptions if requested
	Alternatives []Alternative

	// Word-level timestamps if enabled
	Words []WordTiming
}

// Alternative represents an alternative transcription
type Alternative struct {
	Text       string
	Confidence float32
}

// WordTiming represents timing information for a word
type WordTiming struct {
	Word       string
	StartTime  time.Duration
	EndTime    time.Duration
	Confidence float32
}

// TranscriberConfig holds common configuration for transcribers
type TranscriberConfig struct {
	// Model path or identifier
	ModelPath string

	// Default language
	DefaultLanguage string

	// Enable GPU acceleration if available
	UseGPU bool

	// Number of threads for CPU processing
	NumThreads int

	// Batch size for processing
	BatchSize int

	// Maximum segment length in seconds
	MaxSegmentLength int
}

// Word represents a word with timing information (legacy compatibility)
type Word struct {
	Word  string
	Start time.Duration
	End   time.Duration
}

// TranscribeWithContextHelper is a helper function that provides context-aware transcription
// This is the main function that should be used by the new async pipeline
func TranscribeWithContextHelper(t Transcriber, audio []byte, opts TranscriptionOptions) (string, error) {
	result, err := t.TranscribeWithContext(audio, opts)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}
