package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadySteps_NoDependencies(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.CreatePlanForGoal(1, "Test goal", []PlanStepDef{
		{Description: "Step A", DependsOn: nil},
		{Description: "Step B", DependsOn: nil},
		{Description: "Step C", DependsOn: nil},
	})
	ready := pm.ReadySteps(plan.ID)
	assert.Equal(t, []int{0, 1, 2}, ready)
}

func TestReadySteps_WithDependencies(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.CreatePlanForGoal(1, "Test goal", []PlanStepDef{
		{Description: "Step A", DependsOn: nil},
		{Description: "Step B", DependsOn: []int{0}},
		{Description: "Step C", DependsOn: []int{0}},
		{Description: "Step D", DependsOn: []int{1, 2}},
	})

	ready := pm.ReadySteps(plan.ID)
	assert.Equal(t, []int{0}, ready)

	pm.CompleteStep(plan.ID, 0, true, "done")
	ready = pm.ReadySteps(plan.ID)
	assert.Equal(t, []int{1, 2}, ready)

	pm.CompleteStep(plan.ID, 1, true, "done")
	ready = pm.ReadySteps(plan.ID)
	assert.Equal(t, []int{2}, ready)

	pm.CompleteStep(plan.ID, 2, true, "done")
	ready = pm.ReadySteps(plan.ID)
	assert.Equal(t, []int{3}, ready)
}

func TestReadySteps_SkipsAssigned(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.CreatePlanForGoal(1, "Test goal", []PlanStepDef{
		{Description: "Step A", DependsOn: nil},
		{Description: "Step B", DependsOn: nil},
	})

	pm.ClaimPendingStep("agent-1")

	ready := pm.ReadySteps(plan.ID)
	assert.Equal(t, []int{1}, ready)
}

func TestReadySteps_NonexistentPlan(t *testing.T) {
	pm := NewPlanManager()
	ready := pm.ReadySteps("plan-999")
	assert.Empty(t, ready)
}

func TestCreatePlanForGoal(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.CreatePlanForGoal(42, "Deploy monitoring", []PlanStepDef{
		{Description: "Research", DependsOn: nil},
		{Description: "Implement", DependsOn: []int{0}},
	})

	require.NotNil(t, plan)
	assert.Equal(t, int64(42), plan.GoalID)
	assert.Equal(t, "Deploy monitoring", plan.Goal)
	assert.Equal(t, PlanStatusPending, plan.Status)
	assert.Len(t, plan.Steps, 2)
	assert.Equal(t, "Research", plan.Steps[0].Description)
	assert.Equal(t, PlanStatusPending, plan.Steps[0].Status)
	assert.Empty(t, plan.Steps[0].DependsOn)
	assert.Equal(t, []int{0}, plan.Steps[1].DependsOn)
}

func TestClaimStep(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.CreatePlanForGoal(1, "Test", []PlanStepDef{
		{Description: "Step A"},
		{Description: "Step B"},
	})

	ok := pm.ClaimStep(plan.ID, 0, "agent-x")
	assert.True(t, ok)
	assert.Equal(t, PlanStatusInProgress, plan.Steps[0].Status)
	assert.Equal(t, "agent-x", plan.Steps[0].AssignedTo)

	ok = pm.ClaimStep(plan.ID, 0, "agent-y")
	assert.False(t, ok)

	ok = pm.ClaimStep(plan.ID, 1, "agent-y")
	assert.True(t, ok)
}

func TestGetPlanByGoalID(t *testing.T) {
	pm := NewPlanManager()
	pm.CreatePlanForGoal(42, "Goal A", []PlanStepDef{
		{Description: "Step 1"},
	})
	pm.CreatePlanForGoal(99, "Goal B", []PlanStepDef{
		{Description: "Step 1"},
	})

	plan := pm.GetPlanByGoalID(42)
	require.NotNil(t, plan)
	assert.Equal(t, "Goal A", plan.Goal)

	plan = pm.GetPlanByGoalID(99)
	require.NotNil(t, plan)
	assert.Equal(t, "Goal B", plan.Goal)

	plan = pm.GetPlanByGoalID(123)
	assert.Nil(t, plan)
}
