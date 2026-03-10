package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/checkpoint"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
)

func setupCheckpointTest(t *testing.T) (*CheckpointTool, *memory.MemoryDB) {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	mgr := checkpoint.NewManager(db)
	tool := NewCheckpointTool(mgr, "test-agent")
	tool.SetSessionKey("test-session")

	// Seed session with messages
	_, err = db.GetOrCreateSession("test-session", "test-agent")
	require.NoError(t, err)
	require.NoError(t, db.AppendMessage("test-session", providers.Message{Role: "user", Content: "hi"}))
	require.NoError(t, db.AppendMessage("test-session", providers.Message{Role: "assistant", Content: "hello"}))

	return tool, db
}

func TestCheckpointToolCreate(t *testing.T) {
	tool, _ := setupCheckpointTest(t)

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "create",
		"name":      "before-deploy",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Checkpoint created")
	assert.Contains(t, result.ForLLM, "before-deploy")
}

func TestCheckpointToolList(t *testing.T) {
	tool, _ := setupCheckpointTest(t)

	// Create a checkpoint first
	tool.Execute(context.Background(), map[string]any{
		"operation": "create",
		"name":      "cp1",
	})

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "list",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "cp1")
}

func TestCheckpointToolRollback(t *testing.T) {
	tool, db := setupCheckpointTest(t)

	// Create checkpoint
	tool.Execute(context.Background(), map[string]any{
		"operation": "create",
		"name":      "safe-point",
	})

	// Add more messages after checkpoint
	require.NoError(t, db.AppendMessage("test-session", providers.Message{Role: "user", Content: "bad command"}))
	require.NoError(t, db.AppendMessage("test-session", providers.Message{Role: "assistant", Content: "error"}))

	// Rollback to latest
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "rollback",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Rolled back")
	assert.Contains(t, result.ForLLM, "safe-point")

	// Verify messages are restored
	msgs, err := db.GetMessages("test-session")
	require.NoError(t, err)
	assert.Len(t, msgs, 2) // Only original 2 messages
}

func TestCheckpointToolRollbackByID(t *testing.T) {
	tool, db := setupCheckpointTest(t)

	// Create first checkpoint
	tool.Execute(context.Background(), map[string]any{
		"operation": "create",
		"name":      "first",
	})

	// Add message and create second checkpoint
	require.NoError(t, db.AppendMessage("test-session", providers.Message{Role: "user", Content: "step2"}))
	tool.Execute(context.Background(), map[string]any{
		"operation": "create",
		"name":      "second",
	})

	// Add more messages
	require.NoError(t, db.AppendMessage("test-session", providers.Message{Role: "assistant", Content: "error"}))

	// Rollback to first checkpoint (id=1)
	result := tool.Execute(context.Background(), map[string]any{
		"operation":     "rollback",
		"checkpoint_id": float64(1),
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "first")

	msgs, err := db.GetMessages("test-session")
	require.NoError(t, err)
	assert.Len(t, msgs, 2) // Only the original 2
}

func TestCheckpointToolCleanup(t *testing.T) {
	tool, _ := setupCheckpointTest(t)

	tool.Execute(context.Background(), map[string]any{
		"operation": "create",
		"name":      "cp1",
	})
	tool.Execute(context.Background(), map[string]any{
		"operation": "create",
		"name":      "cp2",
	})

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "cleanup",
	})
	assert.False(t, result.IsError)

	listResult := tool.Execute(context.Background(), map[string]any{
		"operation": "list",
	})
	assert.Contains(t, listResult.ForLLM, "No checkpoints")
}

func TestCheckpointToolNoSession(t *testing.T) {
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	mgr := checkpoint.NewManager(db)
	tool := NewCheckpointTool(mgr, "test-agent")
	// Don't set session key

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "create",
		"name":      "test",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "no active session")
}

func TestCheckpointToolRollbackNoCheckpoints(t *testing.T) {
	tool, _ := setupCheckpointTest(t)

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "rollback",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "No checkpoints")
}
