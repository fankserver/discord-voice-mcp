package bot

import (
	"fmt"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/fankserver/discord-voice-mcp/internal/audio"
	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/sirupsen/logrus"
)

// UserInfo stores user information for transcription
type UserInfo struct {
	UserID   string
	Username string
	Nickname string // Server-specific nickname if available
}

// VoiceBot manages Discord voice connections
type VoiceBot struct {
	discord           *discordgo.Session
	sessions          *session.Manager
	audioProcessor    audio.VoiceProcessor // Now uses interface for flexibility
	voiceConn         *discordgo.VoiceConnection
	followUserID      string             // User ID to follow
	autoFollow        bool               // Whether to auto-follow user
	simpleSSRCManager *SimpleSSRCManager // Simple deterministic SSRC mapping
	mu                sync.Mutex
}

// New creates a new VoiceBot instance
func New(token string, sessionManager *session.Manager, audioProcessor audio.VoiceProcessor) (*VoiceBot, error) {
	discord, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("error creating Discord session: %w", err)
	}

	bot := &VoiceBot{
		discord:           discord,
		sessions:          sessionManager,
		audioProcessor:    audioProcessor,
		simpleSSRCManager: NewSimpleSSRCManager(),
	}

	// Register handlers
	discord.AddHandler(bot.ready)
	discord.AddHandler(bot.voiceStateUpdate)
	// Note: voiceSpeakingUpdate must be registered on VoiceConnection, not Session

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
	if err := vc.Speaking(false); err != nil {
		logrus.WithError(err).Debug("Error setting speaking state")
	}

	logrus.WithFields(logrus.Fields{
		"guild_id":   guildID,
		"channel_id": channelID,
		"receiving":  true,
	}).Debug("Voice connection established")

	vb.voiceConn = vc
	// Register voice speaking handler on the voice connection
	vc.AddHandler(vb.voiceSpeakingUpdate)
	logrus.WithField("handler_count", len(vc.OpusRecv)).Debug("Registered VoiceSpeakingUpdate handler on voice connection")

	// Try to listen for voice data to trigger speaking events
	go func() {
		// Small delay to let connection stabilize
		time.Sleep(500 * time.Millisecond)

		// Send speaking packet to potentially trigger events
		if err := vc.Speaking(true); err != nil {
			logrus.WithError(err).Debug("Error setting speaking flag")
		}
		time.Sleep(100 * time.Millisecond)
		if err := vc.Speaking(false); err != nil {
			logrus.WithError(err).Debug("Error unsetting speaking flag")
		}

		logrus.Debug("Triggered speaking state change to activate voice events")
	}()

	// Set channel context for simple SSRC manager
	vb.simpleSSRCManager.SetChannel(guildID, channelID)

	// Start a new session
	sessionID := vb.sessions.CreateSession(guildID, channelID)
	logrus.WithFields(logrus.Fields{
		"session_id": sessionID,
		"guild_id":   guildID,
		"channel_id": channelID,
	}).Info("Started voice session")

	// Important limitation: Users already speaking need to toggle their mic
	logrus.WithFields(logrus.Fields{
		"guild_id":   guildID,
		"channel_id": channelID,
	}).Warn("⚠️ Users currently speaking need to toggle mute/unmute once for identification (Discord API limitation)")

	// Start processing voice (pass bot as UserResolver)
	go vb.audioProcessor.ProcessVoiceReceive(vc, vb.sessions, sessionID, vb)

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

	// Clear simple SSRC manager state when leaving channel
	vb.simpleSSRCManager.Clear()
}

