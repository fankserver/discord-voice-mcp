package bot

import (
	"fmt"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
)

// SSRCManager handles intelligent SSRC-to-user mapping using multiple strategies
type SSRCManager struct {
	mu sync.RWMutex

	// Confirmed SSRC mappings
	ssrcToUser map[uint32]*UserInfo

	// Users we expect to see (from voice states)
	expectedUsers map[string]*UserInfo // userID -> info

	// SSRCs we've seen but haven't mapped yet
	unmappedSSRCs map[uint32]*SSRCMetadata

	// Reverse mapping for deduction
	userToSSRC map[string]uint32

	// Voice connection details
	guildID   string
	channelID string
	discord   *discordgo.Session
}

// SSRCMetadata tracks information about an unmapped SSRC
type SSRCMetadata struct {
	SSRC           uint32
	FirstSeen      time.Time
	LastSeen       time.Time
	PacketCount    int
	AudioActive    bool
	AveragePacketSize int
}

// NewSSRCManager creates a new SSRC manager
func NewSSRCManager(discord *discordgo.Session) *SSRCManager {
	return &SSRCManager{
		ssrcToUser:    make(map[uint32]*UserInfo),
		expectedUsers: make(map[string]*UserInfo),
		unmappedSSRCs: make(map[uint32]*SSRCMetadata),
		userToSSRC:    make(map[string]uint32),
		discord:       discord,
	}
}

// SetChannel updates the current channel context
func (m *SSRCManager) SetChannel(guildID, channelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.guildID = guildID
	m.channelID = channelID

	// Clear previous state
	m.expectedUsers = make(map[string]*UserInfo)
	m.unmappedSSRCs = make(map[uint32]*SSRCMetadata)
}

// PopulateExpectedUsers loads all users who should be in the channel
func (m *SSRCManager) PopulateExpectedUsers(guildID, channelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get guild state
	guild, err := m.discord.State.Guild(guildID)
	if err != nil {
		logrus.WithError(err).Debug("Could not get guild from state")
		return
	}

	// Find all users in the voice channel
	for _, vs := range guild.VoiceStates {
		if vs != nil && vs.ChannelID == channelID && vs.UserID != m.discord.State.User.ID {
			// Get member info
			member, err := m.discord.State.Member(guildID, vs.UserID)
			if err != nil {
				member, err = m.discord.GuildMember(guildID, vs.UserID)
				if err != nil {
					continue
				}
			}

			if member != nil && member.User != nil {
				username := member.User.Username
				nickname := member.Nick
				if nickname == "" {
					nickname = username
				}

				userInfo := &UserInfo{
					UserID:   vs.UserID,
					Username: username,
					Nickname: nickname,
				}

				m.expectedUsers[vs.UserID] = userInfo

				logrus.WithFields(logrus.Fields{
					"user_id":  vs.UserID,
					"username": username,
					"nickname": nickname,
				}).Debug("Added expected user in voice channel")
			}
		}
	}

	logrus.WithField("expected_users", len(m.expectedUsers)).Info("Populated expected users list")

	// Try to deduce mappings if possible
	m.attemptDeduction()
}

// RegisterAudioPacket tracks an incoming audio packet
func (m *SSRCManager) RegisterAudioPacket(ssrc uint32, packetSize int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If already mapped, nothing to do
	if _, exists := m.ssrcToUser[ssrc]; exists {
		return
	}

	// Track this unmapped SSRC
	metadata, exists := m.unmappedSSRCs[ssrc]
	if !exists {
		metadata = &SSRCMetadata{
			SSRC:      ssrc,
			FirstSeen: time.Now(),
		}
		m.unmappedSSRCs[ssrc] = metadata

		logrus.WithFields(logrus.Fields{
			"ssrc":              ssrc,
			"total_unmapped":    len(m.unmappedSSRCs),
			"expected_users":    len(m.expectedUsers),
			"confirmed_mappings": len(m.ssrcToUser),
		}).Info("New unmapped SSRC detected")
	}

	// Update metadata
	metadata.LastSeen = time.Now()
	metadata.PacketCount++
	if packetSize > 3 { // Not silence
		metadata.AudioActive = true
		// Update average packet size
		if metadata.AveragePacketSize == 0 {
			metadata.AveragePacketSize = packetSize
		} else {
			metadata.AveragePacketSize = (metadata.AveragePacketSize + packetSize) / 2
		}
	}

	// Attempt deduction with new information
	m.attemptDeduction()
}

