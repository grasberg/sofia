package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"text/template"
	"time"
)

// DynamicToolDef is the persisted definition of a dynamically created tool.
type DynamicToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	// How the tool produces output. Exactly one should be set.
	Command  string `json:"command,omitempty"`
	Template string `json:"template,omitempty"`
}

// DynamicTool implements the Tool interface using a persisted definition.
type DynamicTool struct {
	def        DynamicToolDef
	workingDir string
}

// NewDynamicTool creates a tool from a persisted definition.
func NewDynamicTool(
	def DynamicToolDef, workingDir string,
) *DynamicTool {
	return &DynamicTool{def: def, workingDir: workingDir}
}

func (t *DynamicTool) Name() string        { return t.def.Name }
func (t *DynamicTool) Description() string { return t.def.Description }

func (t *DynamicTool) Parameters() map[string]any {
	if t.def.Parameters != nil {
		return t.def.Parameters
	}
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *DynamicTool) Execute(
	ctx context.Context, args map[string]any,
) *ToolResult {
	if t.def.Template != "" {
		return t.executeTemplate(args)
	}
	if t.def.Command != "" {
		return t.executeCommand(ctx, args)
	}
	return ErrorResult(
		"dynamic tool has no command or template defined",
	)
}

func (t *DynamicTool) executeTemplate(
	args map[string]any,
) *ToolResult {
	tmpl, err := template.New(t.def.Name).Parse(t.def.Template)
	if err != nil {
		return ErrorResult(
			fmt.Sprintf("template parse error: %v", err),
		)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, args); err != nil {
		return ErrorResult(
			fmt.Sprintf("template exec error: %v", err),
		)
	}
	return NewToolResult(buf.String())
}

func (t *DynamicTool) executeCommand(
	ctx context.Context, args map[string]any,
) *ToolResult {
	tmpl, err := template.New("cmd").Parse(t.def.Command)
	if err != nil {
		return ErrorResult(
			fmt.Sprintf("command template parse error: %v", err),
		)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, args); err != nil {
		return ErrorResult(
			fmt.Sprintf("command template exec error: %v", err),
		)
	}
	expanded := buf.String()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx, "sh", "-c", expanded,
	) //#nosec G204 -- command is user-defined via dynamic tool creation
	if t.workingDir != "" {
		cmd.Dir = t.workingDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return ErrorResult(fmt.Sprintf(
			"command failed: %s\nstderr: %s", expanded, errMsg,
		))
	}

	output := stdout.String()
	if len(output) > 10000 {
		output = output[:10000] + "\n... (truncated)"
	}
	return NewToolResult(output)
}
