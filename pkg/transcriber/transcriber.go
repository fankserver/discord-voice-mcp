package transcriber

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/sirupsen/logrus"
)

// Transcriber interface for different transcription providers
type Transcriber interface {
	Transcribe(audio []byte) (string, error)
	Close() error
}

// WhisperTranscriber uses whisper.cpp for transcription
type WhisperTranscriber struct {
	modelPath string
}

// NewWhisperTranscriber creates a whisper.cpp based transcriber
func NewWhisperTranscriber(modelPath string) *WhisperTranscriber {
	return &WhisperTranscriber{
		modelPath: modelPath,
	}
}

// Transcribe uses whisper.cpp CLI for transcription
func (wt *WhisperTranscriber) Transcribe(audio []byte) (string, error) {
	// For proof of concept, we'll use exec to call whisper CLI
	// In production, use Go bindings

	// Create temporary WAV file
	cmd := exec.Command("ffmpeg", "-f", "s16le", "-ar", "48000", "-ac", "2", "-i", "-", "-f", "wav", "-")
	cmd.Stdin = bytes.NewReader(audio)

	wavData, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ffmpeg error: %w", err)
	}

	// Call whisper (simplified for PoC)
	// #nosec G204 - modelPath is controlled by server configuration, not user input
	whisperCmd := exec.Command("whisper", "-m", wt.modelPath, "-")
	whisperCmd.Stdin = bytes.NewReader(wavData)

	output, err := whisperCmd.Output()
	if err != nil {
		logrus.WithError(err).Debug("Whisper command failed")
		// For PoC, just return empty on error
		return "", nil
	}

	return string(output), nil
}

func (wt *WhisperTranscriber) Close() error {
	return nil
}

// GoogleTranscriber uses Google Cloud Speech API
type GoogleTranscriber struct {
	// In production, add Google Cloud client
}

func NewGoogleTranscriber() *GoogleTranscriber {
	return &GoogleTranscriber{}
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
	return fmt.Sprintf("[Mock transcript: %d bytes of audio]", len(audio)), nil
}

func (mt *MockTranscriber) Close() error {
	return nil
}
