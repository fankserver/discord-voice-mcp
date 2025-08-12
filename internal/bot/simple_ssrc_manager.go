package bot

import (
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
)

// SimpleSSRCManager provides deterministic SSRC-to-user mapping using ONLY VoiceSpeakingUpdate events
// This is the clean, Discord-API-compliant approach that never guesses or deduces mappings
type SimpleSSRCManager struct {
	mu sync.RWMutex
	
	// Exact mappings from VoiceSpeakingUpdate events ONLY
	ssrcToUser map[uint32]*UserInfo
	userToSSRC map[string]uint32
	
	// Guild and channel context
	guildID   string
	channelID string
}

// NewSimpleSSRCManager creates a new simple SSRC manager
func NewSimpleSSRCManager() *SimpleSSRCManager {
	return &SimpleSSRCManager{
		ssrcToUser: make(map[uint32]*UserInfo),
		userToSSRC: make(map[string]uint32),
	}
}

// SetChannel updates the current channel context
func (m *SimpleSSRCManager) SetChannel(guildID, channelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.guildID = guildID
	m.channelID = channelID
	
	// Clear mappings when changing channels (start fresh)
	m.ssrcToUser = make(map[uint32]*UserInfo)
	m.userToSSRC = make(map[string]uint32)
	
	logrus.WithFields(logrus.Fields{
		"guild_id":   guildID,
		"channel_id": channelID,
	}).Info("Simple SSRC manager initialized for new channel")
}

// MapSSRC creates a confirmed SSRC mapping from VoiceSpeakingUpdate events ONLY
// This is the ONLY way to create mappings in the deterministic approach
func (m *SimpleSSRCManager) MapSSRC(ssrc uint32, userID string, username string, nickname string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Store the exact mapping
	userInfo := &UserInfo{
		UserID:   userID,
		Username: username,
		Nickname: nickname,
	}
	
	m.ssrcToUser[ssrc] = userInfo
	m.userToSSRC[userID] = ssrc
	
	logrus.WithFields(logrus.Fields{
		"ssrc":     ssrc,
		"user_id":  userID,
		"username": username,
		"nickname": nickname,
		"method":   "voicespeakingupdate",
	}).Info("SSRC mapped to user via VoiceSpeakingUpdate event")
}

// GetUserBySSRC returns user info for an SSRC - ONLY exact mappings, no guessing
func (m *SimpleSSRCManager) GetUserBySSRC(ssrc uint32) (userID, username, nickname string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Check for exact mapping from VoiceSpeakingUpdate event
	if info, exists := m.ssrcToUser[ssrc]; exists {
		return info.UserID, info.Username, info.Nickname
	}
	
	// No exact mapping available - return unknown
	// This is the deterministic approach: we never guess
	ssrcStr := fmt.Sprintf("%d", ssrc)
	return ssrcStr, fmt.Sprintf("Unknown-%s", ssrcStr), fmt.Sprintf("Unknown-%s", ssrcStr)
}

// GetStatistics returns current mapping statistics
func (m *SimpleSSRCManager) GetStatistics() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return map[string]int{
		"exact_mappings": len(m.ssrcToUser),
	}
}

// Clear resets all mappings
func (m *SimpleSSRCManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.ssrcToUser = make(map[uint32]*UserInfo)
	m.userToSSRC = make(map[string]uint32)
	
	logrus.Info("Simple SSRC manager cleared")
}

// RegisterAudioPacket is a no-op in the deterministic approach
// We don't track audio patterns or try to deduce mappings
func (m *SimpleSSRCManager) RegisterAudioPacket(ssrc uint32, packetSize int) {
	// Intentionally empty - deterministic approach doesn't analyze audio patterns
}