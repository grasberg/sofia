package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grasberg/sofia/pkg/bus"
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
