package transcriber

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Note: This is the old interface kept for backward compatibility
// New code should use the interface in interface.go

// WhisperTranscriber uses whisper.cpp for transcription
type WhisperTranscriber struct {
	modelPath   string
	whisperPath string
	ffmpegPath  string
	language    string // Language code for transcription (e.g., "en", "de", "auto")
	threads     string // Number of threads for whisper processing
	beamSize    string // Beam size for whisper (1 = faster, 5 = more accurate)
}

// NewWhisperTranscriber creates a whisper.cpp based transcriber
func NewWhisperTranscriber(modelPath string) (*WhisperTranscriber, error) {
	// Validate model file exists
	if _, err := os.Stat(modelPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("whisper model file not found: %s", modelPath)
		}
		return nil, fmt.Errorf("whisper model file not accessible: %w", err)
	}

	// Check for whisper executable and validate it works
	whisperPath, err := exec.LookPath("whisper")
	if err != nil {
		return nil, fmt.Errorf("whisper executable not found in PATH: %w", err)
	}

	// Validate whisper binary works
	// #nosec G204 - whisperPath comes from exec.LookPath which is safe
	if err := exec.Command(whisperPath, "--help").Run(); err != nil {
		return nil, fmt.Errorf("whisper executable found but not working: %w", err)
	}

	// Check for ffmpeg executable and validate it works
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg executable not found in PATH: %w", err)
	}

	// Validate ffmpeg binary works
	// #nosec G204 - ffmpegPath comes from exec.LookPath which is safe
	if err := exec.Command(ffmpegPath, "-version").Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg executable found but not working: %w", err)
	}

	// Get language setting from environment variable (default: auto)
	language := os.Getenv("WHISPER_LANGUAGE")
	if language == "" {
		language = "auto" // Default to auto-detection to preserve original language
	}

	// Get thread count (default: number of CPU cores for optimal performance)
	threads := os.Getenv("WHISPER_THREADS")
	if threads == "" {
		threads = strconv.Itoa(runtime.NumCPU())
	}

	// Get beam size (default: 1 for faster processing, 5 for more accuracy)
	beamSize := os.Getenv("WHISPER_BEAM_SIZE")
	if beamSize == "" {
		beamSize = "1" // Faster processing by default
	}

	logrus.WithFields(logrus.Fields{
		"whisper":   whisperPath,
		"ffmpeg":    ffmpegPath,
		"model":     modelPath,
		"language":  language,
		"threads":   threads,
		"beam_size": beamSize,
	}).Info("Whisper transcriber initialized successfully")

	return &WhisperTranscriber{
		modelPath:   modelPath,
		whisperPath: whisperPath,
		ffmpegPath:  ffmpegPath,
		language:    language,
		threads:     threads,
		beamSize:    beamSize,
	}, nil
}

