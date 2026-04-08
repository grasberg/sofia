package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTodoTool_Name(t *testing.T) {
	tool := NewTodoTool(filepath.Join(t.TempDir(), "todos.json"))
	assert.Equal(t, "todo", tool.Name())
}

func TestTodoTool_CRUD(t *testing.T) {
	todoPath := filepath.Join(t.TempDir(), "todos.json")
	tool := NewTodoTool(todoPath)
	ctx := context.Background()

	// Add
	result := tool.Execute(ctx, map[string]any{
		"action": "add",
		"text":   "Write unit tests",
	})
	assert.False(t, result.IsError, "add failed: %s", result.ForLLM)
	assert.Contains(t, result.ForLLM, "Todo #1 added")

	// Add another
	result = tool.Execute(ctx, map[string]any{
		"action": "add",
		"text":   "Review PR",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Todo #2 added")

	// List
	result = tool.Execute(ctx, map[string]any{"action": "list"})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Write unit tests")
	assert.Contains(t, result.ForLLM, "Review PR")
	assert.Contains(t, result.ForLLM, "- [ ]") // pending checkbox

	// Update to in_progress
	result = tool.Execute(ctx, map[string]any{
		"action": "update",
		"id":     float64(1),
		"status": "in_progress",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "in_progress")

	// Update to done
	result = tool.Execute(ctx, map[string]any{
		"action": "update",
		"id":     float64(1),
		"status": "done",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "done")

	// List should show updated statuses
	result = tool.Execute(ctx, map[string]any{"action": "list"})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "- [x]") // done checkbox

	// Remove
	result = tool.Execute(ctx, map[string]any{
		"action": "remove",
		"id":     float64(2),
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Todo #2 removed")

	// List should only show one item
	result = tool.Execute(ctx, map[string]any{"action": "list"})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Write unit tests")
	assert.NotContains(t, result.ForLLM, "Review PR")
}

func TestTodoTool_Clear(t *testing.T) {
	todoPath := filepath.Join(t.TempDir(), "todos.json")
	tool := NewTodoTool(todoPath)
	ctx := context.Background()

	// Add items
	tool.Execute(ctx, map[string]any{"action": "add", "text": "Task A"})
	tool.Execute(ctx, map[string]any{"action": "add", "text": "Task B"})
	tool.Execute(ctx, map[string]any{"action": "add", "text": "Task C"})

	// Mark some as done
	tool.Execute(ctx, map[string]any{"action": "update", "id": float64(1), "status": "done"})
	tool.Execute(ctx, map[string]any{"action": "update", "id": float64(3), "status": "done"})

	// Clear completed
	result := tool.Execute(ctx, map[string]any{"action": "clear"})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Cleared 2 completed todo(s)")

	// List should only show Task B
	result = tool.Execute(ctx, map[string]any{"action": "list"})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Task B")
	assert.NotContains(t, result.ForLLM, "Task A")
	assert.NotContains(t, result.ForLLM, "Task C")
}

func TestTodoTool_ClearNoop(t *testing.T) {
	todoPath := filepath.Join(t.TempDir(), "todos.json")
	tool := NewTodoTool(todoPath)
	ctx := context.Background()

	tool.Execute(ctx, map[string]any{"action": "add", "text": "Pending task"})

	result := tool.Execute(ctx, map[string]any{"action": "clear"})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "No completed todos to clear")
}

func TestTodoTool_Persistence(t *testing.T) {
	todoPath := filepath.Join(t.TempDir(), "todos.json")
	ctx := context.Background()

	// Create tool, add items
	tool1 := NewTodoTool(todoPath)
	tool1.Execute(ctx, map[string]any{"action": "add", "text": "Persistent item"})
	tool1.Execute(ctx, map[string]any{"action": "update", "id": float64(1), "status": "in_progress"})

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(todoPath)
	require.NoError(t, err)
	var items []TodoItem
	require.NoError(t, json.Unmarshal(data, &items))
	assert.Len(t, items, 1)
	assert.Equal(t, "Persistent item", items[0].Text)
	assert.Equal(t, "in_progress", items[0].Status)

	// Create new tool from same path — should load existing data
	tool2 := NewTodoTool(todoPath)
	result := tool2.Execute(ctx, map[string]any{"action": "list"})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Persistent item")
	assert.Contains(t, result.ForLLM, "in progress")

	// Add new item — IDs should continue from where they left off
	result = tool2.Execute(ctx, map[string]any{"action": "add", "text": "Second item"})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Todo #2 added")
}

func TestTodoTool_EmptyList(t *testing.T) {
	todoPath := filepath.Join(t.TempDir(), "todos.json")
	tool := NewTodoTool(todoPath)

	result := tool.Execute(context.Background(), map[string]any{"action": "list"})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "No todos.")
}

func TestTodoTool_Validation(t *testing.T) {
	todoPath := filepath.Join(t.TempDir(), "todos.json")
	tool := NewTodoTool(todoPath)
	ctx := context.Background()

	t.Run("missing action", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{})
		assert.True(t, result.IsError)
	})

	t.Run("add without text", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{"action": "add"})
		assert.True(t, result.IsError)
	})

	t.Run("update without id", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{"action": "update", "status": "done"})
		assert.True(t, result.IsError)
	})

	t.Run("update without status", func(t *testing.T) {
		tool.Execute(ctx, map[string]any{"action": "add", "text": "test"})
		result := tool.Execute(ctx, map[string]any{"action": "update", "id": float64(1)})
		assert.True(t, result.IsError)
	})

	t.Run("update with invalid status", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{"action": "update", "id": float64(1), "status": "invalid"})
		assert.True(t, result.IsError)
	})

	t.Run("remove nonexistent", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{"action": "remove", "id": float64(999)})
		assert.True(t, result.IsError)
	})

	t.Run("update nonexistent", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{"action": "update", "id": float64(999), "status": "done"})
		assert.True(t, result.IsError)
	})

	t.Run("unknown action", func(t *testing.T) {
		result := tool.Execute(ctx, map[string]any{"action": "unknown"})
		assert.True(t, result.IsError)
	})
}

func TestExtractTodoID(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want int64
		ok   bool
	}{
		{"float64", map[string]any{"id": float64(5)}, 5, true},
		{"int64", map[string]any{"id": int64(3)}, 3, true},
		{"int", map[string]any{"id": 7}, 7, true},
		{"string", map[string]any{"id": "42"}, 42, true},
		{"missing", map[string]any{}, 0, false},
		{"invalid string", map[string]any{"id": "abc"}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractTodoID(tt.args)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestDefaultTodoPath(t *testing.T) {
	p := DefaultTodoPath()
	assert.Contains(t, p, ".sofia")
	assert.Contains(t, p, "todos.json")
}
