package tools

import (
	"fmt"
	"sync"
)

// PlanManager manages active plans.
type PlanManager struct {
	plans       map[string]*Plan
	goalIndex   map[int64]string // goalID → planID for O(1) lookup
	mu          sync.RWMutex
	nextID      int
	persistPath string // if set, auto-saves after mutations
}

// NewPlanManager creates a new PlanManager.
func NewPlanManager() *PlanManager {
	return &PlanManager{
		plans:     make(map[string]*Plan),
		goalIndex: make(map[int64]string),
		nextID:    1,
	}
}

// SetPersistPath sets a file path for auto-saving. Call Load() first to restore.
func (pm *PlanManager) SetPersistPath(path string) {
	pm.persistPath = path
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

// ListAllPlans returns all plans (active + completed + failed).
func (pm *PlanManager) ListAllPlans() []*Plan {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	result := make([]*Plan, 0, len(pm.plans))
	for _, p := range pm.plans {
		result = append(result, p)
	}
	return result
}

// ClearPlan removes all plans.
func (pm *PlanManager) ClearPlan() {
	pm.mu.Lock()
	pm.plans = make(map[string]*Plan)
	pm.goalIndex = make(map[int64]string)
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

// CreatePlanForGoal creates a plan linked to a goal from LLM-generated step definitions.
func (pm *PlanManager) CreatePlanForGoal(goalID int64, goal string, stepDefs []PlanStepDef) *Plan {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	planID := fmt.Sprintf("plan-%d", pm.nextID)
	pm.nextID++

	steps := make([]PlanStep, len(stepDefs))
	for i, def := range stepDefs {
		steps[i] = PlanStep{
			Index:              i,
			Description:        def.Description,
			AcceptanceCriteria: def.AcceptanceCriteria,
			VerifyCommand:      def.VerifyCommand,
			Status:             PlanStatusPending,
			DependsOn:          def.DependsOn,
		}
	}

	plan := &Plan{
		ID:     planID,
		Goal:   goal,
		GoalID: goalID,
		Steps:  steps,
		Status: PlanStatusPending,
	}
	pm.plans[planID] = plan
	if goalID != 0 {
		pm.goalIndex[goalID] = planID
	}

	go pm.autoSave()
	return plan
}

// GetPlanByGoalID returns the plan linked to a specific goal ID, or nil.
// Uses the goalIndex for O(1) lookup instead of scanning all plans.
func (pm *PlanManager) GetPlanByGoalID(goalID int64) *Plan {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if planID, ok := pm.goalIndex[goalID]; ok {
		return pm.plans[planID]
	}
	// Fallback: linear scan for plans created before the index existed.
	for _, plan := range pm.plans {
		if plan.GoalID == goalID {
			return plan
		}
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
