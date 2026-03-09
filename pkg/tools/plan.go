package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// PlanStatus represents the status of a plan or step.
type PlanStatus string

const (
	PlanStatusPending    PlanStatus = "pending"
	PlanStatusInProgress PlanStatus = "in_progress"
	PlanStatusCompleted  PlanStatus = "completed"
	PlanStatusFailed     PlanStatus = "failed"
)

// PlanStep represents a single step in a plan.
type PlanStep struct {
	Index       int        `json:"index"`
	Description string     `json:"description"`
	Status      PlanStatus `json:"status"`
	Result      string     `json:"result,omitempty"`
}

// Plan represents a structured plan for completing a task.
type Plan struct {
	ID     string     `json:"id"`
	Goal   string     `json:"goal"`
	Steps  []PlanStep `json:"steps"`
	Status PlanStatus `json:"status"`
}

// FormatStatus returns a human-readable status string for the plan.
func (p *Plan) FormatStatus() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Plan: %s\nGoal: %s\nStatus: %s\n\nSteps:\n", p.ID, p.Goal, p.Status))
	for _, step := range p.Steps {
		icon := "[ ]"
		switch step.Status {
		case PlanStatusInProgress:
			icon = "[~]"
		case PlanStatusCompleted:
			icon = "[x]"
		case PlanStatusFailed:
			icon = "[!]"
		}
		sb.WriteString(fmt.Sprintf("  %s %d. %s", icon, step.Index+1, step.Description))
		if step.Result != "" {
			sb.WriteString(fmt.Sprintf(" -> %s", step.Result))
		}
		sb.WriteString("\n")
	}

	completed := 0
	for _, s := range p.Steps {
		if s.Status == PlanStatusCompleted {
			completed++
		}
	}
	sb.WriteString(fmt.Sprintf("\nProgress: %d/%d steps completed", completed, len(p.Steps)))
	return sb.String()
}

// PlanManager manages active plans.
type PlanManager struct {
	plans  map[string]*Plan
	mu     sync.RWMutex
	nextID int
}

// NewPlanManager creates a new PlanManager.
func NewPlanManager() *PlanManager {
	return &PlanManager{
		plans:  make(map[string]*Plan),
		nextID: 1,
	}
}

// GetActivePlan returns the currently active (non-completed) plan, if any.
func (pm *PlanManager) GetActivePlan() *Plan {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, plan := range pm.plans {
		if plan.Status == PlanStatusPending || plan.Status == PlanStatusInProgress {
			return plan
		}
	}
	return nil
}

// PlanTool provides plan-then-execute capability to the agent.
type PlanTool struct {
	manager *PlanManager
}

// NewPlanTool creates a new PlanTool.
func NewPlanTool(manager *PlanManager) *PlanTool {
	return &PlanTool{manager: manager}
}

func (t *PlanTool) Name() string        { return "plan" }
func (t *PlanTool) Description() string {
	return "Create and manage structured plans for multi-step tasks. Operations: create (make a new plan with steps), update_step (update a step's status and result), get_status (view current plan progress)."
}

func (t *PlanTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "update_step", "get_status"},
				"description": "The operation to perform",
			},
			"goal": map[string]any{
				"type":        "string",
				"description": "The goal for the plan (required for create)",
			},
			"steps": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "List of step descriptions (required for create)",
			},
			"plan_id": map[string]any{
				"type":        "string",
				"description": "Plan ID (required for update_step, optional for get_status)",
			},
			"step_index": map[string]any{
				"type":        "integer",
				"description": "Step index (0-based, required for update_step)",
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
	default:
		return ErrorResult(fmt.Sprintf("unknown operation: %s", op))
	}
}

func (t *PlanTool) create(args map[string]any) *ToolResult {
	goal, _ := args["goal"].(string)
	if goal == "" {
		return ErrorResult("goal is required for create operation")
	}

	rawSteps, ok := args["steps"]
	if !ok {
		return ErrorResult("steps is required for create operation")
	}

	var stepDescs []string
	switch v := rawSteps.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				stepDescs = append(stepDescs, s)
			}
		}
	case []string:
		stepDescs = v
	default:
		// Try JSON unmarshal
		data, _ := json.Marshal(rawSteps)
		if err := json.Unmarshal(data, &stepDescs); err != nil {
			return ErrorResult("steps must be an array of strings")
		}
	}

	if len(stepDescs) == 0 {
		return ErrorResult("at least one step is required")
	}

	t.manager.mu.Lock()
	planID := fmt.Sprintf("plan-%d", t.manager.nextID)
	t.manager.nextID++

	steps := make([]PlanStep, len(stepDescs))
	for i, desc := range stepDescs {
		steps[i] = PlanStep{
			Index:       i,
			Description: desc,
			Status:      PlanStatusPending,
		}
	}

	plan := &Plan{
		ID:     planID,
		Goal:   goal,
		Steps:  steps,
		Status: PlanStatusInProgress,
	}
	t.manager.plans[planID] = plan
	t.manager.mu.Unlock()

	return SilentResult(plan.FormatStatus())
}

func (t *PlanTool) updateStep(args map[string]any) *ToolResult {
	planID, _ := args["plan_id"].(string)
	if planID == "" {
		return ErrorResult("plan_id is required for update_step")
	}

	stepIdx, ok := args["step_index"].(float64)
	if !ok {
		return ErrorResult("step_index is required for update_step")
	}

	status, _ := args["status"].(string)
	if status == "" {
		return ErrorResult("status is required for update_step")
	}

	result, _ := args["result"].(string)

	t.manager.mu.Lock()
	defer t.manager.mu.Unlock()

	plan, ok := t.manager.plans[planID]
	if !ok {
		return ErrorResult(fmt.Sprintf("plan %q not found", planID))
	}

	idx := int(stepIdx)
	if idx < 0 || idx >= len(plan.Steps) {
		return ErrorResult(fmt.Sprintf("step_index %d out of range (0-%d)", idx, len(plan.Steps)-1))
	}

	plan.Steps[idx].Status = PlanStatus(status)
	if result != "" {
		plan.Steps[idx].Result = result
	}

	// Update plan status based on steps
	allCompleted := true
	anyFailed := false
	for _, s := range plan.Steps {
		if s.Status != PlanStatusCompleted {
			allCompleted = false
		}
		if s.Status == PlanStatusFailed {
			anyFailed = true
		}
	}

	if allCompleted {
		plan.Status = PlanStatusCompleted
	} else if anyFailed {
		plan.Status = PlanStatusFailed
	}

	return SilentResult(plan.FormatStatus())
}

func (t *PlanTool) getStatus(args map[string]any) *ToolResult {
	planID, _ := args["plan_id"].(string)

	t.manager.mu.RLock()
	defer t.manager.mu.RUnlock()

	if planID != "" {
		plan, ok := t.manager.plans[planID]
		if !ok {
			return ErrorResult(fmt.Sprintf("plan %q not found", planID))
		}
		return SilentResult(plan.FormatStatus())
	}

	// Return active plan
	for _, plan := range t.manager.plans {
		if plan.Status == PlanStatusPending || plan.Status == PlanStatusInProgress {
			return SilentResult(plan.FormatStatus())
		}
	}

	return SilentResult("No active plans.")
}
