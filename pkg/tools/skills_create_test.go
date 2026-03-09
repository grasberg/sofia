package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateSkillTool_Execute(t *testing.T) {
	tempDir := t.TempDir()
	tool := NewCreateSkillTool(tempDir)

	if tool.Name() != "create_skill" {
		t.Errorf("expected create_skill, got %s", tool.Name())
	}

	args := map[string]any{
		"slug":        "test-skill",
		"name":        "Test Skill",
		"description": "A test skill.",
		"content":     "# Test\nThis is a test.",
	}

	res := tool.Execute(context.Background(), args)
	if !res.Silent {
		t.Fatalf("expected silent result, got error: %v, message: %s", res.IsError, res.ForLLM)
	}

	skillPath := filepath.Join(tempDir, "skills", "test-skill", "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("failed to read created skill: %v", err)
	}

	expected := "---\nname: Test Skill\ndescription: A test skill.\nversion: 1.0\n---\n\n# Test\nThis is a test."
	if string(content) != expected {
		t.Errorf("unexpected content:\ngot:\n%s\nwant:\n%s", string(content), expected)
	}

	// Test duplicate creation fails
	res2 := tool.Execute(context.Background(), args)
	if !strings.Contains(res2.ForLLM, "already exists") {
		t.Errorf("expected error result for duplicate, got error: %v, message: %s", res2.IsError, res2.ForLLM)
	}
}
