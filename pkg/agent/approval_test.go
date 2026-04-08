package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/config"
)

func newTestApprovalConfig(tools []string, patterns []string) config.ApprovalConfig {
	return config.ApprovalConfig{
		Enabled:       true,
		RequireFor:    tools,
		PatternMatch:  patterns,
		TimeoutSec:    1,
		DefaultAction: "deny",
	}
}

func TestApprovalGate_RequiresApproval_ByToolName(t *testing.T) {
	cfg := newTestApprovalConfig([]string{"exec", "write_file"}, nil)
	gate := NewApprovalGate(cfg)

	assert.True(t, gate.RequiresApproval("", "exec", `{"command":"rm -rf /"}`))
	assert.True(t, gate.RequiresApproval("", "write_file", `{"path":"/etc/passwd"}`))
}

func TestApprovalGate_RequiresApproval_ByPattern(t *testing.T) {
	cfg := newTestApprovalConfig(nil, []string{`rm\s+-rf`, `sudo\s+`})
	gate := NewApprovalGate(cfg)

	assert.True(t, gate.RequiresApproval("", "exec", `{"command":"rm -rf /tmp/data"}`))
	assert.True(t, gate.RequiresApproval("", "exec", `{"command":"sudo reboot"}`))
}

func TestApprovalGate_DoesNotRequireApproval(t *testing.T) {
	cfg := newTestApprovalConfig([]string{"exec"}, []string{`rm\s+-rf`})
	gate := NewApprovalGate(cfg)

	assert.False(t, gate.RequiresApproval("", "read_file", `{"path":"/tmp/foo.txt"}`))
	assert.False(t, gate.RequiresApproval("", "list_dir", `{"path":"."}`))
}

func TestApprovalGate_DoesNotRequireApproval_WhenDisabled(t *testing.T) {
	cfg := newTestApprovalConfig([]string{"exec"}, nil)
	cfg.Enabled = false
	gate := NewApprovalGate(cfg)

	assert.False(t, gate.RequiresApproval("", "exec", `{"command":"rm -rf /"}`))
}

func TestApprovalGate_ApproveFlow(t *testing.T) {
	cfg := newTestApprovalConfig([]string{"exec"}, nil)
	cfg.TimeoutSec = 5
	gate := NewApprovalGate(cfg)

	req := ApprovalRequest{
		ID:       "req-1",
		ToolName: "exec",
		AgentID:  "main",
	}

	var approved bool
	var err error
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		approved, err = gate.RequestApproval(context.Background(), req)
	}()

	// Wait briefly for the request to register
	time.Sleep(50 * time.Millisecond)

	// Verify it appears in pending
	pending := gate.ListPending()
	require.Len(t, pending, 1)
	assert.Equal(t, "req-1", pending[0].ID)

	// Approve it
	require.NoError(t, gate.Approve("req-1"))

	wg.Wait()
	require.NoError(t, err)
	assert.True(t, approved)
}

func TestApprovalGate_DenyFlow(t *testing.T) {
	cfg := newTestApprovalConfig([]string{"exec"}, nil)
	cfg.TimeoutSec = 5
	gate := NewApprovalGate(cfg)

	req := ApprovalRequest{
		ID:       "req-2",
		ToolName: "exec",
		AgentID:  "main",
	}

	var approved bool
	var err error
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		approved, err = gate.RequestApproval(context.Background(), req)
	}()

	time.Sleep(50 * time.Millisecond)

	require.NoError(t, gate.Deny("req-2"))

	wg.Wait()
	require.NoError(t, err)
	assert.False(t, approved)
}

func TestApprovalGate_Timeout(t *testing.T) {
	// Test with default action "deny"
	cfg := newTestApprovalConfig([]string{"exec"}, nil)
	cfg.TimeoutSec = 1
	cfg.DefaultAction = "deny"
	gate := NewApprovalGate(cfg)

	req := ApprovalRequest{
		ID:       "req-timeout-deny",
		ToolName: "exec",
		AgentID:  "main",
	}

	start := time.Now()
	approved, err := gate.RequestApproval(context.Background(), req)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.False(t, approved, "default action 'deny' should return false on timeout")
	assert.GreaterOrEqual(t, elapsed, 900*time.Millisecond, "should wait approximately the timeout")

	// Test with default action "allow"
	cfgAllow := newTestApprovalConfig([]string{"exec"}, nil)
	cfgAllow.TimeoutSec = 1
	cfgAllow.DefaultAction = "allow"
	gateAllow := NewApprovalGate(cfgAllow)

	reqAllow := ApprovalRequest{
		ID:       "req-timeout-allow",
		ToolName: "exec",
		AgentID:  "main",
	}

	approved, err = gateAllow.RequestApproval(context.Background(), reqAllow)
	require.NoError(t, err)
	assert.True(t, approved, "default action 'allow' should return true on timeout")
}

