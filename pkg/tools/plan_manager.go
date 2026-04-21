package tools

import (
	"encoding/json"
	"fmt"
	"os"
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
		// Rebuild the goal index from loaded plans.
		pm.goalIndex = make(map[int64]string, len(pm.plans))
		for _, p := range pm.plans {
			if p.GoalID != 0 {
				pm.goalIndex[p.GoalID] = p.ID
			}
		}
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
	plan.Status = evaluatePlanStatus(plan.Steps)
	pm.mu.Unlock()
	pm.autoSave()
}

// CompleteStepWithVerify marks a step as completed/failed with both result and verification output.
func (pm *PlanManager) CompleteStepWithVerify(planID string, stepIdx int, success bool, result, verifyResult string) {
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
	plan.Steps[stepIdx].VerifyResult = verifyResult
	plan.Status = evaluatePlanStatus(plan.Steps)
	pm.mu.Unlock()
	pm.autoSave()
}

// FailAndRetryStep atomically records a step failure and resets it for retry,
// without ever setting the plan status to failed. This avoids a race where a
// concurrent tick observes a temporarily-failed plan between separate
// CompleteStep/RetryStep calls.
func (pm *PlanManager) FailAndRetryStep(planID string, stepIdx int, result, verifyResult string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	plan, ok := pm.plans[planID]
	if !ok || stepIdx < 0 || stepIdx >= len(plan.Steps) {
		return false
	}
	step := &plan.Steps[stepIdx]

	// Store the failure result for observability, then immediately reset.
	step.Result = result
	step.VerifyResult = verifyResult
	step.Status = PlanStatusPending
	step.AssignedTo = ""
	step.RetryCount++

	if plan.Status == PlanStatusFailed {
		plan.Status = PlanStatusInProgress
	}

	go pm.autoSave()
	return true
}

// ResetPlan resets all failed steps in a plan back to pending and clears their
// results and retry counts. The plan status is set back to in_progress so the
// goal tick can re-dispatch steps. Returns false if the plan doesn't exist.
func (pm *PlanManager) ResetPlan(planID string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	plan, ok := pm.plans[planID]
	if !ok {
		return false
	}

	for i := range plan.Steps {
		s := &plan.Steps[i]
		if s.Status == PlanStatusFailed {
			s.Status = PlanStatusPending
			s.AssignedTo = ""
			s.RetryCount = 0
			s.Result = ""
			s.VerifyResult = ""
		}
	}

	if plan.Status == PlanStatusFailed {
		plan.Status = PlanStatusInProgress
	}

	go pm.autoSave()
	return true
}

// ReviseFailedSteps updates the descriptions of failed steps with revised
// instructions, resets them to pending, and sets the plan back to in_progress.
// The revisions map is stepIndex → new description.
func (pm *PlanManager) ReviseFailedSteps(planID string, revisions map[int]string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	plan, ok := pm.plans[planID]
	if !ok {
		return false
	}

	for i := range plan.Steps {
		s := &plan.Steps[i]
		if s.Status != PlanStatusFailed {
			continue
		}
		if desc, hasRevision := revisions[i]; hasRevision {
			s.Description = desc
		}
		s.Status = PlanStatusPending
		s.AssignedTo = ""
		s.RetryCount = 0
		s.Result = ""
		s.VerifyResult = ""
	}

	if plan.Status == PlanStatusFailed {
		plan.Status = PlanStatusInProgress
	}

	go pm.autoSave()
	return true
}

// ResetStepForRetry resets a single failed step to pending WITHOUT incrementing
// retry_count — used after an out-of-band fix (e.g. auto-installing a missing
// binary) where the step's prior failures no longer reflect the new state.
// Returns false if the step is not currently failed or the plan/index is invalid.
func (pm *PlanManager) ResetStepForRetry(planID string, stepIdx int) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	plan, ok := pm.plans[planID]
	if !ok || stepIdx < 0 || stepIdx >= len(plan.Steps) {
		return false
	}
	step := &plan.Steps[stepIdx]
	if step.Status != PlanStatusFailed {
		return false
	}

	step.Status = PlanStatusPending
	step.AssignedTo = ""
	step.RetryCount = 0
	step.Result = ""
	step.VerifyResult = ""

	if plan.Status == PlanStatusFailed {
		plan.Status = PlanStatusInProgress
	}

	go pm.autoSave()
	return true
}

