package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
	"layeh.com/gopus"
)

const (
	// Audio configuration
	sampleRate = 48000
	channels   = 2
	frameSize  = 960 // 20ms @ 48kHz
	
	// Buffer configuration
	transcriptionBufferSize = sampleRate * channels * 2 * 2 // 2 seconds of audio (samples * channels * bytes per sample * seconds)
)

// Processor handles audio capture and transcription
type Processor struct {
	mu            sync.Mutex
	transcriber   transcriber.Transcriber
	activeStreams map[string]*Stream
}

// Stream represents an active audio stream from a user
type Stream struct {
	UserID   string
	Username string
	Buffer   *bytes.Buffer
	mu       sync.Mutex
}

// NewProcessor creates a new audio processor
func NewProcessor(t transcriber.Transcriber) *Processor {
	return &Processor{
		transcriber:   t,
		activeStreams: make(map[string]*Stream),
	}
}

// ProcessVoiceReceive handles incoming voice packets
func (p *Processor) ProcessVoiceReceive(vc *discordgo.VoiceConnection, sessionManager *session.Manager, activeSessionID string) {
	// Create opus decoder
	decoder, err := gopus.NewDecoder(sampleRate, channels)
	if err != nil {
		log.Printf("Error creating opus decoder: %v", err)
		return
	}

	log.Println("Started processing voice receive")

	// Process incoming audio
	for {
		select {
		case packet, ok := <-vc.OpusRecv:
			if !ok {
				log.Println("Voice receive channel closed")
				return
			}
			
			// Decode opus to PCM
			pcm, err := decoder.Decode(packet.Opus, frameSize, false)
			if err != nil {
				log.Printf("Error decoding opus: %v", err)
				continue
			}

			// Get or create stream for user (using SSRC as ID)
			stream := p.getOrCreateStream(packet.SSRC, fmt.Sprintf("%d", packet.SSRC))

			// Convert PCM to bytes
			pcmBytes := make([]byte, len(pcm)*2) // samples * 2 bytes per sample
			for i := 0; i < len(pcm); i++ {
				binary.LittleEndian.PutUint16(pcmBytes[i*2:], uint16(pcm[i]))
			}

			// Add to buffer
			stream.mu.Lock()
			stream.Buffer.Write(pcmBytes)
			bufferSize := stream.Buffer.Len()
			stream.mu.Unlock()

			// If buffer is large enough, transcribe
			if bufferSize > transcriptionBufferSize {
				go p.transcribeAndClear(stream, sessionManager, activeSessionID)
			}
		}
	}
}

func (p *Processor) getOrCreateStream(ssrc uint32, userID string) *Stream {
	p.mu.Lock()
	defer p.mu.Unlock()

	streamID := fmt.Sprintf("%d", ssrc)
	if stream, exists := p.activeStreams[streamID]; exists {
		return stream
	}

	stream := &Stream{
		UserID:   userID,
		Username: userID, // In production, resolve username
		Buffer:   new(bytes.Buffer),
	}
	p.activeStreams[streamID] = stream
	return stream
}

func (p *Processor) transcribeAndClear(stream *Stream, sessionManager *session.Manager, sessionID string) {
	stream.mu.Lock()
	audioData := stream.Buffer.Bytes()
	stream.Buffer.Reset()
	stream.mu.Unlock()

	if len(audioData) == 0 {
		return
	}

	// Transcribe audio
	text, err := p.transcriber.Transcribe(audioData)
	if err != nil {
		log.Printf("Error transcribing audio: %v", err)
		return
	}

	if text != "" {
		// Add to session
		err = sessionManager.AddTranscript(sessionID, stream.UserID, stream.Username, text)
		if err != nil {
			log.Printf("Error adding transcript: %v", err)
		} else {
			log.Printf("[%s]: %s", stream.Username, text)
		}
	}
}