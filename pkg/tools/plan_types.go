package tools

import (
	"fmt"
	"strings"
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
	DependsOn   []int      `json:"depends_on,omitempty"`  // Indices of steps this depends on
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
	GoalID          int64        `json:"goal_id,omitempty"` // Links to autonomy Goal
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
		fmt.Fprintf(&sb, "Sub-Plan: %s (parent: %s, step %d)\n", p.ID, p.ParentPlanID, p.ParentStepIndex+1)
	} else {
		fmt.Fprintf(&sb, "Plan: %s\n", p.ID)
	}
	fmt.Fprintf(&sb, "Goal: %s\nStatus: %s\n\nSteps:\n", p.Goal, p.Status)

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
		fmt.Fprintf(&sb, "  %s %d. %s", icon, step.Index+1, step.Description)
		if step.SubPlanID != "" {
			fmt.Fprintf(&sb, " → sub-plan: %s", step.SubPlanID)
		}
		if step.Result != "" {
			fmt.Fprintf(&sb, " -> %s", step.Result)
		}
		sb.WriteString("\n")
	}

	completed := 0
	for _, s := range p.Steps {
		if s.Status == PlanStatusCompleted {
			completed++
		}
	}
	fmt.Fprintf(&sb, "\nProgress: %d/%d steps completed", completed, len(p.Steps))

	if p.Assessment != nil {
		fmt.Fprintf(&sb, "\n\nAssessment: effort=%d/10, risk=%d/10, confidence=%.0f%%",
			p.Assessment.Effort, p.Assessment.Risk, p.Assessment.Confidence*100)
		if p.Assessment.Rationale != "" {
			fmt.Fprintf(&sb, "\nRationale: %s", p.Assessment.Rationale)
		}
		if len(p.Assessment.Alternatives) > 0 {
			fmt.Fprintf(&sb, "\nAlternatives: %s", strings.Join(p.Assessment.Alternatives, "; "))
		}
	}

	return sb.String()
}

// PlanStepDef is the LLM-generated definition of a plan step before it becomes a PlanStep.
type PlanStepDef struct {
	Description string `json:"description"`
	DependsOn   []int  `json:"depends_on"`
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
