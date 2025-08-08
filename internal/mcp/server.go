package mcp

import (
	"context"
	"fmt"

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
}

// NewServer creates a new MCP server for Discord voice
func NewServer(voiceBot *bot.VoiceBot, sessionManager *session.Manager) *Server {
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
	}

	// Register all tools
	s.registerTools()

	return s
}

// registerTools registers all available MCP tools
func (s *Server) registerTools() {
	// Join voice channel tool
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

	mcp.AddTool[JoinChannelInput, JoinChannelOutput](s.mcpServer, &mcp.Tool{
		Name:        "join_voice_channel",
		Description: "Join a Discord voice channel",
		InputSchema: joinSchema,
	}, s.handleJoinVoiceChannel)

	// Leave voice channel tool
	leaveSchema := &jsonschema.Schema{
		Type: "object",
	}

	mcp.AddTool[EmptyInput, LeaveChannelOutput](s.mcpServer, &mcp.Tool{
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

	mcp.AddTool[GetTranscriptInput, GetTranscriptOutput](s.mcpServer, &mcp.Tool{
		Name:        "get_transcript",
		Description: "Get transcript for a session",
		InputSchema: transcriptSchema,
	}, s.handleGetTranscript)

	// List sessions tool
	listSchema := &jsonschema.Schema{
		Type: "object",
	}

	mcp.AddTool[EmptyInput, ListSessionsOutput](s.mcpServer, &mcp.Tool{
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

	mcp.AddTool[ExportSessionInput, ExportSessionOutput](s.mcpServer, &mcp.Tool{
		Name:        "export_session",
		Description: "Export a session to JSON file",
		InputSchema: exportSchema,
	}, s.handleExportSession)

	// Get bot status tool
	statusSchema := &jsonschema.Schema{
		Type: "object",
	}

	mcp.AddTool[EmptyInput, BotStatusOutput](s.mcpServer, &mcp.Tool{
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

type JoinChannelOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (s *Server) handleJoinVoiceChannel(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[JoinChannelInput]) (*mcp.CallToolResultFor[JoinChannelOutput], error) {
	logrus.WithFields(logrus.Fields{
		"guild_id":   params.Arguments.GuildID,
		"channel_id": params.Arguments.ChannelID,
	}).Debug("MCP: Join voice channel request")

	err := s.bot.JoinChannel(params.Arguments.GuildID, params.Arguments.ChannelID)

	output := JoinChannelOutput{
		Success: err == nil,
		Message: "Successfully joined voice channel",
	}

	if err != nil {
		output.Message = fmt.Sprintf("Failed to join channel: %v", err)
	}

	return &mcp.CallToolResultFor[JoinChannelOutput]{
		StructuredContent: output,
	}, nil
}

type EmptyInput struct{}

type LeaveChannelOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (s *Server) handleLeaveVoiceChannel(ctx context.Context, session *mcp.ServerSession, params *mcp.CallToolParamsFor[EmptyInput]) (*mcp.CallToolResultFor[LeaveChannelOutput], error) {
	logrus.Debug("MCP: Leave voice channel request")

	s.bot.LeaveChannel()

	return &mcp.CallToolResultFor[LeaveChannelOutput]{
		StructuredContent: LeaveChannelOutput{
			Success: true,
			Message: "Left voice channel",
		},
	}, nil
}

type GetTranscriptInput struct {
	SessionID string `json:"sessionId"`
}

type GetTranscriptOutput struct {
	Session *session.Session `json:"session"`
}

func (s *Server) handleGetTranscript(ctx context.Context, sess *mcp.ServerSession, params *mcp.CallToolParamsFor[GetTranscriptInput]) (*mcp.CallToolResultFor[GetTranscriptOutput], error) {
	logrus.WithField("session_id", params.Arguments.SessionID).Debug("MCP: Get transcript request")

	sessionData, err := s.sessions.GetSession(params.Arguments.SessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	return &mcp.CallToolResultFor[GetTranscriptOutput]{
		StructuredContent: GetTranscriptOutput{
			Session: sessionData,
		},
	}, nil
}

type ListSessionsOutput struct {
	Sessions []session.Session `json:"sessions"`
}

func (s *Server) handleListSessions(ctx context.Context, sess *mcp.ServerSession, params *mcp.CallToolParamsFor[EmptyInput]) (*mcp.CallToolResultFor[ListSessionsOutput], error) {
	logrus.Debug("MCP: List sessions request")

	sessions := s.sessions.ListSessions()

	return &mcp.CallToolResultFor[ListSessionsOutput]{
		StructuredContent: ListSessionsOutput{
			Sessions: sessions,
		},
	}, nil
}

type ExportSessionInput struct {
	SessionID string `json:"sessionId"`
}

type ExportSessionOutput struct {
	Filepath string `json:"filepath"`
}

func (s *Server) handleExportSession(ctx context.Context, sess *mcp.ServerSession, params *mcp.CallToolParamsFor[ExportSessionInput]) (*mcp.CallToolResultFor[ExportSessionOutput], error) {
	logrus.WithField("session_id", params.Arguments.SessionID).Debug("MCP: Export session request")

	filepath, err := s.sessions.ExportSession(params.Arguments.SessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to export session: %w", err)
	}

	return &mcp.CallToolResultFor[ExportSessionOutput]{
		StructuredContent: ExportSessionOutput{
			Filepath: filepath,
		},
	}, nil
}

type BotStatusOutput struct {
	Connected bool   `json:"connected"`
	InVoice   bool   `json:"inVoice"`
	GuildID   string `json:"guildId,omitempty"`
	ChannelID string `json:"channelId,omitempty"`
}

func (s *Server) handleGetBotStatus(ctx context.Context, sess *mcp.ServerSession, params *mcp.CallToolParamsFor[EmptyInput]) (*mcp.CallToolResultFor[BotStatusOutput], error) {
	logrus.Debug("MCP: Get bot status request")

	status := s.bot.GetStatus()

	output := BotStatusOutput{
		Connected: status["connected"].(bool),
		InVoice:   status["inVoice"].(bool),
	}

	if guildID, ok := status["guildID"].(string); ok {
		output.GuildID = guildID
	}
	if channelID, ok := status["channelID"].(string); ok {
		output.ChannelID = channelID
	}

	return &mcp.CallToolResultFor[BotStatusOutput]{
		StructuredContent: output,
	}, nil
}

// Start runs the MCP server
func (s *Server) Start(ctx context.Context) error {
	logrus.Info("Starting MCP server on stdio")

	transport := mcp.NewStdioTransport()
	return s.mcpServer.Run(ctx, transport)
}
