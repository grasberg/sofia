package agent

import (
	"context"
	"fmt"

	"github.com/grasberg/sofia/pkg/bus"
)

func (al *AgentLoop) ProcessDirect(ctx context.Context, content, sessionKey string) (string, error) {
	return al.ProcessDirectWithChannel(ctx, content, sessionKey, "cli", "direct")
}

// ProcessDirectStream sends a message to the default agent and streams the
// response via onChunk. When the configured provider implements
// StreamingProvider, text tokens are delivered as they arrive; iterations
// whose output is tool_calls don't produce deltas but still advance the
// loop. After the turn completes the full text is guaranteed to have been
// delivered: streamed providers emit many onChunk(delta, false) calls,
// non-streaming (or tool-only) paths emit a single onChunk(result, false)
// at the end so callers don't have to branch on provider capability. A
// terminal onChunk("", true) marks the end of the turn.
func (al *AgentLoop) ProcessDirectStream(
	ctx context.Context,
	content, sessionKey string,
	onChunk func(text string, done bool),
) error {
	agent := al.getRegistry().GetDefaultAgent()
	if agent == nil {
		return fmt.Errorf("no default agent configured")
	}
	deltaFired := false
	result, err := al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      sessionKey,
		Channel:         "cli",
		ChatID:          "direct",
		UserMessage:     content,
		DefaultResponse: defaultResponse,
		EnableSummary:   true,
		SendResponse:    false,
		OnTextDelta: func(delta string) {
			if delta == "" {
				return
			}
			deltaFired = true
			if onChunk != nil {
				onChunk(delta, false)
			}
		},
	})
	if err != nil {
		return err
	}
	if onChunk != nil {
		if !deltaFired && result != "" {
			// Non-streaming provider, or a turn that only produced
			// tool_calls with a tail summary from DefaultResponse —
			// deliver the full result as a single chunk so the caller
			// doesn't need to distinguish.
			onChunk(result, false)
		}
		onChunk("", true)
	}
	return nil
}

// ProcessDirectWithImages sends a message with optional image attachments directly
// to the default agent, bypassing channel routing. Images must be base64 data URLs.
func (al *AgentLoop) ProcessDirectWithImages(
	ctx context.Context,
	content, sessionKey string,
	images []string,
) (string, error) {
	agent := al.getRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent configured")
	}
	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      sessionKey,
		Channel:         "cli",
		ChatID:          "direct",
		UserMessage:     content,
		UserImages:      images,
		DefaultResponse: defaultResponse,
		EnableSummary:   true,
		SendResponse:    false,
	})
}

func (al *AgentLoop) ProcessDirectWithChannel(
	ctx context.Context,
	content, sessionKey, channel, chatID string,
) (string, error) {
	msg := bus.InboundMessage{
		Channel:    channel,
		SenderID:   "cron",
		ChatID:     chatID,
		Content:    content,
		SessionKey: sessionKey,
	}

	return al.processMessage(ctx, msg)
}

// ProcessHeartbeat processes a heartbeat request without session history.
// Each heartbeat is independent and doesn't accumulate context.
func (al *AgentLoop) ProcessHeartbeat(ctx context.Context, content, channel, chatID string) (string, error) {
	agent := al.getRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent configured")
	}
	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      "heartbeat",
		Channel:         channel,
		ChatID:          chatID,
		UserMessage:     content,
		DefaultResponse: defaultResponse,
		EnableSummary:   false,
		SendResponse:    false,
		NoHistory:       true,                   // Don't load session history for heartbeat
		ModelOverride:   al.cfg.Heartbeat.Model, // Use dedicated heartbeat model if configured
	})
}
