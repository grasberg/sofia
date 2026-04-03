package triggers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
)

func TestNewTriggerManager(t *testing.T) {
	cfg := &config.Config{
		Triggers: config.TriggersConfig{
			Patterns: []config.PatternTriggerConfig{
				{
					Regex:   "test.*",
					AgentID: "test-agent",
					Prompt:  "Test prompt",
				},
			},
		},
	}
	msgBus := bus.NewMessageBus()
	tm := NewTriggerManager(cfg, msgBus)

	if tm == nil {
		t.Fatal("expected non-nil trigger manager")
	}
	if len(tm.patternTriggers) != 1 {
		t.Errorf("expected 1 pattern trigger, got %d", len(tm.patternTriggers))
	}
}

func TestNewTriggerManagerInvalidRegex(t *testing.T) {
	cfg := &config.Config{
		Triggers: config.TriggersConfig{
			Patterns: []config.PatternTriggerConfig{
				{
					Regex:   "[invalid(regex",
					AgentID: "test-agent",
					Prompt:  "Test prompt",
				},
			},
		},
	}
	msgBus := bus.NewMessageBus()
	tm := NewTriggerManager(cfg, msgBus)

	// Invalid regex should be skipped
	if len(tm.patternTriggers) != 0 {
		t.Errorf("expected 0 pattern triggers after invalid regex, got %d", len(tm.patternTriggers))
	}
}

func TestCheckPatternTriggers(t *testing.T) {
	cfg := &config.Config{
		Triggers: config.TriggersConfig{
			Patterns: []config.PatternTriggerConfig{
				{
					Regex:   "alert:.*",
					AgentID: "alert-agent",
					Prompt:  "Alert detected: {{.Match}}",
				},
			},
		},
	}
	msgBus := bus.NewMessageBus()
	tm := NewTriggerManager(cfg, msgBus)

	msg := bus.InboundMessage{
		Content: "alert: critical error",
		ChatID:  "test-chat",
	}

	fired := tm.CheckPatternTriggers(msg)
	if !fired {
		t.Errorf("expected pattern trigger to fire")
	}
}

func TestCheckPatternTriggersNoMatch(t *testing.T) {
	cfg := &config.Config{
		Triggers: config.TriggersConfig{
			Patterns: []config.PatternTriggerConfig{
				{
					Regex:   "alert:.*",
					AgentID: "alert-agent",
					Prompt:  "Alert detected",
				},
			},
		},
	}
	msgBus := bus.NewMessageBus()
	tm := NewTriggerManager(cfg, msgBus)

	msg := bus.InboundMessage{
		Content: "normal message",
		ChatID:  "test-chat",
	}

	fired := tm.CheckPatternTriggers(msg)
	if fired {
		t.Errorf("expected pattern trigger to not fire")
	}
}

func TestCheckPatternTriggersNoPatterns(t *testing.T) {
	cfg := &config.Config{
		Triggers: config.TriggersConfig{},
	}
	msgBus := bus.NewMessageBus()
	tm := NewTriggerManager(cfg, msgBus)

	msg := bus.InboundMessage{
		Content: "any message",
		ChatID:  "test-chat",
	}

	fired := tm.CheckPatternTriggers(msg)
	if fired {
		t.Errorf("expected no trigger to fire when no patterns configured")
	}
}

func TestVerifyHMAC(t *testing.T) {
	secret := "mysecret"
	body := []byte("test payload")

	// Calculate correct HMAC
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	correctSig := hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name      string
		signature string
		secret    string
		expected  bool
	}{
		{"Valid signature", correctSig, secret, true},
		{"Valid signature with sha256 prefix", "sha256=" + correctSig, secret, true},
		{"Invalid signature", "invalidsig", secret, false},
		{"Empty signature", "", secret, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := verifyHMAC(body, tt.signature, tt.secret)
			if result != tt.expected {
				t.Errorf("verifyHMAC() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestNormalizeTriggerPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "/webhook/test", want: "/webhook/test"},
		{input: "webhook/test", want: "/webhook/test"},
		{input: "", want: "/"},
	}

	for _, tt := range tests {
		if got := normalizeTriggerPath(tt.input); got != tt.want {
			t.Errorf("normalizeTriggerPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestInterpolateTriggerPrompt(t *testing.T) {
	prompt := interpolateTriggerPrompt("File {{.File}} was {{.Event}}", map[string]string{
		"{{.File}}":  "notes.txt",
		"{{.Event}}": "created",
	})

	if prompt != "File notes.txt was created" {
		t.Fatalf("interpolateTriggerPrompt() = %q, want %q", prompt, "File notes.txt was created")
	}
}

func TestRegisterWebhooks(t *testing.T) {
	cfg := &config.Config{
		Triggers: config.TriggersConfig{
			Webhooks: []config.WebhookTriggerConfig{
				{
					Path:    "/webhook/test",
					AgentID: "webhook-agent",
					Secret:  "",
				},
			},
		},
	}
	msgBus := bus.NewMessageBus()
	tm := NewTriggerManager(cfg, msgBus)

	mux := http.NewServeMux()
	tm.RegisterWebhooks(mux)

	// Test webhook endpoint
	req := httptest.NewRequest("POST", "/webhook/test", strings.NewReader("test payload"))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRegisterWebhooksInvalidMethod(t *testing.T) {
	cfg := &config.Config{
		Triggers: config.TriggersConfig{
			Webhooks: []config.WebhookTriggerConfig{
				{
					Path: "/webhook/test",
				},
			},
		},
	}
	msgBus := bus.NewMessageBus()
	tm := NewTriggerManager(cfg, msgBus)

	mux := http.NewServeMux()
	tm.RegisterWebhooks(mux)

	// Test webhook endpoint with GET (should fail)
	req := httptest.NewRequest("GET", "/webhook/test", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestStartStop(t *testing.T) {
	cfg := &config.Config{
		Triggers: config.TriggersConfig{},
	}
	msgBus := bus.NewMessageBus()
	tm := NewTriggerManager(cfg, msgBus)

	ctx := context.Background()
	err := tm.Start(ctx)
	if err != nil {
		t.Logf("Start returned error (expected in test): %v", err)
	}

	tm.Stop()
	// If we get here without panic, Stop worked correctly
}
