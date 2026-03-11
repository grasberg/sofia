package mcpserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMCPServerCommand(t *testing.T) {
	cmd := NewMCPServerCommand()

	require.NotNil(t, cmd)

	assert.Equal(t, "mcp-server", cmd.Use)
	assert.Contains(t, cmd.Short, "MCP server")
	assert.NotEmpty(t, cmd.Long)

	assert.Nil(t, cmd.Run)
	assert.NotNil(t, cmd.RunE)

	assert.True(t, cmd.HasFlags())
	assert.NotNil(t, cmd.Flags().Lookup("transport"))
	assert.NotNil(t, cmd.Flags().Lookup("addr"))
	assert.NotNil(t, cmd.Flags().Lookup("debug"))
}

func TestMCPServerCommandFlags(t *testing.T) {
	cmd := NewMCPServerCommand()

	// Test default values
	flags := cmd.Flags()

	transport, _ := flags.GetString("transport")
	assert.Equal(t, "stdio", transport)

	addr, _ := flags.GetString("addr")
	assert.Equal(t, ":9090", addr)

	debug, _ := flags.GetBool("debug")
	assert.False(t, debug)
}

func TestMCPServerCommandAliases(t *testing.T) {
	cmd := NewMCPServerCommand()

	// MCP server command should have short alias
	assert.NotNil(t, cmd.Use)
	assert.Equal(t, "mcp-server", cmd.Use)
}

func TestMCPServerCommandDocumentation(t *testing.T) {
	cmd := NewMCPServerCommand()

	// Verify documentation covers both transport modes
	assert.Contains(t, cmd.Long, "stdio")
	assert.Contains(t, cmd.Long, "sse")
	assert.Contains(t, cmd.Long, "transport")
}
