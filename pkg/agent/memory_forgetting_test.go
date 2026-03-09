package agent

import (
	"testing"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openForgettingTestDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestPrune_StaleNodes(t *testing.T) {
	db := openForgettingTestDB(t)

	// Create a node that will never be accessed
	_, _ = db.UpsertNode("a1", "concept", "Forgotten", "{}")

	pruner := NewMemoryPruner(db, "a1")
	report, err := pruner.Prune(PruneOptions{
		MaxAge:         1 * time.Millisecond, // very short — everything is "stale"
		MinAccessCount: 1,                    // needs at least 1 access to survive
		ScoreThreshold: 0.1,
		HalfLife:       30,
	})
	require.NoError(t, err)

	// The node should be pruned since it has 0 accesses
	assert.Equal(t, 1, report.Pruned)
	assert.False(t, report.DryRun)

	// Verify node is gone
	assert.Equal(t, 0, db.CountNodes("a1"))
}

func TestPrune_FrequentNodes_Survive(t *testing.T) {
	db := openForgettingTestDB(t)

	id, _ := db.UpsertNode("a1", "concept", "Active", "{}")
	// Make it actively used
	for i := 0; i < 10; i++ {
		db.TouchNode(id)
	}

	pruner := NewMemoryPruner(db, "a1")
	report, err := pruner.Prune(PruneOptions{
		MaxAge:         1 * time.Millisecond,
		MinAccessCount: 5,
		ScoreThreshold: 0.1,
		HalfLife:       30,
	})
	require.NoError(t, err)

	// Active node should survive (10 accesses ≥ minAccessCount of 5)
	assert.Equal(t, 0, report.Pruned)
	assert.Equal(t, 1, db.CountNodes("a1"))
}

func TestPrune_DryRun(t *testing.T) {
	db := openForgettingTestDB(t)

	_, _ = db.UpsertNode("a1", "concept", "ToBeDeleted", "{}")

	pruner := NewMemoryPruner(db, "a1")
	report, err := pruner.Prune(PruneOptions{
		MaxAge:         1 * time.Millisecond,
		MinAccessCount: 1,
		ScoreThreshold: 0.1,
		HalfLife:       30,
		DryRun:         true,
	})
	require.NoError(t, err)

	assert.True(t, report.DryRun)
	assert.Equal(t, 1, report.Pruned)

	// Node should still exist!
	assert.Equal(t, 1, db.CountNodes("a1"))
}

func TestPrune_NilDB(t *testing.T) {
	pruner := NewMemoryPruner(nil, "a1")
	report, err := pruner.Prune(DefaultPruneOptions())
	assert.NoError(t, err)
	assert.Equal(t, 0, report.Pruned)
}

func TestPrune_DefaultOptions(t *testing.T) {
	opts := DefaultPruneOptions()
	assert.Equal(t, 90*24*time.Hour, opts.MaxAge)
	assert.Equal(t, 2, opts.MinAccessCount)
	assert.InDelta(t, 0.1, opts.ScoreThreshold, 0.001)
	assert.InDelta(t, 30.0, opts.HalfLife, 0.001)
	assert.False(t, opts.DryRun)
}
