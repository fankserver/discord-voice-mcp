package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Manager handles transcription sessions
type Manager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// Session represents a transcription session
type Session struct {
	ID          string       `json:"id"`
	GuildID     string       `json:"guildId"`
	ChannelID   string       `json:"channelId"`
	StartTime   time.Time    `json:"startTime"`
	EndTime     *time.Time   `json:"endTime,omitempty"`
	Transcripts []Transcript `json:"transcripts"`
}

// Transcript represents a single transcribed message
type Transcript struct {
	Timestamp time.Time `json:"timestamp"`
	UserID    string    `json:"userId"`
	Username  string    `json:"username"`
	Text      string    `json:"text"`
}

// NewManager creates a new session manager
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

// CreateSession creates a new transcription session
func (m *Manager) CreateSession(guildID, channelID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	session := &Session{
		ID:          uuid.New().String(),
		GuildID:     guildID,
		ChannelID:   channelID,
		StartTime:   time.Now(),
		Transcripts: []Transcript{},
	}

	m.sessions[session.ID] = session
	return session.ID
}

// AddTranscript adds a transcript to a session
func (m *Manager) AddTranscript(sessionID, userID, username, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	transcript := Transcript{
		Timestamp: time.Now(),
		UserID:    userID,
		Username:  username,
		Text:      text,
	}

	session.Transcripts = append(session.Transcripts, transcript)
	return nil
}

// EndSession marks a session as ended
func (m *Manager) EndSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	now := time.Now()
	session.EndTime = &now
	return nil
}

// GetSession retrieves a session by ID
func (m *Manager) GetSession(sessionID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	return session, nil
}

// ListSessions returns all sessions
func (m *Manager) ListSessions() []Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, *session)
	}
	return sessions
}

// ExportSession exports a session to JSON file
func (m *Manager) ExportSession(sessionID string) (string, error) {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return "", err
	}

	// Create exports directory
	exportDir := "exports"
	// #nosec G301 - Export directory needs to be readable for serving files
	if err := os.MkdirAll(exportDir, 0750); err != nil {
		return "", fmt.Errorf("error creating export directory: %w", err)
	}

	// Generate filename
	filename := fmt.Sprintf("session_%s_%s.json", session.ID, session.StartTime.Format("20060102_150405"))
	filepath := filepath.Join(exportDir, filename)

	// Marshal to JSON
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshaling session: %w", err)
	}

	// Write to file
	// #nosec G306 - Export files need to be readable by the user
	if err := os.WriteFile(filepath, data, 0640); err != nil {
		return "", fmt.Errorf("error writing file: %w", err)
	}

	return filepath, nil
}
