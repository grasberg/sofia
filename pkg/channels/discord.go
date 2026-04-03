package channels

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// appendContent safely appends content to existing text
func appendContent(content, suffix string) string {
	if content == "" {
		return suffix
	}
	return content + "\n" + suffix
}

func (c *DiscordChannel) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m == nil || m.Author == nil {
		return
	}

	if m.Author.ID == s.State.User.ID {
		return
	}

	// Check allowlist first to avoid downloading attachments and transcribing for rejected users
	if !c.IsAllowed(m.Author.ID) {
		logger.DebugCF("discord", "Message rejected by allowlist", map[string]any{
			"user_id": m.Author.ID,
		})
		return
	}

	// If configured to only respond to mentions, check if bot is mentioned
	// Skip this check for DMs (GuildID is empty) - DMs should always be responded to
	if c.config.MentionOnly && m.GuildID != "" {
		isMentioned := false
		for _, mention := range m.Mentions {
			if mention.ID == c.botUserID {
				isMentioned = true
				break
			}
		}
		if !isMentioned {
			logger.DebugCF("discord", "Message ignored - bot not mentioned", map[string]any{
				"user_id": m.Author.ID,
			})
			return
		}
	}

	senderID := m.Author.ID
	senderName := m.Author.Username
	if m.Author.Discriminator != "" && m.Author.Discriminator != "0" {
		senderName += "#" + m.Author.Discriminator
	}

	content := m.Content
	content = c.stripBotMention(content)
	mediaPaths := make([]string, 0, len(m.Attachments))
	localFiles := make([]string, 0, len(m.Attachments))

	// Ensure temp files are cleaned up when function returns
	defer func() {
		for _, file := range localFiles {
			if err := os.Remove(file); err != nil {
				logger.DebugCF("discord", "Failed to cleanup temp file", map[string]any{
					"file":  file,
					"error": err.Error(),
				})
			}
		}
	}()

	for _, attachment := range m.Attachments {
		isAudio := utils.IsAudioFile(attachment.Filename, attachment.ContentType)

		if isAudio {
			localPath := c.downloadAttachment(attachment.URL, attachment.Filename)
			if localPath != "" {
				localFiles = append(localFiles, localPath)

				var transcribedText string
				if c.transcriber != nil && c.transcriber.IsAvailable() {
					ctx, cancel := context.WithTimeout(c.getContext(), transcriptionTimeout)
					result, err := c.transcriber.Transcribe(ctx, localPath)
					cancel() // Release context resources immediately to avoid leaks in for loop

					if err != nil {
						logger.ErrorCF("discord", "Voice transcription failed", map[string]any{
							"error": err.Error(),
						})
						transcribedText = fmt.Sprintf("[audio: %s (transcription failed)]", attachment.Filename)
					} else {
						transcribedText = fmt.Sprintf("[audio transcription: %s]", result.Text)
						logger.DebugCF("discord", "Audio transcribed successfully", map[string]any{
							"text": result.Text,
						})
					}
				} else {
					transcribedText = fmt.Sprintf("[audio: %s]", attachment.Filename)
				}

				content = appendContent(content, transcribedText)
			} else {
				logger.WarnCF("discord", "Failed to download audio attachment", map[string]any{
					"url":      attachment.URL,
					"filename": attachment.Filename,
				})
				mediaPaths = append(mediaPaths, attachment.URL)
				content = appendContent(content, fmt.Sprintf("[attachment: %s]", attachment.URL))
			}
		} else {
			mediaPaths = append(mediaPaths, attachment.URL)
			content = appendContent(content, fmt.Sprintf("[attachment: %s]", attachment.URL))
		}
	}

	if content == "" && len(mediaPaths) == 0 {
		return
	}

	if content == "" {
		content = "[media only]"
	}

	// Start typing after all early returns — guaranteed to have a matching Send()
	c.startTyping(m.ChannelID)

	logger.DebugCF("discord", "Received message", map[string]any{
		"sender_name": senderName,
		"sender_id":   senderID,
		"preview":     utils.Truncate(content, 50),
	})

	peerKind := "channel"
	peerID := m.ChannelID
	if m.GuildID == "" {
		peerKind = "direct"
		peerID = senderID
	}

	metadata := map[string]string{
		"message_id":   m.ID,
		"user_id":      senderID,
		"username":     m.Author.Username,
		"display_name": senderName,
		"guild_id":     m.GuildID,
		"channel_id":   m.ChannelID,
		"is_dm":        fmt.Sprintf("%t", m.GuildID == ""),
		"peer_kind":    peerKind,
		"peer_id":      peerID,
	}

	c.HandleMessage(senderID, m.ChannelID, content, mediaPaths, metadata)
}

