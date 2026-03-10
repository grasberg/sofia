package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/grasberg/sofia/pkg/checkpoint"
)

// CheckpointTool allows the agent to create, list, and rollback checkpoints.
type CheckpointTool struct {
	manager    *checkpoint.Manager
	sessionKey string
	agentID    string
}

// NewCheckpointTool creates a new CheckpointTool.
func NewCheckpointTool(manager *checkpoint.Manager, agentID string) *CheckpointTool {
	return &CheckpointTool{
		manager: manager,
		agentID: agentID,
	}
}

// SetSessionKey sets the current session key for checkpoint operations.
func (t *CheckpointTool) SetSessionKey(key string) {
	t.sessionKey = key
}

func (t *CheckpointTool) Name() string { return "checkpoint" }

func (t *CheckpointTool) Description() string {
	return "Save and restore execution state. " +
		"Operations: create (save current state), list (show all checkpoints), " +
		"rollback (restore to a checkpoint), cleanup (remove all checkpoints). " +
		"Use checkpoints before risky operations to enable recovery on failure."
}

func (t *CheckpointTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "list", "rollback", "cleanup"},
				"description": "The operation to perform",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Label for the checkpoint (required for create)",
			},
			"checkpoint_id": map[string]any{
				"type":        "number",
				"description": "ID of the checkpoint to rollback to (optional for rollback; defaults to latest)",
			},
		},
		"required": []string{"operation"},
	}
}

func (t *CheckpointTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	op, _ := args["operation"].(string)

	if t.sessionKey == "" {
		return ErrorResult("checkpoint: no active session")
	}

	switch op {
	case "create":
		return t.create(args)
	case "list":
		return t.list()
	case "rollback":
		return t.rollback(args)
	case "cleanup":
		return t.cleanup()
	default:
		return ErrorResult(fmt.Sprintf("checkpoint: unknown operation %q", op))
	}
}

func (t *CheckpointTool) create(args map[string]any) *ToolResult {
	name, _ := args["name"].(string)
	if name == "" {
		name = "manual"
	}

	cp, err := t.manager.Create(t.sessionKey, t.agentID, name, 0)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to create checkpoint: %s", err))
	}

	return SilentResult(fmt.Sprintf(
		"Checkpoint created: id=%d name=%q messages=%d",
		cp.ID, cp.Name, cp.MsgCount,
	))
}

func (t *CheckpointTool) list() *ToolResult {
	checkpoints, err := t.manager.List(t.sessionKey)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to list checkpoints: %s", err))
	}
	if len(checkpoints) == 0 {
		return SilentResult("No checkpoints saved for this session.")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Checkpoints (%d):\n", len(checkpoints))
	for _, cp := range checkpoints {
		fmt.Fprintf(&sb, "  id=%d name=%q iter=%d msgs=%d created=%s\n",
			cp.ID, cp.Name, cp.Iteration, cp.MsgCount, cp.CreatedAt.Format("15:04:05"))
	}
	return SilentResult(sb.String())
}

func (t *CheckpointTool) rollback(args map[string]any) *ToolResult {
	cpIDRaw, hasID := args["checkpoint_id"]
	if hasID {
		cpID, ok := cpIDRaw.(float64)
		if !ok {
			return ErrorResult("checkpoint_id must be a number")
		}
		cp, err := t.manager.Rollback(t.sessionKey, int64(cpID))
		if err != nil {
			return ErrorResult(fmt.Sprintf("Failed to rollback: %s", err))
		}
		return NewToolResult(fmt.Sprintf(
			"Rolled back to checkpoint id=%d name=%q (restored to %d messages). "+
				"Session state has been restored. Re-evaluate the task from this point.",
			cp.ID, cp.Name, cp.MsgCount,
		))
	}

	// Default: rollback to latest
	cp, _, err := t.manager.RollbackToLatest(t.sessionKey)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to rollback: %s", err))
	}
	if cp == nil {
		return ErrorResult("No checkpoints to rollback to.")
	}
	return NewToolResult(fmt.Sprintf(
		"Rolled back to latest checkpoint id=%d name=%q (restored to %d messages). "+
			"Session state has been restored. Re-evaluate the task from this point.",
		cp.ID, cp.Name, cp.MsgCount,
	))
}

func (t *CheckpointTool) cleanup() *ToolResult {
	if err := t.manager.Cleanup(t.sessionKey); err != nil {
		return ErrorResult(fmt.Sprintf("Failed to cleanup checkpoints: %s", err))
	}
	return SilentResult("All checkpoints removed for this session.")
}
