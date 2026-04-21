package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

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

	var goalID int64
	if gid, ok := args["goal_id"].(float64); ok {
		goalID = int64(gid)
	}

	plan := &Plan{
		ID:     planID,
		Goal:   goal,
		GoalID: goalID,
		Steps:  steps,
		Status: PlanStatusInProgress,
	}
	t.manager.plans[planID] = plan
	if goalID != 0 {
		t.manager.goalIndex[goalID] = planID
	}
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

	plan.Status = evaluatePlanStatus(plan.Steps)

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
	fmt.Fprintf(&sb, "Found %d template(s) matching %q:\n\n", len(templates), query)
	for _, tmpl := range templates {
		fmt.Fprintf(&sb, "📋 %s (used %d times)\n", tmpl.Name, tmpl.UseCount)
		fmt.Fprintf(&sb, "   Goal: %s\n", tmpl.Goal)
		if tmpl.Tags != "" {
			fmt.Fprintf(&sb, "   Tags: %s\n", tmpl.Tags)
		}
		fmt.Fprintf(&sb, "   Steps: %s\n\n", strings.Join(tmpl.Steps, " → "))
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
