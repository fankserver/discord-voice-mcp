package transcriber

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

// FasterWhisperTranscriber uses faster-whisper for transcription
// Provides 4x faster transcription than OpenAI Whisper with prebuilt wheels
type FasterWhisperTranscriber struct {
	modelName   string
	language    string
	device      string // "auto", "cpu", "cuda", "rocm"
	computeType string // "float16", "int8_float16", "int8"
	beamSize    int
	pythonPath  string
}

// FasterWhisperResponse represents the JSON response from faster-whisper
type FasterWhisperResponse struct {
	Text string `json:"text"`
}

// NewFasterWhisperTranscriber creates a faster-whisper based transcriber
func NewFasterWhisperTranscriber(modelName string) (*FasterWhisperTranscriber, error) {
	if modelName == "" {
		modelName = "base.en" // Default to English base model
	}

	// Check for Python executable
	pythonPath, err := exec.LookPath("python3")
	if err != nil {
		pythonPath, err = exec.LookPath("python")
		if err != nil {
			return nil, fmt.Errorf("python executable not found in PATH: %w", err)
		}
	}

	// Check if faster-whisper is installed
	cmd := exec.Command(pythonPath, "-c", "import faster_whisper")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("faster-whisper not installed. Install with: pip install faster-whisper")
	}

	// Get device setting from environment (auto, cpu, cuda, rocm)
	device := os.Getenv("FASTER_WHISPER_DEVICE")
	if device == "" {
		device = "auto" // Auto-detect best device
	}

	// Get compute type from environment (float16, int8_float16, int8)
	computeType := os.Getenv("FASTER_WHISPER_COMPUTE_TYPE")
	if computeType == "" {
		computeType = "float16" // Default to float16 for best speed/quality balance
	}

	// Get language setting
	language := os.Getenv("FASTER_WHISPER_LANGUAGE")
	if language == "" {
		language = "auto"
	}

	// Get beam size (1 = fastest, 5 = most accurate)
	beamSize := 1
	if beamSizeStr := os.Getenv("FASTER_WHISPER_BEAM_SIZE"); beamSizeStr != "" {
		if bs := parseBeamSize(beamSizeStr); bs > 0 {
			beamSize = bs
		}
	}

	logrus.WithFields(logrus.Fields{
		"python":       pythonPath,
		"model":        modelName,
		"device":       device,
		"compute_type": computeType,
		"language":     language,
		"beam_size":    beamSize,
	}).Info("FasterWhisper transcriber initialized successfully")

	return &FasterWhisperTranscriber{
		modelName:   modelName,
		language:    language,
		device:      device,
		computeType: computeType,
		beamSize:    beamSize,
		pythonPath:  pythonPath,
	}, nil
}

// Transcribe uses faster-whisper for transcription
func (ft *FasterWhisperTranscriber) Transcribe(audio []byte) (string, error) {
	return ft.TranscribeWithContext(audio, TranscribeOptions{})
}

// TranscribeWithContext uses faster-whisper with context for better accuracy
func (ft *FasterWhisperTranscriber) TranscribeWithContext(audio []byte, opts TranscribeOptions) (string, error) {
	logrus.WithFields(logrus.Fields{
		"audio_bytes": len(audio),
		"model":       ft.modelName,
		"has_context": opts.PreviousTranscript != "",
	}).Debug("FasterWhisperTranscriber: Starting transcription")

	// Create Python script for transcription
	pythonScript := ft.generatePythonScript(opts.PreviousTranscript)

	// Run Python script with audio data
	cmd := exec.Command(ft.pythonPath, "-c", pythonScript)
	cmd.Stdin = bytes.NewReader(audio)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		logrus.WithFields(logrus.Fields{
			"error":  err,
			"stderr": errBuf.String(),
		}).Error("FasterWhisper transcription failed")
		return "", fmt.Errorf("faster-whisper transcription failed: %w", err)
	}

	// Parse JSON response
	var response FasterWhisperResponse
	if err := json.Unmarshal(outBuf.Bytes(), &response); err != nil {
		// If JSON parsing fails, treat output as plain text
		transcript := string(bytes.TrimSpace(outBuf.Bytes()))
		if transcript == "" {
			return "[No speech detected]", nil
		}
		return transcript, nil
	}

	if response.Text == "" {
		logrus.Debug("FasterWhisperTranscriber: No speech detected")
		return "[No speech detected]", nil
	}

	logrus.WithFields(logrus.Fields{
		"transcript_length": len(response.Text),
		"first_50_chars":    response.Text[:min(50, len(response.Text))],
	}).Debug("FasterWhisperTranscriber: Transcription complete")

	return response.Text, nil
}

// generatePythonScript creates the Python script for transcription
func (ft *FasterWhisperTranscriber) generatePythonScript(previousTranscript string) string {
	contextPrompt := CreateContextPrompt(previousTranscript)
	
	return fmt.Sprintf(`
import sys
import json
import io
import numpy as np
from faster_whisper import WhisperModel
import warnings

# Suppress warnings for cleaner output
warnings.filterwarnings("ignore")

try:
    # Read PCM audio data from stdin (48kHz, 2-channel, 16-bit signed)
    audio_data = sys.stdin.buffer.read()
    
    # Convert PCM bytes to numpy array
    # Input: 48kHz, 2-channel, 16-bit signed little-endian
    audio_array = np.frombuffer(audio_data, dtype=np.int16)
    
    # Reshape to stereo (2 channels) and convert to mono by averaging
    if len(audio_array) %% 2 == 0:
        stereo_audio = audio_array.reshape(-1, 2)
        mono_audio = np.mean(stereo_audio, axis=1, dtype=np.int16)
    else:
        # Odd number of samples, assume mono already
        mono_audio = audio_array
    
    # Convert to float32 and normalize to [-1, 1]
    audio_float = mono_audio.astype(np.float32) / 32768.0
    
    # Resample from 48kHz to 16kHz for Whisper
    # Simple decimation by factor of 3 (48000/16000 = 3)
    audio_16k = audio_float[::3]
    
    # Initialize model with GPU acceleration if available
    model = WhisperModel(
        "%s",  # model_name
        device="%s",  # device
        compute_type="%s"  # compute_type
    )
    
    # Transcribe with context if available
    initial_prompt = %s
    segments, info = model.transcribe(
        audio_16k,
        language="%s" if "%s" != "auto" else None,
        beam_size=%d,
        initial_prompt=initial_prompt
    )
    
    # Collect all text segments
    full_text = ""
    for segment in segments:
        full_text += segment.text
    
    # Output as JSON
    result = {"text": full_text.strip()}
    print(json.dumps(result))
    
except Exception as e:
    # Output error as JSON
    error_result = {"text": "", "error": str(e)}
    print(json.dumps(error_result))
    sys.exit(1)
`,
		ft.modelName,
		ft.device,
		ft.computeType,
		formatPythonString(contextPrompt),
		ft.language,
		ft.language,
		ft.beamSize,
	)
}

func (ft *FasterWhisperTranscriber) Close() error {
	return nil
}

// Helper functions
func parseBeamSize(s string) int {
	switch s {
	case "1":
		return 1
	case "2":
		return 2
	case "3":
		return 3
	case "4":
		return 4
	case "5":
		return 5
	default:
		return 1
	}
}

func formatPythonString(s string) string {
	if s == "" {
		return "None"
	}
	// Escape quotes and newlines for Python string literal
	escaped := s
	escaped = fmt.Sprintf(`"%s"`, escaped)
	return escaped
}