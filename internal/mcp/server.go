package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/fankserver/discord-voice-mcp/internal/bot"
	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/modelcontextprotocol/go-sdk/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
)

// Server wraps the MCP server with our Discord bot
type Server struct {
	mcpServer *mcp.Server
	bot       *bot.VoiceBot
	sessions  *session.Manager
	userID    string // Configured user ID for "my channel" commands
}

// NewServer creates a new MCP server for Discord voice
func NewServer(voiceBot *bot.VoiceBot, sessionManager *session.Manager, userID string) *Server {
	impl := &mcp.Implementation{
		Name:    "discord-voice-mcp",
		Version: "0.1.0",
	}

	opts := &mcp.ServerOptions{
		Instructions: "Discord Voice MCP Server - Provides tools for joining Discord voice channels and transcribing audio",
	}

	mcpServer := mcp.NewServer(impl, opts)

	s := &Server{
		mcpServer: mcpServer,
		bot:       voiceBot,
		sessions:  sessionManager,
		userID:    userID,
	}

	// Register all tools
	s.registerTools()

	return s
}

// registerTools registers all available MCP tools
func (s *Server) registerTools() {
	// Join my voice channel tool (user-centric)
	joinMyChannelSchema := &jsonschema.Schema{
		Type: "object",
	}

	mcp.AddTool[EmptyInput, struct{}](s.mcpServer, &mcp.Tool{
		Name:        "join_my_voice_channel",
		Description: "Join the voice channel where the configured user is",
		InputSchema: joinMyChannelSchema,
	}, s.handleJoinMyVoiceChannel)

	// Follow me tool
	followMeSchema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"enabled": {
				Type:        "boolean",
				Description: "Enable or disable auto-following",
			},
		},
		Required: []string{"enabled"},
	}

	mcp.AddTool[FollowMeInput, struct{}](s.mcpServer, &mcp.Tool{
		Name:        "follow_me",
		Description: "Enable/disable auto-following the configured user between voice channels",
		InputSchema: followMeSchema,
	}, s.handleFollowMe)

	// Join specific voice channel tool (kept for flexibility)
	joinSchema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"guildId": {
				Type:        "string",
				Description: "Discord guild (server) ID",
			},
			"channelId": {
				Type:        "string",
				Description: "Discord voice channel ID",
			},
		},
		Required: []string{"guildId", "channelId"},
	}

	mcp.AddTool[JoinChannelInput, struct{}](s.mcpServer, &mcp.Tool{
		Name:        "join_specific_channel",
		Description: "Join a specific Discord voice channel by ID",
		InputSchema: joinSchema,
	}, s.handleJoinVoiceChannel)

	// Leave voice channel tool
	leaveSchema := &jsonschema.Schema{
		Type: "object",
	}

	mcp.AddTool[EmptyInput, struct{}](s.mcpServer, &mcp.Tool{
		Name:        "leave_voice_channel",
		Description: "Leave the current voice channel",
		InputSchema: leaveSchema,
	}, s.handleLeaveVoiceChannel)

	// Get transcript tool
	transcriptSchema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"sessionId": {
				Type:        "string",
				Description: "Session ID to retrieve transcript for",
			},
		},
		Required: []string{"sessionId"},
	}

	mcp.AddTool[GetTranscriptInput, struct{}](s.mcpServer, &mcp.Tool{
		Name:        "get_transcript",
		Description: "Get transcript for a session",
		InputSchema: transcriptSchema,
	}, s.handleGetTranscript)

	// List sessions tool
	listSchema := &jsonschema.Schema{
		Type: "object",
	}

	mcp.AddTool[EmptyInput, struct{}](s.mcpServer, &mcp.Tool{
		Name:        "list_sessions",
		Description: "List all transcription sessions",
		InputSchema: listSchema,
	}, s.handleListSessions)

	// Export session tool
	exportSchema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"sessionId": {
				Type:        "string",
				Description: "Session ID to export",
			},
		},
		Required: []string{"sessionId"},
	}

	mcp.AddTool[ExportSessionInput, struct{}](s.mcpServer, &mcp.Tool{
		Name:        "export_session",
		Description: "Export a session to JSON file",
		InputSchema: exportSchema,
	}, s.handleExportSession)

	// Get bot status tool
	statusSchema := &jsonschema.Schema{
		Type: "object",
	}

	mcp.AddTool[EmptyInput, struct{}](s.mcpServer, &mcp.Tool{
		Name:        "get_bot_status",
		Description: "Get current bot connection status",
		InputSchema: statusSchema,
	}, s.handleGetBotStatus)
}

