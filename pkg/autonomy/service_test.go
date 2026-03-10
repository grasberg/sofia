package autonomy

import (
	"context"
	"testing"
	"time"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/stretchr/testify/assert"
)

// MockProvider is a simple mock provider to simulate LLM responses for autonomy.
type MockProvider struct {
	ResponseContent string
}

func (m *MockProvider) GetDefaultModel() string {
	return "mock-model"
}

func (m *MockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, options map[string]any) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content: m.ResponseContent,
	}, nil
}

func TestAutonomyService_EvaluateRecentActivity(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	msgBus := bus.NewMessageBus()
	agentID := "agent-1"
	sessionKey := "autonomy-test-session"

	// Create a dummy session roughly an hour ago so it's not "too recent" or "too old"
	db.GetOrCreateSession(sessionKey, "agent-1")
	db.AppendMessage(sessionKey, providers.Message{
		Role:    "user",
		Content: "I want to build a Python microservice.",
	})

	// Override session UpdatedAt to let it trigger (otherwise tests might fail due to SQLite rounding or default time.Now() usage).
	// To reliably test without DB hacking, we just rely on `AddMessage` touching the session. We simulate time passing using monkey patching or just accept that the query needs mockable time.
	db.Exec("UPDATE sessions SET updated_at = ? WHERE key = ?", time.Now().Add(-1*time.Hour).UTC().Format(time.RFC3339), sessionKey)

	// We'll capture outbound messages to verify the bus is hit
	published := make(chan bus.InboundMessage, 1)
	go func() {
		for {
			msg, ok := msgBus.ConsumeInbound(context.Background())
			if !ok {
				break
			}
			if msg.SenderID == "autonomy_service" {
				published <- msg
			}
		}
	}()

	mockLLM := &MockProvider{
		ResponseContent: "You should set up a virtualenv and a basic FastAPI app first.",
	}

	cfg := &config.AutonomyConfig{
		Enabled:         true,
		Suggestions:     true,
		Research:        true,
		IntervalMinutes: 60,
	}

	svc := NewService(cfg, db, msgBus, mockLLM, nil, agentID, "mock-model", "test-workspace", nil)

	// Test the iteration logic synchronously
	svc.evaluateRecentActivity(context.Background())

	// Wait for the message to propagate
	select {
	case msg := <-published:
		assert.Equal(t, "proactive", msg.ChatID)
		assert.Equal(t, sessionKey, msg.SessionKey)
		assert.Contains(t, msg.Content, "[PROACTIVE THOUGHT]")
		assert.Contains(t, msg.Content, "virtualenv")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for proactive message")
	}
}

func TestAutonomyService_NoSuggestion(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	msgBus := bus.NewMessageBus()
	sessionKey := "autonomy-test-session-2"

	db.GetOrCreateSession(sessionKey, "agent-1")
	db.AppendMessage(sessionKey, providers.Message{
		Role:    "user",
		Content: "Looking good.",
	})
	db.Exec("UPDATE sessions SET updated_at = ? WHERE key = ?", time.Now().Add(-1*time.Hour).UTC().Format(time.RFC3339), sessionKey)

	published := make(chan bus.InboundMessage, 1)
	go func() {
		for {
			msg, ok := msgBus.ConsumeInbound(context.Background())
			if !ok {
				break
			}
			if msg.SenderID == "autonomy_service" {
				published <- msg
			}
		}
	}()

	mockLLM := &MockProvider{
		ResponseContent: "NO_SUGGESTION", // Returning NO_SUGGESTION should abort
	}

	cfg := &config.AutonomyConfig{
		Enabled:     true,
		Suggestions: true,
	}

	svc := NewService(cfg, db, msgBus, mockLLM, nil, "agent-1", "mock-model", "test-workspace", nil)

	svc.evaluateRecentActivity(context.Background())

	select {
	case <-published:
		t.Fatal("should not have received a message when NO_SUGGESTION was returned")
	case <-time.After(500 * time.Millisecond):
		// success, no message
	}
}
