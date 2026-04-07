package tools

import (
	"context"
	"fmt"

	"github.com/grasberg/sofia/pkg/memory"
)

// PlanTool provides plan-then-execute capability to the agent.
type PlanTool struct {
	manager *PlanManager
	memDB   *memory.MemoryDB // optional, for template persistence
}

// NewPlanTool creates a new PlanTool.
func NewPlanTool(manager *PlanManager, memDB *memory.MemoryDB) *PlanTool {
	return &PlanTool{manager: manager, memDB: memDB}
}

func (t *PlanTool) Name() string { return "plan" }
func (t *PlanTool) Description() string {
	return "Create and manage structured plans for multi-step tasks. Operations: create (make a new plan with steps), update_step (update a step's status and result), get_status (view current plan progress), replan (insert/remove/reorder steps dynamically), create_subplan (hierarchical sub-plan for a complex step), save_template (save a successful plan as a reusable template), find_templates (search saved plan templates), use_template (create a plan from a template), evaluate (cost/benefit trade-off analysis)."
}

func (t *PlanTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type": "string",
				"enum": []string{
					"create",
					"update_step",
					"get_status",
					"replan",
					"create_subplan",
					"save_template",
					"find_templates",
					"use_template",
					"evaluate",
				},
				"description": "The operation to perform",
			},
			"goal": map[string]any{
				"type":        "string",
				"description": "The goal for the plan (required for create, use_template)",
			},
			"goal_id": map[string]any{
				"type":        "integer",
				"description": "ID of the autonomy goal this plan is for (optional for create, links the plan to a goal)",
			},
			"steps": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "List of step descriptions (required for create, create_subplan)",
			},
			"plan_id": map[string]any{
				"type":        "string",
				"description": "Plan ID (required for update_step, replan, create_subplan, save_template, evaluate; optional for get_status)",
			},
			"step_index": map[string]any{
				"type":        "integer",
				"description": "Step index (0-based, required for update_step, replan insert/remove/reorder, create_subplan)",
			},
			"status": map[string]any{
				"type":        "string",
				"enum":        []string{"pending", "in_progress", "completed", "failed"},
				"description": "New status for the step (required for update_step)",
			},
			"result": map[string]any{
				"type":        "string",
				"description": "Result or note for the step (optional for update_step)",
			},
			// Replan fields
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"insert", "remove", "reorder"},
				"description": "Replan action (required for replan)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Step description (required for replan insert)",
			},
			"new_index": map[string]any{
				"type":        "integer",
				"description": "Target index for reorder (required for replan reorder)",
			},
			// Sub-plan fields
			"parent_plan_id": map[string]any{
				"type":        "string",
				"description": "Parent plan ID (required for create_subplan)",
			},
			"parent_step_index": map[string]any{
				"type":        "integer",
				"description": "Parent step index (required for create_subplan)",
			},
			// Template fields
			"name": map[string]any{
				"type":        "string",
				"description": "Template name (required for save_template, use_template)",
			},
			"tags": map[string]any{
				"type":        "string",
				"description": "Comma-separated tags (optional for save_template)",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Search query (required for find_templates)",
			},
			// Cost/benefit fields
			"effort": map[string]any{
				"type":        "integer",
				"description": "Estimated effort 1-10 (required for evaluate)",
			},
			"risk": map[string]any{
				"type":        "integer",
				"description": "Risk level 1-10 (required for evaluate)",
			},
			"confidence": map[string]any{
				"type":        "number",
				"description": "Confidence 0.0-1.0 (required for evaluate)",
			},
			"rationale": map[string]any{
				"type":        "string",
				"description": "Reasoning for the assessment (optional for evaluate)",
			},
			"alternatives": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Alternative approaches (optional for evaluate)",
			},
		},
		"required": []string{"operation"},
	}
}

func (t *PlanTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	op, _ := args["operation"].(string)
	switch op {
	case "create":
		return t.create(args)
	case "update_step":
		return t.updateStep(args)
	case "get_status":
		return t.getStatus(args)
	case "replan":
		return t.replan(args)
	case "create_subplan":
		return t.createSubplan(args)
	case "save_template":
		return t.saveTemplate(args)
	case "find_templates":
		return t.findTemplates(args)
	case "use_template":
		return t.useTemplate(args)
	case "evaluate":
		return t.evaluate(args)
	default:
		return ErrorResult(fmt.Sprintf("unknown operation: %s", op))
	}
}
