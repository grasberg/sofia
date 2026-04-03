package session

import (
	"strings"
	"testing"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
)

func testDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestGetOrCreate_NewSession(t *testing.T) {
	sm := NewSessionManager(testDB(t), "agent1")

	sess := sm.GetOrCreate("chan:123")
	if sess == nil {
		t.Fatal("expected non-nil session")
	}
	if sess.Key != "chan:123" {
		t.Errorf("Key = %q, want %q", sess.Key, "chan:123")
	}
	if len(sess.Messages) != 0 {
		t.Errorf("expected empty messages, got %d", len(sess.Messages))
	}
}

func TestGetOrCreate_Idempotent(t *testing.T) {
	sm := NewSessionManager(testDB(t), "agent1")

	sm.GetOrCreate("key1")
	sm.AddMessage("key1", "user", "hello")

	sess := sm.GetOrCreate("key1")
	if len(sess.Messages) != 1 {
		t.Errorf("expected 1 message after second GetOrCreate, got %d", len(sess.Messages))
	}
}

func TestAddMessage_And_GetHistory(t *testing.T) {
	sm := NewSessionManager(testDB(t), "agent1")

	sm.GetOrCreate("s1")
	sm.AddMessage("s1", "user", "hello")
	sm.AddMessage("s1", "assistant", "world")

	history := sm.GetHistory("s1")
	if len(history) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(history))
	}
	if history[0].Role != "user" || history[0].Content != "hello" {
		t.Errorf("message[0] = {%s, %q}", history[0].Role, history[0].Content)
	}
	if history[1].Role != "assistant" || history[1].Content != "world" {
		t.Errorf("message[1] = {%s, %q}", history[1].Role, history[1].Content)
	}
}

func TestAddFullMessage_ToolCalls(t *testing.T) {
	sm := NewSessionManager(testDB(t), "agent1")

	sm.GetOrCreate("s2")
	msg := providers.Message{
		Role:    "assistant",
		Content: "calling a tool",
		ToolCalls: []providers.ToolCall{
			{ID: "call1", Type: "function"},
		},
	}
	sm.AddFullMessage("s2", msg)

	history := sm.GetHistory("s2")
	if len(history) != 1 {
		t.Fatalf("expected 1 message, got %d", len(history))
	}
	if len(history[0].ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(history[0].ToolCalls))
	}
}

func TestGetSummary_SetSummary(t *testing.T) {
	sm := NewSessionManager(testDB(t), "agent1")

	sm.GetOrCreate("s3")

	if s := sm.GetSummary("s3"); s != "" {
		t.Errorf("expected empty summary initially, got %q", s)
	}

	sm.SetSummary("s3", "this is a summary")
	if s := sm.GetSummary("s3"); s != "this is a summary" {
		t.Errorf("GetSummary = %q, want %q", s, "this is a summary")
	}
}

func TestTruncateHistory(t *testing.T) {
	sm := NewSessionManager(testDB(t), "agent1")

	sm.GetOrCreate("s4")
	for i := 0; i < 5; i++ {
		sm.AddMessage("s4", "user", "msg")
	}

	sm.TruncateHistory("s4", 2)

	history := sm.GetHistory("s4")
	if len(history) != 2 {
		t.Errorf("after TruncateHistory(2): got %d messages, want 2", len(history))
	}
}

func TestSetHistory(t *testing.T) {
	sm := NewSessionManager(testDB(t), "agent1")

	sm.GetOrCreate("s5")
	sm.AddMessage("s5", "user", "original")

	replacement := []providers.Message{
		{Role: "user", Content: "replaced1"},
		{Role: "assistant", Content: "replaced2"},
	}
	sm.SetHistory("s5", replacement)

	history := sm.GetHistory("s5")
	if len(history) != 2 {
		t.Fatalf("after SetHistory: got %d messages, want 2", len(history))
	}
	if history[0].Content != "replaced1" {
		t.Errorf("history[0].Content = %q, want 'replaced1'", history[0].Content)
	}
}

func TestListSessions(t *testing.T) {
	sm := NewSessionManager(testDB(t), "agent1")

	sm.GetOrCreate("web:ui")
	sm.GetOrCreate("cli:main")
	sm.AddMessage("web:ui", "user", "hello from web")

	metas := sm.ListSessions()
	if len(metas) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(metas))
	}

	// Most recently updated should be first
	if metas[0].Key != "web:ui" && metas[0].Key != "cli:main" {
		t.Errorf("unexpected session key: %q", metas[0].Key)
	}
}

