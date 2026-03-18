package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/grasberg/sofia/pkg/templates"
)

// TemplateTool provides LLM access to the prompt templates library.
// Supported actions: list, show, run.
type TemplateTool struct {
	manager *templates.TemplateManager
}

// NewTemplateTool creates a new TemplateTool backed by the given TemplateManager.
func NewTemplateTool(manager *templates.TemplateManager) *TemplateTool {
	return &TemplateTool{manager: manager}
}

func (t *TemplateTool) Name() string { return "template" }

func (t *TemplateTool) Description() string {
	return "Manage and render prompt templates. Actions: " +
		"list (show available templates), " +
		"show <name> (display template content and metadata), " +
		"run <name> var1=val1 var2=val2 (render a template with variables)."
}

func (t *TemplateTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"list", "show", "run"},
				"description": "The action to perform",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Template name (required for show and run)",
			},
			"variables": map[string]any{
				"type":        "object",
				"description": "Key-value pairs for template variables (required for run)",
				"additionalProperties": map[string]any{
					"type": "string",
				},
			},
		},
		"required": []string{"action"},
	}
}

func (t *TemplateTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)

	switch action {
	case "list":
		return t.executeList()
	case "show":
		return t.executeShow(args)
	case "run":
		return t.executeRun(args)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %q; use list, show, or run", action))
	}
}

func (t *TemplateTool) executeList() *ToolResult {
	tpls := t.manager.List()
	if len(tpls) == 0 {
		return SilentResult("No templates available.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Available templates (%d):\n\n", len(tpls)))
	for _, tpl := range tpls {
		sb.WriteString(fmt.Sprintf("- **%s**", tpl.Name))
		if tpl.Description != "" {
			sb.WriteString(fmt.Sprintf(": %s", tpl.Description))
		}
		if len(tpl.Tags) > 0 {
			sb.WriteString(fmt.Sprintf(" [%s]", strings.Join(tpl.Tags, ", ")))
		}
		sb.WriteString("\n")
	}
	return SilentResult(sb.String())
}

func (t *TemplateTool) executeShow(args map[string]any) *ToolResult {
	name, _ := args["name"].(string)
	if name == "" {
		return ErrorResult("name is required for show action")
	}

	tpl, ok := t.manager.Get(name)
	if !ok {
		return ErrorResult(fmt.Sprintf("template %q not found", name))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Template: %s\n", tpl.Name))
	if tpl.Description != "" {
		sb.WriteString(fmt.Sprintf("Description: %s\n", tpl.Description))
	}
	if len(tpl.Variables) > 0 {
		sb.WriteString(fmt.Sprintf("Variables: %s\n", strings.Join(tpl.Variables, ", ")))
	}
	if len(tpl.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(tpl.Tags, ", ")))
	}
	sb.WriteString(fmt.Sprintf("\n---\n%s", tpl.Content))
	return SilentResult(sb.String())
}

func (t *TemplateTool) executeRun(args map[string]any) *ToolResult {
	name, _ := args["name"].(string)
	if name == "" {
		return ErrorResult("name is required for run action")
	}

	vars := make(map[string]string)
	if rawVars, ok := args["variables"]; ok {
		switch v := rawVars.(type) {
		case map[string]any:
			for k, val := range v {
				vars[k] = fmt.Sprintf("%v", val)
			}
		case map[string]string:
			vars = v
		}
	}

	rendered, err := t.manager.Render(name, vars)
	if err != nil {
		return ErrorResult(fmt.Sprintf("render failed: %s", err))
	}
	return SilentResult(rendered)
}
