package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/grasberg/sofia/pkg/logger"
)

// GoalManager interface breaks the import cycle between tools and autonomy.
type GoalManager interface {
	AddGoal(agentID, name, description, priority string) (any, error)
	UpdateGoalStatus(goalID int64, newStatus string) (any, error)
	ListActiveGoals(agentID string) ([]any, error)
	SetAgentCount(goalID int64, count int) error
}

// ManageGoalsOptions contains the GoalManager.
type ManageGoalsOptions struct {
	GoalManager GoalManager
	AgentID     string
	Workspace   string // root workspace directory for goal folders
}

// ManageGoalsTool allows Sofia to autonomously manage her long-term goals.
type ManageGoalsTool struct {
	mgr       GoalManager
	agentID   string
	workspace string
}

var goalSlugPattern = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// GoalFolderName returns a filesystem-safe folder name for a goal.
// Shared between the manage_goals tool and the autonomy service.
func GoalFolderName(goalID int64, goalName string) string {
	slug := strings.ToLower(strings.TrimSpace(goalSlugPattern.ReplaceAllString(goalName, "-")))
	slug = strings.Trim(slug, "-")
	if len(slug) > 50 {
		slug = slug[:50]
	}
	if slug == "" {
		slug = "goal"
	}
	return fmt.Sprintf("goal-%d-%s", goalID, slug)
}

// NewManageGoalsTool creates a new coordinate tool.
func NewManageGoalsTool(opts ManageGoalsOptions) *ManageGoalsTool {
	return &ManageGoalsTool{
		mgr:       opts.GoalManager,
		agentID:   opts.AgentID,
		workspace: opts.Workspace,
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
		"priority":{"type":"string","description":"low, medium, or high (for add)"},
		"agent_count":{"type":"integer","description":"Number of parallel agents (1-5, 0 = auto). For add only."}
	},"required":["action"]}`), &schema)
	return schema
}

// Execute performs goal manipulations
func (t *ManageGoalsTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
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
		AgentCount  int    `json:"agent_count"`
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
		id, _ := g["id"].(float64)
		goalID := int64(id)

		// Store agent_count if specified.
		if parsedArgs.AgentCount > 0 && goalID > 0 {
			_ = t.mgr.SetAgentCount(goalID, parsedArgs.AgentCount)
		}

		// Create a dedicated folder for this goal and return its path.
		goalDir := t.ensureGoalFolder(goalID, parsedArgs.Name)

		if goalID > 0 {
			return NewToolResult(fmt.Sprintf(
				"Goal successfully added. ID: %d\nGoal folder: %s\n\n"+
					"IMPORTANT: All subagents working on this goal MUST save files under this folder using absolute paths.",
				goalID, goalDir))
		}
		return NewToolResult("Goal successfully added. Folder: " + goalDir)

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
		id, _ := g["id"].(float64)
		status, _ := g["status"].(string)
		return NewToolResult(fmt.Sprintf("Goal %.0f status updated to %s", id, status))

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
			id, _ := g["id"].(float64)
			name, _ := g["name"].(string)
			priority, _ := g["priority"].(string)
			description, _ := g["description"].(string)
			fmt.Fprintf(&out, "- ID: %.0f | Name: %s | Priority: %s\n  %s\n",
				id,
				name,
				priority,
				description)
		}
		return NewToolResult(out.String())

	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", parsedArgs.Action))
	}
}

// ensureGoalFolder creates a dedicated directory for the goal under workspace/goals/.
func (t *ManageGoalsTool) ensureGoalFolder(goalID int64, goalName string) string {
	dir := filepath.Join(t.workspace, "goals", GoalFolderName(goalID, goalName))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.WarnCF("tools", "Failed to create goal folder", map[string]any{
			"path":  dir,
			"error": err.Error(),
		})
	}
	return dir
}
