package autonomy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTriggerManager_AddTrigger(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	tm := NewTriggerManager(db)
	agentID := "agent-1"

	trigAny, err := tm.AddTrigger(agentID, "Alert Me", "User explicitly asks for alert", "Say \"Alerting you now!\"")
	require.NoError(t, err)
	require.NotNil(t, trigAny)

	b, _ := json.Marshal(trigAny)
	var trig ContextTrigger
	err = json.Unmarshal(b, &trig)
	require.NoError(t, err)

	assert.NotZero(t, trig.ID)
	assert.Equal(t, agentID, trig.AgentID)
	assert.Equal(t, "Alert Me", trig.Name)
	assert.Equal(t, "User explicitly asks for alert", trig.Condition)
	assert.Equal(t, "Say \"Alerting you now!\"", trig.Action)
	assert.True(t, trig.IsActive)
}

func TestTriggerManager_ToggleTrigger(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	tm := NewTriggerManager(db)
	agentID := "agent-1"

	trigAny, err := tm.AddTrigger(agentID, "Alert Me", "...", "...")
	require.NoError(t, err)

	b, _ := json.Marshal(trigAny)
	var trig ContextTrigger
	json.Unmarshal(b, &trig)

	updatedAny, err := tm.ToggleTrigger(trig.ID, false)
	require.NoError(t, err)

	b2, _ := json.Marshal(updatedAny)
	var updated ContextTrigger
	json.Unmarshal(b2, &updated)

	assert.False(t, updated.IsActive)
	assert.True(t, updated.UpdatedAt.After(trig.CreatedAt) || updated.UpdatedAt.Equal(trig.CreatedAt))
}

func TestTriggerManager_ListActiveTriggers(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	tm := NewTriggerManager(db)
	agentID := "agent-1"

	// Add an active trigger
	_, err := tm.AddTrigger(agentID, "Trig 1", "Cond 1", "Act 1")
	require.NoError(t, err)

	// Add an active trigger for a different agent
	_, err = tm.AddTrigger("agent-2", "Trig 2", "Cond 2", "Act 2")
	require.NoError(t, err)

	// Add a trigger and then disable it
	t3Any, err := tm.AddTrigger(agentID, "Trig 3", "Cond 3", "Act 3")
	require.NoError(t, err)

	b, _ := json.Marshal(t3Any)
	var t3 ContextTrigger
	json.Unmarshal(b, &t3)

	_, err = tm.ToggleTrigger(t3.ID, false)
	require.NoError(t, err)

	// List active triggers for agent-1
	activeTriggers, err := tm.ListActiveTriggers(agentID)
	require.NoError(t, err)

	// Should only see Trig 1
	require.Len(t, activeTriggers, 1)

	b4, _ := json.Marshal(activeTriggers[0])
	var fetched ContextTrigger
	json.Unmarshal(b4, &fetched)

	assert.Equal(t, "Trig 1", fetched.Name)
}
