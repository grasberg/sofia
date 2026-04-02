package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/memory"
)

func openKGTestDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestKnowledgeGraphTool_AddEntity(t *testing.T) {
	db := openKGTestDB(t)
	tool := NewKnowledgeGraphTool(db, "test-agent")

	result := tool.Execute(context.Background(), map[string]any{
		"operation":  "add_entity",
		"label":      "person",
		"name":       "Alice",
		"properties": map[string]any{"role": "engineer"},
	})
	assert.False(t, result.IsError, "unexpected error: %s", result.ForLLM)
	assert.Contains(t, result.ForLLM, "Alice")
	assert.Contains(t, result.ForLLM, "person")
}

func TestKnowledgeGraphTool_AddEntity_MissingFields(t *testing.T) {
	db := openKGTestDB(t)
	tool := NewKnowledgeGraphTool(db, "test-agent")

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "add_entity",
		"label":     "person",
		// missing name
	})
	assert.True(t, result.IsError)
}

func TestKnowledgeGraphTool_AddRelation(t *testing.T) {
	db := openKGTestDB(t)
	tool := NewKnowledgeGraphTool(db, "test-agent")

	// Add entities first
	tool.Execute(context.Background(), map[string]any{
		"operation": "add_entity",
		"label":     "person",
		"name":      "Alice",
	})
	tool.Execute(context.Background(), map[string]any{
		"operation": "add_entity",
		"label":     "project",
		"name":      "Sofia",
	})

	result := tool.Execute(context.Background(), map[string]any{
		"operation":    "add_relation",
		"source_label": "person",
		"source_name":  "Alice",
		"relation":     "works_on",
		"target_label": "project",
		"target_name":  "Sofia",
		"weight":       0.9,
	})
	assert.False(t, result.IsError, "unexpected error: %s", result.ForLLM)
	assert.Contains(t, result.ForLLM, "works_on")
}

func TestKnowledgeGraphTool_Query(t *testing.T) {
	db := openKGTestDB(t)
	tool := NewKnowledgeGraphTool(db, "test-agent")

	// Populate
	tool.Execute(context.Background(), map[string]any{
		"operation": "add_entity",
		"label":     "person",
		"name":      "Alice",
	})

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "query",
		"query":     "Alice",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Alice")
}

func TestKnowledgeGraphTool_Query_NoResults(t *testing.T) {
	db := openKGTestDB(t)
	tool := NewKnowledgeGraphTool(db, "test-agent")

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "query",
		"query":     "nonexistent",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "No knowledge found")
}

func TestKnowledgeGraphTool_GetEntity(t *testing.T) {
	db := openKGTestDB(t)
	tool := NewKnowledgeGraphTool(db, "test-agent")

	tool.Execute(context.Background(), map[string]any{
		"operation":  "add_entity",
		"label":      "person",
		"name":       "Bob",
		"properties": map[string]any{"age": 25},
	})

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "get_entity",
		"label":     "person",
		"name":      "Bob",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Bob")
}

func TestKnowledgeGraphTool_DeleteEntity(t *testing.T) {
	db := openKGTestDB(t)
	tool := NewKnowledgeGraphTool(db, "test-agent")

	tool.Execute(context.Background(), map[string]any{
		"operation": "add_entity",
		"label":     "concept",
		"name":      "Quantum",
	})

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "delete_entity",
		"label":     "concept",
		"name":      "Quantum",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Deleted")

	// Verify it's gone
	result = tool.Execute(context.Background(), map[string]any{
		"operation": "get_entity",
		"label":     "concept",
		"name":      "Quantum",
	})
	assert.Contains(t, result.ForLLM, "not found")
}

func TestKnowledgeGraphTool_Consolidate(t *testing.T) {
	db := openKGTestDB(t)
	tool := NewKnowledgeGraphTool(db, "test-agent")

	// Create "duplicate" nodes (different case)
	tool.Execute(context.Background(), map[string]any{
		"operation": "add_entity",
		"label":     "person",
		"name":      "Alice",
	})
	tool.Execute(context.Background(), map[string]any{
		"operation": "add_entity",
		"label":     "person",
		"name":      "alice",
	})

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "consolidate",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Consolidation complete")
}

func TestKnowledgeGraphTool_Prune_DryRun(t *testing.T) {
	db := openKGTestDB(t)
	tool := NewKnowledgeGraphTool(db, "test-agent")

	tool.Execute(context.Background(), map[string]any{
		"operation": "add_entity",
		"label":     "concept",
		"name":      "Old forgotten thing",
	})

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "prune",
		"dry_run":   true,
	})
	assert.False(t, result.IsError)
	// Either found stale nodes or no stale memories
	assert.True(t, len(result.ForLLM) > 0)
}

func TestKnowledgeGraphTool_Stats(t *testing.T) {
	db := openKGTestDB(t)
	tool := NewKnowledgeGraphTool(db, "test-agent")

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "stats",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Knowledge graph")
}

func TestKnowledgeGraphTool_UnknownOperation(t *testing.T) {
	db := openKGTestDB(t)
	tool := NewKnowledgeGraphTool(db, "test-agent")

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "invalid",
	})
	assert.True(t, result.IsError)
}

func TestKnowledgeGraphTool_NilDB(t *testing.T) {
	tool := NewKnowledgeGraphTool(nil, "test-agent")

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "add_entity",
		"label":     "test",
		"name":      "test",
	})
	assert.True(t, result.IsError)
}
