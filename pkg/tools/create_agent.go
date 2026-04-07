package tools

import (
	"context"
	"fmt"
	"strings"
)

// CreateAgentConfig holds the dependencies needed by CreateAgentTool.
type CreateAgentConfig struct {
	// RegisterAgent adds a new agent instance to the registry.
	RegisterAgent func(id, name, purpose string, skills []string, model string) error
	// RegisterA2A registers the new agent with the A2A router.
	RegisterA2A func(agentID string)
}

// CreateAgentTool allows the LLM to create new specialized agents at runtime.
type CreateAgentTool struct {
	config CreateAgentConfig
}

// NewCreateAgentTool creates a new CreateAgentTool.
func NewCreateAgentTool(cfg CreateAgentConfig) *CreateAgentTool {
	return &CreateAgentTool{config: cfg}
}

func (t *CreateAgentTool) Name() string { return "create_agent" }
func (t *CreateAgentTool) Description() string {
	return "Create a new specialized agent at runtime. The agent will be available immediately for delegation, orchestration, and direct spawning. Use this when existing agents don't match the task needs."
}

func (t *CreateAgentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "Unique agent ID (lowercase, no spaces, e.g. 'code-reviewer')",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Human-readable agent name (e.g. 'Code Reviewer')",
			},
			"purpose": map[string]any{
				"type":        "string",
				"description": "Purpose/instructions describing what this agent specializes in",
			},
			"skills": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Keywords this agent specializes in (used for delegation scoring)",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Optional model override for this agent (defaults to parent's model)",
			},
		},
		"required": []string{"id", "name", "purpose"},
	}
}

func (t *CreateAgentTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	id, _ := args["id"].(string)
	name, _ := args["name"].(string)
	purpose, _ := args["purpose"].(string)

	if strings.TrimSpace(id) == "" {
		return ErrorResult("'id' is required and must be non-empty")
	}
	if strings.TrimSpace(name) == "" {
		return ErrorResult("'name' is required and must be non-empty")
	}
	if strings.TrimSpace(purpose) == "" {
		return ErrorResult("'purpose' is required and must be non-empty")
	}

	// Parse optional skills
	var skills []string
	if rawSkills, ok := args["skills"]; ok {
		if skillSlice, ok := rawSkills.([]any); ok {
			for _, s := range skillSlice {
				if str, ok := s.(string); ok {
					skills = append(skills, str)
				}
			}
		}
	}

	model, _ := args["model"].(string)

	if t.config.RegisterAgent == nil {
		return ErrorResult("agent creation not configured")
	}

	if err := t.config.RegisterAgent(id, name, purpose, skills, model); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create agent: %v", err))
	}

	// Register with A2A router
	if t.config.RegisterA2A != nil {
		t.config.RegisterA2A(id)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Created agent %q (%s)\n", id, name)
	fmt.Fprintf(&sb, "  Purpose: %s\n", purpose)
	if len(skills) > 0 {
		fmt.Fprintf(&sb, "  Skills: %s\n", strings.Join(skills, ", "))
	}
	if model != "" {
		fmt.Fprintf(&sb, "  Model: %s\n", model)
	}
	sb.WriteString("Agent is now available for delegation, orchestration, and spawning.")

	return SilentResult(sb.String())
}
