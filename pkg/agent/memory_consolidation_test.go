package agent

import (
	"testing"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openConsolidationTestDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestConsolidate_MergeDuplicates(t *testing.T) {
	db := openConsolidationTestDB(t)

	// Create near-duplicate nodes (same label, similar names)
	id1, _ := db.UpsertNode("a1", "person", "Alice", `{"full_name":"Alice Smith"}`)
	id2, _ := db.UpsertNode("a1", "person", "alice", `{"short":true}`)
	id3, _ := db.UpsertNode("a1", "project", "Sofia", "{}")

	// Add edges from both to Sofia
	_ = db.UpsertEdge("a1", id1, id3, "works_on", 1.0, "{}")
	_ = db.UpsertEdge("a1", id2, id3, "works_on", 0.5, "{}")

	// Make Alice (capital) more accessed
	db.TouchNode(id1)
	db.TouchNode(id1)

	mc := NewMemoryConsolidator(db, "a1")
	report, err := mc.Consolidate()
	require.NoError(t, err)

	assert.Greater(t, report.MergedNodes, 0, "should have merged at least one node")
	assert.True(t, len(report.Details) > 0, "should have merge details")

	// "alice" should be gone
	node, _ := db.GetNode("a1", "person", "alice")
	assert.Nil(t, node, "lowercase 'alice' should have been merged away")

	// "Alice" should still exist
	node, _ = db.GetNode("a1", "person", "Alice")
	assert.NotNil(t, node, "capitalized 'Alice' should survive as primary")
}

func TestConsolidate_ResolveConflicts(t *testing.T) {
	db := openConsolidationTestDB(t)

	id1, _ := db.UpsertNode("a1", "person", "Alice", "{}")
	id2, _ := db.UpsertNode("a1", "person", "Bob", "{}")

	// Two different relations between same pair
	_ = db.UpsertEdge("a1", id1, id2, "knows", 1.0, "{}")
	_ = db.UpsertEdge("a1", id1, id2, "dislikes", 0.3, "{}")

	mc := NewMemoryConsolidator(db, "a1")
	report, err := mc.Consolidate()
	require.NoError(t, err)

	assert.Greater(t, report.ResolvedConflict, 0, "should have resolved at least one conflict")

	// Only the stronger edge should remain
	edges, _ := db.GetEdges("a1", id1)
	assert.Len(t, edges, 1)
	assert.Equal(t, "knows", edges[0].Relation)
}

func TestConsolidate_NoChanges(t *testing.T) {
	db := openConsolidationTestDB(t)

	// Create non-duplicate nodes
	_, _ = db.UpsertNode("a1", "person", "Alice", "{}")
	_, _ = db.UpsertNode("a1", "person", "Bob", "{}")
	_, _ = db.UpsertNode("a1", "project", "Sofia", "{}")

	mc := NewMemoryConsolidator(db, "a1")
	report, err := mc.Consolidate()
	require.NoError(t, err)

	assert.Equal(t, 0, report.MergedNodes)
	assert.Equal(t, 0, report.ResolvedConflict)
}

func TestConsolidate_NilDB(t *testing.T) {
	mc := NewMemoryConsolidator(nil, "a1")
	report, err := mc.Consolidate()
	assert.NoError(t, err)
	assert.Equal(t, 0, report.MergedNodes)
}
