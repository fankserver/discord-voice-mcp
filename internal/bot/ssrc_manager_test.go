package bot

import (
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
)

func TestSSRCManagerBasicMapping(t *testing.T) {
	// Create a mock discord session
	discord := &discordgo.Session{}
	manager := NewSSRCManager(discord)

	// Test initial state
	stats := manager.GetStatistics()
	assert.Equal(t, 0, stats["confirmed_mappings"])
	assert.Equal(t, 0, stats["expected_users"])
	assert.Equal(t, 0, stats["unmapped_ssrcs"])

	// Test unmapped SSRC lookup
	userID, username, nickname := manager.GetUserBySSRC(12345)
	assert.Equal(t, "12345", userID)
	assert.Equal(t, "Unknown-12345", username)
	assert.Equal(t, "Unknown-12345", nickname)

	// Map an SSRC
	manager.MapSSRC(12345, "user123", "TestUser", "TestNick")

	// Test mapped SSRC lookup
	userID, username, nickname = manager.GetUserBySSRC(12345)
	assert.Equal(t, "user123", userID)
	assert.Equal(t, "TestUser", username)
	assert.Equal(t, "TestNick", nickname)

	// Verify statistics
	stats = manager.GetStatistics()
	assert.Equal(t, 1, stats["confirmed_mappings"])
}

func TestSSRCManagerAudioPacketTracking(t *testing.T) {
	discord := &discordgo.Session{}
	manager := NewSSRCManager(discord)

	// Register audio packets for unmapped SSRC
	ssrc := uint32(54321)
	manager.RegisterAudioPacket(ssrc, 100) // Non-silence packet
	manager.RegisterAudioPacket(ssrc, 2)   // Silence packet
	manager.RegisterAudioPacket(ssrc, 150) // Another audio packet

	// Verify SSRC is tracked as unmapped
	stats := manager.GetStatistics()
	assert.Equal(t, 1, stats["unmapped_ssrcs"])

	// Map the SSRC
	manager.MapSSRC(ssrc, "audio-user", "AudioUser", "AudioNick")

	// Verify it's no longer unmapped
	stats = manager.GetStatistics()
	assert.Equal(t, 0, stats["unmapped_ssrcs"])
	assert.Equal(t, 1, stats["confirmed_mappings"])
}

func TestSSRCManagerSingleDeduction(t *testing.T) {
	discord := &discordgo.Session{}
	manager := NewSSRCManager(discord)

	// Manually add an expected user (simulating PopulateExpectedUsers)
	manager.mu.Lock()
	manager.expectedUsers["user456"] = &UserInfo{
		UserID:   "user456",
		Username: "ExpectedUser",
		Nickname: "ExpectedNick",
	}
	manager.mu.Unlock()

	// Register audio activity for a single SSRC
	ssrc := uint32(99999)
	manager.RegisterAudioPacket(ssrc, 100) // Audio packet (not silence)

	// Since we have 1 unmapped SSRC with audio activity and 1 expected user, 
	// attemptDeduction should have created the mapping automatically
	userID, username, nickname := manager.GetUserBySSRC(ssrc)
	
	// Should return the expected user info (deduction created real mapping)
	assert.Equal(t, "user456", userID)
	assert.Equal(t, "ExpectedUser", username)
	assert.Equal(t, "ExpectedNick", nickname)

	// Verify the mapping was actually created
	stats := manager.GetStatistics()
	assert.Equal(t, 1, stats["confirmed_mappings"])
	assert.Equal(t, 0, stats["expected_users"]) // Should be cleared after mapping
}

