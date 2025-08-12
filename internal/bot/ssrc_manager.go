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

	// Track mapping sources for rollback capability
	mappingSources map[uint32]string // SSRC -> "exact" or "confidence"

	// Voice connection details
	guildID   string
	channelID string
	discord   *discordgo.Session
}

// SSRCMetadata tracks information about an unmapped SSRC
type SSRCMetadata struct {
	SSRC              uint32
	FirstSeen         time.Time
	LastSeen          time.Time
	PacketCount       int
	AudioActive       bool
	AveragePacketSize int

	// Enhanced pattern tracking for confidence-based mapping
	SpeakingBursts     []SpeakingBurst // Track speaking segments
	LastSpeakingStart  time.Time       // When current speaking started
	IsSpeaking         bool            // Currently speaking
	TotalSpeakingTime  time.Duration   // Total time spent speaking
	TotalSilenceTime   time.Duration   // Total time spent silent
	LargestPacketSize  int             // Peak packet size seen
	SmallestPacketSize int             // Minimum non-silence packet size
}

// SpeakingBurst represents a continuous speaking period
type SpeakingBurst struct {
	Start         time.Time
	End           time.Time
	Duration      time.Duration
	AvgPacketSize int
	PacketCount   int
}

// MappingCandidate represents a potential SSRC-user mapping with confidence
type MappingCandidate struct {
	SSRC       uint32
	UserID     string
	Username   string
	Nickname   string
	Confidence float64
	Reasons    []string
	Source     string // "exact" or "confidence"
}

