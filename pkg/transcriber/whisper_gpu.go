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

	// Check GPU availability
	useGPU := false
	gpuLayers := 0
	
	// Check if GPU is requested via environment
	if os.Getenv("WHISPER_USE_GPU") == "true" {
		// Verify NVIDIA GPU is available
		cmd := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader")
		output, err := cmd.Output()
		
		if err == nil && len(output) > 0 {
			useGPU = true
			gpuName := strings.TrimSpace(string(output))
			
			// Get number of layers to offload to GPU
			if layers := os.Getenv("WHISPER_GPU_LAYERS"); layers != "" {
				if l, err := strconv.Atoi(layers); err == nil {
					gpuLayers = l
				} else {
					gpuLayers = 32 // Default for most models
				}
			} else {
				gpuLayers = 32 // Default
			}
			
			logrus.WithFields(logrus.Fields{
				"gpu_name":   gpuName,
				"gpu_layers": gpuLayers,
			}).Info("GPU acceleration enabled for Whisper")
		} else {
			logrus.Warn("GPU requested but not available, falling back to CPU")
		}
	}

	// Verify whisper binary has CUDA support if GPU is enabled
	if useGPU {
		// Check if whisper was built with CUDA support
		cmd := exec.Command("ldd", whisperPath)
		output, err := cmd.Output()
		if err == nil {
			outputStr := string(output)
			if !strings.Contains(outputStr, "libcuda") && !strings.Contains(outputStr, "libcudart") {
				logrus.Warn("Whisper binary does not appear to have CUDA support, GPU acceleration may not work")
				// Don't disable GPU here, let whisper handle the fallback
			}
		}
	}

	// Get language setting
	language := os.Getenv("WHISPER_LANGUAGE")
	if language == "" {
		language = "auto"
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
		beamSize = "1" // Fast mode by default
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
	startTime := time.Now()
	
	logrus.WithFields(logrus.Fields{
		"audio_bytes": len(audio),
		"model":       wt.modelPath,
		"gpu":         wt.useGPU,
	}).Debug("GPUWhisperTranscriber: Starting transcription")

	// Convert PCM to WAV format using ffmpeg
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

	// Build whisper command with GPU support if available
	whisperArgs := []string{
		"-m", wt.modelPath,
		"-l", wt.language,
		"-t", wt.threads,
		"-bs", wt.beamSize,
		"--no-timestamps",
		"-otxt",
	}

	// Add GPU-specific flags if GPU is available
	if wt.useGPU && wt.gpuLayers > 0 {
		// For whisper.cpp with CUDA support
		whisperArgs = append(whisperArgs, "-ngl", strconv.Itoa(wt.gpuLayers))
		
		// Optional: Add flash attention if supported
		if os.Getenv("WHISPER_FLASH_ATTN") == "true" {
			whisperArgs = append(whisperArgs, "-fa")
		}
	}

	// Add input from stdin
	whisperArgs = append(whisperArgs, "-")

	whisperCmd := exec.Command(wt.whisperPath, whisperArgs...)
	whisperCmd.Stdin = &wavBuf

	var outBuf, errBuf bytes.Buffer
	whisperCmd.Stdout = &outBuf
	whisperCmd.Stderr = &errBuf

	// Set CUDA environment if GPU is enabled
	if wt.useGPU {
		whisperCmd.Env = append(os.Environ(),
			"CUDA_LAUNCH_BLOCKING=0", // Allow async GPU operations
		)
	}

	logrus.WithField("gpu", wt.useGPU).Debug("GPUWhisperTranscriber: Starting whisper process")

	if err := whisperCmd.Run(); err != nil {
		logrus.WithFields(logrus.Fields{
			"error":  err,
			"stderr": errBuf.String(),
		}).Error("Whisper transcription failed")
		return "", fmt.Errorf("whisper transcription failed: %w", err)
	}

	// Clean up the output
	transcript := string(bytes.TrimSpace(outBuf.Bytes()))
	if transcript == "" {
		logrus.Debug("GPUWhisperTranscriber: No speech detected")
		return "[No speech detected]", nil
	}

	// Log performance metrics
	duration := time.Since(startTime)
	audioDuration := time.Duration(len(audio)/96000) * time.Second // 48kHz stereo 16-bit
	rtf := float64(duration) / float64(audioDuration)
	
	logrus.WithFields(logrus.Fields{
		"transcript_length": len(transcript),
		"processing_time":   duration,
		"audio_duration":    audioDuration,
		"rtf":               fmt.Sprintf("%.2fx", rtf),
		"gpu":               wt.useGPU,
	}).Info("GPUWhisperTranscriber: Transcription complete")

	return transcript, nil
}

func (wt *GPUWhisperTranscriber) Close() error {
	return nil
}

// GetGPUInfo returns information about available GPUs
func GetGPUInfo() ([]string, error) {
	cmd := exec.Command("nvidia-smi", "--query-gpu=index,name,memory.total,memory.free,utilization.gpu", "--format=csv,noheader")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to query GPU info: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var gpuInfo []string
	for _, line := range lines {
		if line != "" {
			gpuInfo = append(gpuInfo, line)
		}
	}
	
	return gpuInfo, nil
}

// IsGPUAvailable checks if GPU acceleration is available
func IsGPUAvailable() bool {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader")
	err := cmd.Run()
	return err == nil
}