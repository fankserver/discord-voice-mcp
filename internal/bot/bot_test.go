package bot

import (
	"fmt"
	"sync"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/fankserver/discord-voice-mcp/internal/audio"
	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
	"github.com/stretchr/testify/assert"
)

func TestNewBot(t *testing.T) {
	// Create mock dependencies
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)

	// Test bot creation
	bot, err := New("dummy_token", sessionManager, audioProcessor)
	if err != nil {
		t.Fatalf("Failed to create bot: %v", err)
	}

	if bot == nil {
		t.Fatal("Expected bot to be created")
	}

	// Verify initial state
	if bot.followUserID != "" {
		t.Error("Expected empty followUserID initially")
	}
	if bot.autoFollow {
		t.Error("Expected autoFollow to be false initially")
	}
}

func TestFollowUserSettings(t *testing.T) {
	// Create bot
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	bot, _ := New("dummy_token", sessionManager, audioProcessor)

	// Test setting follow user
	userID := "123456789012345678"
	bot.SetFollowUser(userID, true)

	// Verify settings
	gotUserID, gotAutoFollow := bot.GetFollowStatus()
	if gotUserID != userID {
		t.Errorf("Expected userID %s, got %s", userID, gotUserID)
	}
	if !gotAutoFollow {
		t.Error("Expected autoFollow to be true")
	}

	// Test disabling auto-follow
	bot.SetFollowUser(userID, false)
	_, gotAutoFollow = bot.GetFollowStatus()
	if gotAutoFollow {
		t.Error("Expected autoFollow to be false after disabling")
	}
}

func TestBotStatus(t *testing.T) {
	// Create bot
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	bot, _ := New("dummy_token", sessionManager, audioProcessor)

	// Get initial status
	status := bot.GetStatus()
	
	// Check status fields
	if _, ok := status["connected"].(bool); !ok {
		t.Error("Expected connected to be a boolean")
	}
	if inVoice, ok := status["inVoice"].(bool); !ok || inVoice {
		t.Error("Expected inVoice to be false initially")
	}
	
	// Should not have guild/channel IDs when not in voice
	if _, hasGuildID := status["guildID"]; hasGuildID {
		t.Error("Should not have guildID when not in voice")
	}
	if _, hasChannelID := status["channelID"]; hasChannelID {
		t.Error("Should not have channelID when not in voice")
	}
}

func TestGetUserBySSRC(t *testing.T) {
	// Create a bot instance
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	
	bot, err := New("test-token", sessionManager, audioProcessor)
	assert.NoError(t, err)
	assert.NotNil(t, bot)

	// Test with no mappings - should return SSRC as fallback
	ssrc := uint32(12345)
	userID, username, nickname := bot.GetUserBySSRC(ssrc)
	assert.Equal(t, "12345", userID)
	assert.Equal(t, "12345", username)
	assert.Equal(t, "12345", nickname)

	// Add a user mapping
	bot.ssrcToUser[ssrc] = &UserInfo{
		UserID:   "user-123",
		Username: "TestUser",
		Nickname: "TestNick",
	}

	// Test with existing mapping
	userID, username, nickname = bot.GetUserBySSRC(ssrc)
	assert.Equal(t, "user-123", userID)
	assert.Equal(t, "TestUser", username)
	assert.Equal(t, "TestNick", nickname)

	// Test with different SSRC - should return fallback
	differentSSRC := uint32(67890)
	userID, username, nickname = bot.GetUserBySSRC(differentSSRC)
	assert.Equal(t, "67890", userID)
	assert.Equal(t, "67890", username)
	assert.Equal(t, "67890", nickname)
}

func TestSSRCMappingConcurrency(t *testing.T) {
	// Test concurrent access to SSRC mappings
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	
	bot, err := New("test-token", sessionManager, audioProcessor)
	assert.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ssrc := uint32(1000 + id)
			
			// Simulate adding user mapping (normally done in voiceSpeakingUpdate)
			bot.mu.Lock()
			bot.ssrcToUser[ssrc] = &UserInfo{
				UserID:   fmt.Sprintf("user-%d", id),
				Username: fmt.Sprintf("User%d", id),
				Nickname: fmt.Sprintf("Nick%d", id),
			}
			bot.mu.Unlock()
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ssrc := uint32(1000 + id)
			
			// Read user info
			userID, _, _ := bot.GetUserBySSRC(ssrc)
			// May get fallback or actual value depending on timing
			assert.NotEmpty(t, userID)
		}(i)
	}

	wg.Wait()

	// Verify all mappings were added
	bot.mu.Lock()
	assert.Len(t, bot.ssrcToUser, numGoroutines)
	bot.mu.Unlock()
}

