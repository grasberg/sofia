package tools

import (
	"context"
	"fmt"
	"time"
)

// TerraformTool wraps the Terraform CLI for infrastructure as code operations.
type TerraformTool struct {
	binaryPath string
	timeout    time.Duration
}

func NewTerraformTool(binaryPath string, timeoutSeconds int) *TerraformTool {
	if binaryPath == "" {
		binaryPath = "terraform"
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	return &TerraformTool{
		binaryPath: binaryPath,
		timeout:    time.Duration(timeoutSeconds) * time.Second,
	}
}

func (t *TerraformTool) Name() string { return "terraform" }
func (t *TerraformTool) Description() string {
	return "Manage infrastructure with Terraform. Commands: init, plan, apply, destroy, state, output, validate, fmt, show, import, providers, workspace. Requires Terraform CLI."
}

func (t *TerraformTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type": "string",
				"enum": []string{
					"init", "plan", "apply", "destroy", "state", "output",
					"validate", "fmt", "show", "import", "providers",
					"workspace", "refresh", "taint", "untaint", "graph",
				},
				"description": "Terraform subcommand",
			},
			"args": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Additional arguments",
			},
			"working_dir": map[string]any{
				"type":        "string",
				"description": "Working directory for Terraform (default: current workspace)",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Command timeout in seconds (default 300)",
			},
		},
		"required": []string{"command"},
	}
}

// Commands that require explicit confirmation (destructive).
var terraformDangerousCommands = map[string]bool{
	"destroy": true,
	"taint":   true,
}

func (t *TerraformTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	command, _ := args["command"].(string)
	if command == "" {
		return ErrorResult("command is required")
	}

	if terraformDangerousCommands[command] {
		return ConfirmationResult(
			fmt.Sprintf("terraform %s is a destructive operation. Proceed?", command))
	}

	var cmdArgs []string
	if raw, ok := args["args"]; ok {
		parsed, err := parseStringArgs(raw)
		if err == nil {
			cmdArgs = parsed
		}
	}

	timeout := t.timeout
	if raw, ok := args["timeout_seconds"]; ok {
		if n, ok := parsePositiveInt(raw); ok {
			timeout = time.Duration(n) * time.Second
		}
	}

	finalArgs := []string{command}

	// Add working directory
	if dir, ok := args["working_dir"].(string); ok && dir != "" {
		finalArgs = append(finalArgs, "-chdir="+dir)
	}

	// Auto-add flags for safety
	switch command {
	case "apply":
		// Require explicit -auto-approve or output plan
		if !hasFlag(cmdArgs, "-auto-approve", "") {
			// Default to plan-only mode
			if !hasFlag(cmdArgs, "-", "") {
				// If no plan file is passed, just show the plan
				finalArgs = []string{"plan"}
				if dir, ok := args["working_dir"].(string); ok && dir != "" {
					finalArgs = append(finalArgs, "-chdir="+dir)
				}
			}
		}
	case "plan":
		// Add -no-color for cleaner output
		if !hasFlag(cmdArgs, "-no-color", "") {
			finalArgs = append(finalArgs, "-no-color")
		}
	case "output":
		// Default to JSON output
		if !hasFlag(cmdArgs, "-json", "") {
			finalArgs = append(finalArgs, "-json")
		}
	case "state":
		// state list is safe, state rm needs confirmation
		if len(cmdArgs) > 0 && cmdArgs[0] == "rm" {
			return ConfirmationResult("terraform state rm is destructive. Proceed?")
		}
	}

	finalArgs = append(finalArgs, cmdArgs...)

	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        finalArgs,
		Timeout:     timeout,
		ToolName:    "terraform",
		InstallHint: "Install Terraform: https://developer.hashicorp.com/terraform/install",
	})
}
