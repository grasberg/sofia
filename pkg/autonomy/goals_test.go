package autonomy

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/memory"
)

func setupTestDB(t *testing.T) (*memory.MemoryDB, func()) {
	t.Helper()
	tmpDir := t.TempDir()

	db, err := memory.Open(filepath.Join(tmpDir, "memory.db"))
	require.NoError(t, err)

	cleanup := func() { db.Close() }
	return db, cleanup
}

func TestGoalManager_AddGoal(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	agentID := "agent-1"

	gAny, err := gm.AddGoal(agentID, "Learn Rust", "Read the rust book and write a small CLI", "high")
	require.NoError(t, err)
	require.NotNil(t, gAny)

	b, _ := json.Marshal(gAny)
	var goal Goal
	err = json.Unmarshal(b, &goal)
	require.NoError(t, err)

	assert.NotZero(t, goal.ID)
	assert.Equal(t, agentID, goal.AgentID)
	assert.Equal(t, "Learn Rust", goal.Name)
	assert.Equal(t, "Read the rust book and write a small CLI", goal.Description)
	assert.Equal(t, GoalStatusActive, goal.Status)
	assert.Equal(t, "high", goal.Priority)
}

func TestGoalManager_UpdateGoalStatus(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	agentID := "agent-1"

	gAny, err := gm.AddGoal(agentID, "Learn Rust", "...", "high")
	require.NoError(t, err)

	b, _ := json.Marshal(gAny)
	var goal Goal
	json.Unmarshal(b, &goal)

	updatedAny, err := gm.UpdateGoalStatus(goal.ID, GoalStatusCompleted)
	require.NoError(t, err)

	b2, _ := json.Marshal(updatedAny)
	var updated Goal
	json.Unmarshal(b2, &updated)

	assert.Equal(t, GoalStatusCompleted, updated.Status)
	assert.True(t, updated.UpdatedAt.After(goal.CreatedAt) || updated.UpdatedAt.Equal(goal.CreatedAt))
}

func TestGoalManager_ListActiveGoals(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	agentID := "agent-1"

	// Add an active goal
	_, err := gm.AddGoal(agentID, "Goal 1", "Desc 1", "low")
	require.NoError(t, err)

	// Add a semantic node with same label but different agent to ensure filtering by agent works
	_, err = gm.AddGoal("agent-2", "Goal 2", "Desc 2", "low")
	require.NoError(t, err)

	// Add an active goal that we will complete
	g3Any, err := gm.AddGoal(agentID, "Goal 3", "Desc 3", "high")
	require.NoError(t, err)

	b, _ := json.Marshal(g3Any)
	var g3 Goal
	json.Unmarshal(b, &g3)
	_, err = gm.UpdateGoalStatus(g3.ID, GoalStatusCompleted)
	require.NoError(t, err)

	// List active goals for agent-1
	activeGoals, err := gm.ListActiveGoals(agentID)
	require.NoError(t, err)

	// Should only see Goal 1
	require.Len(t, activeGoals, 1)

	b4, _ := json.Marshal(activeGoals[0])
	var fetched Goal
	json.Unmarshal(b4, &fetched)

	assert.Equal(t, "Goal 1", fetched.Name)
}

func TestGoalManager_ListGoalsByStatus(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	agentID := "agent-1"

	_, err := gm.AddGoal(agentID, "Goal A", "desc a", "high")
	require.NoError(t, err)
	_, err = gm.AddGoal(agentID, "Goal B", "desc b", "medium")
	require.NoError(t, err)

	active, err := gm.ListGoalsByStatus(agentID, GoalStatusActive)
	require.NoError(t, err)
	assert.Len(t, active, 2)

	inProg, err := gm.ListGoalsByStatus(agentID, GoalStatusInProgress)
	require.NoError(t, err)
	assert.Len(t, inProg, 0)
}

