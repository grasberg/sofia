package channels

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/logger"
)

type Channel interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Send(ctx context.Context, msg bus.OutboundMessage) error
	IsRunning() bool
	IsAllowed(senderID string) bool
}

type BaseChannel struct {
	config    any
	bus       *bus.MessageBus
	running   atomic.Bool
	name      string
	allowList []string
}

func NewBaseChannel(name string, config any, bus *bus.MessageBus, allowList []string) *BaseChannel {
	return &BaseChannel{
		config:    config,
		bus:       bus,
		name:      name,
		allowList: allowList,
	}
}

func (c *BaseChannel) Name() string {
	return c.name
}

func (c *BaseChannel) IsRunning() bool {
	return c.running.Load()
}

func (c *BaseChannel) IsAllowed(senderID string) bool {
	if len(c.allowList) == 0 {
		return true
	}

	// Extract parts from compound senderID like "123456|username"
	idPart := senderID
	userPart := ""
	if idx := strings.Index(senderID, "|"); idx > 0 {
		idPart = senderID[:idx]
		userPart = senderID[idx+1:]
	}

	for _, allowed := range c.allowList {
		// Strip leading "@" from allowed value for username matching
		trimmed := strings.TrimPrefix(allowed, "@")
		allowedID := trimmed
		allowedUser := ""
		if idx := strings.Index(trimmed, "|"); idx > 0 {
			allowedID = trimmed[:idx]
			allowedUser = trimmed[idx+1:]
		}

		// Support either side using "id|username" compound form.
		// This keeps backward compatibility with legacy Telegram allowlist entries.
		if senderID == allowed ||
			idPart == allowed ||
			senderID == trimmed ||
			idPart == trimmed ||
			idPart == allowedID ||
			(allowedUser != "" && senderID == allowedUser) ||
			(userPart != "" && (userPart == allowed || userPart == trimmed || userPart == allowedUser)) {
			return true
		}
	}

	return false
}

func (c *BaseChannel) HandleMessage(senderID, chatID, content string, media []string, metadata map[string]string) {
	if !c.IsAllowed(senderID) {
		return
	}

	msg := bus.InboundMessage{
		Channel:  c.name,
		SenderID: senderID,
		ChatID:   chatID,
		Content:  content,
		Media:    media,
		Metadata: metadata,
	}

	c.bus.PublishInbound(msg)
}

func (c *BaseChannel) setRunning(running bool) {
	c.running.Store(running)
}

// defaultBackoff is the shared retry backoff schedule for all channels.
var defaultBackoff = []time.Duration{3 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second}

// ConnectWithRetry retries the given connect function with exponential backoff.
// On success it marks the channel as running and blocks until ctx is canceled.
func (c *BaseChannel) ConnectWithRetry(ctx context.Context, connect func(ctx context.Context) error) {
	attempt := 0
	for {
		if err := connect(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			delay := defaultBackoff[min(attempt, len(defaultBackoff)-1)]
			logger.WarnCF(c.name, "Connection failed, reconnecting", map[string]any{
				"error":    err.Error(),
				"retry_in": delay.String(),
				"attempt":  attempt + 1,
			})
			attempt++
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
				continue
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
		c.setRunning(true)
		<-ctx.Done()
		return
	}
}
