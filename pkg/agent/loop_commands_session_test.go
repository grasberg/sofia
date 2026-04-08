package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/providers"
)

func TestHandleSessionCommand_CheckpointRollbackRestoresSummary(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "session-checkpoint-command-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfg(nil)
	cfg.Agents.Defaults.Workspace = tmpDir
	cfg.MemoryDB = filepath.Join(tmpDir, "memory.db")

	al := NewAgentLoop(cfg, bus.NewMessageBus(), &mockRegistryProvider{})
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	sessionKey := "cli:test-checkpoint"
	agent.Sessions.GetOrCreate(sessionKey)
	agent.Sessions.AddMessage(sessionKey, "user", "hello")
	agent.Sessions.AddMessage(sessionKey, "assistant", "hi")
	agent.Sessions.SetSummary(sessionKey, "summary before checkpoint")

	resp, handled := al.handleSessionCommand(
		context.Background(),
		newTestInboundMessage("/checkpoint create before-risky-change"),
		agent,
		sessionKey,
	)
	if !handled {
		t.Fatal("expected /checkpoint create to be handled")
	}
	if !strings.Contains(resp, "Checkpoint created.") {
		t.Fatalf("unexpected create response: %s", resp)
	}

	agent.Sessions.AddMessage(sessionKey, "user", "make changes")
	agent.Sessions.AddMessage(sessionKey, "assistant", "done")
	agent.Sessions.SetSummary(sessionKey, "summary after checkpoint")

	resp, handled = al.handleSessionCommand(
		context.Background(),
		newTestInboundMessage("/checkpoint rollback latest"),
		agent,
		sessionKey,
	)
	if !handled {
		t.Fatal("expected /checkpoint rollback to be handled")
	}
	if !strings.Contains(resp, "Session summary restored.") {
		t.Fatalf("unexpected rollback response: %s", resp)
	}

	history := agent.Sessions.GetHistory(sessionKey)
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
	if got := agent.Sessions.GetSummary(sessionKey); got != "summary before checkpoint" {
		t.Fatalf("summary after rollback = %q, want %q", got, "summary before checkpoint")
	}
}

func TestHandleSessionCommand_HealthReportsSessionPressure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "session-health-command-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfg(nil)
	cfg.Agents.Defaults.Workspace = tmpDir
	cfg.MemoryDB = filepath.Join(tmpDir, "memory.db")

	al := NewAgentLoop(cfg, bus.NewMessageBus(), &mockRegistryProvider{})
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	sessionKey := "cli:test-health"
	agent.Sessions.GetOrCreate(sessionKey)
	for i := 0; i < 21; i++ {
		agent.Sessions.AddMessage(sessionKey, "user", "health check message")
	}

	resp, handled := al.handleSessionCommand(
		context.Background(),
		newTestInboundMessage("/health"),
		agent,
		sessionKey,
	)
	if !handled {
		t.Fatal("expected /health to be handled")
	}
	if !strings.Contains(resp, "Session health: ATTENTION") {
		t.Fatalf("unexpected health response: %s", resp)
	}
	if !strings.Contains(resp, "message history crossed Sofia's auto-summary threshold") {
		t.Fatalf("expected threshold signal in health response: %s", resp)
	}
	if !strings.Contains(resp, "Run /compact") {
		t.Fatalf("expected compact recommendation in health response: %s", resp)
	}
	if !strings.Contains(resp, "/checkpoint create <name>") {
		t.Fatalf("expected checkpoint recommendation in health response: %s", resp)
	}
}