// MapSSRC creates a confirmed SSRC mapping from VoiceSpeakingUpdate
func (m *SSRCManager) MapSSRC(ssrc uint32, userID string, username string, nickname string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store the mapping
	m.ssrcToUser[ssrc] = &UserInfo{
		UserID:   userID,
		Username: username,
		Nickname: nickname,
	}
	m.userToSSRC[userID] = ssrc

	// Remove from unmapped
	delete(m.unmappedSSRCs, ssrc)

	// Remove from expected users (they're now mapped)
	delete(m.expectedUsers, userID)

	logrus.WithFields(logrus.Fields{
		"ssrc":               ssrc,
		"user_id":            userID,
		"username":           username,
		"remaining_unmapped": len(m.unmappedSSRCs),
		"remaining_expected": len(m.expectedUsers),
	}).Info("SSRC mapped to user")

	// Try to deduce more mappings
	m.attemptDeduction()
}

// GetUserBySSRC returns user info for an SSRC with intelligent fallback
func (m *SSRCManager) GetUserBySSRC(ssrc uint32) (userID, username, nickname string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check confirmed mappings
	if info, exists := m.ssrcToUser[ssrc]; exists {
		return info.UserID, info.Username, info.Nickname
	}

	// Return unknown with SSRC (deduction only happens in attemptDeduction with confirmed audio activity)
	ssrcStr := fmt.Sprintf("%d", ssrc)
	return ssrcStr, fmt.Sprintf("Unknown-%s", ssrcStr), fmt.Sprintf("Unknown-%s", ssrcStr)
}

// attemptDeduction tries to deduce SSRC mappings using logic
func (m *SSRCManager) attemptDeduction() {
	// Don't attempt if numbers don't match or no unmapped items
	if len(m.unmappedSSRCs) == 0 || len(m.expectedUsers) == 0 {
		return
	}

	// Special case: exactly one unmapped SSRC and one expected user
	if len(m.unmappedSSRCs) == 1 && len(m.expectedUsers) == 1 {
		var ssrc uint32
		var metadata *SSRCMetadata
		for s, m := range m.unmappedSSRCs {
			ssrc = s
			metadata = m
			break
		}

		var userInfo *UserInfo
		for _, u := range m.expectedUsers {
			userInfo = u
			break
		}

		// Only deduce if SSRC has been active (received audio)
		if metadata.AudioActive {
			logrus.WithFields(logrus.Fields{
				"ssrc":     ssrc,
				"user_id":  userInfo.UserID,
				"username": userInfo.Username,
				"method":   "elimination",
			}).Info("Deduced SSRC mapping by elimination")

			// Create the mapping
			m.ssrcToUser[ssrc] = userInfo
			m.userToSSRC[userInfo.UserID] = ssrc
			delete(m.unmappedSSRCs, ssrc)
			delete(m.expectedUsers, userInfo.UserID)
		}
	}

	// TODO: Add more sophisticated deduction strategies:
	// - Correlate Discord voice activity indicators with packet timing
	// - Use packet size/frequency patterns
	// - Track speaking patterns unique to users
}

// GetStatistics returns current mapping statistics
func (m *SSRCManager) GetStatistics() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]int{
		"confirmed_mappings": len(m.ssrcToUser),
		"expected_users":     len(m.expectedUsers),
		"unmapped_ssrcs":     len(m.unmappedSSRCs),
	}
}

// Clear resets all mappings
func (m *SSRCManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ssrcToUser = make(map[uint32]*UserInfo)
	m.expectedUsers = make(map[string]*UserInfo)
	m.unmappedSSRCs = make(map[uint32]*SSRCMetadata)
	m.userToSSRC = make(map[string]uint32)
}