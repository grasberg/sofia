package agent

import (
	"context"
	"fmt"

	"github.com/grasberg/sofia/pkg/bus"
)

func (al *AgentLoop) ProcessDirect(ctx context.Context, content, sessionKey string) (string, error) {
	return al.ProcessDirectWithChannel(ctx, content, sessionKey, "cli", "direct")
}

// ProcessDirectStream sends a message to the default agent and streams the response
// back via the onChunk callback. It runs the full agent loop (including tool execution)
// and returns the final response. If the provider supports streaming, text deltas are
// sent incrementally for the first LLM call; tool-use responses are sent as a single chunk.
func (al *AgentLoop) ProcessDirectStream(
	ctx context.Context,
	content, sessionKey string,
	onChunk func(text string, done bool),
) error {
	// Run the full agent loop (handles tools, multi-turn, etc.)
	result, err := al.ProcessDirect(ctx, content, sessionKey)
	if err != nil {
		return err
	}
	onChunk(result, false)
	onChunk("", true)
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
