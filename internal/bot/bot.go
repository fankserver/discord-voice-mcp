package bot

import (
	"fmt"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/fankserver/discord-voice-mcp/internal/audio"
	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/sirupsen/logrus"
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
		if err := vb.voiceConn.Disconnect(); err != nil {
			logrus.WithError(err).Debug("Error disconnecting from previous channel")
		}
	}

	// Join new channel
	vc, err := vb.discord.ChannelVoiceJoin(guildID, channelID, false, false)
	if err != nil {
		return fmt.Errorf("error joining voice channel: %w", err)
	}

	vb.voiceConn = vc

	// Start a new session
	sessionID := vb.sessions.CreateSession(guildID, channelID)
	logrus.WithFields(logrus.Fields{
		"session_id": sessionID,
		"guild_id":   guildID,
		"channel_id": channelID,
	}).Info("Started voice session")

	// Start processing voice
	go vb.audioProcessor.ProcessVoiceReceive(vc, vb.sessions, sessionID)

	return nil
}

// LeaveChannel leaves the current voice channel
func (vb *VoiceBot) LeaveChannel() {
	vb.mu.Lock()
	defer vb.mu.Unlock()

	if vb.voiceConn != nil {
		if err := vb.voiceConn.Disconnect(); err != nil {
			logrus.WithError(err).Debug("Error disconnecting from voice channel")
		}
		vb.voiceConn = nil
		logrus.Info("Left voice channel")
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
	logrus.WithFields(logrus.Fields{
		"username":      s.State.User.Username,
		"discriminator": s.State.User.Discriminator,
	}).Info("Bot is ready")
}

func (vb *VoiceBot) voiceStateUpdate(s *discordgo.Session, vsu *discordgo.VoiceStateUpdate) {
	// Handle voice state updates if needed
	if vsu.UserID == s.State.User.ID {
		logrus.WithField("channel_id", vsu.ChannelID).Debug("Bot voice state updated")
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
			logrus.WithFields(logrus.Fields{
				"guild_id": m.GuildID,
				"error":    err,
			}).Error("Could not find guild in state")
			if _, err := s.ChannelMessageSend(m.ChannelID, "Error: Could not retrieve guild information."); err != nil {
				logrus.WithError(err).Debug("Failed to send error message")
			}
			return
		}
		for _, vs := range g.VoiceStates {
			if vs.UserID == m.Author.ID {
				if err := vb.JoinChannel(m.GuildID, vs.ChannelID); err != nil {
					logrus.WithError(err).Error("Failed to join voice channel")
				}
				if _, err := s.ChannelMessageSend(m.ChannelID, "Joined voice channel!"); err != nil {
					logrus.WithError(err).Debug("Failed to send join confirmation")
				}
				return
			}
		}
		if _, err := s.ChannelMessageSend(m.ChannelID, "You need to be in a voice channel!"); err != nil {
			logrus.WithError(err).Debug("Failed to send error message")
		}

	case "!leave":
		vb.LeaveChannel()
		if _, err := s.ChannelMessageSend(m.ChannelID, "Left voice channel!"); err != nil {
			logrus.WithError(err).Debug("Failed to send leave confirmation")
		}

	case "!status":
		status := vb.GetStatus()
		msg := fmt.Sprintf("Status: Connected=%v, InVoice=%v", status["connected"], status["inVoice"])
		if _, err := s.ChannelMessageSend(m.ChannelID, msg); err != nil {
			logrus.WithError(err).Debug("Failed to send status message")
		}
	}
}
