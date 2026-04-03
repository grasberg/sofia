package notifications

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
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
	Secret  string   `json:"secret,omitempty"` // for HMAC signing
	Events  []string `json:"events"`           // which events to send
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
	webhooks     []WebhookConfig
	client       *http.Client
	sem          chan struct{} // concurrency limiter
	wg           sync.WaitGroup
	allowPrivate bool // skip private IP check (for tests)
}

func signWebhookPayload(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func validateWebhookScheme(scheme string) error {
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported scheme %q: must be http or https", scheme)
	}

	return nil
}

func resolveWebhookHost(host string) ([]net.IP, error) {
	addrs, err := net.LookupHost(host)
	if err != nil {
		return nil, fmt.Errorf("DNS lookup failed for %q: %w", host, err)
	}

	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip != nil {
			ips = append(ips, ip)
		}
	}

	return ips, nil
}

// NewWebhookDispatcher creates a dispatcher for the given webhook configs.
func NewWebhookDispatcher(webhooks []WebhookConfig) *WebhookDispatcher {
	return &WebhookDispatcher{
		webhooks: webhooks,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		sem: make(chan struct{}, 10), // max 10 concurrent webhook deliveries
	}
}

// Close waits for all in-flight webhook dispatches to complete.
func (wd *WebhookDispatcher) Close() {
	wd.wg.Wait()
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
		select {
		case wd.sem <- struct{}{}:
			wd.wg.Add(1)
			go func(wh WebhookConfig, payload WebhookPayload) {
				defer wd.wg.Done()
				defer func() { <-wd.sem }()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := wd.dispatchSingle(ctx, wh, payload); err != nil {
					logger.WarnCF("webhooks", "Webhook dispatch failed", map[string]any{
						"url":   wh.URL,
						"event": event,
						"error": err.Error(),
					})
				}
			}(wh, payload)
		default:
			// at capacity, drop this notification
			logger.WarnCF("notifications", "Webhook dispatch queue full, dropping event", map[string]any{
				"event": event,
				"url":   wh.URL,
			})
		}
	}
}

// dispatchSingle sends a single webhook POST with optional HMAC-SHA256 signing.
// The URL is validated (scheme and host) before sending to prevent SSRF.
func (wd *WebhookDispatcher) dispatchSingle(
	ctx context.Context, wh WebhookConfig, payload WebhookPayload,
) error {
	if err := validateWebhookURL(wh.URL, wd.allowPrivate); err != nil {
		return fmt.Errorf("webhook URL rejected: %w", err)
	}

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
		req.Header.Set("X-Sofia-Signature", signWebhookPayload(body, wh.Secret))
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

// validateWebhookURL checks that a webhook URL uses http(s) and does not
// resolve to a private/loopback IP address (SSRF protection).
// When allowPrivate is true, the private IP check is skipped.
func validateWebhookURL(rawURL string, allowPrivate bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if err := validateWebhookScheme(u.Scheme); err != nil {
		return err
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("missing hostname")
	}
	if !allowPrivate {
		ips, err := resolveWebhookHost(host)
		if err != nil {
			return err
		}
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return fmt.Errorf("webhook URL resolves to private IP %s", ip.String())
			}
		}
	}
	return nil
}

// isPrivateIP returns true for loopback, link-local, and RFC-1918 private addresses.
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	privateRanges := []struct {
		network *net.IPNet
	}{
		{parseCIDR("10.0.0.0/8")},
		{parseCIDR("172.16.0.0/12")},
		{parseCIDR("192.168.0.0/16")},
		{parseCIDR("fc00::/7")},
	}
	for _, r := range privateRanges {
		if r.network.Contains(ip) {
			return true
		}
	}
	return false
}

func parseCIDR(s string) *net.IPNet {
	_, n, _ := net.ParseCIDR(s)
	return n
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
