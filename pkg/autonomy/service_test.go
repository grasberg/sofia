package autonomy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/providers"
)

// MockProvider is a simple mock provider to simulate LLM responses for autonomy.
type MockProvider struct {
	ResponseContent string
}

func (m *MockProvider) GetDefaultModel() string {
	return "mock-model"
}

func (m *MockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content: m.ResponseContent,
	}, nil
}

func newTestService(t *testing.T, cfg *config.AutonomyConfig) *Service {
	t.Helper()
	return NewService(cfg, nil, nil, nil, nil, "agent-1", "mock", "/tmp", nil)
}

func TestService_SetDashboardHub(t *testing.T) {
	svc := newTestService(t, &config.AutonomyConfig{Enabled: true})
	svc.SetDashboardHub(nil) // should not panic
}

func TestService_SetTaskRunner(t *testing.T) {
	svc := newTestService(t, &config.AutonomyConfig{Enabled: true})

	called := false
	svc.SetTaskRunner(func(ctx context.Context, agentID, sessionKey, task, originChannel, originChatID string) (string, error) {
		called = true
		return "ok", nil
	})

	svc.mu.Lock()
	runner := svc.taskRunner
	svc.mu.Unlock()

	if runner == nil {
		t.Fatal("expected taskRunner to be set")
	}
	_, _ = runner(context.Background(), "", "", "", "", "")
	if !called {
		t.Error("expected task runner to be called")
	}
}

func TestService_SetLastChannelFunc(t *testing.T) {
	svc := newTestService(t, &config.AutonomyConfig{Enabled: true})

	svc.SetLastChannelFunc(func() string { return "telegram:123" })

	svc.mu.Lock()
	fn := svc.lastChannelFn
	svc.mu.Unlock()

	if fn == nil {
		t.Fatal("expected lastChannelFn to be set")
	}
	if got := fn(); got != "telegram:123" {
		t.Errorf("lastChannelFn() = %q, want 'telegram:123'", got)
	}
}

func TestService_SetPlanManager(t *testing.T) {
	svc := newTestService(t, &config.AutonomyConfig{Enabled: true})
	svc.SetPlanManager(nil) // should not panic

	svc.mu.Lock()
	pm := svc.planMgr
	svc.mu.Unlock()

	if pm != nil {
		t.Error("expected planMgr to be nil")
	}
}

func TestService_GetSubagentManager(t *testing.T) {
	svc := newTestService(t, &config.AutonomyConfig{Enabled: true})

	sm := svc.GetSubagentManager()
	if sm != nil {
		t.Error("expected nil subagent manager when none was provided")
	}
}

func TestService_StartStop(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cfg := &config.AutonomyConfig{
		Enabled:         true,
		IntervalMinutes: 60,
	}

	svc := NewService(cfg, db, nil, &MockProvider{ResponseContent: "ok"}, nil, "agent-1", "mock", "/tmp", nil)

	ctx := context.Background()
	err := svc.Start(ctx)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	// Starting again should return an error.
	err = svc.Start(ctx)
	if err == nil {
		t.Fatal("expected error when starting already-running service")
	}

	svc.Stop()

	// Stopping again should be a no-op (no panic).
	svc.Stop()
}

func TestService_StartDisabled(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cfg := &config.AutonomyConfig{
		Enabled: false,
	}

	svc := NewService(cfg, db, nil, nil, nil, "agent-1", "mock", "/tmp", nil)

	ctx := context.Background()
	err := svc.Start(ctx)
	if err != nil {
		t.Fatalf("Start with disabled config returned error: %v", err)
	}

	// Even when disabled, finalization ticker runs. Stop should work.
	svc.Stop()
}

func TestService_NotifyUser_WithChannel(t *testing.T) {
	msgBus := bus.NewMessageBus()
	cfg := &config.AutonomyConfig{Enabled: true}
	svc := NewService(cfg, nil, msgBus, nil, nil, "agent-1", "mock", "/tmp", nil)

	svc.SetLastChannelFunc(func() string { return "telegram:12345" })

	// Send a notification (non-blocking)
	svc.notifyUser("test notification")

	// Consume the outbound message
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	msg, ok := msgBus.SubscribeOutbound(ctx)
	if !ok {
		t.Fatal("expected outbound message")
	}
	assert.Equal(t, "telegram", msg.Channel)
	assert.Equal(t, "12345", msg.ChatID)
	assert.Equal(t, "test notification", msg.Content)
}

