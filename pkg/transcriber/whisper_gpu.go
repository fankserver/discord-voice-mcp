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

// GPUWhisperTranscriber uses whisper.cpp with GPU acceleration for transcription
type GPUWhisperTranscriber struct {
	modelPath   string
	whisperPath string
	ffmpegPath  string
	language    string
	threads     string
	beamSize    string
	useGPU      bool
	gpuLayers   int
}

// NewGPUWhisperTranscriber creates a GPU-accelerated whisper.cpp based transcriber
func NewGPUWhisperTranscriber(modelPath string) (*GPUWhisperTranscriber, error) {
	// Validate model file exists
	if _, err := os.Stat(modelPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("whisper model file not found: %s", modelPath)
		}
		return nil, fmt.Errorf("whisper model file not accessible: %w", err)
	}

	// Check for whisper executable
	whisperPath, err := exec.LookPath("whisper")
	if err != nil {
		return nil, fmt.Errorf("whisper executable not found in PATH: %w", err)
	}

	// Check for ffmpeg executable
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg executable not found in PATH: %w", err)
	}

	// Check GPU configuration
	// Let whisper.cpp auto-detect the best available backend (CUDA, ROCm, Vulkan, etc.)
	useGPU := false
	gpuLayers := 0

	// Check if GPU is requested via environment (defaults to true for whisper image)
	gpuEnv := os.Getenv("WHISPER_USE_GPU")
	if gpuEnv == "" || gpuEnv == "true" {
		useGPU = true

		// Get number of layers to offload to GPU
		gpuLayers = 32 // Default for most models
		if layersStr := os.Getenv("WHISPER_GPU_LAYERS"); layersStr != "" {
			if l, err := strconv.Atoi(layersStr); err == nil {
				gpuLayers = l
			} else {
				logrus.WithError(err).WithField("value", layersStr).Warn("Invalid WHISPER_GPU_LAYERS value, using default")
			}
		}

		logrus.WithFields(logrus.Fields{
			"gpu_layers": gpuLayers,
			"backend":    "auto-detect",
		}).Info("GPU acceleration enabled - whisper.cpp will auto-detect backend")
	} else {
		logrus.Info("GPU acceleration disabled by configuration")
	}

	// Note: We don't check for specific GPU libraries (CUDA, ROCm, etc.)
	// whisper.cpp will automatically detect and use the best available backend

	// Get language setting
	language := os.Getenv("WHISPER_LANGUAGE")
	if language == "" {
		language = "auto"
	}

	// Log language setting for debugging
	if language != "auto" {
		logrus.WithField("language", language).Info("Whisper language explicitly set")
	}

	// Get thread count
	threads := os.Getenv("WHISPER_THREADS")
	if threads == "" {
		if useGPU {
			// Use fewer CPU threads when GPU is available
			threads = "4"
		} else {
			threads = strconv.Itoa(runtime.NumCPU())
		}
	}

	// Get beam size
	beamSize := os.Getenv("WHISPER_BEAM_SIZE")
	if beamSize == "" {
		// Use beam size 5 for better accuracy when language is explicitly set
		if language != "auto" {
			beamSize = "5"
		} else {
			beamSize = "1" // Fast mode by default for auto-detect
		}
	}

	logrus.WithFields(logrus.Fields{
		"whisper":    whisperPath,
		"ffmpeg":     ffmpegPath,
		"model":      modelPath,
		"language":   language,
		"threads":    threads,
		"beam_size":  beamSize,
		"gpu":        useGPU,
		"gpu_layers": gpuLayers,
	}).Info("GPU Whisper transcriber initialized")

	return &GPUWhisperTranscriber{
		modelPath:   modelPath,
		whisperPath: whisperPath,
		ffmpegPath:  ffmpegPath,
		language:    language,
		threads:     threads,
		beamSize:    beamSize,
		useGPU:      useGPU,
		gpuLayers:   gpuLayers,
	}, nil
}

