package bot

import (
	"fmt"
	"log"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/fankserver/discord-voice-mcp/internal/audio"
	"github.com/fankserver/discord-voice-mcp/internal/session"
)

// VoiceBot manages Discord voice connections
type VoiceBot struct {
	discord        *discordgo.Session
	sessions       *session.Manager
	audioProcessor *audio.Processor
	voiceConn      *discordgo.VoiceConnection
	mu             sync.Mutex
}

// New creates a new VoiceBot instance
func New(token string, sessionManager *session.Manager, audioProcessor *audio.Processor) (*VoiceBot, error) {
	discord, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("error creating Discord session: %w", err)
	}

	bot := &VoiceBot{
		discord:        discord,
		sessions:       sessionManager,
		audioProcessor: audioProcessor,
	}

	// Register handlers
	discord.AddHandler(bot.ready)
	discord.AddHandler(bot.voiceStateUpdate)
	discord.AddHandler(bot.messageCreate)

	// Set intents
	discord.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildVoiceStates |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent

	return bot, nil
}

// Connect establishes connection to Discord
func (vb *VoiceBot) Connect() error {
	return vb.discord.Open()
}

// Disconnect closes Discord connection
func (vb *VoiceBot) Disconnect() error {
	vb.LeaveChannel()
	return vb.discord.Close()
}

// JoinChannel joins a voice channel
func (vb *VoiceBot) JoinChannel(guildID, channelID string) error {
	vb.mu.Lock()
	defer vb.mu.Unlock()

	// Leave current channel if connected
	if vb.voiceConn != nil {
		vb.voiceConn.Disconnect()
	}

	// Join new channel
	vc, err := vb.discord.ChannelVoiceJoin(guildID, channelID, false, false)
	if err != nil {
		return fmt.Errorf("error joining voice channel: %w", err)
	}

	vb.voiceConn = vc

	// Start a new session
	sessionID := vb.sessions.CreateSession(guildID, channelID)
	log.Printf("Started session %s for guild %s channel %s", sessionID, guildID, channelID)

	// Start processing voice
	go vb.audioProcessor.ProcessVoiceReceive(vc, vb.sessions, sessionID)

	return nil
}

// LeaveChannel leaves the current voice channel
func (vb *VoiceBot) LeaveChannel() {
	vb.mu.Lock()
	defer vb.mu.Unlock()

	if vb.voiceConn != nil {
		vb.voiceConn.Disconnect()
		vb.voiceConn = nil
		log.Println("Left voice channel")
	}
}

// GetStatus returns current bot status
func (vb *VoiceBot) GetStatus() map[string]interface{} {
	vb.mu.Lock()
	defer vb.mu.Unlock()

	status := map[string]interface{}{
		"connected": vb.discord.State.Ready,
		"inVoice":   vb.voiceConn != nil,
	}

	if vb.voiceConn != nil {
		status["guildID"] = vb.voiceConn.GuildID
		status["channelID"] = vb.voiceConn.ChannelID
	}

	return status
}

// Event handlers

func (vb *VoiceBot) ready(s *discordgo.Session, event *discordgo.Ready) {
	log.Printf("Bot is ready! Username: %s#%s", s.State.User.Username, s.State.User.Discriminator)
}

func (vb *VoiceBot) voiceStateUpdate(s *discordgo.Session, vsu *discordgo.VoiceStateUpdate) {
	// Handle voice state updates if needed
	if vsu.UserID == s.State.User.ID {
		log.Printf("Bot voice state updated: Channel %s", vsu.ChannelID)
	}
}

func (vb *VoiceBot) messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore bot messages
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Simple commands
	switch m.Content {
	case "!join":
		// Get user's voice channel
		g, err := s.State.Guild(m.GuildID)
		if err != nil || g == nil {
			log.Printf("Could not find guild %s in state: %v", m.GuildID, err)
			s.ChannelMessageSend(m.ChannelID, "Error: Could not retrieve guild information.")
			return
		}
		for _, vs := range g.VoiceStates {
			if vs.UserID == m.Author.ID {
				vb.JoinChannel(m.GuildID, vs.ChannelID)
				s.ChannelMessageSend(m.ChannelID, "Joined voice channel!")
				return
			}
		}
		s.ChannelMessageSend(m.ChannelID, "You need to be in a voice channel!")

	case "!leave":
		vb.LeaveChannel()
		s.ChannelMessageSend(m.ChannelID, "Left voice channel!")

	case "!status":
		status := vb.GetStatus()
		msg := fmt.Sprintf("Status: Connected=%v, InVoice=%v", status["connected"], status["inVoice"])
		s.ChannelMessageSend(m.ChannelID, msg)
	}
}