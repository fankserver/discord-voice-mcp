package audio

import (
	"github.com/bwmarrin/discordgo"
	"github.com/fankserver/discord-voice-mcp/internal/session"
)

// VoiceProcessor is the interface for audio processors
type VoiceProcessor interface {
	// ProcessVoiceReceive handles incoming voice packets
	ProcessVoiceReceive(vc *discordgo.VoiceConnection, sessionManager *session.Manager, activeSessionID string, userResolver UserResolver)
}

// Ensure both processors implement the interface
var _ VoiceProcessor = (*Processor)(nil)
var _ VoiceProcessor = (*AsyncProcessor)(nil)
