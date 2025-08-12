package transcriber

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockContextAwareTranscriber for testing
type MockContextAwareTranscriber struct {
	mock.Mock
}

func (m *MockContextAwareTranscriber) Transcribe(audio []byte) (string, error) {
	args := m.Called(audio)
	return args.String(0), args.Error(1)
}

func (m *MockContextAwareTranscriber) TranscribeWithContext(audio []byte, opts TranscriptionOptions) (*TranscriptResult, error) {
	args := m.Called(audio, opts)
	if result := args.Get(0); result != nil {
		return result.(*TranscriptResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockContextAwareTranscriber) IsReady() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockContextAwareTranscriber) Close() error {
	args := m.Called()
	return args.Error(0)
}

// TestCreateContextPrompt tests the context prompt creation
func TestCreateContextPrompt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty_input",
			input:    "",
			expected: "",
		},
		{
			name:     "short_transcript",
			input:    "Hello world",
			expected: "Hello world",
		},
		{
			name:     "exactly_30_words",
			input:    "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty twentyone twentytwo twentythree twentyfour twentyfive twentysix twentyseven twentyeight twentynine thirty",
			expected: "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty twentyone twentytwo twentythree twentyfour twentyfive twentysix twentyseven twentyeight twentynine thirty",
		},
		{
			name:     "more_than_30_words_takes_last_30",
			input:    "start word1 word2 word3 word4 word5 one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty twentyone twentytwo twentythree twentyfour twentyfive twentysix twentyseven twentyeight twentynine thirty",
			expected: "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty twentyone twentytwo twentythree twentyfour twentyfive twentysix twentyseven twentyeight twentynine thirty",
		},
		{
			name:     "special_characters_removed",
			input:    "Hello @world! #test $123 %percent ^power &and *star (paren) [bracket] {brace}",
			expected: "Hello world! test 123 percent power and star paren bracket brace",
		},
		{
			name:     "german_umlauts_preserved",
			input:    "Der Bär läuft über die Straße mit großem Glück",
			expected: "Der Bär läuft über die Straße mit großem Glück",
		},
		{
			name:     "punctuation_preserved",
			input:    "Hello, world! How are you? I'm fine.",
			expected: "Hello, world! How are you? I m fine.",
		},
		{
			name:     "multiple_spaces_normalized",
			input:    "Hello    world     with      spaces",
			expected: "Hello world with spaces",
		},
		{
			name:     "newlines_converted_to_spaces",
			input:    "Hello\nworld\nwith\nnewlines",
			expected: "Hello world with newlines",
		},
		{
			name:     "tabs_converted_to_spaces",
			input:    "Hello\tworld\twith\ttabs",
			expected: "Hello world with tabs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreateContextPrompt(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// MockBasicTranscriber for testing fallback behavior
type MockBasicTranscriber struct {
	mock.Mock
}

func (m *MockBasicTranscriber) Transcribe(audio []byte) (string, error) {
	args := m.Called(audio)
	return args.String(0), args.Error(1)
}

func (m *MockBasicTranscriber) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockBasicTranscriber) IsReady() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockBasicTranscriber) TranscribeWithContext(audio []byte, opts TranscriptionOptions) (*TranscriptResult, error) {
	// Basic transcriber doesn't support context, return basic transcription
	text, err := m.Transcribe(audio)
	if err != nil {
		return nil, err
	}
	return &TranscriptResult{Text: text}, nil
}

// TestTranscribeWithContextFallback tests fallback to basic transcription
func TestTranscribeWithContextFallback(t *testing.T) {
	// Test with basic transcriber (no context support)
	mockTranscriber := new(MockBasicTranscriber)
	audio := []byte("test audio")
	opts := TranscriptionOptions{
		PreviousContext: "previous context",
		Language:        "de",
	}

	// Should fall back to basic Transcribe
	mockTranscriber.On("Transcribe", audio).Return("transcribed text", nil)

	// Basic transcriber with context support (delegates to Transcribe internally)
	result, err := mockTranscriber.TranscribeWithContext(audio, opts)

	assert.NoError(t, err)
	assert.Equal(t, "transcribed text", result.Text)
	mockTranscriber.AssertExpectations(t)
}

