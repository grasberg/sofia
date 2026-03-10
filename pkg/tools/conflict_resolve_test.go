package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConflictResolveToolDetect(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewConflictResolveTool(sp)

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "detect",
		"outputs": []any{
			map[string]any{"agent_id": "a1", "content": "Yes, the deploy succeeded"},
			map[string]any{"agent_id": "a2", "content": "No, the deploy failed with errors"},
		},
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "conflict")
}

func TestConflictResolveToolDetectNoConflict(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewConflictResolveTool(sp)

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "detect",
		"outputs": []any{
			map[string]any{"agent_id": "a1", "content": "The server is running on port 8080"},
			map[string]any{"agent_id": "a2", "content": "The server is running on port 8080"},
		},
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "No conflicts")
}

func TestConflictResolveToolDetectTooFewOutputs(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewConflictResolveTool(sp)

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "detect",
		"outputs": []any{
			map[string]any{"agent_id": "a1", "content": "hello"},
		},
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "at least 2")
}

func TestConflictResolveToolResolveMajorityVote(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewConflictResolveTool(sp)

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "resolve",
		"strategy":  "majority_vote",
		"outputs": []any{
			map[string]any{"agent_id": "a1", "content": "Use Redis for caching"},
			map[string]any{"agent_id": "a2", "content": "Use Redis for the caching layer"},
			map[string]any{"agent_id": "a3", "content": "Build a custom distributed cache from scratch"},
		},
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "majority_vote")
	assert.Contains(t, result.ForLLM, "RESOLVED_CONTENT")
	assert.Contains(t, result.ForLLM, "Redis")
}

func TestConflictResolveToolResolvePriority(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewConflictResolveTool(sp)

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "resolve",
		"strategy":  "priority",
		"outputs": []any{
			map[string]any{"agent_id": "junior", "content": "Use SQLite", "priority": float64(1)},
			map[string]any{"agent_id": "senior", "content": "Use PostgreSQL", "priority": float64(10)},
		},
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "senior")
	assert.Contains(t, result.ForLLM, "PostgreSQL")
}

func TestConflictResolveToolResolveMerge(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewConflictResolveTool(sp)

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "resolve",
		"strategy":  "merge",
		"outputs": []any{
			map[string]any{"agent_id": "a1", "content": "The API supports JSON. Rate limit is 100 req/min."},
			map[string]any{"agent_id": "a2", "content": "The API supports JSON. Authentication uses OAuth2."},
		},
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "merge")
	assert.Contains(t, result.ForLLM, "RESOLVED_CONTENT")
}

func TestConflictResolveToolDetectScratchpad(t *testing.T) {
	sp := NewSharedScratchpad()
	sp.Write("goal-1", "task-a", "Yes, the feature is ready for production")
	sp.Write("goal-1", "task-b", "No, the feature has critical bugs that block production")

	tool := NewConflictResolveTool(sp)
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "detect_scratchpad",
		"group":     "goal-1",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "conflict")
}

func TestConflictResolveToolDetectScratchpadNoConflict(t *testing.T) {
	sp := NewSharedScratchpad()
	sp.Write("goal-2", "task-a", "The database migration was completed successfully and all tables are up to date")
	sp.Write("goal-2", "task-b", "The database migration was completed successfully and all tables updated correctly")

	tool := NewConflictResolveTool(sp)
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "detect_scratchpad",
		"group":     "goal-2",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "No conflicts")
}

func TestConflictResolveToolDetectScratchpadTooFew(t *testing.T) {
	sp := NewSharedScratchpad()
	sp.Write("goal-3", "task-a", "Only one result")

	tool := NewConflictResolveTool(sp)
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "detect_scratchpad",
		"group":     "goal-3",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "1 entries")
}

func TestConflictResolveToolDetectScratchpadMissingGroup(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewConflictResolveTool(sp)
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "detect_scratchpad",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "group is required")
}

func TestConflictResolveToolResolveNoOutputs(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewConflictResolveTool(sp)
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "resolve",
		"outputs":   []any{},
	})
	assert.True(t, result.IsError)
}

func TestConflictResolveToolUnknownOperation(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewConflictResolveTool(sp)
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "unknown",
	})
	assert.True(t, result.IsError)
}

func TestConflictResolveToolResolveAll(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewConflictResolveTool(sp)

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "resolve",
		"strategy":  "all",
		"outputs": []any{
			map[string]any{"agent_id": "a1", "content": "Option A"},
			map[string]any{"agent_id": "a2", "content": "Option B"},
		},
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Option A")
	assert.Contains(t, result.ForLLM, "Option B")
	assert.Contains(t, result.ForLLM, "[Agent a1]")
}
