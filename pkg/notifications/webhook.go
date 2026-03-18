package notifications

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
)

// WebhookEvent types
const (
	EventTaskComplete   = "task_complete"
	EventBudgetWarning  = "budget_warning"
	EventBudgetExceeded = "budget_exceeded"
	EventApprovalNeeded = "approval_needed"
	EventCronComplete   = "cron_complete"
	EventAgentError     = "agent_error"
	EventSessionStart   = "session_start"
)

// WebhookConfig configures an outbound webhook.
type WebhookConfig struct {
	URL     string   `json:"url"`
	Secret  string   `json:"secret,omitempty"`  // for HMAC signing
	Events  []string `json:"events"`            // which events to send
	Enabled bool     `json:"enabled"`
}

// WebhookPayload is the JSON body sent to webhook endpoints.
type WebhookPayload struct {
	Event     string    `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	AgentID   string    `json:"agent_id,omitempty"`
	Channel   string    `json:"channel,omitempty"`
	Data      any       `json:"data,omitempty"`
}

// WebhookDispatcher sends events to configured webhook endpoints.
type WebhookDispatcher struct {
	webhooks []WebhookConfig
	client   *http.Client
}

// NewWebhookDispatcher creates a dispatcher for the given webhook configs.
func NewWebhookDispatcher(webhooks []WebhookConfig) *WebhookDispatcher {
	return &WebhookDispatcher{
		webhooks: webhooks,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Dispatch sends an event to all matching, enabled webhooks asynchronously.
func (wd *WebhookDispatcher) Dispatch(event string, agentID, channel string, data any) {
	payload := WebhookPayload{
		Event:     event,
		Timestamp: time.Now().UTC(),
		AgentID:   agentID,
		Channel:   channel,
		Data:      data,
	}

	for _, wh := range wd.webhooks {
		if !wh.Enabled {
			continue
		}
		if !eventMatches(wh.Events, event) {
			continue
		}
		go func(wh WebhookConfig) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := wd.dispatchSingle(ctx, wh, payload); err != nil {
				logger.WarnCF("webhooks", "Webhook dispatch failed", map[string]any{
					"url":   wh.URL,
					"event": event,
					"error": err.Error(),
				})
			}
		}(wh)
	}
}

// dispatchSingle sends a single webhook POST with optional HMAC-SHA256 signing.
func (wd *WebhookDispatcher) dispatchSingle(
	ctx context.Context, wh WebhookConfig, payload WebhookPayload,
) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if wh.Secret != "" {
		mac := hmac.New(sha256.New, []byte(wh.Secret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Sofia-Signature", sig)
	}

	resp, err := wd.client.Do(req)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, wh.URL)
	}
	return nil
}

// eventMatches returns true if event is present in the allowed list.
func eventMatches(allowed []string, event string) bool {
	for _, e := range allowed {
		if e == event {
			return true
		}
	}
	return false
}
