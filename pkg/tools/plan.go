package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/grasberg/sofia/pkg/memory"
)

// PlanStatus represents the status of a plan or step.
type PlanStatus string

const (
	PlanStatusPending    PlanStatus = "pending"
	PlanStatusInProgress PlanStatus = "in_progress"
	PlanStatusCompleted  PlanStatus = "completed"
	PlanStatusFailed     PlanStatus = "failed"
)

// validTransitions defines allowed status transitions for plan steps.
var validTransitions = map[PlanStatus][]PlanStatus{
	PlanStatusPending:    {PlanStatusInProgress, PlanStatusCompleted, PlanStatusFailed},
	PlanStatusInProgress: {PlanStatusCompleted, PlanStatusFailed, PlanStatusPending},
	PlanStatusCompleted:  {},                                        // terminal state
	PlanStatusFailed:     {PlanStatusPending, PlanStatusInProgress}, // allow retry
}

// isValidTransition checks whether transitioning from -> to is allowed.
func isValidTransition(from, to PlanStatus) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// isValidStatus checks whether the given status is a known PlanStatus constant.
func isValidStatus(s PlanStatus) bool {
	switch s {
	case PlanStatusPending, PlanStatusInProgress, PlanStatusCompleted, PlanStatusFailed:
		return true
	default:
		return false
	}
}

// PlanStep represents a single step in a plan.
type PlanStep struct {
	Index       int        `json:"index"`
	Description string     `json:"description"`
	Status      PlanStatus `json:"status"`
	Result      string     `json:"result,omitempty"`
	SubPlanID   string     `json:"sub_plan_id,omitempty"` // Links to a child plan
	AssignedTo  string     `json:"assigned_to,omitempty"` // Agent ID working on this step
}

// CostBenefit holds a trade-off assessment for a plan.
type CostBenefit struct {
	Effort       int      `json:"effort"`       // 1-10 estimated effort
	Risk         int      `json:"risk"`         // 1-10 risk level
	Confidence   float64  `json:"confidence"`   // 0.0-1.0 confidence score
	Rationale    string   `json:"rationale"`    // Reasoning for the assessment
	Alternatives []string `json:"alternatives"` // Alternative approaches considered
}

// Plan represents a structured plan for completing a task.
type Plan struct {
	ID              string       `json:"id"`
	Goal            string       `json:"goal"`
	Steps           []PlanStep   `json:"steps"`
	Status          PlanStatus   `json:"status"`
	ParentPlanID    string       `json:"parent_plan_id,omitempty"`    // For hierarchical plans
	ParentStepIndex int          `json:"parent_step_index,omitempty"` // Step in parent that spawned this
	Assessment      *CostBenefit `json:"assessment,omitempty"`        // Trade-off analysis
}

// FormatStatus returns a human-readable status string for the plan.
func (p *Plan) FormatStatus() string {
	var sb strings.Builder

	if p.ParentPlanID != "" {
		sb.WriteString(fmt.Sprintf("Sub-Plan: %s (parent: %s, step %d)\n", p.ID, p.ParentPlanID, p.ParentStepIndex+1))
	} else {
		sb.WriteString(fmt.Sprintf("Plan: %s\n", p.ID))
	}
	sb.WriteString(fmt.Sprintf("Goal: %s\nStatus: %s\n\nSteps:\n", p.Goal, p.Status))

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
		if step.SubPlanID != "" {
			sb.WriteString(fmt.Sprintf(" → sub-plan: %s", step.SubPlanID))
		}
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

	if p.Assessment != nil {
		sb.WriteString(fmt.Sprintf("\n\nAssessment: effort=%d/10, risk=%d/10, confidence=%.0f%%",
			p.Assessment.Effort, p.Assessment.Risk, p.Assessment.Confidence*100))
		if p.Assessment.Rationale != "" {
			sb.WriteString(fmt.Sprintf("\nRationale: %s", p.Assessment.Rationale))
		}
		if len(p.Assessment.Alternatives) > 0 {
			sb.WriteString(fmt.Sprintf("\nAlternatives: %s", strings.Join(p.Assessment.Alternatives, "; ")))
		}
	}

	return sb.String()
}

