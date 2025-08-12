package bot

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSimpleSSRCManager(t *testing.T) {
	manager := NewSimpleSSRCManager()

	assert.NotNil(t, manager)
	assert.NotNil(t, manager.ssrcToUser)
	assert.NotNil(t, manager.userToSSRC)
	assert.Empty(t, manager.ssrcToUser)
	assert.Empty(t, manager.userToSSRC)
	assert.Empty(t, manager.guildID)
	assert.Empty(t, manager.channelID)
}

func TestSetChannel(t *testing.T) {
	manager := NewSimpleSSRCManager()

	// Test setting channel
	guildID := "guild-123"
	channelID := "channel-456"
	manager.SetChannel(guildID, channelID)

	assert.Equal(t, guildID, manager.guildID)
	assert.Equal(t, channelID, manager.channelID)
	assert.Empty(t, manager.ssrcToUser)
	assert.Empty(t, manager.userToSSRC)
}

func TestSetChannelClearsMappings(t *testing.T) {
	manager := NewSimpleSSRCManager()

	// Add some mappings
	manager.MapSSRC(12345, "user-123", "TestUser", "TestNick")
	assert.Len(t, manager.ssrcToUser, 1)

	// Set new channel should clear mappings
	manager.SetChannel("new-guild", "new-channel")
	assert.Empty(t, manager.ssrcToUser)
	assert.Empty(t, manager.userToSSRC)
}

func TestMapSSRC(t *testing.T) {
	manager := NewSimpleSSRCManager()

	ssrc := uint32(12345)
	userID := "user-123"
	username := "TestUser"
	nickname := "TestNick"

	manager.MapSSRC(ssrc, userID, username, nickname)

	// Check mapping exists
	userInfo, exists := manager.ssrcToUser[ssrc]
	assert.True(t, exists)
	assert.NotNil(t, userInfo)
	assert.Equal(t, userID, userInfo.UserID)
	assert.Equal(t, username, userInfo.Username)
	assert.Equal(t, nickname, userInfo.Nickname)

	// Check reverse mapping
	mappedSSRC, exists := manager.userToSSRC[userID]
	assert.True(t, exists)
	assert.Equal(t, ssrc, mappedSSRC)
}

func TestMapSSRCOverride(t *testing.T) {
	manager := NewSimpleSSRCManager()

	ssrc := uint32(12345)
	userID := "user-123"

	// First mapping
	manager.MapSSRC(ssrc, userID, "OldName", "OldNick")

	// Override with new mapping
	manager.MapSSRC(ssrc, userID, "NewName", "NewNick")

	userInfo := manager.ssrcToUser[ssrc]
	assert.Equal(t, "NewName", userInfo.Username)
	assert.Equal(t, "NewNick", userInfo.Nickname)
}

func TestGetUserBySSRCExisting(t *testing.T) {
	manager := NewSimpleSSRCManager()

	ssrc := uint32(12345)
	userID := "user-123"
	username := "TestUser"
	nickname := "TestNick"

	manager.MapSSRC(ssrc, userID, username, nickname)

	// Get existing mapping
	gotUserID, gotUsername, gotNickname := manager.GetUserBySSRC(ssrc)
	assert.Equal(t, userID, gotUserID)
	assert.Equal(t, username, gotUsername)
	assert.Equal(t, nickname, gotNickname)
}

func TestGetUserBySSRCNonExisting(t *testing.T) {
	manager := NewSimpleSSRCManager()

	ssrc := uint32(99999)

	// Get non-existing mapping
	gotUserID, gotUsername, gotNickname := manager.GetUserBySSRC(ssrc)

	expectedUserID := "99999"
	expectedUsername := "Unknown-99999"
	expectedNickname := "Unknown-99999"

	assert.Equal(t, expectedUserID, gotUserID)
	assert.Equal(t, expectedUsername, gotUsername)
	assert.Equal(t, expectedNickname, gotNickname)
}

