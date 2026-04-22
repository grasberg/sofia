package channels

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegohandler"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/utils"
	"github.com/grasberg/sofia/pkg/voice"
)

type TelegramChannel struct {
	*BaseChannel
	bot          *telego.Bot
	commands     TelegramCommander
	config       *config.Config
	chatIDs      sync.Map // string -> int64
	transcriber  *voice.GroqTranscriber
	placeholders sync.Map // chatID -> messageID
	stopThinking sync.Map // chatID -> thinkingCancel
}

type thinkingCancel struct {
	fn context.CancelFunc
}

func (c *thinkingCancel) Cancel() {
	if c != nil && c.fn != nil {
		c.fn()
	}
}

func NewTelegramChannel(cfg *config.Config, bus *bus.MessageBus) (*TelegramChannel, error) {
	var opts []telego.BotOption
	telegramCfg := cfg.Channels.Telegram

	if telegramCfg.Proxy != "" {
		proxyURL, parseErr := url.Parse(telegramCfg.Proxy)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid proxy URL %q: %w", telegramCfg.Proxy, parseErr)
		}
		opts = append(opts, telego.WithHTTPClient(&http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		}))
	} else if os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" {
		// Use environment proxy if configured
		opts = append(opts, telego.WithHTTPClient(&http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		}))
	}

	bot, err := telego.NewBot(telegramCfg.Token, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	base := NewBaseChannel("telegram", telegramCfg, bus, telegramCfg.AllowFrom)

	return &TelegramChannel{
		BaseChannel: base,
		commands:    NewTelegramCommands(bot, cfg),
		bot:         bot,
		config:      cfg,
		transcriber: nil,
	}, nil
}

func (c *TelegramChannel) SetTranscriber(transcriber *voice.GroqTranscriber) {
	c.transcriber = transcriber
}

func (c *TelegramChannel) Start(ctx context.Context) error {
	logger.InfoC("telegram", "Starting Telegram bot (polling mode)...")

	go c.ConnectWithRetry(ctx, c.connect)
	return nil
}

func (c *TelegramChannel) connect(ctx context.Context) error {
	updates, err := c.bot.UpdatesViaLongPolling(ctx, &telego.GetUpdatesParams{
		Timeout: 30,
	})
	if err != nil {
		return fmt.Errorf("failed to start long polling: %w", err)
	}

	bh, err := telegohandler.NewBotHandler(c.bot, updates)
	if err != nil {
		return fmt.Errorf("failed to create bot handler: %w", err)
	}

	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		c.commands.Help(ctx, message)
		return nil
	}, th.CommandEqual("help"))
	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.commands.Start(ctx, message)
	}, th.CommandEqual("start"))

	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.commands.Show(ctx, message)
	}, th.CommandEqual("show"))

	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.commands.List(ctx, message)
	}, th.CommandEqual("list"))

	bh.HandleMessage(func(ctx *th.Context, message telego.Message) error {
		return c.handleMessage(ctx, &message)
	}, th.AnyMessage())

	logger.InfoCF("telegram", "Telegram bot connected", map[string]any{
		"username": c.bot.Username(),
	})

	go bh.Start()

	go func() {
		<-ctx.Done()
		bh.Stop()
	}()

	return nil
}

func (c *TelegramChannel) Stop(ctx context.Context) error {
	logger.InfoC("telegram", "Stopping Telegram bot...")
	c.setRunning(false)
	return nil
}

func (c *TelegramChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("telegram bot not running")
	}

	chatID, err := parseChatID(msg.ChatID)
	if err != nil {
		return fmt.Errorf("invalid chat ID: %w", err)
	}

	if msg.Type == "thinking" {
		// Thinking indicator
		botErr := c.bot.SendChatAction(ctx, tu.ChatAction(tu.ID(chatID), telego.ChatActionTyping))
		if botErr != nil {
			logger.ErrorCF("telegram", "Failed to send chat action", map[string]any{
				"error": botErr.Error(),
			})
		}

		// Stop any previous thinking animation
		if prevStop, ok := c.stopThinking.Load(msg.ChatID); ok {
			if cf, ok := prevStop.(*thinkingCancel); ok && cf != nil {
				cf.Cancel()
			}
		}

		// Create cancel function for thinking state
		_, thinkCancel := context.WithTimeout(ctx, 5*time.Minute)
		c.stopThinking.Store(msg.ChatID, &thinkingCancel{fn: thinkCancel})

		pMsg, pErr := c.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), "Thinking... 💭"))
		if pErr == nil {
			pID := pMsg.MessageID
			c.placeholders.Store(msg.ChatID, pID)
		}
		return nil
	}

	// Stop thinking animation
	if stop, ok := c.stopThinking.Load(msg.ChatID); ok {
		if cf, ok := stop.(*thinkingCancel); ok && cf != nil {
			cf.Cancel()
		}
		c.stopThinking.Delete(msg.ChatID)
	}

	// Send file attachments if present
	if len(msg.Files) > 0 {
		c.sendFiles(ctx, chatID, msg.Files)
	}

	htmlContent := markdownToTelegramHTML(msg.Content)

	// Try to edit placeholder
	if pID, ok := c.placeholders.Load(msg.ChatID); ok {
		c.placeholders.Delete(msg.ChatID)
		editMsg := tu.EditMessageText(tu.ID(chatID), pID.(int), htmlContent)
		editMsg.ParseMode = telego.ModeHTML

		if _, err = c.bot.EditMessageText(ctx, editMsg); err == nil {
			return nil
		}
		// Fallback to new message if edit fails
	}

	tgMsg := tu.Message(tu.ID(chatID), htmlContent)
	tgMsg.ParseMode = telego.ModeHTML

	if _, err = c.bot.SendMessage(ctx, tgMsg); err != nil {
		logger.ErrorCF("telegram", "HTML parse failed, falling back to plain text", map[string]any{
			"error": err.Error(),
		})
		tgMsg.ParseMode = ""
		_, err = c.bot.SendMessage(ctx, tgMsg)
		return err
	}

	return nil
}

// sendFiles sends file attachments to a Telegram chat. Images (png, jpg, gif, webp) are
// sent via SendPhoto for inline display; all other file types use SendDocument.
func (c *TelegramChannel) sendFiles(ctx context.Context, chatID int64, filePaths []string) {
	for _, filePath := range filePaths {
		f, err := os.Open(filePath)
		if err != nil {
			logger.WarnCF("telegram", "Failed to open file for sending", map[string]any{
				"path":  filePath,
				"error": err.Error(),
			})
			continue
		}

		baseName := filepath.Base(filePath)
		inputFile := tu.File(tu.NameReader(f, baseName))

		if utils.IsImageFile(baseName) {
			params := tu.Photo(tu.ID(chatID), inputFile)
			if _, sendErr := c.bot.SendPhoto(ctx, params); sendErr != nil {
				logger.ErrorCF("telegram", "Failed to send photo", map[string]any{
					"path":  filePath,
					"error": sendErr.Error(),
				})
			}
		} else {
			params := tu.Document(tu.ID(chatID), inputFile)
			if _, sendErr := c.bot.SendDocument(ctx, params); sendErr != nil {
				logger.ErrorCF("telegram", "Failed to send document", map[string]any{
					"path":  filePath,
					"error": sendErr.Error(),
				})
			}
		}

		f.Close()
	}
}

func parseChatID(chatIDStr string) (int64, error) {
	var id int64
	_, err := fmt.Sscanf(chatIDStr, "%d", &id)
	return id, err
}
