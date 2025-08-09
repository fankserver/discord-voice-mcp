package mcp

import (
	"context"
	"os"
	"testing"

	"github.com/fankserver/discord-voice-mcp/internal/audio"
	"github.com/fankserver/discord-voice-mcp/internal/bot"
	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestHandleGetTranscriptNonExistentSession(t *testing.T) {
	// Setup
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	voiceBot, _ := bot.New("test-token", sessionManager, audioProcessor)
	server := NewServer(voiceBot, sessionManager, "test-user-id")

	// Test getting transcript for non-existent session
	ctx := context.Background()
	sess := &mcp.ServerSession{}
	params := &mcp.CallToolParamsFor[GetTranscriptInput]{
		Arguments: GetTranscriptInput{
			SessionID: "non-existent-session",
		},
	}

	result, err := server.handleGetTranscript(ctx, sess, params)

	// Should return an error for non-existent session
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "session not found")
}

func TestHandleExportSessionNonExistent(t *testing.T) {
	// Setup
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	voiceBot, _ := bot.New("test-token", sessionManager, audioProcessor)
	server := NewServer(voiceBot, sessionManager, "test-user-id")

	// Test exporting non-existent session
	ctx := context.Background()
	sess := &mcp.ServerSession{}
	params := &mcp.CallToolParamsFor[ExportSessionInput]{
		Arguments: ExportSessionInput{
			SessionID: "non-existent-session",
		},
	}

	result, err := server.handleExportSession(ctx, sess, params)

	// Should return an error for non-existent session
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to export session")

	// Clean up any exports directory that might have been created
	_ = os.RemoveAll("exports")
}

func TestHandleJoinMyVoiceChannelNoUserConfigured(t *testing.T) {
	// Setup with empty user ID
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	voiceBot, _ := bot.New("test-token", sessionManager, audioProcessor)
	server := NewServer(voiceBot, sessionManager, "") // Empty user ID

	// Test joining "my" channel with no user configured
	ctx := context.Background()
	sess := &mcp.ServerSession{}
	params := &mcp.CallToolParamsFor[EmptyInput]{
		Arguments: EmptyInput{},
	}

	result, err := server.handleJoinMyVoiceChannel(ctx, sess, params)

	// Should return a message about no user configured
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Content, 1)
	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "No user ID configured")
}

func TestHandleFollowMeNoUserConfigured(t *testing.T) {
	// Setup with empty user ID
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	voiceBot, _ := bot.New("test-token", sessionManager, audioProcessor)
	server := NewServer(voiceBot, sessionManager, "") // Empty user ID

	// Test follow me with no user configured
	ctx := context.Background()
	sess := &mcp.ServerSession{}
	params := &mcp.CallToolParamsFor[FollowMeInput]{
		Arguments: FollowMeInput{
			Enabled: true,
		},
	}

	result, err := server.handleFollowMe(ctx, sess, params)

	// Should return a message about no user configured
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Content, 1)
	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "No user ID configured")
}

func TestHandleListSessionsEmpty(t *testing.T) {
	// Setup
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	voiceBot, _ := bot.New("test-token", sessionManager, audioProcessor)
	server := NewServer(voiceBot, sessionManager, "test-user-id")

	// Test listing sessions when none exist
	ctx := context.Background()
	sess := &mcp.ServerSession{}
	params := &mcp.CallToolParamsFor[EmptyInput]{
		Arguments: EmptyInput{},
	}

	result, err := server.handleListSessions(ctx, sess, params)

	// Should succeed with "No sessions found" message
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Content, 1)
	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "No sessions found")
}

func TestHandleListSessionsWithData(t *testing.T) {
	// Setup
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	voiceBot, _ := bot.New("test-token", sessionManager, audioProcessor)
	server := NewServer(voiceBot, sessionManager, "test-user-id")

	// Create some sessions
	session1 := sessionManager.CreateSession("guild1", "channel1")
	session2 := sessionManager.CreateSession("guild2", "channel2")

	// Add data to sessions
	_ = sessionManager.AddTranscript(session1, "user1", "User1", "Message 1")
	_ = sessionManager.AddPendingTranscription(session2, "user2", "User2", 5.0)

	// Test listing sessions
	ctx := context.Background()
	sess := &mcp.ServerSession{}
	params := &mcp.CallToolParamsFor[EmptyInput]{
		Arguments: EmptyInput{},
	}

	result, err := server.handleListSessions(ctx, sess, params)

	// Should succeed and show both sessions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Content, 1)
	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "Found 2 session(s)")
	assert.Contains(t, textContent.Text, session1)
	assert.Contains(t, textContent.Text, session2)
	assert.Contains(t, textContent.Text, "1 pending") // Session 2 has pending
}

func TestHandleGetBotStatus(t *testing.T) {
	// Setup
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	voiceBot, _ := bot.New("test-token", sessionManager, audioProcessor)
	server := NewServer(voiceBot, sessionManager, "test-user-id")

	// Set follow status
	voiceBot.SetFollowUser("test-user-id", true)

	// Test getting bot status
	ctx := context.Background()
	sess := &mcp.ServerSession{}
	params := &mcp.CallToolParamsFor[EmptyInput]{
		Arguments: EmptyInput{},
	}

	result, err := server.handleGetBotStatus(ctx, sess, params)

	// Should succeed
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Content, 1)
	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "Bot Status")
	assert.Contains(t, textContent.Text, "Connected:")
	assert.Contains(t, textContent.Text, "In Voice:")
	assert.Contains(t, textContent.Text, "Following User: test-user-id")
	assert.Contains(t, textContent.Text, "Auto-Follow: true")
}

func TestHandleGetTranscriptWithPendingTranscriptions(t *testing.T) {
	// Setup
	sessionManager := session.NewManager()
	trans := &transcriber.MockTranscriber{}
	audioProcessor := audio.NewProcessor(trans)
	voiceBot, _ := bot.New("test-token", sessionManager, audioProcessor)
	server := NewServer(voiceBot, sessionManager, "test-user-id")

	// Create session with both completed and pending transcriptions
	sessionID := sessionManager.CreateSession("guild", "channel")
	_ = sessionManager.AddTranscript(sessionID, "user1", "User1", "Completed message")
	_ = sessionManager.AddPendingTranscription(sessionID, "user2", "User2", 3.5)

	// Test getting transcript
	ctx := context.Background()
	sess := &mcp.ServerSession{}
	params := &mcp.CallToolParamsFor[GetTranscriptInput]{
		Arguments: GetTranscriptInput{
			SessionID: sessionID,
		},
	}

	result, err := server.handleGetTranscript(ctx, sess, params)

	// Should succeed and show both completed and pending
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Content, 1)
	textContent, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "Completed message")
	assert.Contains(t, textContent.Text, "Pending Transcriptions")
	assert.Contains(t, textContent.Text, "User2")
	assert.Contains(t, textContent.Text, "3.5s")
}