// Transcribe implements the basic Transcriber interface
func (wt *WhisperTranscriber) Transcribe(audio []byte) (string, error) {
	result, err := wt.TranscribeWithContext(audio, TranscriptionOptions{})
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// TranscribeWithContext implements the new Transcriber interface with enhanced options
func (wt *WhisperTranscriber) TranscribeWithContext(audio []byte, opts TranscriptionOptions) (*TranscriptResult, error) {
	startTime := time.Now()
	
	// Convert old TranscribeOptions if needed for backward compatibility
	var previousTranscript string
	var overlapAudio []byte
	
	// Use new options
	previousTranscript = opts.PreviousContext
	overlapAudio = opts.OverlapAudio
	if opts.Language != "" && opts.Language != "auto" {
		wt.language = opts.Language
	}
	
	// Call the legacy implementation
	text, err := wt.transcribeInternal(audio, previousTranscript, overlapAudio)
	if err != nil {
		return nil, err
	}
	
	// Build result
	return &TranscriptResult{
		Text:       text,
		Confidence: 0.95, // Whisper doesn't provide confidence scores
		Language:   wt.language,
		Duration:   time.Since(startTime),
	}, nil
}

// IsReady implements the new Transcriber interface
func (wt *WhisperTranscriber) IsReady() bool {
	// Check if model file still exists and is accessible
	if _, err := os.Stat(wt.modelPath); err != nil {
		return false
	}
	return true
}

// transcribeInternal is the internal implementation (legacy)
func (wt *WhisperTranscriber) transcribeInternal(audio []byte, previousTranscript string, overlapAudio []byte) (string, error) {
	// Use only the current audio chunk without overlap
	// The overlap context is now provided via the --prompt parameter
	finalAudio := audio

	// Note: We don't prepend overlap audio anymore as it causes duplicates
	// Context is maintained through the prompt parameter instead
	if len(overlapAudio) > 0 {
		logrus.Debug("Overlap audio available but not prepended (using prompt for context instead)")
	}

	logrus.WithFields(logrus.Fields{
		"audio_bytes": len(finalAudio),
		"model":       wt.modelPath,
		"has_context": previousTranscript != "",
	}).Debug("WhisperTranscriber: Starting transcription")

	// Convert PCM to WAV format using ffmpeg
	// Input: 48kHz, 2 channel, 16-bit signed PCM
	// #nosec G204 - ffmpegPath is validated during initialization, arguments are hardcoded
	cmd := exec.Command(wt.ffmpegPath,
		"-f", "s16le", // Input format: signed 16-bit little-endian
		"-ar", "48000", // Sample rate: 48kHz
		"-ac", "2", // Channels: 2 (stereo)
		"-i", "-", // Input from stdin
		"-ar", "16000", // Resample to 16kHz for Whisper
		"-ac", "1", // Convert to mono for Whisper
		"-f", "wav", // Output format: WAV
		"-", // Output to stdout
	)
	cmd.Stdin = bytes.NewReader(finalAudio)

	var wavBuf bytes.Buffer
	var ffmpegErr bytes.Buffer
	cmd.Stdout = &wavBuf
	cmd.Stderr = &ffmpegErr

	if err := cmd.Run(); err != nil {
		logrus.WithFields(logrus.Fields{
			"error":  err,
			"stderr": ffmpegErr.String(),
		}).Error("Failed to convert audio to WAV")
		return "", fmt.Errorf("audio conversion failed: %w", err)
	}

	logrus.WithField("wav_bytes", wavBuf.Len()).Debug("WhisperTranscriber: Audio converted to WAV")

	// Call whisper for transcription
	// Using more specific parameters for better transcription
	// #nosec G204 - modelPath is controlled by server configuration, not user input
	whisperArgs := []string{
		"-m", wt.modelPath, // Model path
		"-l", wt.language, // Language: configurable, defaults to auto-detect
		"-t", wt.threads, // Threads: configurable for performance tuning
		"-bs", wt.beamSize, // Beam size: smaller = faster, larger = more accurate
		"--no-timestamps", // Don't include timestamps in output
		"-otxt",           // Output format: plain text
	}

	// Add context from previous transcript as initial prompt
	// This helps maintain continuity across chunk boundaries
	// IMPORTANT: Use --prompt (not -p) for text prompts
	// The -p flag expects an integer for parallel processing
	if prompt := CreateContextPrompt(previousTranscript); prompt != "" {
		// Log the exact prompt for debugging
		logrus.WithFields(logrus.Fields{
			"prompt":       prompt,
			"prompt_len":   len(prompt),
			"prompt_words": len(strings.Fields(prompt)),
		}).Debug("Using previous transcript as prompt")

		// Use --prompt (not -p) for text prompts
		// The -p flag is for number of processors, not prompt text!
		whisperArgs = append(whisperArgs, "--prompt", prompt)
	}

	whisperArgs = append(whisperArgs, "-") // Read from stdin
	// #nosec G204 - whisperPath is validated during initialization, arguments are controlled
	whisperCmd := exec.Command(wt.whisperPath, whisperArgs...)
	whisperCmd.Stdin = &wavBuf

	var outBuf, errBuf bytes.Buffer
	whisperCmd.Stdout = &outBuf
	whisperCmd.Stderr = &errBuf

	logrus.Debug("WhisperTranscriber: Starting whisper process")

	if err := whisperCmd.Run(); err != nil {
		logrus.WithFields(logrus.Fields{
			"error":  err,
			"stderr": errBuf.String(),
		}).Error("Whisper transcription failed")
		return "", fmt.Errorf("whisper transcription failed: %w", err)
	}

	// Clean up the output (remove extra whitespace)
	transcript := string(bytes.TrimSpace(outBuf.Bytes()))
	if transcript == "" {
		logrus.Debug("WhisperTranscriber: No speech detected")
		return "[No speech detected]", nil
	}

	logrus.WithFields(logrus.Fields{
		"transcript_length": len(transcript),
		"first_50_chars":    transcript[:min(50, len(transcript))],
	}).Debug("WhisperTranscriber: Transcription complete")

	return transcript, nil
}

func (wt *WhisperTranscriber) Close() error {
	return nil
}

// GoogleTranscriber uses Google Cloud Speech API
type GoogleTranscriber struct {
	// In production, add Google Cloud client
}

func NewGoogleTranscriber() (*GoogleTranscriber, error) {
	return &GoogleTranscriber{}, nil
}

func (gt *GoogleTranscriber) Transcribe(audio []byte) (string, error) {
	// Simplified for PoC
	return "Google transcription not implemented in PoC", nil
}

func (gt *GoogleTranscriber) TranscribeWithContext(audio []byte, opts TranscriptionOptions) (*TranscriptResult, error) {
	// TODO: Implement Google Speech-to-Text with speech context
	return &TranscriptResult{
		Text:       "Google transcription not implemented in PoC",
		Confidence: 0.0,
		Language:   "en",
		Duration:   time.Millisecond,
	}, nil
}

func (gt *GoogleTranscriber) IsReady() bool {
	// TODO: Check Google client status
	return false
}

func (gt *GoogleTranscriber) Close() error {
	return nil
}

// MockTranscriber for testing without actual transcription
type MockTranscriber struct{}

func (mt *MockTranscriber) Transcribe(audio []byte) (string, error) {
	logrus.WithField("audio_bytes", len(audio)).Debug("MockTranscriber: Generating mock transcript")
	return fmt.Sprintf("[Mock transcript: %d bytes of audio]", len(audio)), nil
}

func (mt *MockTranscriber) TranscribeWithContext(audio []byte, opts TranscriptionOptions) (*TranscriptResult, error) {
	startTime := time.Now()
	text := fmt.Sprintf("[Mock transcript: %d bytes of audio]", len(audio))
	if opts.PreviousContext != "" {
		text = fmt.Sprintf("[Mock transcript with context: %d bytes]", len(audio))
	}
	
	return &TranscriptResult{
		Text:       text,
		Confidence: 1.0,
		Language:   "en",
		Duration:   time.Since(startTime),
	}, nil
}

func (mt *MockTranscriber) IsReady() bool {
	return true
}

func (mt *MockTranscriber) Close() error {
	return nil
}
