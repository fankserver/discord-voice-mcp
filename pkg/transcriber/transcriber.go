package transcriber

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/sirupsen/logrus"
)

// Transcriber interface for different transcription providers
type Transcriber interface {
	Transcribe(audio []byte) (string, error)
	Close() error
}

// WhisperTranscriber uses whisper.cpp for transcription
type WhisperTranscriber struct {
	modelPath    string
	whisperPath  string
	ffmpegPath   string
	language     string  // Language code for transcription (e.g., "en", "de", "auto")
	threads      string  // Number of threads for whisper processing
	beamSize     string  // Beam size for whisper (1 = faster, 5 = more accurate)
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
		language = "auto"  // Default to auto-detection to preserve original language
	}
	
	// Get thread count (default: number of CPU cores for optimal performance)
	threads := os.Getenv("WHISPER_THREADS")
	if threads == "" {
		threads = strconv.Itoa(runtime.NumCPU())
	}
	
	// Get beam size (default: 1 for faster processing, 5 for more accuracy)
	beamSize := os.Getenv("WHISPER_BEAM_SIZE")
	if beamSize == "" {
		beamSize = "1"  // Faster processing by default
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

// Transcribe uses whisper.cpp CLI for transcription
func (wt *WhisperTranscriber) Transcribe(audio []byte) (string, error) {
	logrus.WithFields(logrus.Fields{
		"audio_bytes": len(audio),
		"model":       wt.modelPath,
	}).Debug("WhisperTranscriber: Starting transcription")
	
	// Convert PCM to WAV format using ffmpeg
	// Input: 48kHz, 2 channel, 16-bit signed PCM
	// #nosec G204 - ffmpegPath is validated during initialization, arguments are hardcoded
	cmd := exec.Command(wt.ffmpegPath, 
		"-f", "s16le",      // Input format: signed 16-bit little-endian
		"-ar", "48000",     // Sample rate: 48kHz
		"-ac", "2",         // Channels: 2 (stereo)
		"-i", "-",          // Input from stdin
		"-ar", "16000",     // Resample to 16kHz for Whisper
		"-ac", "1",         // Convert to mono for Whisper
		"-f", "wav",        // Output format: WAV
		"-",                // Output to stdout
	)
	cmd.Stdin = bytes.NewReader(audio)
	
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
	whisperCmd := exec.Command(wt.whisperPath,
		"-m", wt.modelPath,     // Model path
		"-l", wt.language,      // Language: configurable, defaults to auto-detect
		"-t", wt.threads,       // Threads: configurable for performance tuning
		"-bs", wt.beamSize,     // Beam size: smaller = faster, larger = more accurate
		"--no-timestamps",      // Don't include timestamps in output
		"-otxt",                // Output format: plain text
		"-",                    // Read from stdin
	)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

func (gt *GoogleTranscriber) Close() error {
	return nil
}

// MockTranscriber for testing without actual transcription
type MockTranscriber struct{}

func (mt *MockTranscriber) Transcribe(audio []byte) (string, error) {
	logrus.WithField("audio_bytes", len(audio)).Debug("MockTranscriber: Generating mock transcript")
	return fmt.Sprintf("[Mock transcript: %d bytes of audio]", len(audio)), nil
}

func (mt *MockTranscriber) Close() error {
	return nil
}