func TestSSRCManagerNoDeductionWithoutAudio(t *testing.T) {
	discord := &discordgo.Session{}
	manager := NewSSRCManager(discord)

	// Add expected user
	manager.mu.Lock()
	manager.expectedUsers["user789"] = &UserInfo{
		UserID:   "user789",
		Username: "SilentUser",
		Nickname: "SilentNick",
	}
	manager.mu.Unlock()

	// Register only silence packets (no real audio)
	ssrc := uint32(88888)
	manager.RegisterAudioPacket(ssrc, 2) // Silence
	manager.RegisterAudioPacket(ssrc, 1) // Silence

	// Should NOT deduce mapping without real audio activity
	userID, username, nickname := manager.GetUserBySSRC(ssrc)
	assert.Equal(t, "88888", userID)
	assert.Equal(t, "Unknown-88888", username)
	assert.Equal(t, "Unknown-88888", nickname)
}

func TestSSRCManagerMultipleUsersNoDeduction(t *testing.T) {
	discord := &discordgo.Session{}
	manager := NewSSRCManager(discord)

	// Add multiple expected users
	manager.mu.Lock()
	manager.expectedUsers["user1"] = &UserInfo{UserID: "user1", Username: "User1", Nickname: "Nick1"}
	manager.expectedUsers["user2"] = &UserInfo{UserID: "user2", Username: "User2", Nickname: "Nick2"}
	manager.mu.Unlock()

	// Register audio for multiple SSRCs
	manager.RegisterAudioPacket(11111, 100)
	manager.RegisterAudioPacket(22222, 100)

	// Should NOT deduce when multiple possibilities exist
	userID1, username1, _ := manager.GetUserBySSRC(11111)
	userID2, username2, _ := manager.GetUserBySSRC(22222)

	assert.Equal(t, "11111", userID1)
	assert.Equal(t, "Unknown-11111", username1)
	assert.Equal(t, "22222", userID2)
	assert.Equal(t, "Unknown-22222", username2)
}

func TestSSRCManagerClearState(t *testing.T) {
	discord := &discordgo.Session{}
	manager := NewSSRCManager(discord)

	// Add various mappings
	manager.MapSSRC(1001, "user1", "User1", "Nick1")
	manager.RegisterAudioPacket(2002, 100)
	
	manager.mu.Lock()
	manager.expectedUsers["user3"] = &UserInfo{UserID: "user3", Username: "User3", Nickname: "Nick3"}
	manager.mu.Unlock()

	// Verify state exists
	stats := manager.GetStatistics()
	assert.Greater(t, stats["confirmed_mappings"], 0)

	// Clear and verify empty state
	manager.Clear()
	stats = manager.GetStatistics()
	assert.Equal(t, 0, stats["confirmed_mappings"])
	assert.Equal(t, 0, stats["expected_users"])
	assert.Equal(t, 0, stats["unmapped_ssrcs"])
}

func TestSSRCManagerSetChannel(t *testing.T) {
	discord := &discordgo.Session{}
	manager := NewSSRCManager(discord)

	// Add some state
	manager.MapSSRC(1001, "user1", "User1", "Nick1")
	manager.RegisterAudioPacket(2002, 100)

	// Set new channel (should clear expected users and unmapped SSRCs)
	manager.SetChannel("guild123", "channel456")

	stats := manager.GetStatistics()
	assert.Equal(t, 1, stats["confirmed_mappings"]) // Confirmed mappings persist
	assert.Equal(t, 0, stats["expected_users"])     // Expected users cleared
	assert.Equal(t, 0, stats["unmapped_ssrcs"])     // Unmapped SSRCs cleared
}

func TestSSRCManagerConcurrentAccess(t *testing.T) {
	discord := &discordgo.Session{}
	manager := NewSSRCManager(discord)

	// Test concurrent operations
	go func() {
		for i := 0; i < 100; i++ {
			manager.RegisterAudioPacket(uint32(i), 100)
		}
	}()

	go func() {
		for i := 0; i < 50; i++ {
			manager.MapSSRC(uint32(i), "user", "User", "Nick")
		}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			manager.GetUserBySSRC(uint32(i))
		}
	}()

	// Give goroutines time to complete
	time.Sleep(100 * time.Millisecond)

	// Should not panic and should have some mappings
	stats := manager.GetStatistics()
	assert.GreaterOrEqual(t, stats["confirmed_mappings"], 0)
}