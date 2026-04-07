package tools

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// VercelTool wraps the Vercel CLI (vercel) for deploying and managing projects.
type VercelTool struct {
	binaryPath      string
	timeout         time.Duration
	allowedCommands map[string]struct{}
}

// NewVercelTool creates a VercelTool with the given configuration.
func NewVercelTool(binaryPath string, timeoutSeconds int, allowedCommands []string) *VercelTool {
	if strings.TrimSpace(binaryPath) == "" {
		binaryPath = "vercel"
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 120
	}

	allow := make(map[string]struct{}, len(allowedCommands))
	for _, cmd := range allowedCommands {
		normalized := strings.ToLower(strings.TrimSpace(cmd))
		if normalized != "" {
			allow[normalized] = struct{}{}
		}
	}

	return &VercelTool{
		binaryPath:      binaryPath,
		timeout:         time.Duration(timeoutSeconds) * time.Second,
		allowedCommands: allow,
	}
}

func (t *VercelTool) Name() string { return "vercel" }

func (t *VercelTool) Description() string {
	return "Deploy and manage projects on Vercel. Supports deploying sites, listing projects, " +
		"managing domains, checking deployment status, viewing logs, setting environment variables, " +
		"and promoting deployments to production. Requires the Vercel CLI to be installed and authenticated."
}

func (t *VercelTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"args": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
				"description": "Vercel CLI command and arguments. Examples:\n" +
					"  Deploy: [\"deploy\", \"--prod\"]\n" +
					"  List projects: [\"ls\"]\n" +
					"  List deployments: [\"ls\", \"my-project\"]\n" +
					"  Inspect deployment: [\"inspect\", \"<url>\"]\n" +
					"  Set env var: [\"env\", \"add\", \"KEY\", \"production\"]\n" +
					"  List env vars: [\"env\", \"ls\"]\n" +
					"  List domains: [\"domains\", \"ls\"]\n" +
					"  Add domain: [\"domains\", \"add\", \"example.com\"]\n" +
					"  View logs: [\"logs\", \"<url>\"]\n" +
					"  Promote to prod: [\"promote\", \"<url>\"]\n" +
					"  Remove deployment: [\"remove\", \"<url>\", \"--yes\"]\n" +
					"  Pull env/project settings: [\"pull\"]\n" +
					"  Link project: [\"link\"]",
			},
			"project": map[string]any{
				"type":        "string",
				"description": "Optional project name or ID to target",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Optional timeout override in seconds",
				"minimum":     1.0,
			},
		},
		"required": []string{"args"},
	}
}

func (t *VercelTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	commandArgs, err := parseStringArgs(args["args"])
	if err != nil {
		return ErrorResult("vercel: " + err.Error())
	}
	if len(commandArgs) == 0 {
		return ErrorResult("vercel: args array must not be empty")
	}

	topLevel := strings.ToLower(strings.TrimSpace(commandArgs[0]))

	// Block sensitive commands
	blocked := map[string]bool{
		"login": true, "logout": true, "switch": true,
	}
	if blocked[topLevel] {
		return ErrorResult(fmt.Sprintf("vercel: command %q is blocked for security reasons", topLevel))
	}

	// Check allowlist
	if len(t.allowedCommands) > 0 {
		if _, ok := t.allowedCommands[topLevel]; !ok {
			allowed := make([]string, 0, len(t.allowedCommands))
			for k := range t.allowedCommands {
				allowed = append(allowed, k)
			}
			return ErrorResult(fmt.Sprintf("vercel: command %q not in allowed_commands: %v", topLevel, allowed))
		}
	}

	// Build args
	finalArgs := make([]string, 0, len(commandArgs)+4)
	finalArgs = append(finalArgs, commandArgs...)

	// Add --yes to suppress confirmation prompts for non-interactive use
	if topLevel == "deploy" || topLevel == "remove" || topLevel == "link" {
		if !hasFlag(finalArgs, "--yes", "-y") {
			finalArgs = append(finalArgs, "--yes")
		}
	}

	// Add project scope if specified
	if project, _ := args["project"].(string); project != "" {
		if !hasFlag(finalArgs, "--scope", "-S") {
			// For some commands, use positional arg; for others, it's implicit from cwd
		}
	}

	// Timeout
	timeout := t.timeout
	if ts, ok := args["timeout_seconds"].(float64); ok && ts > 0 {
		timeout = time.Duration(ts) * time.Second
	}

	// Execute the CLI command using shared helper
	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        finalArgs,
		Timeout:     timeout,
		ToolName:    "vercel",
		InstallHint: "Install Vercel CLI: npm i -g vercel",
	})
}
