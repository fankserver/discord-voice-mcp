package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fankserver/discord-voice-mcp/internal/audio"
	"github.com/fankserver/discord-voice-mcp/internal/bot"
	"github.com/fankserver/discord-voice-mcp/internal/mcp"
	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

var (
	Token  string
	UserID string
)

func init() {
	flag.StringVar(&Token, "token", "", "Discord Bot Token")
	flag.Parse()

	// Load from environment
	_ = godotenv.Load()
	if Token == "" {
		Token = os.Getenv("DISCORD_TOKEN")
	}
	UserID = os.Getenv("DISCORD_USER_ID")
}

func main() {
	// Configure logrus
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Set log level from environment
	logLevel := os.Getenv("LOG_LEVEL")
	switch strings.ToLower(logLevel) {
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "warn", "warning":
		logrus.SetLevel(logrus.WarnLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}

	if Token == "" {
		logrus.Fatal("Discord token is required. Use -token flag or DISCORD_TOKEN env var")
	}

	// Set up signal handling with context for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer cancel()

	// Create session manager
	sessionManager := session.NewManager()
	logrus.Debug("Session manager created")

	// Create transcriber (mock for PoC)
	trans := &transcriber.MockTranscriber{}
	logrus.Debug("Transcriber initialized (mock mode)")

	// Create audio processor
	audioProcessor := audio.NewProcessor(trans)
	logrus.Debug("Audio processor created")

	// Create bot
	voiceBot, err := bot.New(Token, sessionManager, audioProcessor)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating bot")
	}
	logrus.Info("Discord bot created successfully")

	// Always start MCP server - this is an MCP-first application
	mcpServer := mcp.NewServer(voiceBot, sessionManager, UserID)
	go func() {
		if err := mcpServer.Start(ctx); err != nil {
			logrus.WithError(err).Error("MCP server error")
		}
	}()
	logrus.Info("MCP server started")

	// Connect to Discord
	err = voiceBot.Connect()
	if err != nil {
		logrus.WithError(err).Fatal("Error connecting to Discord")
	}
	defer func() {
		if err := voiceBot.Disconnect(); err != nil {
			logrus.WithError(err).Warn("Failed to disconnect voice bot")
		}
	}()
	logrus.Info("Connected to Discord")

	// Log user configuration if provided
	if UserID != "" {
		logrus.WithField("user_id", UserID).Info("Configured to follow user")
	}

	// Wait for context cancellation
	logrus.Info("Bot is running. Press CTRL-C to exit.")
	<-ctx.Done()

	logrus.Info("Shutting down gracefully...")
	// Give components time to clean up
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	<-shutdownCtx.Done()
}
