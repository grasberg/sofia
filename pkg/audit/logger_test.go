package audit

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestLogger(t *testing.T) *AuditLogger {
	t.Helper()
	al, err := NewAuditLogger(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = al.Close() })
	return al
}

func TestAuditLogger_LogAndQuery(t *testing.T) {
	al := openTestLogger(t)

	err := al.Log(AuditEntry{
		AgentID:    "agent-1",
		SessionKey: "sess-abc",
		Channel:    "telegram",
		Action:     "llm_call",
		Detail:     "Called GPT-4o",
		Input:      "Hello",
		Output:     "Hi there!",
		Duration:   250,
		Success:    true,
		Metadata:   `{"model":"gpt-4o"}`,
	})
	require.NoError(t, err)

	err = al.Log(AuditEntry{
		AgentID: "agent-1",
		Action:  "tool_call",
		Detail:  "exec: ls -la",
		Success: true,
	})
	require.NoError(t, err)

	entries, err := al.Query(QueryOpts{})
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	// Newest first
	assert.Equal(t, "tool_call", entries[0].Action)
	assert.Equal(t, "llm_call", entries[1].Action)

	// Check full entry fields
	llm := entries[1]
	assert.Equal(t, "agent-1", llm.AgentID)
	assert.Equal(t, "sess-abc", llm.SessionKey)
	assert.Equal(t, "telegram", llm.Channel)
	assert.Equal(t, "Called GPT-4o", llm.Detail)
	assert.Equal(t, "Hello", llm.Input)
	assert.Equal(t, "Hi there!", llm.Output)
	assert.Equal(t, int64(250), llm.Duration)
	assert.True(t, llm.Success)
	assert.Equal(t, `{"model":"gpt-4o"}`, llm.Metadata)
	assert.False(t, llm.Timestamp.IsZero())
}

func TestAuditLogger_QueryByAction(t *testing.T) {
	al := openTestLogger(t)

	_ = al.Log(AuditEntry{AgentID: "a1", Action: "llm_call", Detail: "call 1", Success: true})
	_ = al.Log(AuditEntry{AgentID: "a1", Action: "tool_call", Detail: "tool 1", Success: true})
	_ = al.Log(AuditEntry{AgentID: "a1", Action: "llm_call", Detail: "call 2", Success: true})
	_ = al.Log(AuditEntry{AgentID: "a1", Action: "config_change", Detail: "updated model", Success: true})

	entries, err := al.Query(QueryOpts{Action: "llm_call"})
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	for _, e := range entries {
		assert.Equal(t, "llm_call", e.Action)
	}

	entries, err = al.Query(QueryOpts{Action: "config_change"})
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "updated model", entries[0].Detail)
}

func TestAuditLogger_QueryByAgent(t *testing.T) {
	al := openTestLogger(t)

	_ = al.Log(AuditEntry{AgentID: "agent-alpha", Action: "llm_call", Detail: "alpha call", Success: true})
	_ = al.Log(AuditEntry{AgentID: "agent-beta", Action: "llm_call", Detail: "beta call", Success: true})
	_ = al.Log(AuditEntry{AgentID: "agent-alpha", Action: "tool_call", Detail: "alpha tool", Success: true})

	entries, err := al.Query(QueryOpts{AgentID: "agent-alpha"})
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	for _, e := range entries {
		assert.Equal(t, "agent-alpha", e.AgentID)
	}

	entries, err = al.Query(QueryOpts{AgentID: "agent-beta"})
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "beta call", entries[0].Detail)
}

func TestAuditLogger_QueryWithLimit(t *testing.T) {
	al := openTestLogger(t)

	for i := range 10 {
		_ = al.Log(AuditEntry{
			AgentID: "a1",
			Action:  "llm_call",
			Detail:  fmt.Sprintf("call %d", i),
			Success: true,
		})
	}

	// Page 1
	page1, err := al.Query(QueryOpts{Limit: 3, Offset: 0})
	require.NoError(t, err)
	assert.Len(t, page1, 3)

	// Page 2
	page2, err := al.Query(QueryOpts{Limit: 3, Offset: 3})
	require.NoError(t, err)
	assert.Len(t, page2, 3)

	// Pages should not overlap
	assert.NotEqual(t, page1[0].ID, page2[0].ID)

	// Fetch all
	all, err := al.Query(QueryOpts{Limit: 100})
	require.NoError(t, err)
	assert.Len(t, all, 10)
}

func TestAuditLogger_QueryTimeRange(t *testing.T) {
	al := openTestLogger(t)

	t1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 1, 1, 14, 0, 0, 0, time.UTC)

	_ = al.Log(AuditEntry{AgentID: "a1", Action: "llm_call", Detail: "morning", Timestamp: t1, Success: true})
	_ = al.Log(AuditEntry{AgentID: "a1", Action: "llm_call", Detail: "noon", Timestamp: t2, Success: true})
	_ = al.Log(AuditEntry{AgentID: "a1", Action: "llm_call", Detail: "afternoon", Timestamp: t3, Success: true})

	// Since noon
	entries, err := al.Query(QueryOpts{Since: t2})
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Equal(t, "afternoon", entries[0].Detail)
	assert.Equal(t, "noon", entries[1].Detail)

	// Until noon (inclusive)
	entries, err = al.Query(QueryOpts{Until: t2})
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Equal(t, "noon", entries[0].Detail)
	assert.Equal(t, "morning", entries[1].Detail)

	// Exact window: noon to noon
	entries, err = al.Query(QueryOpts{Since: t2, Until: t2})
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "noon", entries[0].Detail)
}