// NewSSRCManager creates a new SSRC manager
func NewSSRCManager(discord *discordgo.Session) *SSRCManager {
	return &SSRCManager{
		ssrcToUser:     make(map[uint32]*UserInfo),
		expectedUsers:  make(map[string]*UserInfo),
		unmappedSSRCs:  make(map[uint32]*SSRCMetadata),
		userToSSRC:     make(map[string]uint32),
		mappingSources: make(map[uint32]string),
		discord:        discord,
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

// RegisterAudioPacket tracks an incoming audio packet with enhanced pattern tracking
func (m *SSRCManager) RegisterAudioPacket(ssrc uint32, packetSize int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If already mapped, nothing to do
	if _, exists := m.ssrcToUser[ssrc]; exists {
		return
	}

	now := time.Now()
	isSpeech := packetSize > 3 // Packets > 3 bytes are considered speech

	// Track this unmapped SSRC
	metadata, exists := m.unmappedSSRCs[ssrc]
	if !exists {
		metadata = &SSRCMetadata{
			SSRC:               ssrc,
			FirstSeen:          now,
			SmallestPacketSize: packetSize,
			LargestPacketSize:  packetSize,
		}
		m.unmappedSSRCs[ssrc] = metadata

		logrus.WithFields(logrus.Fields{
			"ssrc":               ssrc,
			"total_unmapped":     len(m.unmappedSSRCs),
			"expected_users":     len(m.expectedUsers),
			"confirmed_mappings": len(m.ssrcToUser),
		}).Info("New unmapped SSRC detected")
	}

	// Update basic metadata
	metadata.LastSeen = now
	metadata.PacketCount++

	// Track packet size patterns
	if isSpeech {
		metadata.AudioActive = true
		if metadata.LargestPacketSize < packetSize {
			metadata.LargestPacketSize = packetSize
		}
		if metadata.SmallestPacketSize == 0 || metadata.SmallestPacketSize > packetSize {
			metadata.SmallestPacketSize = packetSize
		}

		// Update average packet size
		if metadata.AveragePacketSize == 0 {
			metadata.AveragePacketSize = packetSize
		} else {
			metadata.AveragePacketSize = (metadata.AveragePacketSize + packetSize) / 2
		}
	}

	// Track speaking state transitions and bursts
	if isSpeech && !metadata.IsSpeaking {
		// Starting to speak
		metadata.IsSpeaking = true
		metadata.LastSpeakingStart = now
	} else if !isSpeech && metadata.IsSpeaking {
		// Stopped speaking - complete the burst
		if !metadata.LastSpeakingStart.IsZero() {
			duration := now.Sub(metadata.LastSpeakingStart)
			metadata.TotalSpeakingTime += duration

			burst := SpeakingBurst{
				Start:         metadata.LastSpeakingStart,
				End:           now,
				Duration:      duration,
				AvgPacketSize: metadata.AveragePacketSize,
				PacketCount:   1, // Simplified for now
			}
			metadata.SpeakingBursts = append(metadata.SpeakingBursts, burst)

			// Limit burst history to prevent memory growth
			if len(metadata.SpeakingBursts) > 10 {
				metadata.SpeakingBursts = metadata.SpeakingBursts[1:]
			}
		}
		metadata.IsSpeaking = false
	}

	// Attempt deduction with new information
	m.attemptDeduction()
}

// MapSSRC creates a confirmed SSRC mapping from VoiceSpeakingUpdate with rollback support
func (m *SSRCManager) MapSSRC(ssrc uint32, userID string, username string, nickname string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if this SSRC was previously mapped via confidence to a different user
	if existingInfo, exists := m.ssrcToUser[ssrc]; exists {
		if existingInfo.UserID != userID {
			// Rollback: confidence mapping was wrong
			if source, hasSource := m.mappingSources[ssrc]; hasSource && source == "confidence" {
				logrus.WithFields(logrus.Fields{
					"ssrc":             ssrc,
					"old_user_id":      existingInfo.UserID,
					"old_username":     existingInfo.Username,
					"correct_user_id":  userID,
					"correct_username": username,
				}).Warn("Rolling back incorrect confidence-based mapping")

				// Remove old mapping
				delete(m.userToSSRC, existingInfo.UserID)

				// Re-add old user to expected users if not already there
				if _, expectedExists := m.expectedUsers[existingInfo.UserID]; !expectedExists {
					m.expectedUsers[existingInfo.UserID] = existingInfo
				}
			}
		}
	}

	// Check if this user was previously mapped to a different SSRC via confidence
	if existingSSRC, exists := m.userToSSRC[userID]; exists && existingSSRC != ssrc {
		if source, hasSource := m.mappingSources[existingSSRC]; hasSource && source == "confidence" {
			logrus.WithFields(logrus.Fields{
				"user_id":      userID,
				"old_ssrc":     existingSSRC,
				"correct_ssrc": ssrc,
			}).Warn("Rolling back incorrect confidence-based user mapping")

			// Remove old mapping
			delete(m.ssrcToUser, existingSSRC)
			delete(m.mappingSources, existingSSRC)

			// Re-add SSRC as unmapped if it had metadata
			if metadata, exists := m.unmappedSSRCs[existingSSRC]; exists {
				// Metadata should still be there unless cleaned up
				_ = metadata
			}
		}
	}

	// Store the correct mapping
	m.ssrcToUser[ssrc] = &UserInfo{
		UserID:   userID,
		Username: username,
		Nickname: nickname,
	}
	m.userToSSRC[userID] = ssrc
	m.mappingSources[ssrc] = "exact" // Mark as exact mapping

	// Remove from unmapped
	delete(m.unmappedSSRCs, ssrc)

	// Remove from expected users (they're now mapped)
	delete(m.expectedUsers, userID)

	logrus.WithFields(logrus.Fields{
		"ssrc":               ssrc,
		"user_id":            userID,
		"username":           username,
		"method":             "exact",
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

	// Try confidence-based mapping if exact deduction failed
	m.attemptConfidenceBasedMapping()
}

// attemptConfidenceBasedMapping uses pattern analysis for probabilistic mapping
func (m *SSRCManager) attemptConfidenceBasedMapping() {
	// Only attempt if we have multiple unmapped SSRCs and multiple expected users
	if len(m.unmappedSSRCs) < 2 || len(m.expectedUsers) < 2 {
		return
	}

	// Calculate confidence thresholds based on time elapsed
	minDataAge := 15 * time.Second    // Need at least 15s of data
	highConfidenceThreshold := 85.0   // 85% confidence required initially
	mediumConfidenceThreshold := 75.0 // 75% after 30s
	lowConfidenceThreshold := 65.0    // 65% after 60s

	now := time.Now()
	candidates := make([]MappingCandidate, 0)

	// Generate all possible SSRC-user combinations
	for ssrc, metadata := range m.unmappedSSRCs {
		// Skip if not enough data collected
		if now.Sub(metadata.FirstSeen) < minDataAge || !metadata.AudioActive {
			continue
		}

		for userID, userInfo := range m.expectedUsers {
			confidence, reasons := m.calculateMappingConfidence(metadata, userInfo)

			if confidence > 0 {
				candidates = append(candidates, MappingCandidate{
					SSRC:       ssrc,
					UserID:     userID,
					Username:   userInfo.Username,
					Nickname:   userInfo.Nickname,
					Confidence: confidence,
					Reasons:    reasons,
					Source:     "confidence",
				})
			}
		}
	}

	// Sort candidates by confidence (highest first)
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].Confidence > candidates[i].Confidence {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Determine appropriate threshold based on elapsed time
	var threshold float64
	oldestSSRC := time.Time{}
	for _, metadata := range m.unmappedSSRCs {
		if oldestSSRC.IsZero() || metadata.FirstSeen.Before(oldestSSRC) {
			oldestSSRC = metadata.FirstSeen
		}
	}

	age := now.Sub(oldestSSRC)
	if age < 30*time.Second {
		threshold = highConfidenceThreshold
	} else if age < 60*time.Second {
		threshold = mediumConfidenceThreshold
	} else {
		threshold = lowConfidenceThreshold
	}

	// Map the highest confidence candidate if above threshold
	mappedSSRCs := make(map[uint32]bool)
	mappedUsers := make(map[string]bool)

	for _, candidate := range candidates {
		// Skip if already mapped this SSRC or user in this round
		if mappedSSRCs[candidate.SSRC] || mappedUsers[candidate.UserID] {
			continue
		}

		if candidate.Confidence >= threshold {
			logrus.WithFields(logrus.Fields{
				"ssrc":       candidate.SSRC,
				"user_id":    candidate.UserID,
				"username":   candidate.Username,
				"confidence": candidate.Confidence,
				"threshold":  threshold,
				"reasons":    candidate.Reasons,
				"method":     "confidence",
			}).Info("Confidence-based SSRC mapping created")

			// Create the mapping
			m.ssrcToUser[candidate.SSRC] = &UserInfo{
				UserID:   candidate.UserID,
				Username: candidate.Username,
				Nickname: candidate.Nickname,
			}
			m.userToSSRC[candidate.UserID] = candidate.SSRC
			m.mappingSources[candidate.SSRC] = "confidence" // Track as confidence-based mapping
			delete(m.unmappedSSRCs, candidate.SSRC)
			delete(m.expectedUsers, candidate.UserID)

			mappedSSRCs[candidate.SSRC] = true
			mappedUsers[candidate.UserID] = true

			// Only map one at a time to be conservative
			break
		}
	}
}

// calculateMappingConfidence calculates confidence score for SSRC-user mapping
func (m *SSRCManager) calculateMappingConfidence(metadata *SSRCMetadata, userInfo *UserInfo) (float64, []string) {
	var confidence float64
	var reasons []string

	// Base confidence from audio activity
	if metadata.AudioActive && metadata.PacketCount > 10 {
		confidence += 20
		reasons = append(reasons, "confirmed audio activity")
	}

	// Packet consistency patterns (up to 25 points)
	if len(metadata.SpeakingBursts) > 0 {
		confidence += 15
		reasons = append(reasons, "speaking pattern detected")

		// Bonus for consistent speaking patterns
		if len(metadata.SpeakingBursts) >= 3 {
			confidence += 10
			reasons = append(reasons, "consistent speaking pattern")
		}
	}

	// Audio quality indicators (up to 20 points)
	if metadata.AveragePacketSize > 50 && metadata.AveragePacketSize < 500 {
		confidence += 15
		reasons = append(reasons, "normal audio quality")
	}

	// Activity level assessment (up to 25 points)
	now := time.Now()
	totalObservationTime := now.Sub(metadata.FirstSeen)
	if totalObservationTime > 0 {
		activityRatio := float64(metadata.TotalSpeakingTime) / float64(totalObservationTime)

		if activityRatio > 0.1 && activityRatio < 0.8 { // 10-80% speaking time is normal
			confidence += 20
			reasons = append(reasons, "normal activity level")
		} else if activityRatio > 0.05 { // At least some activity
			confidence += 10
			reasons = append(reasons, "some activity detected")
		}
	}

	// Packet size variance suggests natural speech patterns (up to 10 points)
	if metadata.LargestPacketSize > metadata.SmallestPacketSize {
		variance := float64(metadata.LargestPacketSize - metadata.SmallestPacketSize)
		if variance > 20 && variance < 200 { // Natural speech has some variance
			confidence += 10
			reasons = append(reasons, "natural speech patterns")
		}
	}

	return confidence, reasons
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
	m.mappingSources = make(map[uint32]string)
}
