package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *MemoryDB {
	t.Helper()
	db, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSchemaMigrationV2(t *testing.T) {
	db := openTestDB(t)

	// Verify that v2 tables exist by doing basic operations
	id, err := db.UpsertNode("agent1", "person", "Alice", `{"role":"engineer"}`)
	assert.NoError(t, err)
	assert.Greater(t, id, int64(0))

	err = db.UpsertEdge("agent1", id, id, "self", 1.0, "{}")
	assert.NoError(t, err)

	err = db.RecordStat("agent1", "test", &id, "migration test")
	assert.NoError(t, err)
}

func TestUpsertNode(t *testing.T) {
	db := openTestDB(t)

	// Insert
	id1, err := db.UpsertNode("a1", "person", "Bob", `{"age":30}`)
	require.NoError(t, err)
	assert.Greater(t, id1, int64(0))

	// Upsert same key → should return same ID
	id2, err := db.UpsertNode("a1", "person", "Bob", `{"age":31}`)
	require.NoError(t, err)
	assert.Equal(t, id1, id2)

	// Verify properties updated
	node, err := db.GetNode("a1", "person", "Bob")
	require.NoError(t, err)
	require.NotNil(t, node)
	assert.Equal(t, `{"age":31}`, node.Properties)

	// Different agent same key → different node
	id3, err := db.UpsertNode("a2", "person", "Bob", `{}`)
	require.NoError(t, err)
	assert.NotEqual(t, id1, id3)
}

func TestGetNode_NotFound(t *testing.T) {
	db := openTestDB(t)
	node, err := db.GetNode("a1", "person", "Nobody")
	assert.NoError(t, err)
	assert.Nil(t, node)
}

func TestFindNodes(t *testing.T) {
	db := openTestDB(t)

	_, _ = db.UpsertNode("a1", "person", "Alice", "{}")
	_, _ = db.UpsertNode("a1", "person", "Bob", "{}")
	_, _ = db.UpsertNode("a1", "project", "Sofia", "{}")
	_, _ = db.UpsertNode("a1", "person", "Charlie", "{}")

	// Find by label
	persons, err := db.FindNodes("a1", "person", "", 0)
	require.NoError(t, err)
	assert.Len(t, persons, 3)

	// Find by name pattern
	results, err := db.FindNodes("a1", "", "%li%", 0)
	require.NoError(t, err)
	assert.Len(t, results, 2) // Alice, Charlie

	// Find with limit
	limited, err := db.FindNodes("a1", "person", "", 1)
	require.NoError(t, err)
	assert.Len(t, limited, 1)
}

func TestUpsertEdge(t *testing.T) {
	db := openTestDB(t)

	id1, _ := db.UpsertNode("a1", "person", "Alice", "{}")
	id2, _ := db.UpsertNode("a1", "project", "Sofia", "{}")

	err := db.UpsertEdge("a1", id1, id2, "works_on", 0.8, "{}")
	require.NoError(t, err)

	edges, err := db.GetEdges("a1", id1)
	require.NoError(t, err)
	assert.Len(t, edges, 1)
	assert.Equal(t, "works_on", edges[0].Relation)
	assert.InDelta(t, 0.8, edges[0].Weight, 0.01)
	assert.Equal(t, "Alice", edges[0].SourceName)
	assert.Equal(t, "Sofia", edges[0].TargetName)
}

func TestDeleteNodeCascade(t *testing.T) {
	db := openTestDB(t)

	id1, _ := db.UpsertNode("a1", "person", "Alice", "{}")
	id2, _ := db.UpsertNode("a1", "project", "Sofia", "{}")
	_ = db.UpsertEdge("a1", id1, id2, "works_on", 1.0, "{}")

	// Verify edge exists
	edges, _ := db.GetEdges("a1", id1)
	assert.Len(t, edges, 1)

	// Delete node — edge should cascade
	err := db.DeleteNode(id1)
	require.NoError(t, err)

	// Node gone
	node, _ := db.GetNode("a1", "person", "Alice")
	assert.Nil(t, node)

	// Edge gone (cascaded)
	edges, _ = db.GetEdges("a1", id2)
	assert.Len(t, edges, 0)
}

func TestTouchNode(t *testing.T) {
	db := openTestDB(t)

	id, _ := db.UpsertNode("a1", "concept", "Go", "{}")

	// Initially zero
	node, _ := db.GetNode("a1", "concept", "Go")
	assert.Equal(t, 0, node.AccessCount)
	assert.Nil(t, node.LastAccessed)

	// Touch it
	db.TouchNode(id)
	db.TouchNode(id)
	db.TouchNode(id)

	node, _ = db.GetNode("a1", "concept", "Go")
	assert.Equal(t, 3, node.AccessCount)
	assert.NotNil(t, node.LastAccessed)
}

func TestQueryGraph(t *testing.T) {
	db := openTestDB(t)

	id1, _ := db.UpsertNode("a1", "person", "Alice", "{}")
	id2, _ := db.UpsertNode("a1", "project", "Sofia", "{}")
	_ = db.UpsertEdge("a1", id1, id2, "created", 1.0, "{}")

	results, err := db.QueryGraph("a1", "Alice", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Alice", results[0].Node.Name)
	assert.Len(t, results[0].Edges, 1)
}

func TestGetStaleNodes(t *testing.T) {
	db := openTestDB(t)

	// Create nodes — they start with access_count=0 and last_accessed=NULL
	_, _ = db.UpsertNode("a1", "concept", "Old Thing", "{}")
	id2, _ := db.UpsertNode("a1", "concept", "Active Thing", "{}")

	// Make one "active"
	db.TouchNode(id2)
	db.TouchNode(id2)
	db.TouchNode(id2)

	// Get stale nodes (minAccessCount=2 means < 2 accesses)
	stale, err := db.GetStaleNodes("a1", 1*time.Hour, 2)
	require.NoError(t, err)
	assert.Len(t, stale, 1)
	assert.Equal(t, "Old Thing", stale[0].Name)
}

func TestDeleteNodes_Batch(t *testing.T) {
	db := openTestDB(t)

	id1, _ := db.UpsertNode("a1", "concept", "A", "{}")
	id2, _ := db.UpsertNode("a1", "concept", "B", "{}")
	_, _ = db.UpsertNode("a1", "concept", "C", "{}")

	err := db.DeleteNodes([]int64{id1, id2})
	require.NoError(t, err)

	assert.Equal(t, 1, db.CountNodes("a1"))
}

func TestRecordStat_And_GetNodeStats(t *testing.T) {
	db := openTestDB(t)

	id, _ := db.UpsertNode("a1", "person", "Alice", "{}")
	_ = db.RecordStat("a1", "query", &id, "test query")
	_ = db.RecordStat("a1", "hit", &id, "test hit")
	_ = db.RecordStat("a1", "query", &id, "another query")

	stats, err := db.GetNodeStats("a1")
	require.NoError(t, err)
	assert.Len(t, stats, 1)
	assert.Equal(t, "Alice", stats[0].Name)
	assert.Equal(t, 2, stats[0].QueryCount)
	assert.Equal(t, 1, stats[0].HitCount)
}

func TestFindDuplicateNodes(t *testing.T) {
	db := openTestDB(t)

	_, _ = db.UpsertNode("a1", "person", "Alice", "{}")
	_, _ = db.UpsertNode("a1", "person", "alice", "{}") // case variation
	_, _ = db.UpsertNode("a1", "person", "Bob", "{}")
	_, _ = db.UpsertNode("a1", "project", "Sofia", "{}")

	duplicates, err := db.FindDuplicateNodes("a1")
	require.NoError(t, err)
	// "Alice" and "alice" should be considered similar (Levenshtein distance 1 after lowercasing = 0)
	// But they have the unique constraint on (agent_id, label, name), so if they're different names...
	// Actually "Alice" != "alice" in the DB (case-sensitive), but isSimilarName lowercases them → identical
	assert.GreaterOrEqual(t, len(duplicates), 1)
}

func TestMergeNodes(t *testing.T) {
	db := openTestDB(t)

	id1, _ := db.UpsertNode("a1", "person", "Alice", `{"full":true}`)
	id2, _ := db.UpsertNode("a1", "person", "alice", `{"partial":true}`)
	id3, _ := db.UpsertNode("a1", "project", "Sofia", "{}")

	// Alice → Sofia
	_ = db.UpsertEdge("a1", id1, id3, "works_on", 1.0, "{}")
	// alice → Sofia (duplicate)
	_ = db.UpsertEdge("a1", id2, id3, "works_on", 0.5, "{}")

	err := db.MergeNodes(id1, []int64{id2})
	require.NoError(t, err)

	// alice node should be gone
	node, _ := db.GetNode("a1", "person", "alice")
	assert.Nil(t, node)

	// Alice should still have an edge to Sofia
	edges, _ := db.GetEdges("a1", id1)
	assert.Len(t, edges, 1)
	assert.Equal(t, "Sofia", edges[0].TargetName)
}

func TestGetConflictingEdges(t *testing.T) {
	db := openTestDB(t)

	id1, _ := db.UpsertNode("a1", "person", "Alice", "{}")
	id2, _ := db.UpsertNode("a1", "person", "Bob", "{}")

	_ = db.UpsertEdge("a1", id1, id2, "knows", 1.0, "{}")
	_ = db.UpsertEdge("a1", id1, id2, "dislikes", 0.3, "{}")

	conflicts, err := db.GetConflictingEdges("a1")
	require.NoError(t, err)
	assert.Len(t, conflicts, 1)
	assert.Len(t, conflicts[0], 2)
	// Highest weight should be first
	assert.Equal(t, "knows", conflicts[0][0].Relation)
}

func TestReinforceEdge(t *testing.T) {
	db := openTestDB(t)

	id1, _ := db.UpsertNode("a1", "person", "Alice", "{}")
	id2, _ := db.UpsertNode("a1", "person", "Bob", "{}")
	_ = db.UpsertEdge("a1", id1, id2, "knows", 0.5, "{}")

	edges, _ := db.GetEdges("a1", id1)
	require.Len(t, edges, 1)
	assert.InDelta(t, 0.5, edges[0].Weight, 0.01)

	db.ReinforceEdge(edges[0].ID, 0.1)

	edges, _ = db.GetEdges("a1", id1)
	assert.InDelta(t, 0.6, edges[0].Weight, 0.01)

	// Reinforce to cap at 1.0
	db.ReinforceEdge(edges[0].ID, 0.9)
	edges, _ = db.GetEdges("a1", id1)
	assert.InDelta(t, 1.0, edges[0].Weight, 0.01)
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "b", 1},
		{"alice", "alice", 0},
		{"alice", "alce", 1},
		{"kitten", "sitting", 3},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, levenshteinDistance(tt.a, tt.b),
			"levenshtein(%q, %q)", tt.a, tt.b)
	}
}

func TestIsSimilarName(t *testing.T) {
	assert.True(t, isSimilarName("Alice", "alice"))
	assert.True(t, isSimilarName("Bob", "Bobby"))   // prefix
	assert.True(t, isSimilarName("Sofia", "Sofía")) // close Levenshtein (TODO: UTF-8)
	assert.False(t, isSimilarName("Alice", "Bob"))
	assert.False(t, isSimilarName("completely", "different"))
}