// FormatStatusHierarchical returns the plan status with sub-plans expanded inline.
func (p *Plan) FormatStatusHierarchical(mgr *PlanManager) string {
	var sb strings.Builder
	sb.WriteString(p.FormatStatus())

	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	for _, step := range p.Steps {
		if step.SubPlanID != "" {
			if sub, ok := mgr.plans[step.SubPlanID]; ok {
				sb.WriteString("\n\n--- Sub-plan for step " + fmt.Sprintf("%d", step.Index+1) + " ---\n")
				sb.WriteString(sub.FormatStatus())
			}
		}
	}
	return sb.String()
}

// PlanManager manages active plans.
type PlanManager struct {
	plans       map[string]*Plan
	mu          sync.RWMutex
	nextID      int
	persistPath string // if set, auto-saves after mutations
}

// NewPlanManager creates a new PlanManager.
func NewPlanManager() *PlanManager {
	return &PlanManager{
		plans:  make(map[string]*Plan),
		nextID: 1,
	}
}

// SetPersistPath sets a file path for auto-saving. Call Load() first to restore.
func (pm *PlanManager) SetPersistPath(path string) {
	pm.persistPath = path
}

// autoSave saves to persistPath if set. It collects state under RLock
// and writes to disk outside the lock to avoid deadlock.
func (pm *PlanManager) autoSave() {
	if pm.persistPath == "" {
		return
	}
	// Collect the data under RLock.
	pm.mu.RLock()
	state := planPersistState{Plans: pm.plans, NextID: pm.nextID}
	pm.mu.RUnlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(pm.persistPath, data, 0o600)
}

// planPersistState is the JSON-serializable snapshot of the PlanManager.
type planPersistState struct {
	Plans  map[string]*Plan `json:"plans"`
	NextID int              `json:"next_id"`
}

// Save persists all plans to the given file path.
func (pm *PlanManager) Save(path string) error {
	pm.mu.RLock()
	state := planPersistState{Plans: pm.plans, NextID: pm.nextID}
	pm.mu.RUnlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("plan: marshal: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// Load restores plans from the given file path. Missing file is not an error.
func (pm *PlanManager) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("plan: read: %w", err)
	}
	var state planPersistState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("plan: unmarshal: %w", err)
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()
	if state.Plans != nil {
		pm.plans = state.Plans
	}
	if state.NextID > pm.nextID {
		pm.nextID = state.NextID
	}
	return nil
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

// ClearPlan removes all plans.
func (pm *PlanManager) ClearPlan() {
	pm.mu.Lock()
	pm.plans = make(map[string]*Plan)
	pm.mu.Unlock()
	pm.autoSave()
}

// ClaimPendingStep atomically finds and claims the next pending step for an agent.
// Returns the plan ID, step index, and step description, or empty if nothing available.
func (pm *PlanManager) ClaimPendingStep(agentID string) (planID string, stepIdx int, description string, ok bool) {
	pm.mu.Lock()
	for _, plan := range pm.plans {
		if plan.Status != PlanStatusPending && plan.Status != PlanStatusInProgress {
			continue
		}
		for i := range plan.Steps {
			if plan.Steps[i].Status == PlanStatusPending && plan.Steps[i].AssignedTo == "" {
				plan.Steps[i].Status = PlanStatusInProgress
				plan.Steps[i].AssignedTo = agentID
				plan.Status = PlanStatusInProgress
				pm.mu.Unlock()
				pm.autoSave()
				return plan.ID, i, plan.Steps[i].Description, true
			}
		}
	}
	pm.mu.Unlock()
	return "", 0, "", false
}

// CompleteStep marks a step as completed (or failed) with a result, and updates the plan status.
func (pm *PlanManager) CompleteStep(planID string, stepIdx int, success bool, result string) {
	pm.mu.Lock()
	plan, exists := pm.plans[planID]
	if !exists || stepIdx < 0 || stepIdx >= len(plan.Steps) {
		pm.mu.Unlock()
		return
	}
	if success {
		plan.Steps[stepIdx].Status = PlanStatusCompleted
	} else {
		plan.Steps[stepIdx].Status = PlanStatusFailed
	}
	plan.Steps[stepIdx].Result = result

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
	pm.mu.Unlock()
	pm.autoSave()
}

// HasPendingSteps returns true if any plan has unclaimed pending steps.
func (pm *PlanManager) HasPendingSteps() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, plan := range pm.plans {
		if plan.Status != PlanStatusPending && plan.Status != PlanStatusInProgress {
			continue
		}
		for _, s := range plan.Steps {
			if s.Status == PlanStatusPending && s.AssignedTo == "" {
				return true
			}
		}
	}
	return false
}

