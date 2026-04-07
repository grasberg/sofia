package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grasberg/sofia/pkg/fileutil"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/utils"
)

// CreateSkillTool allows the agent to synthesize its experiences into a new reusable skill.
type CreateSkillTool struct {
	workspace string
}

// NewCreateSkillTool creates a new CreateSkillTool.
func NewCreateSkillTool(workspace string) *CreateSkillTool {
	return &CreateSkillTool{workspace: workspace}
}

func (t *CreateSkillTool) Name() string { return "create_skill" }

func (t *CreateSkillTool) Description() string {
	return "Create a new agent skill from successful experiences or patterns. Writes a new SKILL.md file with YAML frontmatter."
}

func (t *CreateSkillTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"slug": map[string]any{
				"type":        "string",
				"description": "Unique identifier for the skill (e.g., 'go-debugging', 'react-patterns'). Only alphanumeric and hyphens.",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Human-readable name for the skill (e.g., 'Go Debugging Mastery').",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Short summary of what this skill does and when to use it.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The Markdown body of the skill. Include principles, code examples, patterns, and anti-patterns. Do NOT include the YAML frontmatter here.",
			},
		},
		"required": []string{"slug", "name", "description", "content"},
	}
}

func (t *CreateSkillTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	slug, _ := args["slug"].(string)
	name, _ := args["name"].(string)
	desc, _ := args["description"].(string)
	content, _ := args["content"].(string)

	if err := utils.ValidateSkillIdentifier(slug); err != nil {
		return ErrorResult(fmt.Sprintf("Invalid slug %q: %s", slug, err.Error()))
	}

	if name == "" || desc == "" || content == "" {
		return ErrorResult("name, description, and content are all required")
	}

	skillsDir := filepath.Join(t.workspace, "skills")
	targetDir := filepath.Join(skillsDir, slug)

	if _, err := os.Stat(targetDir); err == nil {
		return ErrorResult(fmt.Sprintf("Skill %q already exists. Use update_skill to modify it.", slug))
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return ErrorResult(fmt.Sprintf("Failed to create skill directory: %v", err))
	}

	// build frontmatter
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "name: %s\n", strings.ReplaceAll(name, "\n", " "))
	fmt.Fprintf(&sb, "description: %s\n", strings.ReplaceAll(desc, "\n", " "))
	sb.WriteString("version: 1.0\n")
	sb.WriteString("---\n\n")
	sb.WriteString(content)

	skillPath := filepath.Join(targetDir, "SKILL.md")
	if err := fileutil.WriteFileAtomic(skillPath, []byte(sb.String()), 0o644); err != nil {
		os.RemoveAll(targetDir) // rollback
		logger.ErrorCF("tool", "Failed to write skill file", map[string]any{"error": err.Error(), "path": skillPath})
		return ErrorResult(fmt.Sprintf("Failed to write skill file: %v", err))
	}

	return SilentResult(
		fmt.Sprintf(
			"Successfully created new skill %q at %s\nYou can now use this skill in future tasks.",
			slug,
			skillPath,
		),
	)
}
