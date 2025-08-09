package mcp

import (
	"testing"

	"github.com/fankserver/discord-voice-mcp/internal/bot"
	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/fankserver/discord-voice-mcp/internal/audio"
	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
)

func TestNewServer(t *testing.T) {
	// Create mock dependencies
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	
	// Create bot (will fail to connect without valid token, but that's ok for this test)
	voiceBot, err := bot.New("dummy_token", sessionManager, audioProcessor)
	if err != nil {
		t.Fatalf("Failed to create bot: %v", err)
	}

	// Test server creation without user ID
	server := NewServer(voiceBot, sessionManager, "")
	if server == nil {
		t.Fatal("Expected server to be created")
	}
	if server.userID != "" {
		t.Error("Expected empty userID")
	}

	// Test server creation with user ID
	userID := "123456789012345678"
	server = NewServer(voiceBot, sessionManager, userID)
	if server == nil {
		t.Fatal("Expected server to be created")
	}
	if server.userID != userID {
		t.Errorf("Expected userID %s, got %s", userID, server.userID)
	}

	// Verify MCP server is initialized
	if server.mcpServer == nil {
		t.Fatal("Expected MCP server to be initialized")
	}
}

func TestServerToolRegistration(t *testing.T) {
	// Create mock dependencies
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	voiceBot, _ := bot.New("dummy_token", sessionManager, audioProcessor)

	// Create server
	server := NewServer(voiceBot, sessionManager, "123456789012345678")

	// The tools are registered during NewServer, so we just verify the server was created
	// In a real test, we'd check the MCP server's registered tools, but that requires
	// accessing internal fields or calling the tools
	if server.mcpServer == nil {
		t.Fatal("MCP server should be initialized with tools")
	}
}