package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/grasberg/sofia/pkg/fileutil"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/utils"
)

var reFrontmatter = regexp.MustCompile(`(?s)^---(?:\r\n|\n|\r)(.*?)(?:\r\n|\n|\r)---`)

// UpdateSkillTool allows the agent to update and refine existing skills.
type UpdateSkillTool struct {
	workspace string
}

// NewUpdateSkillTool creates a new UpdateSkillTool.
func NewUpdateSkillTool(workspace string) *UpdateSkillTool {
	return &UpdateSkillTool{workspace: workspace}
}

func (t *UpdateSkillTool) Name() string { return "update_skill" }

func (t *UpdateSkillTool) Description() string {
	return "Update an existing agent skill. Rewrites the skill instructions while preserving the YAML frontmatter. Use this to refine a skill after learning from mistakes or discovering new patterns."
}

func (t *UpdateSkillTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"slug": map[string]any{
				"type":        "string",
				"description": "Unique identifier for the skill (e.g., 'go-debugging').",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Optional updated short summary of the skill.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The updated Markdown body of the skill. Include principles, code examples, patterns, and anti-patterns. Do NOT include the YAML frontmatter here.",
			},
		},
		"required": []string{"slug", "content"},
	}
}

func (t *UpdateSkillTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	slug, _ := args["slug"].(string)
	desc, _ := args["description"].(string)
	content, _ := args["content"].(string)

	if err := utils.ValidateSkillIdentifier(slug); err != nil {
		return ErrorResult(fmt.Sprintf("Invalid slug %q: %s", slug, err.Error()))
	}

	if content == "" {
		return ErrorResult("content is required")
	}

	skillsDir := filepath.Join(t.workspace, "skills")
	targetDir := filepath.Join(skillsDir, slug)
	skillPath := filepath.Join(targetDir, "SKILL.md")

	existingContent, err := os.ReadFile(skillPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to read skill %q. Does it exist? Error: %v", slug, err))
	}

	existingStr := string(existingContent)
	var frontmatter string

	match := reFrontmatter.FindStringSubmatch(existingStr)
	if len(match) > 1 {
		frontmatter = match[1]
	} else {
		// No frontmatter found, we should probably just create a basic one.
		frontmatter = "name: " + slug + "\ndescription: " + slug + "\nversion: 1.0"
	}

	// Update description in frontmatter if provided
	if desc != "" {
		cleanDesc := strings.ReplaceAll(desc, "\n", " ")
		descRegex := regexp.MustCompile(`(?m)^description:.*$`)
		if descRegex.MatchString(frontmatter) {
			frontmatter = descRegex.ReplaceAllString(frontmatter, "description: "+cleanDesc)
		} else {
			frontmatter += "\ndescription: " + cleanDesc
		}
	}

	// Build new file content
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(strings.TrimSpace(frontmatter))
	sb.WriteString("\n---\n\n")
	sb.WriteString(content)

	if err := fileutil.WriteFileAtomic(skillPath, []byte(sb.String()), 0644); err != nil {
		logger.ErrorCF("tool", "Failed to update skill file", map[string]any{"error": err.Error(), "path": skillPath})
		return ErrorResult(fmt.Sprintf("Failed to update skill file: %v", err))
	}

	return SilentResult(fmt.Sprintf("Successfully updated skill %q at %s\nThe new knowledge is now available for future tasks.", slug, skillPath))
}