// Transcribe uses whisper.cpp CLI with optional GPU acceleration
func (wt *GPUWhisperTranscriber) Transcribe(audio []byte) (string, error) {
	result, err := wt.TranscribeWithContext(audio, TranscriptionOptions{})
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

// TranscribeWithContext uses whisper.cpp CLI with context for better accuracy
func (wt *GPUWhisperTranscriber) TranscribeWithContext(audio []byte, opts TranscriptionOptions) (*TranscriptResult, error) {
	startTime := time.Now()

	// Use only the current audio chunk without overlap
	// The overlap context is now provided via the --prompt parameter
	finalAudio := audio

	// Note: We don't prepend overlap audio anymore as it causes duplicates
	// Context is maintained through the prompt parameter instead
	if len(opts.OverlapAudio) > 0 {
		logrus.Debug("Overlap audio available but not prepended (using prompt for context instead)")
	}

	logrus.WithFields(logrus.Fields{
		"audio_bytes":       len(finalAudio),
		"audio_duration_ms": len(finalAudio) * 1000 / 192000,
		"model":             wt.modelPath,
		"gpu":               wt.useGPU,
		"gpu_layers":        wt.gpuLayers,
		"has_context":       opts.PreviousContext != "",
	}).Debug("GPUWhisperTranscriber: Starting transcription")

	// Convert PCM to WAV format using ffmpeg
	// #nosec G204 - ffmpegPath is validated at initialization
	cmd := exec.Command(wt.ffmpegPath,
		"-f", "s16le",
		"-ar", "48000",
		"-ac", "2",
		"-i", "-",
		"-ar", "16000",
		"-ac", "1",
		"-f", "wav",
		"-",
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
		return nil, fmt.Errorf("audio conversion failed: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"wav_size":     wavBuf.Len(),
		"pcm_size":     len(audio),
		"duration_sec": float64(len(audio)) / 192000.0,
	}).Debug("GPUWhisperTranscriber: Converted PCM to WAV")

	// Build whisper command with GPU support if available
	whisperArgs := []string{
		"-m", wt.modelPath,
		"-l", wt.language,
		"-t", wt.threads,
		"-bs", wt.beamSize,
		"--no-timestamps",
		"-otxt",
	}

	// Add context from previous transcript as initial prompt
	// This helps maintain continuity across chunk boundaries
	// IMPORTANT: Use --prompt (not -p) for text prompts
	// The -p flag expects an integer for parallel processing
	if prompt := CreateContextPrompt(opts.PreviousContext); prompt != "" {
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

	// Add additional accuracy parameters for non-English languages
	if wt.language != "auto" && wt.language != "en" {
		// Higher temperature for better accuracy with non-English
		whisperArgs = append(whisperArgs, "-tp", "0.8")
		// Use best_of for better quality
		whisperArgs = append(whisperArgs, "-bo", "5")
	}

	// Add GPU-specific flags if available
	if wt.useGPU {
		// The prebuilt whisper binary uses GPU by default when available
		// Only add --no-gpu flag if we want to disable it
		// Flash attention is supported with -fa flag
		if os.Getenv("WHISPER_FLASH_ATTN") == "true" {
			whisperArgs = append(whisperArgs, "-fa")
		}
	} else {
		// Explicitly disable GPU if not wanted
		whisperArgs = append(whisperArgs, "--no-gpu")
	}

	// Add input from stdin
	whisperArgs = append(whisperArgs, "-")

	// #nosec G204 - whisperPath is validated at initialization
	whisperCmd := exec.Command(wt.whisperPath, whisperArgs...)
	whisperCmd.Stdin = &wavBuf

	var outBuf, errBuf bytes.Buffer
	whisperCmd.Stdout = &outBuf
	whisperCmd.Stderr = &errBuf

	// Let whisper.cpp handle GPU environment configuration
	whisperCmd.Env = os.Environ()

	logrus.WithField("gpu", wt.useGPU).Debug("GPUWhisperTranscriber: Starting whisper process")

	if err := whisperCmd.Run(); err != nil {
		logrus.WithFields(logrus.Fields{
			"error":  err,
			"stderr": errBuf.String(),
		}).Error("Whisper transcription failed")
		return nil, fmt.Errorf("whisper transcription failed: %w", err)
	}

	// Log stderr output for debugging (includes model loading and performance info)
	if errBuf.Len() > 0 {
		logrus.WithField("stderr", errBuf.String()).Debug("Whisper stderr output")
	}

	// Clean up the output
	transcript := string(bytes.TrimSpace(outBuf.Bytes()))
	duration := time.Since(startTime)

	if transcript == "" {
		logrus.WithFields(logrus.Fields{
			"audio_duration_ms": len(audio) * 1000 / 192000,
			"stderr_len":        errBuf.Len(),
		}).Debug("GPUWhisperTranscriber: No speech detected")
		return &TranscriptResult{
			Text:       "[No speech detected]",
			Confidence: 0.0,
			Language:   wt.language,
			Duration:   duration,
		}, nil
	}

	// Log performance metrics
	// 48kHz stereo 16-bit = 48000 samples/sec * 2 channels * 2 bytes/sample = 192000 bytes/sec
	audioDuration := time.Duration(len(audio)/192000) * time.Second
	rtf := float64(duration) / float64(audioDuration)

	logrus.WithFields(logrus.Fields{
		"transcript_length": len(transcript),
		"processing_time":   duration,
		"audio_duration":    audioDuration,
		"rtf":               fmt.Sprintf("%.2fx", rtf),
		"gpu":               wt.useGPU,
	}).Info("GPUWhisperTranscriber: Transcription complete")

	return &TranscriptResult{
		Text:       transcript,
		Confidence: 1.0, // Whisper doesn't provide confidence scores
		Language:   wt.language,
		Duration:   duration,
	}, nil
}

// IsReady returns true if the transcriber is ready to process audio
func (wt *GPUWhisperTranscriber) IsReady() bool {
	// Check that all required paths are still valid
	if _, err := os.Stat(wt.modelPath); err != nil {
		return false
	}
	if _, err := exec.LookPath("whisper"); err != nil {
		return false
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return false
	}
	return true
}

func (wt *GPUWhisperTranscriber) Close() error {
	return nil
}
