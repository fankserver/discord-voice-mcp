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
	followUserID   string // User ID to follow
	autoFollow     bool   // Whether to auto-follow user
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

	// Set intents - only need guilds and voice states, no messages
	discord.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildVoiceStates

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

	// Join new channel - muted but NOT deafened to receive voice
	vc, err := vb.discord.ChannelVoiceJoin(guildID, channelID, true, false)
	if err != nil {
		return fmt.Errorf("error joining voice channel: %w", err)
	}
	
	// Enable voice receive
	vc.Speaking(false)
	
	logrus.WithFields(logrus.Fields{
		"guild_id":   guildID,
		"channel_id": channelID,
		"receiving":  true,
	}).Debug("Voice connection established")

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

// FindUserVoiceChannel finds which voice channel a user is in
func (vb *VoiceBot) FindUserVoiceChannel(userID string) (guildID, channelID string, err error) {
	// Search across all guilds the bot is in
	for _, guild := range vb.discord.State.Guilds {
		for _, vs := range guild.VoiceStates {
			if vs.UserID == userID && vs.ChannelID != "" {
				return guild.ID, vs.ChannelID, nil
			}
		}
	}
	return "", "", fmt.Errorf("user %s is not in any voice channel", userID)
}

// JoinUserChannel joins the voice channel where the specified user is
func (vb *VoiceBot) JoinUserChannel(userID string) error {
	guildID, channelID, err := vb.FindUserVoiceChannel(userID)
	if err != nil {
		return err
	}
	
	logrus.WithFields(logrus.Fields{
		"user_id":    userID,
		"guild_id":   guildID,
		"channel_id": channelID,
	}).Info("Joining user's voice channel")
	
	return vb.JoinChannel(guildID, channelID)
}

// SetFollowUser sets the user to follow and enables/disables auto-follow
func (vb *VoiceBot) SetFollowUser(userID string, autoFollow bool) {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	
	vb.followUserID = userID
	vb.autoFollow = autoFollow
	
	logrus.WithFields(logrus.Fields{
		"user_id":     userID,
		"auto_follow": autoFollow,
	}).Info("Follow settings updated")
}

// GetFollowStatus returns current follow settings
func (vb *VoiceBot) GetFollowStatus() (userID string, autoFollow bool) {
	vb.mu.Lock()
	defer vb.mu.Unlock()
	return vb.followUserID, vb.autoFollow
}

// GetStatus returns current bot status
func (vb *VoiceBot) GetStatus() map[string]interface{} {
	vb.mu.Lock()
	defer vb.mu.Unlock()

	// Check if state exists and has a session ID (indicates ready)
	connected := vb.discord.State != nil && vb.discord.State.SessionID != ""
	
	status := map[string]interface{}{
		"connected": connected,
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
	// Handle bot's own voice state updates
	if vsu.UserID == s.State.User.ID {
		logrus.WithField("channel_id", vsu.ChannelID).Debug("Bot voice state updated")
		return
	}

	// Handle followed user's voice state updates
	vb.mu.Lock()
	followUserID := vb.followUserID
	autoFollow := vb.autoFollow
	vb.mu.Unlock()

	if followUserID != "" && vsu.UserID == followUserID && autoFollow {
		if vsu.ChannelID == "" {
			// User left voice channel
			logrus.WithField("user_id", followUserID).Info("Followed user left voice, leaving channel")
			vb.LeaveChannel()
		} else {
			// User joined or moved to a new channel
			logrus.WithFields(logrus.Fields{
				"user_id":    followUserID,
				"guild_id":   vsu.GuildID,
				"channel_id": vsu.ChannelID,
			}).Info("Followed user changed voice channel, following")
			
			// Only join if we're not already in that channel
			vb.mu.Lock()
			currentConn := vb.voiceConn
			vb.mu.Unlock()
			
			if currentConn == nil || currentConn.ChannelID != vsu.ChannelID {
				if err := vb.JoinChannel(vsu.GuildID, vsu.ChannelID); err != nil {
					logrus.WithError(err).Error("Failed to follow user to new channel")
				}
			}
		}
	}
}
