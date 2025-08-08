package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/fankserver/discord-voice-mcp/internal/bot"
	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/sirupsen/logrus"
)

// Server implements the MCP server protocol
type Server struct {
	bot      *bot.VoiceBot
	sessions *session.Manager
	scanner  *bufio.Scanner
	writer   *bufio.Writer
}

// NewServer creates a new MCP server
func NewServer(voiceBot *bot.VoiceBot, sessionManager *session.Manager) *Server {
	return &Server{
		bot:      voiceBot,
		sessions: sessionManager,
		scanner:  bufio.NewScanner(os.Stdin),
		writer:   bufio.NewWriter(os.Stdout),
	}
}

// Start begins the MCP server loop
func (s *Server) Start() {
	logrus.Info("MCP Server started")

	// Send initialization
	s.sendMessage(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "0.1.0",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{
					"list": []map[string]interface{}{
						{
							"name":        "join_voice_channel",
							"description": "Join a Discord voice channel",
							"inputSchema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"guildId":   map[string]string{"type": "string"},
									"channelId": map[string]string{"type": "string"},
								},
								"required": []string{"guildId", "channelId"},
							},
						},
						{
							"name":        "leave_voice_channel",
							"description": "Leave the current voice channel",
						},
						{
							"name":        "get_transcript",
							"description": "Get transcript for a session",
							"inputSchema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"sessionId": map[string]string{"type": "string"},
								},
								"required": []string{"sessionId"},
							},
						},
						{
							"name":        "list_sessions",
							"description": "List all transcription sessions",
						},
						{
							"name":        "export_session",
							"description": "Export a session to JSON file",
							"inputSchema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"sessionId": map[string]string{"type": "string"},
								},
								"required": []string{"sessionId"},
							},
						},
					},
				},
			},
		},
	})

	// Process messages
	for s.scanner.Scan() {
		line := s.scanner.Text()

		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			logrus.WithError(err).Debug("Error parsing MCP message")
			continue
		}

		s.handleMessage(msg)
	}
}

func (s *Server) handleMessage(msg map[string]interface{}) {
	method, ok := msg["method"].(string)
	if !ok {
		return
	}

	id, hasID := msg["id"]

	switch method {
	case "initialize":
		if hasID {
			s.sendResponse(id, map[string]interface{}{
				"protocolVersion": "0.1.0",
				"serverInfo": map[string]interface{}{
					"name":    "discord-voice-mcp",
					"version": "0.1.0",
				},
			})
		}

	case "tools/call":
		params, ok := msg["params"].(map[string]interface{})
		if !ok {
			logrus.Warn("Invalid params in tools/call")
			return
		}
		toolName, ok := params["name"].(string)
		if !ok {
			logrus.Warn("Invalid tool name in tools/call")
			return
		}
		toolArgs, _ := params["arguments"].(map[string]interface{})

		result := s.executeTool(toolName, toolArgs)
		if hasID {
			s.sendResponse(id, result)
		}
	}
}

func (s *Server) executeTool(name string, args map[string]interface{}) interface{} {
	switch name {
	case "join_voice_channel":
		if args == nil {
			return map[string]interface{}{"error": "missing arguments"}
		}
		guildID, ok := args["guildId"].(string)
		if !ok {
			return map[string]interface{}{"error": "missing or invalid 'guildId' parameter"}
		}
		channelID, ok := args["channelId"].(string)
		if !ok {
			return map[string]interface{}{"error": "missing or invalid 'channelId' parameter"}
		}
		err := s.bot.JoinChannel(guildID, channelID)
		if err != nil {
			return map[string]interface{}{"error": err.Error()}
		}
		return map[string]interface{}{"success": true, "message": "Joined voice channel"}

	case "leave_voice_channel":
		s.bot.LeaveChannel()
		return map[string]interface{}{"success": true, "message": "Left voice channel"}

	case "get_transcript":
		if args == nil {
			return map[string]interface{}{"error": "missing arguments"}
		}
		sessionID, ok := args["sessionId"].(string)
		if !ok {
			return map[string]interface{}{"error": "missing or invalid 'sessionId' parameter"}
		}
		session, err := s.sessions.GetSession(sessionID)
		if err != nil {
			return map[string]interface{}{"error": err.Error()}
		}
		return session

	case "list_sessions":
		sessions := s.sessions.ListSessions()
		return map[string]interface{}{"sessions": sessions}

	case "export_session":
		if args == nil {
			return map[string]interface{}{"error": "missing arguments"}
		}
		sessionID, ok := args["sessionId"].(string)
		if !ok {
			return map[string]interface{}{"error": "missing or invalid 'sessionId' parameter"}
		}
		filepath, err := s.sessions.ExportSession(sessionID)
		if err != nil {
			return map[string]interface{}{"error": err.Error()}
		}
		return map[string]interface{}{"filepath": filepath}

	default:
		return map[string]interface{}{"error": fmt.Sprintf("unknown tool: %s", name)}
	}
}

func (s *Server) sendMessage(msg map[string]interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		logrus.WithError(err).Error("Failed to marshal MCP message")
		return
	}
	s.writer.Write(data)
	s.writer.WriteByte('\n')
	s.writer.Flush()
}

func (s *Server) sendResponse(id interface{}, result interface{}) {
	s.sendMessage(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}