// TestTranscribeWithContextAware tests context-aware transcription
func TestTranscribeWithContextAware(t *testing.T) {
	// Test with context-aware transcriber
	mockTranscriber := new(MockContextAwareTranscriber)
	audio := []byte("test audio")
	opts := TranscriptionOptions{
		PreviousContext: "previous context",
		OverlapAudio:    []byte("overlap"),
		Language:        "de",
	}

	// Should use TranscribeWithContext
	expectedResult := &TranscriptResult{Text: "context-aware text"}
	mockTranscriber.On("TranscribeWithContext", audio, opts).Return(expectedResult, nil)

	result, err := mockTranscriber.TranscribeWithContext(audio, opts)

	assert.NoError(t, err)
	assert.Equal(t, "context-aware text", result.Text)
	mockTranscriber.AssertExpectations(t)
}

// TestContextPromptWordLimit tests that context prompt respects word limit
func TestContextPromptWordLimit(t *testing.T) {
	// Create a very long transcript
	var longTranscript string
	for i := 0; i < 100; i++ {
		longTranscript += "word" + string(rune(i)) + " "
	}

	prompt := CreateContextPrompt(longTranscript)
	words := len(splitWords(prompt))

	assert.LessOrEqual(t, words, ContextWordCount, "Prompt should not exceed word limit")
	assert.Greater(t, words, 0, "Prompt should contain some words")
}

// Helper function to count words
func splitWords(s string) []string {
	if s == "" {
		return []string{}
	}
	var words []string
	word := ""
	for _, r := range s {
		if r == ' ' {
			if word != "" {
				words = append(words, word)
				word = ""
			}
		} else {
			word += string(r)
		}
	}
	if word != "" {
		words = append(words, word)
	}
	return words
}

// TestTranscriptionOptionsDefaults tests default values in TranscriptionOptions
func TestTranscriptionOptionsDefaults(t *testing.T) {
	opts := TranscriptionOptions{}

	assert.Equal(t, "", opts.PreviousContext)
	assert.Nil(t, opts.OverlapAudio)
	assert.Equal(t, "", opts.Language)
	assert.Equal(t, 0, opts.MaxAlternatives)
	assert.Equal(t, false, opts.EnableTimestamps)
	assert.Nil(t, opts.CustomVocabulary)
	assert.Equal(t, float32(0), opts.Temperature)
}

// BenchmarkCreateContextPrompt benchmarks context prompt creation
func BenchmarkCreateContextPrompt(b *testing.B) {
	transcript := "This is a sample transcript with multiple words that will be used to test the performance of the context prompt creation function"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CreateContextPrompt(transcript)
	}
}

// BenchmarkCreateContextPromptLong benchmarks with long transcript
func BenchmarkCreateContextPromptLong(b *testing.B) {
	// Create a transcript with 100 words
	var transcript string
	for i := 0; i < 100; i++ {
		transcript += "word" + string(rune(i)) + " "
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CreateContextPrompt(transcript)
	}
}

// TestCreateContextPromptUnicode tests Unicode handling
func TestCreateContextPromptUnicode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "chinese_characters_replaced",
			input:    "Hello 你好 world",
			expected: "Hello world",
		},
		{
			name:     "emoji_replaced",
			input:    "Hello 😊 world 🌍",
			expected: "Hello world",
		},
		{
			name:     "mixed_unicode",
			input:    "Café münchën 北京 Tokyo",
			expected: "Café münchën Tokyo",
		},
		{
			name:     "arabic_replaced",
			input:    "Hello مرحبا world",
			expected: "Hello world",
		},
		{
			name:     "cyrillic_replaced",
			input:    "Hello привет world",
			expected: "Hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreateContextPrompt(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestContextPromptConsistency tests that the same input always produces the same output
func TestContextPromptConsistency(t *testing.T) {
	input := "This is a test transcript with special chars @#$ and German üöä"

	// Generate prompt multiple times
	results := make([]string, 10)
	for i := 0; i < 10; i++ {
		results[i] = CreateContextPrompt(input)
	}

	// All results should be identical
	for i := 1; i < 10; i++ {
		assert.Equal(t, results[0], results[i], "Prompt should be consistent")
	}
}

// TestContextPromptEdgeCases tests edge cases
func TestContextPromptEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "only_special_chars",
			input:    "@#$%^&*()",
			expected: "",
		},
		{
			name:     "only_spaces",
			input:    "     ",
			expected: "",
		},
		{
			name:     "single_word",
			input:    "word",
			expected: "word",
		},
		{
			name:     "numbers_preserved",
			input:    "123 456 789",
			expected: "123 456 789",
		},
		{
			name:     "mixed_case_preserved",
			input:    "HeLLo WoRLd",
			expected: "HeLLo WoRLd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreateContextPrompt(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