func TestApprovalGate_ListPending(t *testing.T) {
	cfg := newTestApprovalConfig([]string{"exec"}, nil)
	cfg.TimeoutSec = 5
	gate := NewApprovalGate(cfg)

	// Initially empty
	assert.Empty(t, gate.ListPending())

	// Add two pending requests
	var wg sync.WaitGroup
	for _, id := range []string{"req-a", "req-b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = gate.RequestApproval(context.Background(), ApprovalRequest{
				ID:       id,
				ToolName: "exec",
				AgentID:  "main",
			})
		}()
	}

	time.Sleep(50 * time.Millisecond)

	pending := gate.ListPending()
	assert.Len(t, pending, 2)

	ids := map[string]bool{}
	for _, p := range pending {
		ids[p.ID] = true
	}
	assert.True(t, ids["req-a"])
	assert.True(t, ids["req-b"])

	// Approve both to unblock
	_ = gate.Approve("req-a")
	_ = gate.Approve("req-b")
	wg.Wait()
}

func TestApprovalGate_ContextCancellation(t *testing.T) {
	cfg := newTestApprovalConfig([]string{"exec"}, nil)
	cfg.TimeoutSec = 60
	gate := NewApprovalGate(cfg)

	ctx, cancel := context.WithCancel(context.Background())

	req := ApprovalRequest{
		ID:       "req-ctx",
		ToolName: "exec",
		AgentID:  "main",
	}

	var approved bool
	var err error
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		approved, err = gate.RequestApproval(ctx, req)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	wg.Wait()
	assert.Error(t, err)
	assert.False(t, approved)
}

func TestApprovalGate_ApproveNonexistent(t *testing.T) {
	cfg := newTestApprovalConfig(nil, nil)
	gate := NewApprovalGate(cfg)

	err := gate.Approve("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestApprovalGate_DenyNonexistent(t *testing.T) {
	cfg := newTestApprovalConfig(nil, nil)
	gate := NewApprovalGate(cfg)

	err := gate.Deny("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestApprovalGate_InvalidPattern(t *testing.T) {
	cfg := newTestApprovalConfig(nil, []string{`[invalid`})
	gate := NewApprovalGate(cfg)

	// Invalid pattern should be skipped, so nothing matches
	assert.False(t, gate.RequiresApproval("", "exec", `{"command":"anything"}`))
	assert.Empty(t, gate.patterns)
}

func TestApprovalGate_SetBypass(t *testing.T) {
	cfg := newTestApprovalConfig([]string{"exec"}, nil)
	gate := NewApprovalGate(cfg)

	sessionKey := "test-session-123"

	// Not bypassed by default
	assert.False(t, gate.IsBypassed(sessionKey))

	// Enable bypass
	gate.SetBypass(sessionKey, true)
	assert.True(t, gate.IsBypassed(sessionKey))

	// Disable bypass
	gate.SetBypass(sessionKey, false)
	assert.False(t, gate.IsBypassed(sessionKey))
}

func TestApprovalGate_BypassSkipsApproval(t *testing.T) {
	cfg := newTestApprovalConfig([]string{"exec"}, []string{`rm\s+-rf`})
	gate := NewApprovalGate(cfg)

	sessionKey := "yolo-session"
	otherSession := "normal-session"

	// Without bypass, exec requires approval
	assert.True(t, gate.RequiresApproval(sessionKey, "exec", `{"command":"rm -rf /tmp"}`))
	assert.True(t, gate.RequiresApproval(otherSession, "exec", `{"command":"rm -rf /tmp"}`))

	// Enable bypass for one session only
	gate.SetBypass(sessionKey, true)

	// Bypassed session skips approval even for tools that would normally require it
	assert.False(t, gate.RequiresApproval(sessionKey, "exec", `{"command":"rm -rf /tmp"}`))
	assert.False(t, gate.RequiresApproval(sessionKey, "exec", `{"command":"anything"}`))

	// Non-bypassed session still requires approval
	assert.True(t, gate.RequiresApproval(otherSession, "exec", `{"command":"rm -rf /tmp"}`))

	// Disabling bypass restores normal behaviour
	gate.SetBypass(sessionKey, false)
	assert.True(t, gate.RequiresApproval(sessionKey, "exec", `{"command":"rm -rf /tmp"}`))
}
