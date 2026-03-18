package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersonaManager_Switch(t *testing.T) {
	pm := NewPersonaManager(map[string]*Persona{
		"coder": {Name: "coder", SystemPrompt: "You are a coding assistant.", Description: "Coding persona"},
		"writer": {Name: "writer", SystemPrompt: "You are a creative writer.", Description: "Writing persona"},
	})

	err := pm.Switch("session-1", "coder")
	require.NoError(t, err)

	p := pm.GetActive("session-1")
	require.NotNil(t, p)
	assert.Equal(t, "coder", p.Name)
	assert.Equal(t, "You are a coding assistant.", p.SystemPrompt)
}

func TestPersonaManager_SwitchInvalidPersona(t *testing.T) {
	pm := NewPersonaManager(map[string]*Persona{
		"coder": {Name: "coder", SystemPrompt: "You are a coding assistant."},
	})

	err := pm.Switch("session-1", "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPersonaManager_GetActiveNoOverride(t *testing.T) {
	pm := NewPersonaManager(map[string]*Persona{
		"coder": {Name: "coder", SystemPrompt: "You are a coding assistant."},
	})

	p := pm.GetActive("session-1")
	assert.Nil(t, p)
}

func TestPersonaManager_Clear(t *testing.T) {
	pm := NewPersonaManager(map[string]*Persona{
		"coder": {Name: "coder", SystemPrompt: "You are a coding assistant."},
	})

	require.NoError(t, pm.Switch("session-1", "coder"))
	require.NotNil(t, pm.GetActive("session-1"))

	pm.Clear("session-1")
	assert.Nil(t, pm.GetActive("session-1"))
}

func TestPersonaManager_List(t *testing.T) {
	pm := NewPersonaManager(map[string]*Persona{
		"writer": {Name: "writer"},
		"coder":  {Name: "coder"},
		"admin":  {Name: "admin"},
	})

	names := pm.List()
	assert.Equal(t, []string{"admin", "coder", "writer"}, names)
}

func TestPersonaManager_ListEmpty(t *testing.T) {
	pm := NewPersonaManager(nil)
	names := pm.List()
	assert.Empty(t, names)
}

func TestPersonaManager_SwitchOverwrite(t *testing.T) {
	pm := NewPersonaManager(map[string]*Persona{
		"coder":  {Name: "coder", SystemPrompt: "coding"},
		"writer": {Name: "writer", SystemPrompt: "writing"},
	})

	require.NoError(t, pm.Switch("session-1", "coder"))
	assert.Equal(t, "coder", pm.GetActive("session-1").Name)

	require.NoError(t, pm.Switch("session-1", "writer"))
	assert.Equal(t, "writer", pm.GetActive("session-1").Name)
}

func TestPersonaManager_IndependentSessions(t *testing.T) {
	pm := NewPersonaManager(map[string]*Persona{
		"coder":  {Name: "coder"},
		"writer": {Name: "writer"},
	})

	require.NoError(t, pm.Switch("session-1", "coder"))
	require.NoError(t, pm.Switch("session-2", "writer"))

	assert.Equal(t, "coder", pm.GetActive("session-1").Name)
	assert.Equal(t, "writer", pm.GetActive("session-2").Name)

	pm.Clear("session-1")
	assert.Nil(t, pm.GetActive("session-1"))
	assert.Equal(t, "writer", pm.GetActive("session-2").Name)
}

func TestPersonaManager_WithModelOverride(t *testing.T) {
	pm := NewPersonaManager(map[string]*Persona{
		"fast": {Name: "fast", SystemPrompt: "Be fast.", Model: "gpt-4o-mini"},
	})

	require.NoError(t, pm.Switch("session-1", "fast"))
	p := pm.GetActive("session-1")
	require.NotNil(t, p)
	assert.Equal(t, "gpt-4o-mini", p.Model)
}

func TestPersonaManager_WithAllowedTools(t *testing.T) {
	pm := NewPersonaManager(map[string]*Persona{
		"safe": {
			Name:         "safe",
			SystemPrompt: "Be safe.",
			AllowedTools: []string{"read_file", "list_dir"},
		},
	})

	require.NoError(t, pm.Switch("session-1", "safe"))
	p := pm.GetActive("session-1")
	require.NotNil(t, p)
	assert.Equal(t, []string{"read_file", "list_dir"}, p.AllowedTools)
}
