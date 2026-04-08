package tools

import (
	"context"
	"fmt"
	"time"
)

// KubectlTool wraps kubectl for Kubernetes cluster management.
type KubectlTool struct {
	binaryPath string
	timeout    time.Duration
}

func NewKubectlTool(binaryPath string, timeoutSeconds int) *KubectlTool {
	if binaryPath == "" {
		binaryPath = "kubectl"
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60
	}
	return &KubectlTool{
		binaryPath: binaryPath,
		timeout:    time.Duration(timeoutSeconds) * time.Second,
	}
}

func (t *KubectlTool) Name() string { return "kubectl" }
func (t *KubectlTool) Description() string {
	return "Manage Kubernetes resources. Commands: get, describe, apply, delete, logs, exec, scale, rollout, config, top, port-forward. Requires kubectl and cluster access."
}

func (t *KubectlTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type": "string",
				"enum": []string{
					"get", "describe", "apply", "delete", "logs", "exec",
					"scale", "rollout", "config", "top", "port-forward",
					"create", "edit", "patch", "label", "annotate",
					"api-resources", "explain", "auth",
				},
				"description": "kubectl subcommand",
			},
			"args": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Additional arguments",
			},
			"namespace": map[string]any{
				"type":        "string",
				"description": "Kubernetes namespace (maps to -n flag)",
			},
			"output_format": map[string]any{
				"type":        "string",
				"description": "Output format (json, yaml, wide, name)",
				"enum":        []string{"json", "yaml", "wide", "name"},
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Command timeout in seconds (default 60)",
			},
		},
		"required": []string{"command"},
	}
}

var kubectlBlockedCommands = map[string]bool{
	"proxy":       true,
	"certificate": true,
}

func (t *KubectlTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	command, _ := args["command"].(string)
	if command == "" {
		return ErrorResult("command is required")
	}

	if kubectlBlockedCommands[command] {
		return ErrorResult(fmt.Sprintf("command %q is blocked for security reasons", command))
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
	finalArgs = append(finalArgs, cmdArgs...)

	// Add namespace if provided
	if ns, ok := args["namespace"].(string); ok && ns != "" && !hasFlag(cmdArgs, "-n", "--namespace") {
		finalArgs = append(finalArgs, "-n", ns)
	}

	// Add output format
	if format, ok := args["output_format"].(string); ok && format != "" && !hasFlag(cmdArgs, "-o", "--output") {
		finalArgs = append(finalArgs, "-o", format)
	} else if command == "get" && !hasFlag(cmdArgs, "-o", "--output") {
		// Default to wide for get commands
		finalArgs = append(finalArgs, "-o", "wide")
	}

	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        finalArgs,
		Timeout:     timeout,
		ToolName:    "kubectl",
		InstallHint: "Install kubectl: https://kubernetes.io/docs/tasks/tools/",
	})
}
