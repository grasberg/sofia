package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDockerTool_Name(t *testing.T) {
	tool := NewDockerTool("", 0)
	assert.Equal(t, "docker", tool.Name())
}

func TestDockerTool_BlockedCommands(t *testing.T) {
	tool := NewDockerTool("", 0)

	blocked := []string{"login", "logout", "push", "secret", "swarm", "plugin"}
	for _, cmd := range blocked {
		result := tool.Execute(t.Context(), map[string]any{"command": cmd})
		assert.True(t, result.IsError, "expected %s to be blocked", cmd)
		assert.Contains(t, result.ForLLM, "blocked")
	}
}

func TestDockerTool_BlockedFlags(t *testing.T) {
	tool := NewDockerTool("", 0)

	result := tool.Execute(t.Context(), map[string]any{
		"command": "run",
		"args":    []any{"--privileged", "alpine"},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "blocked")
}

func TestDockerTool_Validation(t *testing.T) {
	tool := NewDockerTool("", 0)

	result := tool.Execute(t.Context(), map[string]any{})
	assert.True(t, result.IsError)
}
