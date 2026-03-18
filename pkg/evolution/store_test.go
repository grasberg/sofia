package evolution

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/memory"
)

func openTestDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestAgentStore_SaveAndGet(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)

	cfg := EvolutionAgentConfig{
		AgentConfig: config.AgentConfig{
			ID:   "test-agent",
			Name: "Test Agent",
		},
		PurposePrompt: "help with testing",
		ModelID:        "gpt-4o",
	}

	err := store.Save("test-agent", cfg)
	require.NoError(t, err)

	got, status, err := store.Get("test-agent")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "active", status)
	assert.Equal(t, "test-agent", got.ID)
	assert.Equal(t, "Test Agent", got.Name)
	assert.Equal(t, "help with testing", got.PurposePrompt)
	assert.Equal(t, "gpt-4o", got.ModelID)
}

func TestAgentStore_GetNotFound(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)

	got, status, err := store.Get("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.Empty(t, status)
}

func TestAgentStore_ListActive(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)

	// Save two active agents.
	require.NoError(t, store.Save("a1", EvolutionAgentConfig{
		AgentConfig: config.AgentConfig{ID: "a1", Name: "Agent 1"},
	}))
	require.NoError(t, store.Save("a2", EvolutionAgentConfig{
		AgentConfig: config.AgentConfig{ID: "a2", Name: "Agent 2"},
	}))

	// Save one and retire it.
	require.NoError(t, store.Save("a3", EvolutionAgentConfig{
		AgentConfig: config.AgentConfig{ID: "a3", Name: "Agent 3"},
	}))
	require.NoError(t, store.MarkRetired("a3", "underperforming"))

	active, err := store.ListActive()
	require.NoError(t, err)
	assert.Len(t, active, 2)
	assert.Equal(t, "a1", active[0].ID)
	assert.Equal(t, "a2", active[1].ID)
}

func TestAgentStore_ListRetired(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)

	require.NoError(t, store.Save("a1", EvolutionAgentConfig{
		AgentConfig: config.AgentConfig{ID: "a1"},
	}))
	require.NoError(t, store.Save("a2", EvolutionAgentConfig{
		AgentConfig: config.AgentConfig{ID: "a2"},
	}))
	require.NoError(t, store.MarkRetired("a2", "obsolete"))

	retired, err := store.ListRetired()
	require.NoError(t, err)
	assert.Equal(t, []string{"a2"}, retired)
}

func TestAgentStore_MarkRetired(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)

	require.NoError(t, store.Save("agent-x", EvolutionAgentConfig{
		AgentConfig: config.AgentConfig{ID: "agent-x", Name: "Agent X"},
	}))

	// Verify active first.
	_, status, err := store.Get("agent-x")
	require.NoError(t, err)
	assert.Equal(t, "active", status)

	// Mark retired.
	err = store.MarkRetired("agent-x", "low performance")
	require.NoError(t, err)

	_, status, err = store.Get("agent-x")
	require.NoError(t, err)
	assert.Equal(t, "retired", status)
}

func TestAgentStore_MarkRetiredNotFound(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)

	err := store.MarkRetired("ghost", "does not exist")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestAgentStore_SaveOverwrite(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)

	cfg1 := EvolutionAgentConfig{
		AgentConfig: config.AgentConfig{ID: "dup", Name: "Original"},
		ModelID:     "model-v1",
	}
	require.NoError(t, store.Save("dup", cfg1))

	// Overwrite with updated config.
	cfg2 := EvolutionAgentConfig{
		AgentConfig: config.AgentConfig{ID: "dup", Name: "Updated"},
		ModelID:     "model-v2",
	}
	require.NoError(t, store.Save("dup", cfg2))

	got, status, err := store.Get("dup")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "active", status)
	assert.Equal(t, "Updated", got.Name)
	assert.Equal(t, "model-v2", got.ModelID)
}
