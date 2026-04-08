package tools

import (
	"context"
	"fmt"
	"time"
)

// GitTool provides structured git operations with parsed output.
type GitTool struct {
	workspace string
}

func NewGitTool(workspace string) *GitTool {
	return &GitTool{workspace: workspace}
}

func (t *GitTool) Name() string { return "git" }
func (t *GitTool) Description() string {
	return "Run git commands with structured output. Supports: status, diff, log, blame, branch, commit, stash, show, tag, cherry-pick. Operates within the workspace directory."
}

func (t *GitTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Git subcommand: status, diff, log, blame, branch, commit, stash, show, tag, cherry-pick, add, reset, checkout, merge, rebase, fetch, pull, push, remote",
				"enum": []string{
					"status",
					"diff",
					"log",
					"blame",
					"branch",
					"commit",
					"stash",
					"show",
					"tag",
					"cherry-pick",
					"add",
					"reset",
					"checkout",
					"merge",
					"rebase",
					"fetch",
					"pull",
					"push",
					"remote",
				},
			},
			"args": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Additional arguments for the git command",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Command timeout in seconds (default 30)",
			},
		},
		"required": []string{"command"},
	}
}

// blocked commands that could be destructive or leak credentials.
var gitBlockedFlags = map[string]bool{
	"--force":     true,
	"-f":          true,
	"--no-verify": true,
}

// commands that require confirmation for safety.
var gitDangerousCommands = map[string]bool{
	"push":        true,
	"reset":       true,
	"rebase":      true,
	"merge":       true,
	"cherry-pick": true,
}

func (t *GitTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	command, ok := args["command"].(string)
	if !ok || command == "" {
		return ErrorResult("command is required")
	}

	var cmdArgs []string
	if raw, ok := args["args"]; ok {
		parsed, err := parseStringArgs(raw)
		if err == nil {
			cmdArgs = parsed
		}
	}

	timeout := 30 * time.Second
	if raw, ok := args["timeout_seconds"]; ok {
		if n, ok := parsePositiveInt(raw); ok {
			timeout = time.Duration(n) * time.Second
		}
	}

	// Check for blocked flags
	for _, arg := range cmdArgs {
		if gitBlockedFlags[arg] {
			return ErrorResult(fmt.Sprintf("flag %q is blocked for safety. Use the shell tool if you need this.", arg))
		}
	}

	// Build git command
	gitArgs := []string{"-C", t.workspace, command}
	gitArgs = append(gitArgs, cmdArgs...)

	// Add useful defaults per command
	gitArgs = t.addDefaults(command, gitArgs, cmdArgs)

	result := ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  "git",
		Args:        gitArgs,
		Timeout:     timeout,
		ToolName:    "git",
		InstallHint: "Install git: https://git-scm.com/downloads",
	})

	return result
}

func (t *GitTool) addDefaults(command string, gitArgs, userArgs []string) []string {
	switch command {
	case "log":
		// Add sensible defaults if user didn't specify format
		if !hasFlag(userArgs, "--format", "") && !hasFlag(userArgs, "--pretty", "") &&
			!hasFlag(userArgs, "--oneline", "") {
			gitArgs = append(gitArgs, "--oneline", "-20")
		}
	case "diff":
		// Add stat by default for overview, unless user wants full diff
		if !hasFlag(userArgs, "--stat", "") && !hasFlag(userArgs, "--no-stat", "") && len(userArgs) == 0 {
			gitArgs = append(gitArgs, "--stat")
		}
	case "branch":
		// Show all branches with last commit info
		if !hasFlag(userArgs, "-v", "--verbose") && !hasFlag(userArgs, "-d", "-D") &&
			!hasFlag(userArgs, "-m", "-M") && len(userArgs) == 0 {
			gitArgs = append(gitArgs, "-v")
		}
	case "status":
		if !hasFlag(userArgs, "-s", "--short") && !hasFlag(userArgs, "--porcelain", "") {
			gitArgs = append(gitArgs, "--short", "--branch")
		}
	}
	return gitArgs
}