// RetryStep resets a failed step to pending for re-dispatch, incrementing its retry count.
func (pm *PlanManager) RetryStep(planID string, stepIdx int) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	plan, ok := pm.plans[planID]
	if !ok || stepIdx < 0 || stepIdx >= len(plan.Steps) {
		return false
	}
	step := &plan.Steps[stepIdx]
	if step.Status != PlanStatusFailed {
		return false
	}

	step.Status = PlanStatusPending
	step.AssignedTo = ""
	step.RetryCount++
	step.Result = ""
	step.VerifyResult = ""

	// Reset plan status from failed back to in_progress so dispatch continues.
	if plan.Status == PlanStatusFailed {
		plan.Status = PlanStatusInProgress
	}

	go pm.autoSave()
	return true
}

// evaluatePlanStatus determines the correct plan status from step states.
//   - completed: all steps completed
//   - failed: at least one step failed AND no pending/in-progress steps can
//     still make progress (either none exist, or all are blocked by a failed
//     dependency)
//   - in_progress: otherwise
func evaluatePlanStatus(steps []PlanStep) PlanStatus {
	allCompleted := true
	failedSet := make(map[int]bool)

	for i, s := range steps {
		if s.Status != PlanStatusCompleted {
			allCompleted = false
		}
		if s.Status == PlanStatusFailed {
			failedSet[i] = true
		}
	}
	if allCompleted {
		return PlanStatusCompleted
	}
	if len(failedSet) == 0 {
		return PlanStatusInProgress
	}

	// At least one step failed. Check whether any pending/in-progress step
	// can still make progress (i.e. none of its dependencies are failed).
	for _, s := range steps {
		if s.Status != PlanStatusPending && s.Status != PlanStatusInProgress {
			continue
		}
		blocked := false
		for _, dep := range s.DependsOn {
			if failedSet[dep] {
				blocked = true
				break
			}
		}
		if !blocked {
			return PlanStatusInProgress
		}
	}

	return PlanStatusFailed
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

// ReadySteps returns indices of steps that are pending, unassigned, and have all
// dependencies satisfied (all DependsOn steps are completed).
func (pm *PlanManager) ReadySteps(planID string) []int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	plan, ok := pm.plans[planID]
	if !ok {
		return nil
	}

	var ready []int
	for i, step := range plan.Steps {
		if step.Status != PlanStatusPending || step.AssignedTo != "" {
			continue
		}
		allDepsCompleted := true
		for _, dep := range step.DependsOn {
			if dep < 0 || dep >= len(plan.Steps) || plan.Steps[dep].Status != PlanStatusCompleted {
				allDepsCompleted = false
				break
			}
		}
		if allDepsCompleted {
			ready = append(ready, i)
		}
	}
	return ready
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

// ClaimStep marks a specific step as in_progress and assigns it to the given agent.
// Returns false if the plan or step does not exist, or if the step is not pending.
func (pm *PlanManager) ClaimStep(planID string, stepIdx int, assignee string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	plan, ok := pm.plans[planID]
	if !ok || stepIdx < 0 || stepIdx >= len(plan.Steps) {
		return false
	}
	if plan.Steps[stepIdx].Status != PlanStatusPending {
		return false
	}

	plan.Steps[stepIdx].Status = PlanStatusInProgress
	plan.Steps[stepIdx].AssignedTo = assignee
	if plan.Status == PlanStatusPending {
		plan.Status = PlanStatusInProgress
	}

	go pm.autoSave()
	return true
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
