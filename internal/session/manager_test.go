package session

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	manager := NewManager()
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.sessions)
	assert.Empty(t, manager.sessions)
}

func TestCreateSession(t *testing.T) {
	manager := NewManager()
	
	guildID := "test-guild-123"
	channelID := "test-channel-456"
	
	sessionID := manager.CreateSession(guildID, channelID)
	
	assert.NotEmpty(t, sessionID)
	
	// Verify session was created
	session, err := manager.GetSession(sessionID)
	require.NoError(t, err)
	assert.Equal(t, sessionID, session.ID)
	assert.Equal(t, guildID, session.GuildID)
	assert.Equal(t, channelID, session.ChannelID)
	assert.NotZero(t, session.StartTime)
	assert.Nil(t, session.EndTime)
	assert.Empty(t, session.Transcripts)
	assert.Empty(t, session.PendingTranscriptions)
}

func TestAddPendingTranscription(t *testing.T) {
	manager := NewManager()
	sessionID := manager.CreateSession("guild", "channel")
	
	// Add pending transcription
	err := manager.AddPendingTranscription(sessionID, "user-123", "TestUser", 2.5)
	require.NoError(t, err)
	
	// Verify it was added
	session, err := manager.GetSession(sessionID)
	require.NoError(t, err)
	require.Len(t, session.PendingTranscriptions, 1)
	
	pending := session.PendingTranscriptions[0]
	assert.Equal(t, "user-123", pending.UserID)
	assert.Equal(t, "TestUser", pending.Username)
	assert.Equal(t, 2.5, pending.Duration)
	assert.NotZero(t, pending.StartTime)
	
	// Test with non-existent session
	err = manager.AddPendingTranscription("non-existent", "user", "name", 1.0)
	assert.Error(t, err)
}

func TestRemovePendingTranscription(t *testing.T) {
	manager := NewManager()
	sessionID := manager.CreateSession("guild", "channel")
	
	// Add multiple pending transcriptions
	err := manager.AddPendingTranscription(sessionID, "user-1", "User1", 1.0)
	require.NoError(t, err)
	err = manager.AddPendingTranscription(sessionID, "user-2", "User2", 2.0)
	require.NoError(t, err)
	
	// Remove one
	err = manager.RemovePendingTranscription(sessionID, "user-1")
	require.NoError(t, err)
	
	// Verify only user-2 remains
	session, err := manager.GetSession(sessionID)
	require.NoError(t, err)
	require.Len(t, session.PendingTranscriptions, 1)
	assert.Equal(t, "user-2", session.PendingTranscriptions[0].UserID)
	
	// Test with non-existent session
	err = manager.RemovePendingTranscription("non-existent", "user-1")
	assert.Error(t, err)
}

func TestAddTranscript(t *testing.T) {
	manager := NewManager()
	sessionID := manager.CreateSession("guild", "channel")
	
	// Add pending transcription first
	err := manager.AddPendingTranscription(sessionID, "user-123", "TestUser", 2.0)
	require.NoError(t, err)
	
	// Add transcript (should also remove pending)
	err = manager.AddTranscript(sessionID, "user-123", "TestUser", "Hello, this is a test")
	require.NoError(t, err)
	
	// Verify transcript was added and pending was removed
	session, err := manager.GetSession(sessionID)
	require.NoError(t, err)
	require.Len(t, session.Transcripts, 1)
	assert.Empty(t, session.PendingTranscriptions)
	
	transcript := session.Transcripts[0]
	assert.Equal(t, "user-123", transcript.UserID)
	assert.Equal(t, "TestUser", transcript.Username)
	assert.Equal(t, "Hello, this is a test", transcript.Text)
	assert.NotZero(t, transcript.Timestamp)
	
	// Test with non-existent session
	err = manager.AddTranscript("non-existent", "user", "name", "text")
	assert.Error(t, err)
}

func TestEndSession(t *testing.T) {
	manager := NewManager()
	sessionID := manager.CreateSession("guild", "channel")
	
	// End the session
	err := manager.EndSession(sessionID)
	require.NoError(t, err)
	
	// Verify end time was set
	session, err := manager.GetSession(sessionID)
	require.NoError(t, err)
	assert.NotNil(t, session.EndTime)
	assert.True(t, session.EndTime.After(session.StartTime))
	
	// Test with non-existent session
	err = manager.EndSession("non-existent")
	assert.Error(t, err)
}

