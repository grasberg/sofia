package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskTool_Name(t *testing.T) {
	tool := NewTaskTool()
	assert.Equal(t, "task", tool.Name())
}

func TestTaskTool_CRUD(t *testing.T) {
	tool := NewTaskTool()
	ctx := context.Background()

	// Create
	result := tool.Execute(ctx, map[string]any{
		"action": "create",
		"title":  "Fix the bug",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Task #1 created")

	// Create another
	result = tool.Execute(ctx, map[string]any{
		"action":      "create",
		"title":       "Write tests",
		"description": "Unit tests for new feature",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Task #2 created")

	// List
	result = tool.Execute(ctx, map[string]any{"action": "list"})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Fix the bug")
	assert.Contains(t, result.ForLLM, "Write tests")
	assert.Contains(t, result.ForLLM, "pending")

	// Update status
	result = tool.Execute(ctx, map[string]any{
		"action": "update",
		"id":     "1",
		"status": "in_progress",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "in_progress")

	// Complete
	result = tool.Execute(ctx, map[string]any{
		"action": "update",
		"id":     "1",
		"status": "completed",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "completed")

	// Delete
	result = tool.Execute(ctx, map[string]any{
		"action": "delete",
		"id":     "2",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "deleted")

	// Delete non-existent
	result = tool.Execute(ctx, map[string]any{
		"action": "delete",
		"id":     "999",
	})
	assert.True(t, result.IsError)
}

func TestTaskTool_Validation(t *testing.T) {
	tool := NewTaskTool()
	ctx := context.Background()

	t.Run("missing action", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{})
		assert.True(t, result.IsError)
	})

	t.Run("create without title", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{"action": "create"})
		assert.True(t, result.IsError)
	})

	t.Run("update without id", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{"action": "update", "status": "completed"})
		assert.True(t, result.IsError)
	})

	t.Run("unknown action", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{"action": "unknown"})
		assert.True(t, result.IsError)
	})
}