func TestHandleSessionCommand_PauseAndResume(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "session-pause-resume-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfg(nil)
	cfg.Agents.Defaults.Workspace = tmpDir
	cfg.MemoryDB = filepath.Join(tmpDir, "memory.db")

	al := NewAgentLoop(cfg, bus.NewMessageBus(), &mockRegistryProvider{})
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	sessionKey := "cli:test-pause"
	agent.Sessions.GetOrCreate(sessionKey)
	agent.Sessions.AddMessage(sessionKey, "user", "working on feature X")
	agent.Sessions.AddMessage(sessionKey, "assistant", "sure, started on feature X")
	agent.Sessions.SetSummary(sessionKey, "discussion about feature X")

	// Pause session with a note.
	resp, handled := al.handleSessionCommand(
		context.Background(),
		newTestInboundMessage("/pause need to pick this up tomorrow"),
		agent,
		sessionKey,
	)
	if !handled {
		t.Fatal("expected /pause to be handled")
	}
	if !strings.Contains(resp, "Session paused.") {
		t.Fatalf("unexpected pause response: %s", resp)
	}
	if !strings.Contains(resp, "need to pick this up tomorrow") {
		t.Fatalf("expected note in pause response: %s", resp)
	}
	if !strings.Contains(resp, "Checkpoint:") {
		t.Fatalf("expected checkpoint in pause response: %s", resp)
	}

	// Verify handoff is stored in DB.
	raw := al.memDB.GetNote(agent.ID, handoffNoteKind, sessionKey)
	if raw == "" {
		t.Fatal("expected handoff note in DB")
	}
	var h Handoff
	if err := json.Unmarshal([]byte(raw), &h); err != nil {
		t.Fatalf("failed to parse stored handoff: %v", err)
	}
	if h.Note != "need to pick this up tomorrow" {
		t.Fatalf("handoff note = %q, want %q", h.Note, "need to pick this up tomorrow")
	}
	if h.Messages != 2 {
		t.Fatalf("handoff messages = %d, want 2", h.Messages)
	}
	if h.Summary != "discussion about feature X" {
		t.Fatalf("handoff summary = %q, want %q", h.Summary, "discussion about feature X")
	}
	if len(h.Context) != 2 {
		t.Fatalf("handoff context lines = %d, want 2", len(h.Context))
	}

	// Resume the session.
	resp, handled = al.handleSessionCommand(
		context.Background(),
		newTestInboundMessage("/resume"),
		agent,
		sessionKey,
	)
	if !handled {
		t.Fatal("expected /resume to be handled")
	}
	if !strings.Contains(resp, "Resuming session.") {
		t.Fatalf("unexpected resume response: %s", resp)
	}
	if !strings.Contains(resp, "need to pick this up tomorrow") {
		t.Fatalf("expected handoff note in resume response: %s", resp)
	}
	if !strings.Contains(resp, "discussion about feature X") {
		t.Fatalf("expected summary in resume response: %s", resp)
	}
	if !strings.Contains(resp, "Last activity:") {
		t.Fatalf("expected context lines in resume response: %s", resp)
	}

	// Handoff should be cleared after resume.
	raw = al.memDB.GetNote(agent.ID, handoffNoteKind, sessionKey)
	if raw != "" {
		t.Fatal("expected handoff to be cleared after resume")
	}
}

func TestHandleSessionCommand_ResumeListsPausedSessions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "session-resume-list-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfg(nil)
	cfg.Agents.Defaults.Workspace = tmpDir
	cfg.MemoryDB = filepath.Join(tmpDir, "memory.db")

	al := NewAgentLoop(cfg, bus.NewMessageBus(), &mockRegistryProvider{})
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	// Pause a different session so there is a handoff in the DB.
	otherKey := "cli:other-session"
	agent.Sessions.GetOrCreate(otherKey)
	agent.Sessions.AddMessage(otherKey, "user", "some work")

	_, _ = al.handleSessionCommand(
		context.Background(),
		newTestInboundMessage("/pause paused other session"),
		agent,
		otherKey,
	)

	// Resume from a new session where no handoff exists — should list the paused one.
	newKey := "cli:fresh-session"
	agent.Sessions.GetOrCreate(newKey)

	resp, handled := al.handleSessionCommand(
		context.Background(),
		newTestInboundMessage("/resume"),
		agent,
		newKey,
	)
	if !handled {
		t.Fatal("expected /resume to be handled")
	}
	if !strings.Contains(resp, "No handoff for the current session") {
		t.Fatalf("expected listing header: %s", resp)
	}
	if !strings.Contains(resp, otherKey) {
		t.Fatalf("expected other session key in listing: %s", resp)
	}
	if !strings.Contains(resp, "paused other session") {
		t.Fatalf("expected note text in listing: %s", resp)
	}
}