func TestGoalManager_SetGoalResult(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	agentID := "agent-1"

	gAny, err := gm.AddGoal(agentID, "Goal A", "desc", "high")
	require.NoError(t, err)
	goal := gAny.(*Goal)

	result := GoalResult{
		Summary:     "Deployed the stack",
		Artifacts:   []string{"/workspace/goals/goal-1/docker-compose.yml"},
		NextSteps:   []string{"Run ./deploy.sh"},
		CompletedAt: "2026-04-07T15:00:00Z",
	}
	err = gm.SetGoalResult(goal.ID, result)
	require.NoError(t, err)

	updated, err := gm.GetGoalByID(goal.ID)
	require.NoError(t, err)
	assert.NotNil(t, updated.GoalResult)
	assert.Equal(t, "Deployed the stack", updated.GoalResult.Summary)
	assert.Equal(t, []string{"/workspace/goals/goal-1/docker-compose.yml"}, updated.GoalResult.Artifacts)
	assert.Equal(t, []string{"Run ./deploy.sh"}, updated.GoalResult.NextSteps)
}

func TestGoalManager_AddGoal_DefaultPriority(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	gAny, err := gm.AddGoal("agent-1", "Default Priority", "desc", "")
	require.NoError(t, err)
	goal := gAny.(*Goal)
	assert.Equal(t, "medium", goal.Priority)
}

func TestGoalManager_GetGoalByID_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	_, err := gm.GetGoalByID(99999)
	assert.Error(t, err)
}

func TestGoalManager_UpdateGoalStatus_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	_, err := gm.UpdateGoalStatus(99999, GoalStatusCompleted)
	assert.Error(t, err)
}

func TestGoalManager_UpdateGoalResult(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	gAny, err := gm.AddGoal("agent-1", "Result Test", "desc", "high")
	require.NoError(t, err)
	goal := gAny.(*Goal)

	err = gm.UpdateGoalResult(goal.ID, "all done successfully")
	require.NoError(t, err)

	updated, err := gm.GetGoalByID(goal.ID)
	require.NoError(t, err)
	assert.Equal(t, "all done successfully", updated.Result)
}

func TestGoalManager_UpdateGoalResult_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	err := gm.UpdateGoalResult(99999, "nope")
	assert.Error(t, err)
}

func TestGoalManager_UpdateGoalPhase(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	gAny, err := gm.AddGoal("agent-1", "Phase Test", "desc", "medium")
	require.NoError(t, err)
	goal := gAny.(*Goal)

	err = gm.UpdateGoalPhase(goal.ID, GoalPhaseImplement)
	require.NoError(t, err)

	updated, err := gm.GetGoalByID(goal.ID)
	require.NoError(t, err)
	assert.Equal(t, GoalPhaseImplement, updated.Phase)
}

func TestGoalManager_UpdateGoalPhase_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	err := gm.UpdateGoalPhase(99999, GoalPhasePlan)
	assert.Error(t, err)
}

func TestGoalManager_SetAgentCount(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	gAny, err := gm.AddGoal("agent-1", "Agent Count Test", "desc", "low")
	require.NoError(t, err)
	goal := gAny.(*Goal)

	err = gm.SetAgentCount(goal.ID, 3)
	require.NoError(t, err)

	updated, err := gm.GetGoalByID(goal.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, updated.AgentCount)
}

func TestGoalManager_SetAgentCount_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	err := gm.SetAgentCount(99999, 5)
	assert.Error(t, err)
}

func TestGoalManager_SetGoalSpec(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	gAny, err := gm.AddGoal("agent-1", "Spec Test", "desc", "high")
	require.NoError(t, err)
	goal := gAny.(*Goal)

	spec := GoalSpec{
		Requirements:    []string{"API endpoint", "Database migration"},
		SuccessCriteria: []string{"Tests pass", "Endpoint returns 200"},
		Constraints:     []string{"No breaking changes"},
		Context:         "Existing REST API",
	}
	err = gm.SetGoalSpec(goal.ID, spec)
	require.NoError(t, err)

	updated, err := gm.GetGoalByID(goal.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.Spec)
	assert.Equal(t, []string{"API endpoint", "Database migration"}, updated.Spec.Requirements)
	assert.Equal(t, []string{"Tests pass", "Endpoint returns 200"}, updated.Spec.SuccessCriteria)
	assert.Equal(t, []string{"No breaking changes"}, updated.Spec.Constraints)
	assert.Equal(t, "Existing REST API", updated.Spec.Context)
}

