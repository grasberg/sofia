package evolution

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChangelogWriter_WriteAndGet(t *testing.T) {
	db := openTestDB(t)
	cw := NewChangelogWriter(db)

	entry := ChangelogEntry{
		ID:        "entry-1",
		Timestamp: time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC),
		Action:    "spawn_agent",
		Summary:   "Created research agent",
		Details:   map[string]any{"model": "gpt-4o", "reason": "task delegation"},
	}

	err := cw.Write(&entry)
	require.NoError(t, err)

	got, err := cw.Get("entry-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "entry-1", got.ID)
	assert.Equal(t, "spawn_agent", got.Action)
	assert.Equal(t, "Created research agent", got.Summary)
	assert.Equal(t, "gpt-4o", got.Details["model"])
	assert.Equal(t, "task delegation", got.Details["reason"])
}

func TestChangelogWriter_WriteAutoID(t *testing.T) {
	db := openTestDB(t)
	cw := NewChangelogWriter(db)

	entry := ChangelogEntry{
		Action:  "modify_skill",
		Summary: "Updated web search skill",
	}
	err := cw.Write(&entry)
	require.NoError(t, err)

	// ID should have been auto-generated (non-empty UUID).
	assert.NotEmpty(t, entry.ID)
}

func TestChangelogWriter_GetNotFound(t *testing.T) {
	db := openTestDB(t)
	cw := NewChangelogWriter(db)

	got, err := cw.Get("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestChangelogWriter_Query(t *testing.T) {
	db := openTestDB(t)
	cw := NewChangelogWriter(db)

	base := time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		require.NoError(t, cw.Write(&ChangelogEntry{
			ID:        fmt.Sprintf("q-%d", i),
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			Action:    "test_action",
			Summary:   fmt.Sprintf("Entry %d", i),
		}))
	}

	// Query with limit 2, should return newest first.
	results, err := cw.Query(base, 2)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "Entry 2", results[0].Summary)
	assert.Equal(t, "Entry 1", results[1].Summary)
}

func TestChangelogWriter_QuerySince(t *testing.T) {
	db := openTestDB(t)
	cw := NewChangelogWriter(db)

	t1 := time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)

	require.NoError(t, cw.Write(&ChangelogEntry{
		ID: "old", Timestamp: t1, Action: "old_action", Summary: "Old entry",
	}))
	require.NoError(t, cw.Write(&ChangelogEntry{
		ID: "mid", Timestamp: t2, Action: "mid_action", Summary: "Mid entry",
	}))
	require.NoError(t, cw.Write(&ChangelogEntry{
		ID: "new", Timestamp: t3, Action: "new_action", Summary: "New entry",
	}))

	// Query since t2 should exclude the oldest entry.
	results, err := cw.Query(t2, 10)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "New entry", results[0].Summary)
	assert.Equal(t, "Mid entry", results[1].Summary)
}

func TestChangelogWriter_UpdateOutcome(t *testing.T) {
	db := openTestDB(t)
	cw := NewChangelogWriter(db)

	entry := ChangelogEntry{
		ID:        "outcome-test",
		Timestamp: time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC),
		Action:    "modify_prompt",
		Summary:   "Tweaked system prompt",
	}
	require.NoError(t, cw.Write(&entry))

	err := cw.UpdateOutcome("outcome-test", ActionOutcome{
		Result:       "improved",
		MetricBefore: 0.72,
		MetricAfter:  0.85,
	})
	require.NoError(t, err)

	got, err := cw.Get("outcome-test")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "improved", got.Outcome)
	assert.NotNil(t, got.VerifiedAt)
	assert.InDelta(t, 0.72, got.MetricBefore, 0.001)
	assert.InDelta(t, 0.85, got.MetricAfter, 0.001)
}

func TestChangelogWriter_UpdateOutcomeNotFound(t *testing.T) {
	db := openTestDB(t)
	cw := NewChangelogWriter(db)

	err := cw.UpdateOutcome("ghost", ActionOutcome{Result: "improved"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestChangelogWriter_QueryUnverified(t *testing.T) {
	db := openTestDB(t)
	cw := NewChangelogWriter(db)

	// Write one entry with outcome and one without.
	require.NoError(t, cw.Write(&ChangelogEntry{
		ID:        "verified-1",
		Timestamp: time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC),
		Action:    "spawn_agent",
		Summary:   "Already verified",
	}))
	require.NoError(t, cw.UpdateOutcome("verified-1", ActionOutcome{
		Result:       "improved",
		MetricBefore: 0.5,
		MetricAfter:  0.9,
	}))

	require.NoError(t, cw.Write(&ChangelogEntry{
		ID:        "unverified-1",
		Timestamp: time.Date(2026, 3, 18, 13, 0, 0, 0, time.UTC),
		Action:    "modify_skill",
		Summary:   "Pending verification",
	}))

	results, err := cw.QueryUnverified(10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "unverified-1", results[0].ID)
	assert.Equal(t, "Pending verification", results[0].Summary)
}