// GetPlan returns a specific plan by ID.
func (pm *PlanManager) GetPlan(planID string) *Plan {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.plans[planID]
}

// InsertStep inserts a new step at the given index in the plan.
func (pm *PlanManager) InsertStep(planID string, index int, description string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	plan, ok := pm.plans[planID]
	if !ok {
		return fmt.Errorf("plan %q not found", planID)
	}
	if index < 0 || index > len(plan.Steps) {
		return fmt.Errorf("step_index %d out of range (0-%d)", index, len(plan.Steps))
	}

	newStep := PlanStep{
		Index:       index,
		Description: description,
		Status:      PlanStatusPending,
	}

	// Insert at position
	plan.Steps = append(plan.Steps, PlanStep{})
	copy(plan.Steps[index+1:], plan.Steps[index:])
	plan.Steps[index] = newStep

	// Reindex all steps
	for i := range plan.Steps {
		plan.Steps[i].Index = i
	}
	return nil
}

// RemoveStep removes a step at the given index from the plan.
func (pm *PlanManager) RemoveStep(planID string, index int) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	plan, ok := pm.plans[planID]
	if !ok {
		return fmt.Errorf("plan %q not found", planID)
	}
	if index < 0 || index >= len(plan.Steps) {
		return fmt.Errorf("step_index %d out of range (0-%d)", index, len(plan.Steps)-1)
	}
	if len(plan.Steps) <= 1 {
		return fmt.Errorf("cannot remove the last step from a plan")
	}

	plan.Steps = append(plan.Steps[:index], plan.Steps[index+1:]...)
	for i := range plan.Steps {
		plan.Steps[i].Index = i
	}
	return nil
}

// ReorderStep moves a step from oldIndex to newIndex.
func (pm *PlanManager) ReorderStep(planID string, oldIndex, newIndex int) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	plan, ok := pm.plans[planID]
	if !ok {
		return fmt.Errorf("plan %q not found", planID)
	}
	if oldIndex < 0 || oldIndex >= len(plan.Steps) {
		return fmt.Errorf("step_index %d out of range (0-%d)", oldIndex, len(plan.Steps)-1)
	}
	if newIndex < 0 || newIndex >= len(plan.Steps) {
		return fmt.Errorf("new_index %d out of range (0-%d)", newIndex, len(plan.Steps)-1)
	}
	if oldIndex == newIndex {
		return nil
	}

	step := plan.Steps[oldIndex]
	plan.Steps = append(plan.Steps[:oldIndex], plan.Steps[oldIndex+1:]...)

	// Insert at new position
	rear := make([]PlanStep, len(plan.Steps[newIndex:]))
	copy(rear, plan.Steps[newIndex:])
	plan.Steps = append(plan.Steps[:newIndex], step)
	plan.Steps = append(plan.Steps, rear...)

	for i := range plan.Steps {
		plan.Steps[i].Index = i
	}
	return nil
}

