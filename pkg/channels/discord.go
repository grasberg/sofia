package channels

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/utils"
	"github.com/grasberg/sofia/pkg/voice"
)

const (
	transcriptionTimeout = 30 * time.Second
	sendTimeout          = 10 * time.Second
)

type DiscordChannel struct {
	*BaseChannel
	session     *discordgo.Session
	config      config.DiscordConfig
	transcriber *voice.GroqTranscriber
	ctx         context.Context
	typingMu    sync.Mutex
	typingStop  map[string]chan struct{} // chatID → stop signal
	botUserID   string                   // stored for mention checking
}

func NewDiscordChannel(cfg config.DiscordConfig, bus *bus.MessageBus) (*DiscordChannel, error) {
	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create discord session: %w", err)
	}

	base := NewBaseChannel("discord", cfg, bus, cfg.AllowFrom)

	return &DiscordChannel{
		BaseChannel: base,
		session:     session,
		config:      cfg,
		transcriber: nil,
		ctx:         context.Background(),
		typingStop:  make(map[string]chan struct{}),
	}, nil
}

func (c *DiscordChannel) SetTranscriber(transcriber *voice.GroqTranscriber) {
	c.transcriber = transcriber
}

func (c *DiscordChannel) getContext() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c *DiscordChannel) Start(ctx context.Context) error {
	logger.InfoC("discord", "Starting Discord bot")
	c.ctx = ctx

	// Register the message handler once before any connection attempts
	// to avoid duplicate registrations on reconnect.
	c.session.AddHandler(c.handleMessage)

	go c.ConnectWithRetry(ctx, c.connect)
	return nil
}

func (c *DiscordChannel) connect(ctx context.Context) error {
	// Get bot user ID before opening session to avoid race condition
	botUser, err := c.session.User("@me")
	if err != nil {
		return fmt.Errorf("failed to get bot user: %w", err)
	}
	c.botUserID = botUser.ID

	if err := c.session.Open(); err != nil {
		return fmt.Errorf("failed to open discord session: %w", err)
	}

	logger.InfoCF("discord", "Discord bot connected", map[string]any{
		"username": botUser.Username,
		"user_id":  botUser.ID,
	})

	return nil
}

func (c *DiscordChannel) Stop(ctx context.Context) error {
	logger.InfoC("discord", "Stopping Discord bot")
	c.setRunning(false)

	// Stop all typing goroutines before closing session
	c.typingMu.Lock()
	for chatID, stop := range c.typingStop {
		close(stop)
		delete(c.typingStop, chatID)
	}
	c.typingMu.Unlock()

	if err := c.session.Close(); err != nil {
		return fmt.Errorf("failed to close discord session: %w", err)
	}

	return nil
}

func (c *DiscordChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	c.stopTyping(msg.ChatID)

	if !c.IsRunning() {
		return fmt.Errorf("discord bot not running")
	}

	channelID := msg.ChatID
	if channelID == "" {
		return fmt.Errorf("channel ID is empty")
	}

	// Send file attachments if present
	if len(msg.Files) > 0 {
		if err := c.sendFiles(ctx, channelID, msg.Files); err != nil {
			logger.ErrorCF("discord", "Failed to send files", map[string]any{
				"error": err.Error(),
				"count": len(msg.Files),
			})
			// Continue to send text content even if file sending fails
		}
	}

	runes := []rune(msg.Content)
	if len(runes) == 0 {
		return nil
	}

	formatted := formatDiscordMarkdown(msg.Content)
	chunks := utils.SplitMessage(formatted, 2000) // Split messages into chunks, Discord length limit: 2000 chars

	for _, chunk := range chunks {
		if err := c.sendChunk(ctx, channelID, chunk); err != nil {
			return err
		}
	}

	return nil
}

func (c *DiscordChannel) sendChunk(ctx context.Context, channelID, content string) error {
	// Use the passed ctx for timeout control
	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.session.ChannelMessageSend(channelID, content)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("failed to send discord message: %w", err)
		}
		return nil
	case <-sendCtx.Done():
		return fmt.Errorf("send message timeout: %w", sendCtx.Err())
	}
}

// sendFiles sends file attachments to a Discord channel using ChannelMessageSendComplex.
func (c *DiscordChannel) sendFiles(ctx context.Context, channelID string, filePaths []string) error {
	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	var files []*discordgo.File
	var openedFiles []*os.File

	defer func() {
		for _, f := range openedFiles {
			f.Close()
		}
	}()

	for _, filePath := range filePaths {
		f, err := os.Open(filePath)
		if err != nil {
			logger.WarnCF("discord", "Failed to open file for sending", map[string]any{
				"path":  filePath,
				"error": err.Error(),
			})
			continue
		}
		openedFiles = append(openedFiles, f)

		files = append(files, &discordgo.File{
			Name:   filepath.Base(filePath),
			Reader: f,
		})
	}

	if len(files) == 0 {
		return nil
	}

	done := make(chan error, 1)
	go func() {
		_, err := c.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Files: files,
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("failed to send discord files: %w", err)
		}
		return nil
	case <-sendCtx.Done():
		return fmt.Errorf("send files timeout: %w", sendCtx.Err())
	}
}
