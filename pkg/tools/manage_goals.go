package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// GoalManager interface breaks the import cycle between tools and autonomy.
type GoalManager interface {
	AddGoal(agentID, name, description, priority string) (any, error)
	UpdateGoalStatus(goalID int64, newStatus string) (any, error)
	ListActiveGoals(agentID string) ([]any, error)
}

// ManageGoalsOptions contains the GoalManager.
type ManageGoalsOptions struct {
	GoalManager GoalManager
	AgentID     string
}

// ManageGoalsTool allows Sofia to autonomously manage her long-term goals.
type ManageGoalsTool struct {
	mgr     GoalManager
	agentID string
}

// NewManageGoalsTool creates a new coordinate tool.
func NewManageGoalsTool(opts ManageGoalsOptions) *ManageGoalsTool {
	return &ManageGoalsTool{
		mgr:     opts.GoalManager,
		agentID: opts.AgentID,
	}
}

func (t *ManageGoalsTool) Name() string {
	return "manage_goals"
}

func (t *ManageGoalsTool) Description() string {
	return `Manage long-term agent goals. 
Actions: 
 - "add": create a new goal (requires name, description, priority).
 - "update_status": update an existing goal's status to "completed", "failed", or "paused" (requires goal_id, status).
 - "list": view all active goals.`
}

func (t *ManageGoalsTool) Parameters() map[string]any {
	var schema map[string]any
	json.Unmarshal([]byte(`{"type":"object","properties":{
		"action":{"type":"string","description":"add, update_status, or list"},
		"goal_id":{"type":"integer","description":"The numeric ID of the goal (for update_status)"},
		"name":{"type":"string","description":"Name of the goal (for add)"},
		"description":{"type":"string","description":"Details of the goal (for add)"},
		"status":{"type":"string","description":"New status (for update_status)"},
		"priority":{"type":"string","description":"low, medium, or high (for add)"}
	},"required":["action"]}`), &schema)
	return schema
}

// Execute performs goal manipulations
func (t *ManageGoalsTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	if t.mgr == nil {
		return ErrorResult("manage_goals not configured: GoalManager is nil")
	}

	bArgs, _ := json.Marshal(args)
	var parsedArgs struct {
		Action      string `json:"action"`
		GoalID      int64  `json:"goal_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Status      string `json:"status"`
		Priority    string `json:"priority"`
	}

	if err := json.Unmarshal(bArgs, &parsedArgs); err != nil {
		return ErrorResult(fmt.Sprintf("invalid arguments: %v", err))
	}

	switch strings.ToLower(parsedArgs.Action) {
	case "add":
		if parsedArgs.Name == "" || parsedArgs.Description == "" {
			return ErrorResult("name and description required for 'add' action")
		}
		gAny, err := t.mgr.AddGoal(t.agentID, parsedArgs.Name, parsedArgs.Description, parsedArgs.Priority)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to add goal: %v", err))
		}
		// Goal is unmarshalled as map since autonomy.Goal cannot be imported due to cycle.
		b, _ := json.Marshal(gAny)
		var g map[string]any
		json.Unmarshal(b, &g)
		return NewToolResult(fmt.Sprintf("Goal successfully added. ID: %.0f", g["id"].(float64)))

	case "update_status":
		if parsedArgs.GoalID == 0 || parsedArgs.Status == "" {
			return ErrorResult("goal_id and status required for 'update_status' action")
		}
		// Prevent marking goals as completed without evidence.
		// The autonomy system handles completion — agents should not self-complete goals.
		if parsedArgs.Status == "completed" {
			return ErrorResult("Goals cannot be marked as completed directly. " +
				"Continue working on the goal — the autonomy system will mark it complete " +
				"when all steps are verified. Use 'paused' if you need to stop temporarily.")
		}
		gAny, err := t.mgr.UpdateGoalStatus(parsedArgs.GoalID, parsedArgs.Status)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to update goal: %v", err))
		}
		b, _ := json.Marshal(gAny)
		var g map[string]any
		json.Unmarshal(b, &g)
		return NewToolResult(fmt.Sprintf("Goal %.0f status updated to %s", g["id"].(float64), g["status"]))

	case "list":
		goalsAny, err := t.mgr.ListActiveGoals(t.agentID)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to list goals: %v", err))
		}
		if len(goalsAny) == 0 {
			return NewToolResult("No active goals found.")
		}
		var out strings.Builder
		for _, gAny := range goalsAny {
			b, _ := json.Marshal(gAny)
			var g map[string]any
			json.Unmarshal(b, &g)
			out.WriteString(fmt.Sprintf("- ID: %.0f | Name: %s | Priority: %s\n  %s\n", g["id"].(float64), g["name"], g["priority"], g["description"]))
		}
		return NewToolResult(out.String())

	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", parsedArgs.Action))
	}
}
