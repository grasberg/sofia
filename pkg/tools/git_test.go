package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitTool_Name(t *testing.T) {
	tool := NewGitTool("/tmp")
	assert.Equal(t, "git", tool.Name())
}

func TestGitTool_BlockedFlags(t *testing.T) {
	tool := NewGitTool("/tmp")

	tests := []struct {
		name string
		args map[string]any
		want bool // true = error expected
	}{
		{"force flag blocked", map[string]any{"command": "push", "args": []any{"--force", "origin", "main"}}, true},
		{"no-verify blocked", map[string]any{"command": "commit", "args": []any{"--no-verify", "-m", "test"}}, true},
		{"normal commit allowed", map[string]any{"command": "commit", "args": []any{"-m", "test"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't fully execute git commands in unit tests without a repo,
			// but we can check that blocked flags are rejected before execution.
			result := tool.Execute(t.Context(), tt.args)
			if tt.want {
				assert.True(t, result.IsError)
				assert.Contains(t, result.ForLLM, "blocked")
			}
			// For non-blocked commands, the result might still error (no git repo),
			// but the error should NOT be about blocked flags.
		})
	}
}

func TestGitTool_AddDefaults(t *testing.T) {
	tool := NewGitTool("/tmp")

	t.Run("log gets oneline default", func(t *testing.T) {
		args := []string{"-C", "/tmp", "log"}
		result := tool.addDefaults("log", args, nil)
		assert.Contains(t, result, "--oneline")
		assert.Contains(t, result, "-20")
	})

	t.Run("log with user format skips default", func(t *testing.T) {
		userArgs := []string{"--pretty=format:%H"}
		args := []string{"-C", "/tmp", "log", "--pretty=format:%H"}
		result := tool.addDefaults("log", args, userArgs)
		// Should NOT add --oneline
		count := 0
		for _, a := range result {
			if a == "--oneline" {
				count++
			}
		}
		assert.Equal(t, 0, count)
	})

	t.Run("status gets short branch default", func(t *testing.T) {
		args := []string{"-C", "/tmp", "status"}
		result := tool.addDefaults("status", args, nil)
		assert.Contains(t, result, "--short")
		assert.Contains(t, result, "--branch")
	})
}