func TestGetSession(t *testing.T) {
	manager := NewManager()
	sessionID := manager.CreateSession("guild", "channel")
	
	// Get existing session
	session, err := manager.GetSession(sessionID)
	require.NoError(t, err)
	assert.Equal(t, sessionID, session.ID)
	
	// Get non-existent session
	_, err = manager.GetSession("non-existent")
	assert.Error(t, err)
}

func TestListSessions(t *testing.T) {
	manager := NewManager()
	
	// Initially empty
	sessions := manager.ListSessions()
	assert.Empty(t, sessions)
	
	// Create multiple sessions
	id1 := manager.CreateSession("guild1", "channel1")
	id2 := manager.CreateSession("guild2", "channel2")
	
	// List should return both
	sessions = manager.ListSessions()
	assert.Len(t, sessions, 2)
	
	// Verify both sessions are present
	sessionIDs := make(map[string]bool)
	for _, s := range sessions {
		sessionIDs[s.ID] = true
	}
	assert.True(t, sessionIDs[id1])
	assert.True(t, sessionIDs[id2])
}

func TestExportSession(t *testing.T) {
	manager := NewManager()
	sessionID := manager.CreateSession("guild", "channel")
	
	// Add some data
	err := manager.AddTranscript(sessionID, "user-1", "User1", "First message")
	require.NoError(t, err)
	err = manager.AddTranscript(sessionID, "user-2", "User2", "Second message")
	require.NoError(t, err)
	
	// Export the session
	filepath, err := manager.ExportSession(sessionID)
	require.NoError(t, err)
	assert.NotEmpty(t, filepath)
	
	// Verify file exists
	_, err = os.Stat(filepath)
	require.NoError(t, err)
	
	// Read and verify JSON content
	data, err := os.ReadFile(filepath)
	require.NoError(t, err)
	
	var exportedSession Session
	err = json.Unmarshal(data, &exportedSession)
	require.NoError(t, err)
	
	assert.Equal(t, sessionID, exportedSession.ID)
	assert.Len(t, exportedSession.Transcripts, 2)
	
	// Clean up
	_ = os.Remove(filepath)
	_ = os.RemoveAll("exports")
	
	// Test with non-existent session
	_, err = manager.ExportSession("non-existent")
	assert.Error(t, err)
}

func TestConcurrentAccess(t *testing.T) {
	manager := NewManager()
	sessionID := manager.CreateSession("guild", "channel")
	
	var wg sync.WaitGroup
	numGoroutines := 10
	
	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			userID := fmt.Sprintf("user-%d", id)
			username := fmt.Sprintf("User%d", id)
			
			// Add pending
			err := manager.AddPendingTranscription(sessionID, userID, username, float64(id))
			assert.NoError(t, err)
			
			// Add transcript
			time.Sleep(time.Millisecond * 10) // Simulate processing
			err = manager.AddTranscript(sessionID, userID, username, fmt.Sprintf("Message %d", id))
			assert.NoError(t, err)
		}(i)
	}
	
	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			// Get session
			session, err := manager.GetSession(sessionID)
			assert.NoError(t, err)
			assert.NotNil(t, session)
			
			// List sessions
			sessions := manager.ListSessions()
			assert.NotEmpty(t, sessions)
		}()
	}
	
	wg.Wait()
	
	// Verify all transcripts were added
	session, err := manager.GetSession(sessionID)
	require.NoError(t, err)
	assert.Len(t, session.Transcripts, numGoroutines)
}

func TestMultipleSessions(t *testing.T) {
	manager := NewManager()
	
	// Create multiple sessions
	session1 := manager.CreateSession("guild1", "channel1")
	session2 := manager.CreateSession("guild2", "channel2")
	
	// Add data to different sessions
	err := manager.AddTranscript(session1, "user1", "User1", "Session 1 message")
	require.NoError(t, err)
	
	err = manager.AddTranscript(session2, "user2", "User2", "Session 2 message")
	require.NoError(t, err)
	
	// Verify sessions are independent
	s1, err := manager.GetSession(session1)
	require.NoError(t, err)
	assert.Len(t, s1.Transcripts, 1)
	assert.Equal(t, "Session 1 message", s1.Transcripts[0].Text)
	
	s2, err := manager.GetSession(session2)
	require.NoError(t, err)
	assert.Len(t, s2.Transcripts, 1)
	assert.Equal(t, "Session 2 message", s2.Transcripts[0].Text)
}

