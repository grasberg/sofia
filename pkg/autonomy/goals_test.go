package autonomy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/memory"
)

func setupTestDB(t *testing.T) (*memory.MemoryDB, func()) {
	tmpDir, err := os.MkdirTemp("", "sofia_autonomy_test_*")
	require.NoError(t, err)

	db, err := memory.Open(filepath.Join(tmpDir, "memory.db"))
	require.NoError(t, err)

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}
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
