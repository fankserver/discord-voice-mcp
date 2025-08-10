package transcriber

import "strings"

const (
	// ContextWordCount is the number of words to use as context from previous transcripts
	ContextWordCount = 30
)

// NOTE: TranscribeOptions and ContextAwareTranscriber have been moved to interface.go
// This file now only contains the helper function CreateContextPrompt

// CreateContextPrompt creates a prompt from the previous transcript for whisper
// It takes the last N words (ContextWordCount) to stay within token limits
func CreateContextPrompt(previousTranscript string) string {
	if previousTranscript == "" {
		return ""
	}

	// Remove any special characters that might break command-line parsing
	// Keep only alphanumeric, spaces, and basic punctuation
	cleanTranscript := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == ' ' || r == '.' || r == ',' || r == '!' || r == '?':
			return r
		case r >= 'À' && r <= 'ÿ': // Latin extended characters (for German umlauts, etc.)
			return r
		default:
			return ' ' // Replace other characters with space
		}
	}, previousTranscript)

	// Normalize multiple spaces to single space
	cleanTranscript = strings.Join(strings.Fields(cleanTranscript), " ")

	words := strings.Fields(cleanTranscript)
	if len(words) > ContextWordCount {
		words = words[len(words)-ContextWordCount:]
	}
	return strings.Join(words, " ")
}
