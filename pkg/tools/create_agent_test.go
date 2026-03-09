package tools

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAgentTool_BasicCreation(t *testing.T) {
	created := make(map[string]bool)
	a2aRegistered := make(map[string]bool)

	tool := NewCreateAgentTool(CreateAgentConfig{
		RegisterAgent: func(id, name, purpose string, skills []string, model string) error {
			created[id] = true
			return nil
		},
		RegisterA2A: func(agentID string) {
			a2aRegistered[agentID] = true
		},
	})

	result := tool.Execute(context.Background(), map[string]any{
		"id":      "code-reviewer",
		"name":    "Code Reviewer",
		"purpose": "Reviews code for quality and best practices",
		"skills":  []any{"code", "review", "quality"},
		"model":   "gpt-4",
	})

	require.False(t, result.IsError, "should succeed: %s", result.ForLLM)
	assert.True(t, created["code-reviewer"])
	assert.True(t, a2aRegistered["code-reviewer"])
	assert.Contains(t, result.ForLLM, "code-reviewer")
	assert.Contains(t, result.ForLLM, "Code Reviewer")
}

func TestCreateAgentTool_MissingID(t *testing.T) {
	tool := NewCreateAgentTool(CreateAgentConfig{})
	result := tool.Execute(context.Background(), map[string]any{
		"name":    "Test",
		"purpose": "test",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "'id' is required")
}

func TestCreateAgentTool_MissingName(t *testing.T) {
	tool := NewCreateAgentTool(CreateAgentConfig{})
	result := tool.Execute(context.Background(), map[string]any{
		"id":      "test",
		"purpose": "test",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "'name' is required")
}

func TestCreateAgentTool_MissingPurpose(t *testing.T) {
	tool := NewCreateAgentTool(CreateAgentConfig{})
	result := tool.Execute(context.Background(), map[string]any{
		"id":   "test",
		"name": "Test",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "'purpose' is required")
}

func TestCreateAgentTool_DuplicateID(t *testing.T) {
	tool := NewCreateAgentTool(CreateAgentConfig{
		RegisterAgent: func(id, name, purpose string, skills []string, model string) error {
			return fmt.Errorf("agent %q already registered", id)
		},
	})

	result := tool.Execute(context.Background(), map[string]any{
		"id":      "existing",
		"name":    "Existing",
		"purpose": "already exists",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "already registered")
}

func TestCreateAgentTool_MinimalCreation(t *testing.T) {
	tool := NewCreateAgentTool(CreateAgentConfig{
		RegisterAgent: func(id, name, purpose string, skills []string, model string) error {
			return nil
		},
	})

	result := tool.Execute(context.Background(), map[string]any{
		"id":      "minimal",
		"name":    "Minimal Agent",
		"purpose": "Does minimal things",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "minimal")
}