// CreateSubPlan creates a child plan linked to a parent step.
func (pm *PlanManager) CreateSubPlan(
	parentPlanID string,
	parentStepIndex int,
	goal string,
	stepDescs []string,
) (*Plan, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	parent, ok := pm.plans[parentPlanID]
	if !ok {
		return nil, fmt.Errorf("parent plan %q not found", parentPlanID)
	}
	if parentStepIndex < 0 || parentStepIndex >= len(parent.Steps) {
		return nil, fmt.Errorf("parent_step_index %d out of range (0-%d)", parentStepIndex, len(parent.Steps)-1)
	}

	subPlanID := fmt.Sprintf("plan-%d", pm.nextID)
	pm.nextID++

	steps := make([]PlanStep, len(stepDescs))
	for i, desc := range stepDescs {
		steps[i] = PlanStep{
			Index:       i,
			Description: desc,
			Status:      PlanStatusPending,
		}
	}

	subPlan := &Plan{
		ID:              subPlanID,
		Goal:            goal,
		Steps:           steps,
		Status:          PlanStatusInProgress,
		ParentPlanID:    parentPlanID,
		ParentStepIndex: parentStepIndex,
	}
	pm.plans[subPlanID] = subPlan

	// Link parent step to sub-plan
	parent.Steps[parentStepIndex].SubPlanID = subPlanID

	return subPlan, nil
}

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
	t.manager.autoSave()

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

	newStatus := PlanStatus(status)
	if !isValidStatus(newStatus) {
		return ErrorResult(fmt.Sprintf("invalid status %q: must be pending, in_progress, completed, or failed", status))
	}

	result, _ := args["result"].(string)

	t.manager.mu.Lock()

	plan, ok := t.manager.plans[planID]
	if !ok {
		t.manager.mu.Unlock()
		return ErrorResult(fmt.Sprintf("plan %q not found", planID))
	}

	idx := int(stepIdx)
	if idx < 0 || idx >= len(plan.Steps) {
		t.manager.mu.Unlock()
		return ErrorResult(fmt.Sprintf("step_index %d out of range (0-%d)", idx, len(plan.Steps)-1))
	}

	currentStatus := plan.Steps[idx].Status
	if !isValidTransition(currentStatus, newStatus) {
		t.manager.mu.Unlock()
		return ErrorResult(fmt.Sprintf("invalid transition from %q to %q for step %d", currentStatus, newStatus, idx))
	}

	plan.Steps[idx].Status = newStatus
	if result != "" {
		plan.Steps[idx].Result = result
	}

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

	formatted := plan.FormatStatus()
	t.manager.mu.Unlock()
	t.manager.autoSave()

	return SilentResult(formatted)
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
		return SilentResult(plan.FormatStatusHierarchical(t.manager))
	}

	// Return active plan
	for _, plan := range t.manager.plans {
		if plan.Status == PlanStatusPending || plan.Status == PlanStatusInProgress {
			return SilentResult(plan.FormatStatusHierarchical(t.manager))
		}
	}

	return SilentResult("No active plans.")
}

// replan handles dynamic re-planning: insert, remove, or reorder steps.
func (t *PlanTool) replan(args map[string]any) *ToolResult {
	planID, _ := args["plan_id"].(string)
	if planID == "" {
		return ErrorResult("plan_id is required for replan")
	}

	action, _ := args["action"].(string)
	if action == "" {
		return ErrorResult("action is required for replan (insert, remove, reorder)")
	}

	stepIdx, _ := args["step_index"].(float64)
	idx := int(stepIdx)

	switch action {
	case "insert":
		desc, _ := args["description"].(string)
		if desc == "" {
			return ErrorResult("description is required for replan insert")
		}
		if err := t.manager.InsertStep(planID, idx, desc); err != nil {
			return ErrorResult(err.Error())
		}

	case "remove":
		if err := t.manager.RemoveStep(planID, idx); err != nil {
			return ErrorResult(err.Error())
		}

	case "reorder":
		newIdx, ok := args["new_index"].(float64)
		if !ok {
			return ErrorResult("new_index is required for replan reorder")
		}
		if err := t.manager.ReorderStep(planID, idx, int(newIdx)); err != nil {
			return ErrorResult(err.Error())
		}

	default:
		return ErrorResult(fmt.Sprintf("unknown replan action: %s (use insert, remove, or reorder)", action))
	}

	plan := t.manager.GetPlan(planID)
	if plan == nil {
		return ErrorResult(fmt.Sprintf("plan %q not found", planID))
	}
	return SilentResult(plan.FormatStatus())
}

