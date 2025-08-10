package transcriber

import "strings"

const (
	// ContextWordCount is the number of words to use as context from previous transcripts
	ContextWordCount = 30
)

// TranscribeOptions provides additional context for transcription
type TranscribeOptions struct {
	// PreviousTranscript provides context from the last transcription
	// This helps maintain continuity across chunk boundaries
	PreviousTranscript string
	
	// OverlapAudio contains the last ~1 second of the previous chunk
	// This prevents words from being cut off mid-syllable at boundaries
	OverlapAudio []byte
	
	// Language hint for better accuracy (e.g., "de" for German)
	Language string
}

// ContextAwareTranscriber extends the basic Transcriber with context support
type ContextAwareTranscriber interface {
	Transcriber
	// TranscribeWithContext performs transcription with additional context
	TranscribeWithContext(audio []byte, opts TranscribeOptions) (string, error)
}

// TranscribeWithContext attempts to use context-aware transcription if available,
// falling back to basic transcription if not supported
func TranscribeWithContext(t Transcriber, audio []byte, opts TranscribeOptions) (string, error) {
	if cat, ok := t.(ContextAwareTranscriber); ok {
		return cat.TranscribeWithContext(audio, opts)
	}
	// Fallback to basic transcription without context
	return t.Transcribe(audio)
}

// CreateContextPrompt creates a prompt from the previous transcript for whisper
// It takes the last N words (ContextWordCount) to stay within token limits
func CreateContextPrompt(previousTranscript string) string {
	if previousTranscript == "" {
		return ""
	}
	
	words := strings.Fields(previousTranscript)
	if len(words) > ContextWordCount {
		words = words[len(words)-ContextWordCount:]
	}
	return strings.Join(words, " ")
}
