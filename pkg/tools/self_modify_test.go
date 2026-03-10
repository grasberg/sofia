package tools

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSelfModifyTool_Execute_Guardrails(t *testing.T) {
	workspace := t.TempDir()
	tool := NewSelfModifyTool(workspace)

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"config_json", "config.json", "golden config file"},
		{"config_yaml", "/tmp/config.yaml", "golden config file"},
		{"env_file", ".env", "golden config file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{
				"path":    tt.path,
				"content": "new config",
			}
			result := tool.Execute(context.Background(), args)

			if !result.IsError {
				t.Fatalf("Expected error blocking modification to %s", tt.path)
			}
			if !stringsContains(result.ForLLM, tt.expected) {
				t.Errorf("Expected error to contain %q, but got: %s", tt.expected, result.ForLLM)
			}
		})
	}
}

func TestSelfModifyTool_Execute_ConfirmationRequired(t *testing.T) {
	workspace := t.TempDir()
	tool := NewSelfModifyTool(workspace)

	args := map[string]any{
		"path":    "main.go",
		"content": "package main",
	}

	result := tool.Execute(context.Background(), args)

	if !result.ConfirmationRequired {
		t.Fatalf("Expected ConfirmationRequired to be true")
	}

	if result.ConfirmationPrompt == "" {
		t.Errorf("Expected a valid confirmation prompt")
	}

	if !stringsContains(result.ForLLM, "confirm_hash") {
		t.Errorf("Expected ForLLM to mention confirm_hash, got: %s", result.ForLLM)
	}
}

func TestSelfModifyTool_Execute_Success(t *testing.T) {
	workspace := t.TempDir()
	tool := NewSelfModifyTool(workspace)

	// First call to get hash
	args := map[string]any{
		"path":    filepath.Join(workspace, "test_code.go"),
		"content": "package test",
	}
	_ = tool.Execute(context.Background(), args)

	// We can explicitly generate the hash in the test rather than relying on splitting strings
	expectedHash := fmt.Sprintf("%x", sha256.Sum256([]byte(args["path"].(string)+args["content"].(string))))[:16]

	// Add hash and call again
	args["confirm_hash"] = expectedHash
	result2 := tool.Execute(context.Background(), args)

	if result2.IsError {
		t.Fatalf("Execution failed: %s", result2.ForLLM)
	}
	if result2.ConfirmationRequired {
		t.Fatalf("Execution should not require confirmation again")
	}

	// Verify file was written
	content, err := os.ReadFile(filepath.Join(workspace, "test_code.go"))
	if err != nil {
		t.Fatalf("Failed to read modified file: %v", err)
	}
	if string(content) != "package test" {
		t.Errorf("File content mismatch")
	}

	// Verify audit log
	auditLog, err := os.ReadFile(filepath.Join(workspace, "self_modifications.log"))
	if err != nil {
		t.Fatalf("Failed to read audit log: %v", err)
	}
	if !stringsContains(string(auditLog), "test_code.go") {
		t.Errorf("Audit log does not contain modification record")
	}
}

// helper since we didn't import strings for strings.Contains in the test file scope easily without clashing
func stringsContains(s, substr string) bool {
	return stringsIndex(s, substr) >= 0
}

func stringsIndex(s, substr string) int {
	if n := len(substr); n == 0 {
		return 0
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func stringsLastIndex(s, substr string) int {
	if len(substr) == 0 {
		return len(s)
	}
	for i := len(s) - len(substr); i >= 0; i-- {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