func TestGoalManager_SetGoalSpec_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	err := gm.SetGoalSpec(99999, GoalSpec{})
	assert.Error(t, err)
}

func TestGoalManager_ListAllGoals(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	_, err := gm.AddGoal("agent-1", "Goal A", "desc", "high")
	require.NoError(t, err)
	_, err = gm.AddGoal("agent-1", "Goal B", "desc", "low")
	require.NoError(t, err)

	goals, err := gm.ListAllGoals("agent-1")
	require.NoError(t, err)
	assert.Len(t, goals, 2)
}

func TestGoalManager_ListAllGoalsGlobal(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	_, err := gm.AddGoal("agent-1", "Goal A", "desc", "high")
	require.NoError(t, err)
	_, err = gm.AddGoal("agent-2", "Goal B", "desc", "low")
	require.NoError(t, err)

	goals, err := gm.ListAllGoalsGlobal()
	require.NoError(t, err)
	assert.Len(t, goals, 2)
}

func TestGoalManager_DeleteGoal(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	gAny, err := gm.AddGoal("agent-1", "To Delete", "desc", "low")
	require.NoError(t, err)
	goal := gAny.(*Goal)

	err = gm.DeleteGoal(goal.ID)
	require.NoError(t, err)

	_, err = gm.GetGoalByID(goal.ID)
	assert.Error(t, err)
}

func TestGoalManager_DeleteAllGoals(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	_, err := gm.AddGoal("agent-1", "Goal A", "desc", "high")
	require.NoError(t, err)
	_, err = gm.AddGoal("agent-1", "Goal B", "desc", "low")
	require.NoError(t, err)
	_, err = gm.AddGoal("agent-2", "Goal C", "desc", "medium")
	require.NoError(t, err)

	deleted, err := gm.DeleteAllGoals("agent-1")
	require.NoError(t, err)
	assert.Equal(t, 2, deleted)

	// agent-2's goal should still exist
	goals, err := gm.ListAllGoals("agent-2")
	require.NoError(t, err)
	assert.Len(t, goals, 1)
}

func TestGoalManager_SetGoalResult_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	err := gm.SetGoalResult(99999, GoalResult{Summary: "nope"})
	assert.Error(t, err)
}

// TestGoalManager_UpdateStatusPreservesProperties is a regression test:
// UpdateGoalStatus must not wipe properties stored by SetGoalResult/SetGoalSpec.
func TestGoalManager_UpdateStatusPreservesProperties(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	gAny, err := gm.AddGoal("agent-1", "Goal X", "build something", "high")
	require.NoError(t, err)
	goal := gAny.(*Goal)

	// Store a spec (complex nested object)
	err = gm.SetGoalSpec(goal.ID, GoalSpec{
		Requirements:    []string{"req1"},
		SuccessCriteria: []string{"crit1"},
	})
	require.NoError(t, err)

	// Store a goal result (another complex nested object)
	err = gm.SetGoalResult(goal.ID, GoalResult{
		Summary:   "done",
		Artifacts: []string{"file.txt"},
	})
	require.NoError(t, err)

	// Now update status — this must NOT wipe the properties
	_, err = gm.UpdateGoalStatus(goal.ID, GoalStatusCompleted)
	require.NoError(t, err)

	// Verify everything is preserved
	g, err := gm.GetGoalByID(goal.ID)
	require.NoError(t, err)

	assert.Equal(t, GoalStatusCompleted, g.Status)
	assert.Equal(t, "build something", g.Description, "description was wiped by UpdateGoalStatus")
	assert.Equal(t, "high", g.Priority, "priority was wiped by UpdateGoalStatus")
	assert.NotNil(t, g.Spec, "spec was wiped by UpdateGoalStatus")
	assert.Equal(t, []string{"req1"}, g.Spec.Requirements)
	assert.NotNil(t, g.GoalResult, "goal_result was wiped by UpdateGoalStatus")
	assert.Equal(t, "done", g.GoalResult.Summary)
	assert.Equal(t, []string{"file.txt"}, g.GoalResult.Artifacts)
}