// FindUserVoiceChannel finds which voice channel a user is in
func (vb *VoiceBot) FindUserVoiceChannel(userID string) (guildID, channelID string, err error) {
	// Search across all guilds the bot is in
	for _, guild := range vb.discord.State.Guilds {
		// Skip nil guilds (shouldn't happen but defensive programming)
		if guild == nil {
			continue
		}
		for _, vs := range guild.VoiceStates {
			if vs != nil && vs.UserID == userID && vs.ChannelID != "" {
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

func (vb *VoiceBot) voiceSpeakingUpdate(vc *discordgo.VoiceConnection, vsu *discordgo.VoiceSpeakingUpdate) {
	// CRITICAL: This is the ONLY deterministic way to map SSRCs to users
	logrus.WithFields(logrus.Fields{
		"user_id":  vsu.UserID,
		"ssrc":     vsu.SSRC,
		"speaking": vsu.Speaking,
	}).Info("🎤 VoiceSpeakingUpdate event received - This is the ONLY reliable SSRC mapping source")

	// Check for overflow before conversion
	if vsu.SSRC < 0 || vsu.SSRC > int(^uint32(0)) {
		logrus.WithField("ssrc", vsu.SSRC).Error("SSRC value out of uint32 range")
		return
	}

	ssrc := uint32(vsu.SSRC)

	// Try to get member from guild state first
	var username, nickname string
	if vb.voiceConn != nil && vb.voiceConn.GuildID != "" {
		member, err := vb.discord.State.Member(vb.voiceConn.GuildID, vsu.UserID)
		if err == nil && member != nil && member.User != nil {
			username = member.User.Username
			nickname = member.Nick
			if nickname == "" {
				nickname = username
			}
		} else {
			// If not in state, try fetching from API
			logrus.WithFields(logrus.Fields{
				"user_id":  vsu.UserID,
				"guild_id": vb.voiceConn.GuildID,
			}).Debug("Member not in state, fetching from API")

			member, err = vb.discord.GuildMember(vb.voiceConn.GuildID, vsu.UserID)
			if err == nil && member != nil && member.User != nil {
				username = member.User.Username
				nickname = member.Nick
				if nickname == "" {
					nickname = username
				}
			}
		}
	}

	// If we still couldn't get the username, try getting user directly
	if username == "" {
		user, err := vb.discord.User(vsu.UserID)
		if err == nil && user != nil {
			username = user.Username
			nickname = username // Use username as nickname if we don't have guild info
		}
	}

	// Last resort: use UserID but log it as a warning
	if username == "" {
		logrus.WithField("user_id", vsu.UserID).Warn("Could not resolve username for user, using ID as fallback")
		username = fmt.Sprintf("User-%s", vsu.UserID)
		nickname = username
	}

	// Register the mapping with the SIMPLE SSRC manager (deterministic approach)
	vb.simpleSSRCManager.MapSSRC(ssrc, vsu.UserID, username, nickname)

	action := "stopped"
	if vsu.Speaking {
		action = "started"
	}

	// Get current statistics
	stats := vb.simpleSSRCManager.GetStatistics()

	logrus.WithFields(logrus.Fields{
		"ssrc":           ssrc,
		"user_id":        vsu.UserID,
		"username":       username,
		"nickname":       nickname,
		"action":         action,
		"exact_mappings": stats["exact_mappings"],
		"method":         "deterministic",
	}).Info(fmt.Sprintf("✅ User %s speaking - SSRC mapped via VoiceSpeakingUpdate (DETERMINISTIC)", action))
}


// GetUserBySSRC returns user information for a given SSRC (implements UserResolver)
// DETERMINISTIC APPROACH: Only returns exact mappings from VoiceSpeakingUpdate events
func (vb *VoiceBot) GetUserBySSRC(ssrc uint32) (userID, username, nickname string) {
	// Use the simple deterministic SSRC manager
	return vb.simpleSSRCManager.GetUserBySSRC(ssrc)
}

// RegisterAudioPacket is called by the audio processor for each packet
// DETERMINISTIC APPROACH: We don't analyze packets to guess mappings
func (vb *VoiceBot) RegisterAudioPacket(ssrc uint32, packetSize int) {
	// No-op in deterministic approach - we only map via VoiceSpeakingUpdate events
	vb.simpleSSRCManager.RegisterAudioPacket(ssrc, packetSize)
}