func TestGetStatistics(t *testing.T) {
	manager := NewSimpleSSRCManager()

	// Initially no mappings
	stats := manager.GetStatistics()
	assert.Equal(t, 0, stats["exact_mappings"])

	// Add some mappings
	manager.MapSSRC(12345, "user-1", "User1", "Nick1")
	manager.MapSSRC(67890, "user-2", "User2", "Nick2")

	stats = manager.GetStatistics()
	assert.Equal(t, 2, stats["exact_mappings"])
}

func TestClear(t *testing.T) {
	manager := NewSimpleSSRCManager()

	// Add some mappings
	manager.MapSSRC(12345, "user-1", "User1", "Nick1")
	manager.MapSSRC(67890, "user-2", "User2", "Nick2")
	assert.Len(t, manager.ssrcToUser, 2)
	assert.Len(t, manager.userToSSRC, 2)

	// Clear mappings
	manager.Clear()
	assert.Empty(t, manager.ssrcToUser)
	assert.Empty(t, manager.userToSSRC)

	stats := manager.GetStatistics()
	assert.Equal(t, 0, stats["exact_mappings"])
}

func TestRegisterAudioPacketNoOp(t *testing.T) {
	manager := NewSimpleSSRCManager()

	// RegisterAudioPacket should be a no-op
	manager.RegisterAudioPacket(12345, 1024)

	// Should not create any mappings
	assert.Empty(t, manager.ssrcToUser)
	assert.Empty(t, manager.userToSSRC)

	stats := manager.GetStatistics()
	assert.Equal(t, 0, stats["exact_mappings"])
}

func TestConcurrentAccess(t *testing.T) {
	manager := NewSimpleSSRCManager()

	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Start multiple goroutines performing different operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				ssrc := uint32(id*1000 + j)
				userID := fmt.Sprintf("user-%d-%d", id, j)
				username := fmt.Sprintf("User%d_%d", id, j)
				nickname := fmt.Sprintf("Nick%d_%d", id, j)

				// Map SSRC
				manager.MapSSRC(ssrc, userID, username, nickname)

				// Get user by SSRC
				_, _, _ = manager.GetUserBySSRC(ssrc)

				// Get statistics
				_ = manager.GetStatistics()

				// RegisterAudioPacket (no-op)
				manager.RegisterAudioPacket(ssrc, 1024)
			}
		}(i)
	}

	wg.Wait()

	// Verify we have all expected mappings
	stats := manager.GetStatistics()
	assert.Equal(t, numGoroutines*numOperations, stats["exact_mappings"])
}

func TestConcurrentSetChannelAndMap(t *testing.T) {
	manager := NewSimpleSSRCManager()

	const numGoroutines = 5
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// One goroutine sets channel repeatedly
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			manager.SetChannel(fmt.Sprintf("guild-%d", i), fmt.Sprintf("channel-%d", i))
		}
	}()

	// Other goroutines map SSRCs
	for i := 1; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				ssrc := uint32(id*100 + j)
				userID := fmt.Sprintf("user-%d-%d", id, j)
				manager.MapSSRC(ssrc, userID, fmt.Sprintf("User%d", id), fmt.Sprintf("Nick%d", id))
			}
		}(i)
	}

	wg.Wait()

	// Should not panic and should have some mappings (depends on timing)
	stats := manager.GetStatistics()
	assert.GreaterOrEqual(t, stats["exact_mappings"], 0)
}

func TestMapSSRCUserSwitch(t *testing.T) {
	manager := NewSimpleSSRCManager()

	ssrc1 := uint32(12345)
	ssrc2 := uint32(67890)
	userID := "user-123"

	// Map user to first SSRC
	manager.MapSSRC(ssrc1, userID, "TestUser", "TestNick")

	// Map same user to different SSRC
	manager.MapSSRC(ssrc2, userID, "TestUser", "TestNick")

	// User should now map to second SSRC
	assert.Equal(t, ssrc2, manager.userToSSRC[userID])

	// Both SSRCs should map to the user
	userInfo1 := manager.ssrcToUser[ssrc1]
	userInfo2 := manager.ssrcToUser[ssrc2]
	assert.NotNil(t, userInfo1)
	assert.NotNil(t, userInfo2)
	assert.Equal(t, userID, userInfo1.UserID)
	assert.Equal(t, userID, userInfo2.UserID)
}
