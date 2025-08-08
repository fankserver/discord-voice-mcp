package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/fankserver/discord-voice-mcp/internal/audio"
	"github.com/fankserver/discord-voice-mcp/internal/bot"
	"github.com/fankserver/discord-voice-mcp/internal/mcp"
	"github.com/fankserver/discord-voice-mcp/internal/session"
	"github.com/fankserver/discord-voice-mcp/pkg/transcriber"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

var (
	Token     string
	ChannelID string
	GuildID   string
	MCPMode   bool
)

func init() {
	flag.StringVar(&Token, "token", "", "Discord Bot Token")
	flag.StringVar(&ChannelID, "channel", "", "Voice Channel ID")
	flag.StringVar(&GuildID, "guild", "", "Guild ID")
	flag.BoolVar(&MCPMode, "mcp", false, "Run as MCP server")
	flag.Parse()

	// Load from environment if not provided
	if Token == "" {
		_ = godotenv.Load()
		Token = os.Getenv("DISCORD_TOKEN")
	}
	if ChannelID == "" {
		ChannelID = os.Getenv("DISCORD_CHANNEL_ID")
	}
	if GuildID == "" {
		GuildID = os.Getenv("DISCORD_GUILD_ID")
	}
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

	// Start MCP server if requested
	if MCPMode {
		mcpServer := mcp.NewServer(voiceBot, sessionManager)
		go func() {
			if err := mcpServer.Start(context.Background()); err != nil {
				logrus.WithError(err).Error("MCP server error")
			}
		}()
		logrus.Info("MCP server started")
	}

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

	// Auto-join if channel provided
	if ChannelID != "" && GuildID != "" {
		logrus.WithFields(logrus.Fields{
			"guild_id":   GuildID,
			"channel_id": ChannelID,
		}).Info("Auto-joining voice channel")
		
		err = voiceBot.JoinChannel(GuildID, ChannelID)
		if err != nil {
			logrus.WithError(err).Error("Error joining channel")
		} else {
			logrus.Info("Successfully joined voice channel")
		}
	}

	// Wait for interrupt
	logrus.Info("Bot is running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	logrus.Info("Shutting down gracefully...")
}