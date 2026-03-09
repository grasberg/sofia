package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateSkillTool_Execute(t *testing.T) {
	tempDir := t.TempDir()

	// First create a skill
	createTool := NewCreateSkillTool(tempDir)
	createTool.Execute(context.Background(), map[string]any{
		"slug":        "test-skill",
		"name":        "Test Skill",
		"description": "Old desc.",
		"content":     "Old content.",
	})

	tool := NewUpdateSkillTool(tempDir)
	if tool.Name() != "update_skill" {
		t.Errorf("expected update_skill, got %s", tool.Name())
	}

	args := map[string]any{
		"slug":        "test-skill",
		"description": "New desc.",
		"content":     "# New\nThis is new content.",
	}

	res := tool.Execute(context.Background(), args)
	if !res.Silent {
		t.Fatalf("expected silent result, got error: %v, message: %s", res.IsError, res.ForLLM)
	}

	skillPath := filepath.Join(tempDir, "skills", "test-skill", "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("failed to read updated skill: %v", err)
	}

	expected := "---\nname: Test Skill\ndescription: New desc.\nversion: 1.0\n---\n\n# New\nThis is new content."
	if string(content) != expected {
		t.Errorf("unexpected content:\ngot:\n%s\nwant:\n%s", string(content), expected)
	}

	// Test missing skill fails
	missingArgs := map[string]any{
		"slug":    "missing",
		"content": "test",
	}
	res2 := tool.Execute(context.Background(), missingArgs)
	if !strings.Contains(res2.ForLLM, "Failed to read skill") {
		t.Errorf("expected error result for missing skill, got error: %v, message: %s", res2.IsError, res2.ForLLM)
	}
}