func TestService_NotifyUser_InvalidChannel(t *testing.T) {
	msgBus := bus.NewMessageBus()
	svc := NewService(&config.AutonomyConfig{Enabled: true}, nil, msgBus, nil, nil, "agent-1", "mock", "/tmp", nil)

	// No colon separator — should be a no-op
	svc.SetLastChannelFunc(func() string { return "invalid-no-colon" })
	svc.notifyUser("test") // should not panic or publish

	// Empty string — should be a no-op
	svc.SetLastChannelFunc(func() string { return "" })
	svc.notifyUser("test") // should not panic or publish
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
	db.Exec(
		"UPDATE sessions SET updated_at = ? WHERE key = ?",
		time.Now().Add(-1*time.Hour).UTC().Format(time.RFC3339),
		sessionKey,
	)

	// We'll capture outbound messages to verify the bus is hit
	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	defer consumerCancel()

	published := make(chan bus.InboundMessage, 1)
	go func() {
		for {
			msg, ok := msgBus.ConsumeInbound(consumerCtx)
			if !ok {
				return
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

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello..."},
		{"empty", "", 5, ""},
		{"single char limit", "hello", 1, "h..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestService_CheckBudget(t *testing.T) {
	cfg := &config.AutonomyConfig{
		Enabled:       true,
		MaxCostPerDay: 0.5,
	}
	svc := newTestService(t, cfg)

	// First call initializes the reset date and confirms budget is available.
	if !svc.checkBudget() {
		t.Fatal("expected budget to be available initially")
	}

	// Spend some budget (after initialization)
	svc.trackCost(50000) // 50K tokens = $0.50
	if svc.checkBudget() {
		t.Fatal("expected budget to be exhausted after spending $0.50")
	}
}

func TestService_CheckBudget_DefaultLimit(t *testing.T) {
	cfg := &config.AutonomyConfig{
		Enabled: true,
		// MaxCostPerDay is zero, should default to 1.0
	}
	svc := newTestService(t, cfg)

	// Initialize the budget reset date first.
	svc.checkBudget()

	svc.trackCost(99000) // 99K tokens = $0.99
	if !svc.checkBudget() {
		t.Fatal("expected budget to be available under default $1.0 limit")
	}

	svc.trackCost(2000) // push over $1.0
	if svc.checkBudget() {
		t.Fatal("expected budget to be exhausted over default $1.0 limit")
	}
}

func TestService_TrackCost(t *testing.T) {
	cfg := &config.AutonomyConfig{Enabled: true}
	svc := newTestService(t, cfg)

	svc.trackCost(1000)
	// 1000 tokens * $0.01/1K = $0.01
	if svc.budgetSpent < 0.009 || svc.budgetSpent > 0.011 {
		t.Errorf("budgetSpent = %f, want ~0.01", svc.budgetSpent)
	}

	svc.trackCost(4000)
	// total: 5000 tokens = $0.05
	if svc.budgetSpent < 0.049 || svc.budgetSpent > 0.051 {
		t.Errorf("budgetSpent = %f, want ~0.05", svc.budgetSpent)
	}
}

func TestService_MaxStepRetries(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := &config.AutonomyConfig{Enabled: true}
		svc := newTestService(t, cfg)
		if got := svc.maxStepRetries(); got != 2 {
			t.Errorf("maxStepRetries() = %d, want 2", got)
		}
	})

	t.Run("configured", func(t *testing.T) {
		cfg := &config.AutonomyConfig{Enabled: true, MaxStepRetries: 5}
		svc := newTestService(t, cfg)
		if got := svc.maxStepRetries(); got != 5 {
			t.Errorf("maxStepRetries() = %d, want 5", got)
		}
	})
}

func TestSetGoalAutoFixCount(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	gAny, err := gm.AddGoal("agent-1", "Fix Test", "desc", "high")
	if err != nil {
		t.Fatalf("AddGoal: %v", err)
	}
	goal := gAny.(*Goal)

	// Initially zero
	count := getGoalAutoFixCount(gm, goal.ID)
	if count != 0 {
		t.Errorf("initial auto_fix_count = %d, want 0", count)
	}

	// Set to 1
	SetGoalAutoFixCount(gm, goal.ID, 1)
	count = getGoalAutoFixCount(gm, goal.ID)
	if count != 1 {
		t.Errorf("auto_fix_count = %d, want 1", count)
	}

	// Set to 2
	SetGoalAutoFixCount(gm, goal.ID, 2)
	count = getGoalAutoFixCount(gm, goal.ID)
	if count != 2 {
		t.Errorf("auto_fix_count = %d, want 2", count)
	}
}

func TestGetGoalAutoFixCount_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	count := getGoalAutoFixCount(gm, 99999)
	if count != 0 {
		t.Errorf("expected 0 for nonexistent goal, got %d", count)
	}
}

func TestService_NotifyUser_NilDeps(t *testing.T) {
	// notifyUser should not panic when lastChannelFn or bus is nil.
	cfg := &config.AutonomyConfig{Enabled: true}
	svc := newTestService(t, cfg)
	svc.notifyUser("test message") // should not panic
}

func TestService_Broadcast_NilHub(t *testing.T) {
	// broadcast should not panic when hub is nil.
	cfg := &config.AutonomyConfig{Enabled: true}
	svc := newTestService(t, cfg)
	svc.broadcast(map[string]any{"type": "test"}) // should not panic
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
	db.Exec(
		"UPDATE sessions SET updated_at = ? WHERE key = ?",
		time.Now().Add(-1*time.Hour).UTC().Format(time.RFC3339),
		sessionKey,
	)

	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	defer consumerCancel()

	published := make(chan bus.InboundMessage, 1)
	go func() {
		for {
			msg, ok := msgBus.ConsumeInbound(consumerCtx)
			if !ok {
				return
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