func TestLeaveChannelClearsSSRCMappings(t *testing.T) {
	// Test that leaving a channel clears SSRC mappings
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	
	bot, err := New("test-token", sessionManager, audioProcessor)
	assert.NoError(t, err)

	// Add some SSRC mappings
	bot.mu.Lock()
	bot.ssrcToUser[1001] = &UserInfo{UserID: "user1", Username: "User1", Nickname: "Nick1"}
	bot.ssrcToUser[1002] = &UserInfo{UserID: "user2", Username: "User2", Nickname: "Nick2"}
	bot.ssrcToUser[1003] = &UserInfo{UserID: "user3", Username: "User3", Nickname: "Nick3"}
	bot.mu.Unlock()

	// Verify mappings exist
	bot.mu.Lock()
	assert.Len(t, bot.ssrcToUser, 3)
	bot.mu.Unlock()

	// Leave channel (should clear mappings)
	bot.LeaveChannel()

	// Verify mappings were cleared
	bot.mu.Lock()
	assert.Empty(t, bot.ssrcToUser)
	bot.mu.Unlock()
}

func TestFindUserVoiceChannel(t *testing.T) {
	// Test finding which voice channel a user is in
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	
	bot, err := New("test-token", sessionManager, audioProcessor)
	assert.NoError(t, err)
	
	// Initialize bot's Discord state
	bot.discord.State.Guilds = []*discordgo.Guild{
		{ID: "guild1"},
		{ID: "guild2"},
	}
	
	// Test 1: User not in any voice channel
	guildID, channelID, err := bot.FindUserVoiceChannel("user-not-in-voice")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in any voice channel")
	assert.Empty(t, guildID)
	assert.Empty(t, channelID)
	
	// Test 2: Add user to voice channel in guild1
	bot.discord.State.Guilds[0].VoiceStates = []*discordgo.VoiceState{
		{
			UserID:    "user-in-voice",
			ChannelID: "voice-channel-1",
		},
	}
	
	guildID, channelID, err = bot.FindUserVoiceChannel("user-in-voice")
	assert.NoError(t, err)
	assert.Equal(t, "guild1", guildID)
	assert.Equal(t, "voice-channel-1", channelID)
	
	// Test 3: User in guild2
	bot.discord.State.Guilds[1].VoiceStates = []*discordgo.VoiceState{
		{
			UserID:    "user-in-guild2",
			ChannelID: "voice-channel-2",
		},
	}
	
	guildID, channelID, err = bot.FindUserVoiceChannel("user-in-guild2")
	assert.NoError(t, err)
	assert.Equal(t, "guild2", guildID)
	assert.Equal(t, "voice-channel-2", channelID)
	
	// Test 4: User with empty channel ID (not actually in voice)
	bot.discord.State.Guilds[0].VoiceStates = append(bot.discord.State.Guilds[0].VoiceStates, &discordgo.VoiceState{
		UserID:    "user-no-channel",
		ChannelID: "", // Empty channel ID means not in voice
	})
	
	_, _, err = bot.FindUserVoiceChannel("user-no-channel")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in any voice channel")
}

func TestJoinUserChannel(t *testing.T) {
	// Test the logic for finding and joining a user's channel
	// Note: We can't actually join without a valid Discord connection,
	// so we'll focus on testing the FindUserVoiceChannel logic
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	
	bot, err := New("test-token", sessionManager, audioProcessor)
	assert.NoError(t, err)
	
	// Initialize bot's Discord state
	bot.discord.State.Guilds = []*discordgo.Guild{
		{
			ID: "guild1",
			VoiceStates: []*discordgo.VoiceState{
				{
					UserID:    "target-user",
					ChannelID: "target-channel",
				},
			},
		},
	}
	
	// Test 1: User not in any channel
	err = bot.JoinUserChannel("non-existent-user")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in any voice channel")
	
	// Test 2: Test FindUserVoiceChannel directly for a user in voice
	guildID, channelID, err := bot.FindUserVoiceChannel("target-user")
	assert.NoError(t, err)
	assert.Equal(t, "guild1", guildID)
	assert.Equal(t, "target-channel", channelID)
	
	// Note: We can't test the actual join without a valid Discord connection
	// as it would require mocking the Discord API
}