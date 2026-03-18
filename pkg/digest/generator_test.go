package digest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
)

func openTestDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestDigestGenerator_Generate(t *testing.T) {
	db := openTestDB(t)

	// Seed a session with messages and tool calls.
	_, err := db.GetOrCreateSession("test-session-1", "main")
	require.NoError(t, err)

	require.NoError(t, db.AppendMessage("test-session-1", providers.Message{
		Role:    "user",
		Content: "Deploy the new service",
	}))
	require.NoError(t, db.AppendMessage("test-session-1", providers.Message{
		Role:    "assistant",
		Content: "Deploying now.",
		ToolCalls: []providers.ToolCall{
			{ID: "tc1", Function: &providers.FunctionCall{Name: "exec", Arguments: "{}"}},
			{ID: "tc2", Function: &providers.FunctionCall{Name: "exec", Arguments: "{}"}},
		},
	}))

	// Seed a memory note.
	require.NoError(t, db.SetNote("main", "observation", "2026-03-18", "User prefers concise responses"))

	dg := NewDigestGenerator(db)

	since := time.Now().Add(-1 * time.Hour)
	until := time.Now().Add(1 * time.Hour)

	prompt, err := dg.Generate(context.Background(), since, until, DigestConfig{
		Period:        "daily",
		Channel:       "telegram",
		ChatID:        "123",
		AgentID:       "main",
		IncludeMemory: true,
	})
	require.NoError(t, err)

	// Verify the prompt contains expected sections.
	assert.Contains(t, prompt, "digest report for the period")
	assert.Contains(t, prompt, "Activity summary:")
	assert.Contains(t, prompt, "1 sessions")
	assert.Contains(t, prompt, "2 tool calls executed")
	assert.Contains(t, prompt, "Topics discussed:")
	assert.Contains(t, prompt, "Recent memory notes:")
	assert.Contains(t, prompt, "User prefers concise responses")
	assert.Contains(t, prompt, "A brief overview of activity")
	assert.Contains(t, prompt, "Key topics and decisions")
	assert.Contains(t, prompt, "outstanding items")
	assert.Contains(t, prompt, "Notable patterns or trends")
}

func TestDigestGenerator_EmptyPeriod(t *testing.T) {
	db := openTestDB(t)

	dg := NewDigestGenerator(db)

	// Use a time range in the far future so nothing matches.
	since := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC)

	prompt, err := dg.Generate(context.Background(), since, until, DigestConfig{
		Period:        "daily",
		IncludeMemory: true,
	})
	require.NoError(t, err)

	assert.Contains(t, prompt, "No activity was recorded during this period")
	assert.Contains(t, prompt, "digest report for the period")
}

func TestDigestGenerator_NoMemoryNotes(t *testing.T) {
	db := openTestDB(t)

	// Create a session but no memory notes.
	_, err := db.GetOrCreateSession("test-session-2", "main")
	require.NoError(t, err)

	require.NoError(t, db.AppendMessage("test-session-2", providers.Message{
		Role:    "user",
		Content: "Hello world",
	}))

	dg := NewDigestGenerator(db)

	since := time.Now().Add(-1 * time.Hour)
	until := time.Now().Add(1 * time.Hour)

	prompt, err := dg.Generate(context.Background(), since, until, DigestConfig{
		Period:        "daily",
		IncludeMemory: true,
	})
	require.NoError(t, err)

	assert.Contains(t, prompt, "Activity summary:")
	assert.Contains(t, prompt, "1 sessions")
	assert.NotContains(t, prompt, "Recent memory notes:")
}

func TestDigestGenerator_MemoryDisabled(t *testing.T) {
	db := openTestDB(t)

	// Create session and memory note.
	_, err := db.GetOrCreateSession("test-session-3", "main")
	require.NoError(t, err)

	require.NoError(t, db.AppendMessage("test-session-3", providers.Message{
		Role:    "user",
		Content: "Test message",
	}))
	require.NoError(t, db.SetNote("main", "observation", "2026-03-18", "Some note"))

	dg := NewDigestGenerator(db)

	since := time.Now().Add(-1 * time.Hour)
	until := time.Now().Add(1 * time.Hour)

	prompt, err := dg.Generate(context.Background(), since, until, DigestConfig{
		Period:        "weekly",
		IncludeMemory: false, // memory notes disabled
	})
	require.NoError(t, err)

	// Should have activity but no memory notes section.
	assert.Contains(t, prompt, "Activity summary:")
	assert.NotContains(t, prompt, "Recent memory notes:")
}
