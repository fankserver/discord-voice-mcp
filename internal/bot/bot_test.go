package bot

import (
	"testing"

	"github.com/fankserver/discord-voice-mcp/internal/audio"
	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
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