// Tool handlers - updated to match MCP SDK signature

type JoinChannelInput struct {
	GuildID   string `json:"guildId"`
	ChannelID string `json:"channelId"`
}


func (s *Server) handleJoinVoiceChannel(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[JoinChannelInput]) (*mcp.CallToolResultFor[struct{}], error) {
	logrus.WithFields(logrus.Fields{
		"guild_id":   params.Arguments.GuildID,
		"channel_id": params.Arguments.ChannelID,
	}).Debug("MCP: Join voice channel request")

	err := s.bot.JoinChannel(params.Arguments.GuildID, params.Arguments.ChannelID)

	var message string
	if err == nil {
		message = "Successfully joined voice channel"
	} else {
		message = fmt.Sprintf("Failed to join channel: %v", err)
	}

	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: message},
		},
	}, nil
}

type EmptyInput struct{}

func (s *Server) handleLeaveVoiceChannel(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[EmptyInput]) (*mcp.CallToolResultFor[struct{}], error) {
	logrus.Debug("MCP: Leave voice channel request")

	s.bot.LeaveChannel()

	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Left voice channel"},
		},
	}, nil
}

type GetTranscriptInput struct {
	SessionID string `json:"sessionId"`
}

func (s *Server) handleGetTranscript(ctx context.Context, sess *mcp.ServerSession, params *mcp.CallToolParamsFor[GetTranscriptInput]) (*mcp.CallToolResultFor[struct{}], error) {
	logrus.WithField("session_id", params.Arguments.SessionID).Debug("MCP: Get transcript request")

	sessionData, err := s.sessions.GetSession(params.Arguments.SessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Format session data as text
	transcript := fmt.Sprintf("Session %s\nStarted: %s\n", 
		sessionData.ID, sessionData.StartTime.Format("2006-01-02 15:04:05"))
	
	// Show pending transcriptions if any
	if len(sessionData.PendingTranscriptions) > 0 {
		transcript += "\nPending Transcriptions (processing):\n"
		for _, p := range sessionData.PendingTranscriptions {
			elapsed := time.Since(p.StartTime).Seconds()
			transcript += fmt.Sprintf("  ⏳ %s: Processing %.1fs of audio (elapsed: %.1fs)\n", 
				p.Username, p.Duration, elapsed)
		}
	}
	
	// Show completed transcripts
	transcript += "\nTranscripts:\n"
	for _, t := range sessionData.Transcripts {
		transcript += fmt.Sprintf("[%s] %s: %s\n", 
			t.Timestamp.Format("15:04:05"), t.Username, t.Text)
	}
	
	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: transcript},
		},
	}, nil
}

func (s *Server) handleListSessions(ctx context.Context, sess *mcp.ServerSession, params *mcp.CallToolParamsFor[EmptyInput]) (*mcp.CallToolResultFor[struct{}], error) {
	logrus.Debug("MCP: List sessions request")

	sessions := s.sessions.ListSessions()

	// Format sessions as text
	var output string
	if len(sessions) == 0 {
		output = "No sessions found"
	} else {
		output = fmt.Sprintf("Found %d session(s):\n\n", len(sessions))
		for _, s := range sessions {
			pendingIndicator := ""
			if len(s.PendingTranscriptions) > 0 {
				pendingIndicator = fmt.Sprintf(" (⏳ %d pending)", len(s.PendingTranscriptions))
			}
			output += fmt.Sprintf("Session %s\n  Started: %s\n  Transcripts: %d%s\n\n",
				s.ID, s.StartTime.Format("2006-01-02 15:04:05"), len(s.Transcripts), pendingIndicator)
		}
	}

	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: output},
		},
	}, nil
}

