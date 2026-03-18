package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoleGetBuiltinRole_Valid(t *testing.T) {
	role, ok := GetBuiltinRole("researcher")
	require.True(t, ok)
	assert.Equal(t, "Researcher", role.Name)
	assert.NotEmpty(t, role.Description)
	assert.NotEmpty(t, role.SystemPrompt)
	assert.InDelta(t, 0.3, role.Temperature, 0.001)
}

func TestRoleGetBuiltinRole_Invalid(t *testing.T) {
	_, ok := GetBuiltinRole("nonexistent")
	assert.False(t, ok)
}

func TestRoleListBuiltinRoles_Count(t *testing.T) {
	roles := ListBuiltinRoles()
	assert.Len(t, roles, 6)
}

func TestRoleListBuiltinRoles_Sorted(t *testing.T) {
	roles := ListBuiltinRoles()
	// ListBuiltinRoles sorts by map key. Verify the expected key order is
	// reflected in the returned slice by checking against the known names
	// in alphabetical key order: analyst, assistant, developer, devops, researcher, writer.
	expected := []string{"Analyst", "Personal Assistant", "Developer", "DevOps", "Researcher", "Writer"}
	require.Len(t, roles, len(expected))
	for i, name := range expected {
		assert.Equal(t, name, roles[i].Name, "role at index %d", i)
	}
}

func TestRoleBuiltinRolesHaveRequiredFields(t *testing.T) {
	for key, role := range BuiltinRoles {
		t.Run(key, func(t *testing.T) {
			assert.NotEmpty(t, role.Name, "role %q must have a non-empty Name", key)
			assert.NotEmpty(t, role.Description, "role %q must have a non-empty Description", key)
			assert.NotEmpty(t, role.SystemPrompt, "role %q must have a non-empty SystemPrompt", key)
		})
	}
}