// createSubplan creates a hierarchical sub-plan for a complex step.
func (t *PlanTool) createSubplan(args map[string]any) *ToolResult {
	parentPlanID, _ := args["parent_plan_id"].(string)
	if parentPlanID == "" {
		parentPlanID, _ = args["plan_id"].(string)
	}
	if parentPlanID == "" {
		return ErrorResult("parent_plan_id is required for create_subplan")
	}

	parentStepIdx, ok := args["parent_step_index"].(float64)
	if !ok {
		parentStepIdx2, ok2 := args["step_index"].(float64)
		if !ok2 {
			return ErrorResult("parent_step_index is required for create_subplan")
		}
		parentStepIdx = parentStepIdx2
	}

	goal, _ := args["goal"].(string)
	if goal == "" {
		return ErrorResult("goal is required for create_subplan")
	}

	rawSteps, ok := args["steps"]
	if !ok {
		return ErrorResult("steps is required for create_subplan")
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
		data, _ := json.Marshal(rawSteps)
		if err := json.Unmarshal(data, &stepDescs); err != nil {
			return ErrorResult("steps must be an array of strings")
		}
	}

	if len(stepDescs) == 0 {
		return ErrorResult("at least one step is required for sub-plan")
	}

	subPlan, err := t.manager.CreateSubPlan(parentPlanID, int(parentStepIdx), goal, stepDescs)
	if err != nil {
		return ErrorResult(err.Error())
	}

	// Show parent with hierarchy
	parent := t.manager.GetPlan(parentPlanID)
	if parent != nil {
		return SilentResult(parent.FormatStatusHierarchical(t.manager))
	}
	return SilentResult(subPlan.FormatStatus())
}

// saveTemplate saves a completed plan as a reusable template.
func (t *PlanTool) saveTemplate(args map[string]any) *ToolResult {
	if t.memDB == nil {
		return ErrorResult("plan templates require a memory database (not available)")
	}

	planID, _ := args["plan_id"].(string)
	if planID == "" {
		return ErrorResult("plan_id is required for save_template")
	}

	name, _ := args["name"].(string)
	if name == "" {
		return ErrorResult("name is required for save_template")
	}

	tags, _ := args["tags"].(string)

	plan := t.manager.GetPlan(planID)
	if plan == nil {
		return ErrorResult(fmt.Sprintf("plan %q not found", planID))
	}

	stepDescs := make([]string, len(plan.Steps))
	for i, s := range plan.Steps {
		stepDescs[i] = s.Description
	}

	if err := t.memDB.SavePlanTemplate(name, plan.Goal, stepDescs, tags); err != nil {
		return ErrorResult(fmt.Sprintf("failed to save template: %v", err))
	}

	return SilentResult(fmt.Sprintf("Template %q saved with %d steps (goal: %s)", name, len(stepDescs), plan.Goal))
}