func TestHandleSessionCommand_PauseEmptySession(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "session-pause-empty-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfg(nil)
	cfg.Agents.Defaults.Workspace = tmpDir
	cfg.MemoryDB = filepath.Join(tmpDir, "memory.db")

	al := NewAgentLoop(cfg, bus.NewMessageBus(), &mockRegistryProvider{})
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	sessionKey := "cli:empty-session"
	agent.Sessions.GetOrCreate(sessionKey)

	resp, handled := al.handleSessionCommand(
		context.Background(),
		newTestInboundMessage("/pause"),
		agent,
		sessionKey,
	)
	if !handled {
		t.Fatal("expected /pause to be handled")
	}
	if !strings.Contains(resp, "Nothing to pause") {
		t.Fatalf("expected empty-session message: %s", resp)
	}
}

// TestBtw_DoesNotModifySessionHistory verifies that /btw does not persist
// the exchange to session history — the session is unchanged after the call.
func TestBtw_DoesNotModifySessionHistory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "btw-session-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfg(nil)
	cfg.Agents.Defaults.Workspace = tmpDir
	cfg.MemoryDB = filepath.Join(tmpDir, "memory.db")

	al := NewAgentLoop(cfg, bus.NewMessageBus(), &mockRegistryProvider{})
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	sessionKey := "cli:test-btw"
	agent.Sessions.GetOrCreate(sessionKey)
	agent.Sessions.AddMessage(sessionKey, "user", "initial message")
	agent.Sessions.AddMessage(sessionKey, "assistant", "initial response")

	historyBefore := agent.Sessions.GetHistory(sessionKey)

	// Issue a /btw command — should not touch session history.
	msg := newTestInboundMessage("/btw what does this function do?")
	resp, handled := al.handleSessionCommand(
		context.Background(),
		msg,
		agent,
		sessionKey,
	)
	if !handled {
		t.Fatal("expected /btw to be handled")
	}
	// Response should be prefixed with [btw]
	if !strings.HasPrefix(resp, "[btw]") {
		t.Fatalf("expected [btw] prefix in response, got: %s", resp)
	}

	// Session history must be unchanged.
	historyAfter := agent.Sessions.GetHistory(sessionKey)
	if len(historyAfter) != len(historyBefore) {
		t.Fatalf("session history length changed: before=%d after=%d",
			len(historyBefore), len(historyAfter))
	}
}

// TestBtw_EmptyQuestionReturnsUsage verifies that /btw without a question
// returns a usage hint.
func TestBtw_EmptyQuestionReturnsUsage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "btw-empty-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfg(nil)
	cfg.Agents.Defaults.Workspace = tmpDir
	cfg.MemoryDB = filepath.Join(tmpDir, "memory.db")

	al := NewAgentLoop(cfg, bus.NewMessageBus(), &mockRegistryProvider{})
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	sessionKey := "cli:test-btw-empty"
	agent.Sessions.GetOrCreate(sessionKey)

	msg := newTestInboundMessage("/btw")
	resp, handled := al.handleSessionCommand(
		context.Background(),
		msg,
		agent,
		sessionKey,
	)
	if !handled {
		t.Fatal("expected /btw to be handled")
	}
	if !strings.Contains(resp, "Usage:") {
		t.Fatalf("expected Usage hint, got: %s", resp)
	}
}

