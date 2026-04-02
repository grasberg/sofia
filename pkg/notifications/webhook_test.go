package notifications

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookDispatcher_DispatchToMatchingEvent(t *testing.T) {
	var mu sync.Mutex
	var received WebhookPayload
	called := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		called = true
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wd := NewWebhookDispatcher([]WebhookConfig{
		{
			URL:     srv.URL,
			Events:  []string{EventTaskComplete, EventAgentError},
			Enabled: true,
		},
	})
	wd.allowPrivate = true // allow localhost in tests

	wd.Dispatch(EventTaskComplete, "agent-1", "telegram", map[string]string{"task": "demo"})

	// Wait for async goroutine to finish.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return called
	}, 2*time.Second, 10*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, EventTaskComplete, received.Event)
	assert.Equal(t, "agent-1", received.AgentID)
	assert.Equal(t, "telegram", received.Channel)
	assert.False(t, received.Timestamp.IsZero())
}

func TestWebhookDispatcher_SkipsNonMatchingEvent(t *testing.T) {
	var mu sync.Mutex
	called := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wd := NewWebhookDispatcher([]WebhookConfig{
		{
			URL:     srv.URL,
			Events:  []string{EventBudgetWarning},
			Enabled: true,
		},
	})

	wd.Dispatch(EventTaskComplete, "agent-1", "cli", nil)

	// Give it a moment; the request should NOT arrive.
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.False(t, called, "webhook should not have been called for non-matching event")
}

func TestWebhookDispatcher_HMACSignature(t *testing.T) {
	secret := "super-secret-key"
	var mu sync.Mutex
	var sigHeader string
	var bodyBytes []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		sigHeader = r.Header.Get("X-Sofia-Signature")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		bodyBytes = buf
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wd := NewWebhookDispatcher([]WebhookConfig{
		{
			URL:     srv.URL,
			Secret:  secret,
			Events:  []string{EventCronComplete},
			Enabled: true,
		},
	})
	wd.allowPrivate = true // allow localhost in tests

	wd.Dispatch(EventCronComplete, "cron-agent", "cron", nil)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return sigHeader != ""
	}, 2*time.Second, 10*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Recompute expected HMAC over the captured body.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(bodyBytes)
	expected := hex.EncodeToString(mac.Sum(nil))

	assert.Equal(t, expected, sigHeader, "HMAC signature mismatch")
}

func TestWebhookDispatcher_DisabledWebhook(t *testing.T) {
	var mu sync.Mutex
	called := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wd := NewWebhookDispatcher([]WebhookConfig{
		{
			URL:     srv.URL,
			Events:  []string{EventAgentError},
			Enabled: false,
		},
	})

	wd.Dispatch(EventAgentError, "agent-1", "discord", nil)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.False(t, called, "disabled webhook should not be called")
}

func TestWebhookDispatcher_NoWebhooks(t *testing.T) {
	wd := NewWebhookDispatcher(nil)
	// Must not panic.
	wd.Dispatch(EventSessionStart, "agent-1", "web", nil)

	wd2 := NewWebhookDispatcher([]WebhookConfig{})
	wd2.Dispatch(EventSessionStart, "agent-1", "web", nil)
}