type ExportSessionInput struct {
	SessionID string `json:"sessionId"`
}

func (s *Server) handleExportSession(ctx context.Context, sess *mcp.ServerSession, params *mcp.CallToolParamsFor[ExportSessionInput]) (*mcp.CallToolResultFor[struct{}], error) {
	logrus.WithField("session_id", params.Arguments.SessionID).Debug("MCP: Export session request")

	filepath, err := s.sessions.ExportSession(params.Arguments.SessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to export session: %w", err)
	}

	message := fmt.Sprintf("Session exported to: %s", filepath)

	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: message},
		},
	}, nil
}

func (s *Server) handleGetBotStatus(ctx context.Context, sess *mcp.ServerSession, params *mcp.CallToolParamsFor[EmptyInput]) (*mcp.CallToolResultFor[struct{}], error) {
	logrus.Debug("MCP: Get bot status request")

	status := s.bot.GetStatus()
	followUser, autoFollow := s.bot.GetFollowStatus()

	statusText := fmt.Sprintf("Bot Status:\n")
	statusText += fmt.Sprintf("  Connected: %v\n", status["connected"])
	statusText += fmt.Sprintf("  In Voice: %v\n", status["inVoice"])
	
	if guildID, ok := status["guildID"].(string); ok {
		statusText += fmt.Sprintf("  Guild ID: %s\n", guildID)
	}
	if channelID, ok := status["channelID"].(string); ok {
		statusText += fmt.Sprintf("  Channel ID: %s\n", channelID)
	}
	
	if followUser != "" {
		statusText += fmt.Sprintf("  Following User: %s\n", followUser)
	}
	statusText += fmt.Sprintf("  Auto-Follow: %v", autoFollow)

	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: statusText},
		},
	}, nil
}

// handleJoinMyVoiceChannel joins the voice channel where the configured user is
func (s *Server) handleJoinMyVoiceChannel(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[EmptyInput]) (*mcp.CallToolResultFor[struct{}], error) {
	logrus.Debug("MCP: Join my voice channel request")

	if s.userID == "" {
		return &mcp.CallToolResultFor[struct{}]{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "No user ID configured. Set DISCORD_USER_ID environment variable"},
			},
		}, nil
	}

	err := s.bot.JoinUserChannel(s.userID)

	var message string
	if err == nil {
		message = "Successfully joined your voice channel"
	} else {
		message = fmt.Sprintf("Failed to join your channel: %v", err)
	}

	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: message},
		},
	}, nil
}

// FollowMeInput represents the input for the follow_me tool
type FollowMeInput struct {
	Enabled bool `json:"enabled"`
}

// handleFollowMe enables or disables auto-following
func (s *Server) handleFollowMe(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[FollowMeInput]) (*mcp.CallToolResultFor[struct{}], error) {
	logrus.WithField("enabled", params.Arguments.Enabled).Debug("MCP: Follow me request")

	if s.userID == "" {
		return &mcp.CallToolResultFor[struct{}]{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "No user ID configured. Set DISCORD_USER_ID environment variable"},
			},
		}, nil
	}

	s.bot.SetFollowUser(s.userID, params.Arguments.Enabled)

	message := "Auto-follow disabled"
	if params.Arguments.Enabled {
		message = fmt.Sprintf("Now auto-following user %s", s.userID)
		// Try to join their current channel if they're in one
		if err := s.bot.JoinUserChannel(s.userID); err != nil {
			logrus.WithError(err).Debug("User not currently in voice channel")
		}
	}

	return &mcp.CallToolResultFor[struct{}]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: message},
		},
	}, nil
}

// Start runs the MCP server
func (s *Server) Start(ctx context.Context) error {
	logrus.Info("Starting MCP server on stdio")

	transport := mcp.NewStdioTransport()
	return s.mcpServer.Run(ctx, transport)
}