// TestBtw_GatewayChannelReturnsUnsupportedMessage verifies that /btw in a
// non-CLI/web channel returns the "not supported" message instead of doing an LLM call.
func TestBtw_GatewayChannelReturnsUnsupportedMessage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "btw-gateway-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfg(nil)
	cfg.Agents.Defaults.Workspace = tmpDir
	cfg.MemoryDB = filepath.Join(tmpDir, "memory.db")

	al := NewAgentLoop(cfg, bus.NewMessageBus(), &mockRegistryProvider{})
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	sessionKey := "telegram:test-btw"
	agent.Sessions.GetOrCreate(sessionKey)

	// Simulate a Telegram message using /btw
	msg := bus.InboundMessage{
		Content:  "/btw what does this function do?",
		Channel:  "telegram",
		ChatID:   "12345",
		SenderID: "user",
	}
	resp, handled := al.handleSessionCommand(
		context.Background(),
		msg,
		agent,
		sessionKey,
	)
	if !handled {
		t.Fatal("expected /btw to be handled in gateway channel")
	}
	if !strings.Contains(resp, "not supported") {
		t.Fatalf("expected 'not supported' message for gateway channel, got: %s", resp)
	}
}

// toolCallThenTextProvider is a stateful mock that returns a tool call on the
// first Chat() invocation and a plain text response on the second. This drives
// the tool-result branch of runLLMIteration so the unguarded AddFullMessage
// path (loop_llm.go) is exercised.
type toolCallThenTextProvider struct {
	calls atomic.Int32
}

func (m *toolCallThenTextProvider) Chat(
	_ context.Context,
	_ []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	n := m.calls.Add(1)
	if n == 1 {
		// First call: return a single tool call. The tool name "noop_btw_test"
		// is not registered, so ExecuteWithContext returns an error result and
		// the loop continues to a second LLM call.
		return &providers.LLMResponse{
			ToolCalls: []providers.ToolCall{
				{
					ID:   "call-btw-test-1",
					Type: "function",
					Function: &providers.FunctionCall{
						Name:      "noop_btw_test",
						Arguments: "{}",
					},
				},
			},
			FinishReason: "tool_calls",
		}, nil
	}
	// Second call: plain text — no more tool calls.
	return &providers.LLMResponse{
		Content:      "Tool result processed.",
		FinishReason: "stop",
	}, nil
}

func (m *toolCallThenTextProvider) GetDefaultModel() string { return "mock-tool-model" }

// TestBtw_WithToolCall_DoesNotModifySessionHistory verifies that a /btw question
// that triggers a tool call does NOT leak tool-result messages into session history.
// This specifically covers the AddFullMessage path at loop_llm.go:952 which must
// be guarded by !opts.Ephemeral.
func TestBtw_WithToolCall_DoesNotModifySessionHistory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "btw-toolcall-session-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfg(nil)
	cfg.Agents.Defaults.Workspace = tmpDir
	cfg.MemoryDB = filepath.Join(tmpDir, "memory.db")

	al := NewAgentLoop(cfg, bus.NewMessageBus(), &mockRegistryProvider{})
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	// Replace the agent's provider with one that returns a tool call on the first
	// LLM call and a text response on the second.
	agent.Provider = &toolCallThenTextProvider{}

	sessionKey := "cli:test-btw-toolcall"
	agent.Sessions.GetOrCreate(sessionKey)
	agent.Sessions.AddMessage(sessionKey, "user", "initial message")
	agent.Sessions.AddMessage(sessionKey, "assistant", "initial response")

	historyBefore := agent.Sessions.GetHistory(sessionKey)

	msg := newTestInboundMessage("/btw what does this function do?")
	resp, handled := al.handleSessionCommand(
		context.Background(),
		msg,
		agent,
		sessionKey,
	)
	if !handled {
		t.Fatal("expected /btw to be handled")
	}
	if !strings.HasPrefix(resp, "[btw]") {
		t.Fatalf("expected [btw] prefix in response, got: %s", resp)
	}

	// Session history must be unchanged — neither the assistant tool-call message
	// nor the tool-result message must have been persisted.
	historyAfter := agent.Sessions.GetHistory(sessionKey)
	if len(historyAfter) != len(historyBefore) {
		t.Fatalf("session history length changed: before=%d after=%d (tool-result leaked into session)",
			len(historyBefore), len(historyAfter))
	}
}