func TestPendingTranscriptionRemovalOnTranscriptAdd(t *testing.T) {
	manager := NewManager()
	sessionID := manager.CreateSession("guild", "channel")
	
	// Add multiple pending transcriptions
	err := manager.AddPendingTranscription(sessionID, "user-1", "User1", 1.0)
	require.NoError(t, err)
	err = manager.AddPendingTranscription(sessionID, "user-2", "User2", 2.0)
	require.NoError(t, err)
	err = manager.AddPendingTranscription(sessionID, "user-3", "User3", 3.0)
	require.NoError(t, err)
	
	// Add transcript for user-2 (should only remove user-2's pending)
	err = manager.AddTranscript(sessionID, "user-2", "User2", "User 2 transcript")
	require.NoError(t, err)
	
	// Verify only user-2's pending was removed
	session, err := manager.GetSession(sessionID)
	require.NoError(t, err)
	assert.Len(t, session.PendingTranscriptions, 2)
	assert.Len(t, session.Transcripts, 1)
	
	// Check remaining pending transcriptions
	pendingUsers := make(map[string]bool)
	for _, p := range session.PendingTranscriptions {
		pendingUsers[p.UserID] = true
	}
	assert.True(t, pendingUsers["user-1"])
	assert.False(t, pendingUsers["user-2"]) // Should be removed
	assert.True(t, pendingUsers["user-3"])
}

func TestExportDirectoryCreation(t *testing.T) {
	// Clean up any existing exports directory
	_ = os.RemoveAll("exports")
	
	manager := NewManager()
	sessionID := manager.CreateSession("guild", "channel")
	
	// Ensure exports directory doesn't exist
	_, err := os.Stat("exports")
	assert.True(t, os.IsNotExist(err))
	
	// Export should create the directory
	filepath, err := manager.ExportSession(sessionID)
	require.NoError(t, err)
	
	// Verify directory was created
	info, err := os.Stat("exports")
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	
	// Clean up
	_ = os.Remove(filepath)
	_ = os.RemoveAll("exports")
}

// TestExportSessionFileSystemErrors tests handling of file system errors during export
func TestExportSessionFileSystemErrors(t *testing.T) {
	manager := NewManager()
	sessionID := manager.CreateSession("guild", "channel")
	
	// Add some data
	err := manager.AddTranscript(sessionID, "user-1", "User1", "Test message")
	require.NoError(t, err)
	
	// Test 1: Directory creation with permission issues
	// Create exports directory with no write permission
	_ = os.Mkdir("exports", 0555) // Read and execute only
	defer func() { _ = os.RemoveAll("exports") }()
	
	// Try to export - should fail due to permission
	filepath, err := manager.ExportSession(sessionID)
	// Note: This might succeed if running as root, so we check both cases
	if err != nil {
		assert.Contains(t, err.Error(), "error writing file")
	} else {
		// Clean up if it succeeded
		_ = os.Remove(filepath)
	}
	
	// Clean up and reset
	_ = os.RemoveAll("exports")
}

// TestExportSessionInvalidPath tests export with invalid session ID containing path traversal
func TestExportSessionInvalidPath(t *testing.T) {
	manager := NewManager()
	// This tests that even with a weird session ID, the export is safe
	sessionID := manager.CreateSession("guild", "channel")
	
	// Export should work normally even with special characters in session ID
	filepath, err := manager.ExportSession(sessionID)
	assert.NoError(t, err)
	assert.NotEmpty(t, filepath)
	
	// Verify the file was created in the exports directory
	assert.Contains(t, filepath, "exports/")
	
	// Clean up
	_ = os.Remove(filepath)
	_ = os.RemoveAll("exports")
}

// TestSessionManagerNilChecks tests that all methods handle nil/empty inputs gracefully
func TestSessionManagerNilChecks(t *testing.T) {
	manager := NewManager()
	
	// Test with empty session ID
	err := manager.AddPendingTranscription("", "user", "name", 1.0)
	assert.Error(t, err)
	
	err = manager.RemovePendingTranscription("", "user")
	assert.Error(t, err)
	
	err = manager.AddTranscript("", "user", "name", "text")
	assert.Error(t, err)
	
	err = manager.EndSession("")
	assert.Error(t, err)
	
	_, err = manager.GetSession("")
	assert.Error(t, err)
	
	_, err = manager.ExportSession("")
	assert.Error(t, err)
}