// startTyping starts a continuous typing indicator loop for the given chatID.
// It stops any existing typing loop for that chatID before starting a new one.
func (c *DiscordChannel) startTyping(chatID string) {
	c.typingMu.Lock()
	// Stop existing loop for this chatID if any
	if stop, ok := c.typingStop[chatID]; ok {
		close(stop)
	}
	stop := make(chan struct{})
	c.typingStop[chatID] = stop
	c.typingMu.Unlock()

	go func() {
		if err := c.session.ChannelTyping(chatID); err != nil {
			logger.DebugCF("discord", "ChannelTyping error", map[string]any{"chatID": chatID, "err": err})
		}
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		timeout := time.After(5 * time.Minute)
		for {
			select {
			case <-stop:
				return
			case <-timeout:
				return
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				if err := c.session.ChannelTyping(chatID); err != nil {
					logger.DebugCF("discord", "ChannelTyping error", map[string]any{"chatID": chatID, "err": err})
				}
			}
		}
	}()
}

// stopTyping stops the typing indicator loop for the given chatID.
func (c *DiscordChannel) stopTyping(chatID string) {
	c.typingMu.Lock()
	defer c.typingMu.Unlock()
	if stop, ok := c.typingStop[chatID]; ok {
		close(stop)
		delete(c.typingStop, chatID)
	}
}

func (c *DiscordChannel) downloadAttachment(url, filename string) string {
	return utils.DownloadFile(url, filename, utils.DownloadOptions{
		LoggerPrefix: "discord",
	})
}

// stripBotMention removes the bot mention from the message content.
// Discord mentions have the format <@USER_ID> or <@!USER_ID> (with nickname).
func (c *DiscordChannel) stripBotMention(text string) string {
	if c.botUserID == "" {
		return text
	}
	// Remove both regular mention <@USER_ID> and nickname mention <@!USER_ID>
	text = strings.ReplaceAll(text, fmt.Sprintf("<@%s>", c.botUserID), "")
	text = strings.ReplaceAll(text, fmt.Sprintf("<@!%s>", c.botUserID), "")
	return strings.TrimSpace(text)
}

// reDiscordCodeBlock matches fenced code blocks preserving the entire block verbatim,
// including the language hint (e.g. ```go ... ```). This differs from reCodeBlock used
// for Telegram which strips the language specifier.
var reDiscordCodeBlock = regexp.MustCompile("(?s)```[\\w]*\\n?[\\s\\S]*?```")

// reDiscordHeading matches markdown headings with multi-line mode so it works on
// lines within a larger text block, not just the full-string boundary.
var reDiscordHeading = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)

// formatDiscordMarkdown converts standard markdown to Discord-compatible markdown.
// Discord natively supports: **bold**, *italic*, ~~strikethrough~~, `code`,
// ```code blocks```, > blockquotes, and ||spoiler||.
// The main conversions needed are for headings (not natively rendered) and links.
func formatDiscordMarkdown(text string) string {
	if text == "" {
		return ""
	}

	// Extract code blocks verbatim (preserving language hints for syntax highlighting)
	// to protect them from heading/link transformations.
	var codeBlocksFull []string
	cbIdx := 0
	text = reDiscordCodeBlock.ReplaceAllStringFunc(text, func(m string) string {
		codeBlocksFull = append(codeBlocksFull, m)
		placeholder := fmt.Sprintf("\x00DCB%d\x00", cbIdx)
		cbIdx++
		return placeholder
	})

	// Extract inline codes to protect them as well.
	inlineCodes := extractInlineCodes(text)
	text = inlineCodes.text

	// Convert markdown headings to bold text (Discord doesn't render # headings).
	// Uses reDiscordHeading with (?m) multi-line mode for proper line matching.
	text = reDiscordHeading.ReplaceAllString(text, "**$1**")

	// Convert markdown links [text](url) to Discord-friendly format.
	// Discord supports masked links in embeds but in regular messages [text](url) renders
	// poorly, so convert to: **text** (<url>)
	text = reLink.ReplaceAllStringFunc(text, func(s string) string {
		match := reLink.FindStringSubmatch(s)
		if len(match) < 3 {
			return s
		}
		linkText := match[1]
		linkURL := match[2]
		if linkText == linkURL {
			return linkURL
		}
		return fmt.Sprintf("**%s** (<%s>)", linkText, linkURL)
	})

	// Restore inline codes
	for i, code := range inlineCodes.codes {
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00IC%d\x00", i), fmt.Sprintf("`%s`", code))
	}

	// Restore code blocks verbatim
	for i, block := range codeBlocksFull {
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00DCB%d\x00", i), block)
	}

	return text
}