func TestDeleteSession(t *testing.T) {
	sm := NewSessionManager(testDB(t), "agent1")

	sm.GetOrCreate("del1")
	sm.AddMessage("del1", "user", "hello")

	if err := sm.DeleteSession("del1"); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	history := sm.GetHistory("del1")
	if len(history) != 0 {
		t.Errorf("expected 0 messages after delete, got %d", len(history))
	}

	metas := sm.ListSessions()
	for _, m := range metas {
		if m.Key == "del1" {
			t.Error("deleted session still appears in ListSessions")
		}
	}
}

func TestSave_IsNoop(t *testing.T) {
	sm := NewSessionManager(testDB(t), "agent1")
	sm.GetOrCreate("noop")
	sm.AddMessage("noop", "user", "hello")

	if err := sm.Save("noop"); err != nil {
		t.Errorf("Save should be a no-op and return nil, got: %v", err)
	}
}

func TestInferChannel(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"web:ui:2026-03-04T10:00:00Z", "web"},
		{"agent:main:telegram:direct:123", "telegram"},
		{"agent:main:discord:server:456", "discord"},
		{"agent:main:slack:C01234:789", "slack"},
		{"subagent:helper:session1", "subagent"},
		{"heartbeat", "heartbeat"},
		{"cli-session", "cli"},
	}
	for _, tt := range tests {
		got := inferChannel(tt.key)
		if got != tt.want {
			t.Errorf("inferChannel(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestTruncatePreview(t *testing.T) {
	short := truncatePreview("short", sessionPreviewMaxLen)
	if short != "short" {
		t.Fatalf("truncatePreview short = %q, want %q", short, "short")
	}

	longInput := strings.Repeat("a", sessionPreviewMaxLen+5)
	truncated := truncatePreview(longInput, sessionPreviewMaxLen)
	if len(truncated) != sessionPreviewMaxLen+len("…") {
		t.Fatalf("truncatePreview length = %d, want %d", len(truncated), sessionPreviewMaxLen+len("…"))
	}
	if !strings.HasSuffix(truncated, "…") {
		t.Fatalf("truncatePreview should append ellipsis, got %q", truncated)
	}
}

func TestShouldRotateHelpers(t *testing.T) {
	if !shouldRotateByMessageCount(6, 5) {
		t.Fatal("shouldRotateByMessageCount should rotate when over threshold")
	}
	if shouldRotateByMessageCount(5, 5) {
		t.Fatal("shouldRotateByMessageCount should not rotate at threshold")
	}

	msgs := []providers.Message{{Content: strings.Repeat("a", 40)}}
	if !shouldRotateByTokenEstimate(msgs, 9) {
		t.Fatal("shouldRotateByTokenEstimate should rotate when estimate exceeds threshold")
	}
	if shouldRotateByTokenEstimate(msgs, 10) {
		t.Fatal("shouldRotateByTokenEstimate should not rotate at threshold")
	}

	now := time.Now()
	if !shouldRotateByAge(now.Add(-2*time.Hour), time.Hour, now) {
		t.Fatal("shouldRotateByAge should rotate when session is older than max age")
	}
	if shouldRotateByAge(now.Add(-30*time.Minute), time.Hour, now) {
		t.Fatal("shouldRotateByAge should not rotate when session is newer than max age")
	}
}

func TestShouldRotate(t *testing.T) {
	sm := NewSessionManager(testDB(t), "agent1")
	sessionKey := "rotate-test"
	sm.GetOrCreate(sessionKey)

	for i := 0; i < 3; i++ {
		sm.AddMessage(sessionKey, "user", strings.Repeat("a", 40))
	}

	if !sm.ShouldRotate(sessionKey, SessionRotationPolicy{MaxMessages: 2}) {
		t.Fatal("ShouldRotate should rotate on message-count threshold")
	}
	if !sm.ShouldRotate(sessionKey, SessionRotationPolicy{MaxTokenEstimate: 20}) {
		t.Fatal("ShouldRotate should rotate on token-estimate threshold")
	}
	if sm.ShouldRotate(sessionKey, SessionRotationPolicy{MaxMessages: 10, MaxTokenEstimate: 1000}) {
		t.Fatal("ShouldRotate should not rotate when thresholds are not exceeded")
	}
}