// findTemplates searches for plan templates matching a query.
func (t *PlanTool) findTemplates(args map[string]any) *ToolResult {
	if t.memDB == nil {
		return ErrorResult("plan templates require a memory database (not available)")
	}

	query, _ := args["query"].(string)
	if query == "" {
		return ErrorResult("query is required for find_templates")
	}

	templates, err := t.memDB.FindPlanTemplates(query, 10)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to search templates: %v", err))
	}

	if len(templates) == 0 {
		return SilentResult(fmt.Sprintf("No templates found matching %q", query))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d template(s) matching %q:\n\n", len(templates), query))
	for _, tmpl := range templates {
		sb.WriteString(fmt.Sprintf("📋 %s (used %d times)\n", tmpl.Name, tmpl.UseCount))
		sb.WriteString(fmt.Sprintf("   Goal: %s\n", tmpl.Goal))
		if tmpl.Tags != "" {
			sb.WriteString(fmt.Sprintf("   Tags: %s\n", tmpl.Tags))
		}
		sb.WriteString(fmt.Sprintf("   Steps: %s\n\n", strings.Join(tmpl.Steps, " → ")))
	}

	return SilentResult(sb.String())
}

// useTemplate creates a new plan from a saved template.
func (t *PlanTool) useTemplate(args map[string]any) *ToolResult {
	if t.memDB == nil {
		return ErrorResult("plan templates require a memory database (not available)")
	}

	name, _ := args["name"].(string)
	if name == "" {
		return ErrorResult("name is required for use_template")
	}

	tmpl, err := t.memDB.GetPlanTemplate(name)
	if err != nil {
		return ErrorResult(fmt.Sprintf("template %q not found: %v", name, err))
	}

	// Allow goal override
	goal, _ := args["goal"].(string)
	if goal == "" {
		goal = tmpl.Goal
	}

	// Create the plan from template steps
	createArgs := map[string]any{
		"goal":  goal,
		"steps": tmpl.Steps,
	}

	result := t.create(createArgs)

	// Increment usage counter
	_ = t.memDB.IncrementTemplateUseCount(name)

	if result.IsError {
		return result
	}

	return SilentResult(fmt.Sprintf("Plan created from template %q (used %d times before)\n\n%s",
		name, tmpl.UseCount, result.ForLLM))
}

// evaluate performs cost/benefit analysis on a plan.
func (t *PlanTool) evaluate(args map[string]any) *ToolResult {
	planID, _ := args["plan_id"].(string)
	if planID == "" {
		return ErrorResult("plan_id is required for evaluate")
	}

	effort, _ := args["effort"].(float64)
	risk, _ := args["risk"].(float64)
	confidence, _ := args["confidence"].(float64)
	rationale, _ := args["rationale"].(string)

	var alternatives []string
	if rawAlts, ok := args["alternatives"]; ok {
		switch v := rawAlts.(type) {
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					alternatives = append(alternatives, s)
				}
			}
		case []string:
			alternatives = v
		}
	}

	// Clamp values
	effortInt := int(effort)
	if effortInt < 1 {
		effortInt = 1
	}
	if effortInt > 10 {
		effortInt = 10
	}
	riskInt := int(risk)
	if riskInt < 1 {
		riskInt = 1
	}
	if riskInt > 10 {
		riskInt = 10
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1.0 {
		confidence = 1.0
	}

	t.manager.mu.Lock()
	defer t.manager.mu.Unlock()

	plan, ok := t.manager.plans[planID]
	if !ok {
		return ErrorResult(fmt.Sprintf("plan %q not found", planID))
	}

	plan.Assessment = &CostBenefit{
		Effort:       effortInt,
		Risk:         riskInt,
		Confidence:   confidence,
		Rationale:    rationale,
		Alternatives: alternatives,
	}

	// Build a recommendation summary
	var recommendation string
	score := confidence * float64(10-riskInt) / float64(effortInt)
	switch {
	case score >= 1.0:
		recommendation = "RECOMMENDED — High confidence, favorable cost/benefit ratio."
	case score >= 0.5:
		recommendation = "PROCEED WITH CAUTION — Moderate cost/benefit ratio."
	default:
		recommendation = "RECONSIDER — Low confidence or unfavorable cost/benefit ratio."
	}

	return SilentResult(
		fmt.Sprintf("%s\n\n%s\nScore: %.2f (higher is better)", plan.FormatStatus(), recommendation, score),
	)
}
