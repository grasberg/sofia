package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GitHubCLITool wraps the GitHub CLI (gh) for repository, issue, PR, and workflow management.
type GitHubCLITool struct {
	binaryPath      string
	timeout         time.Duration
	allowedCommands map[string]struct{}
}

// NewGitHubCLITool creates a new GitHub CLI tool.
func NewGitHubCLITool(binaryPath string, timeoutSeconds int, allowedCommands []string) *GitHubCLITool {
	if strings.TrimSpace(binaryPath) == "" {
		binaryPath = "gh"
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60
	}

	allow := make(map[string]struct{}, len(allowedCommands))
	for _, cmd := range allowedCommands {
		normalized := strings.ToLower(strings.TrimSpace(cmd))
		if normalized != "" {
			allow[normalized] = struct{}{}
		}
	}

	return &GitHubCLITool{
		binaryPath:      binaryPath,
		timeout:         time.Duration(timeoutSeconds) * time.Second,
		allowedCommands: allow,
	}
}

func (t *GitHubCLITool) Name() string {
	return "github_cli"
}

func (t *GitHubCLITool) Description() string {
	return "Run GitHub CLI (gh) commands to manage repositories, issues, pull requests, releases, " +
		"workflows, gists, and query the GitHub API. Requires gh to be installed and authenticated."
}

func (t *GitHubCLITool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"args": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "gh command args, e.g. [\"pr\",\"list\",\"--state\",\"open\"] or [\"issue\",\"create\",\"--title\",\"Bug\",\"--body\",\"Details\"]",
			},
			"repo": map[string]any{
				"type":        "string",
				"description": "Optional owner/repo to target (maps to gh --repo). E.g. \"octocat/hello-world\"",
			},
			"json": map[string]any{
				"type":        "boolean",
				"description": "Request JSON output where supported (default true)",
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

func (t *GitHubCLITool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	commandArgs, err := parseStringArgs(args["args"])
	if err != nil {
		return ErrorResult(err.Error())
	}

	topLevel := strings.ToLower(strings.TrimSpace(commandArgs[0]))
	if strings.HasPrefix(topLevel, "-") {
		return ErrorResult(
			"args must start with a gh top-level command (e.g. pr, issue, repo, run, api, release, gist)",
		)
	}

	// Block dangerous commands that could leak credentials or modify auth
	blocked := map[string]bool{
		"auth": true, "config": true, "ssh-key": true, "gpg-key": true,
		"secret": true, "variable": true, "environment": true,
	}
	if blocked[topLevel] {
		return ErrorResult(fmt.Sprintf("command %q is blocked for security reasons", topLevel))
	}

	if len(t.allowedCommands) > 0 {
		if _, ok := t.allowedCommands[topLevel]; !ok {
			return ErrorResult(fmt.Sprintf("command %q is not in allowed_commands", topLevel))
		}
	}

	jsonEnabled := true
	if raw, ok := args["json"]; ok {
		value, ok := raw.(bool)
		if !ok {
			return ErrorResult("json must be a boolean")
		}
		jsonEnabled = value
	}

	timeout := t.timeout
	if raw, ok := args["timeout_seconds"]; ok {
		seconds, ok := parsePositiveInt(raw)
		if !ok {
			return ErrorResult("timeout_seconds must be a positive integer")
		}
		timeout = time.Duration(seconds) * time.Second
	}

	repo := ""
	if raw, ok := args["repo"]; ok {
		s, ok := raw.(string)
		if !ok {
			return ErrorResult("repo must be a string")
		}
		repo = strings.TrimSpace(s)
	}

	finalArgs := make([]string, 0, len(commandArgs)+4)

	// Inject --repo if provided and not already present
	if repo != "" && !hasFlag(commandArgs, "--repo", "-R") {
		finalArgs = append(finalArgs, "--repo", repo)
	}

	finalArgs = append(finalArgs, commandArgs...)

	// Append --json for list-type commands that support it
	if jsonEnabled && supportsJSON(commandArgs) && !hasFlag(commandArgs, "--json", "") {
		// For list/view commands, request common fields
		fields := jsonFieldsFor(topLevel, commandArgs)
		if fields != "" {
			finalArgs = append(finalArgs, "--json", fields)
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, t.binaryPath, finalArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	output := strings.TrimSpace(stdout.String())
	if stderr.Len() > 0 {
		errOut := strings.TrimSpace(stderr.String())
		if errOut != "" {
			if output != "" {
				output += "\n\n"
			}
			output += "STDERR:\n" + errOut
		}
	}
	if output == "" {
		output = "(no output)"
	}

	const maxLen = 12000
	if len(output) > maxLen {
		output = output[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(output)-maxLen)
	}

	if err != nil {
		if isBinaryNotFound(err) {
			return ErrorResult(
				fmt.Sprintf("gh binary not found at %q. Install GitHub CLI: brew install gh", t.binaryPath),
			)
		}
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			msg := fmt.Sprintf("gh command timed out after %v", timeout)
			if output != "(no output)" {
				msg += "\n\n" + output
			}
			return &ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
		}
		output += fmt.Sprintf("\n\nExit error: %v", err)
		return &ToolResult{ForLLM: output, ForUser: output, IsError: true}
	}

	return &ToolResult{ForLLM: output, ForUser: output, IsError: false}
}

func hasFlag(args []string, long, short string) bool {
	for _, a := range args {
		if a == long || (short != "" && a == short) || strings.HasPrefix(a, long+"=") {
			return true
		}
	}
	return false
}

// supportsJSON returns true if the command likely supports --json output.
func supportsJSON(args []string) bool {
	if len(args) < 2 {
		return false
	}
	cmd := strings.ToLower(args[0])
	sub := strings.ToLower(args[1])

	jsonCmds := map[string]map[string]bool{
		"pr":      {"list": true, "view": true, "status": true, "checks": true},
		"issue":   {"list": true, "view": true, "status": true},
		"repo":    {"list": true, "view": true},
		"run":     {"list": true, "view": true},
		"release": {"list": true, "view": true},
	}

	if subs, ok := jsonCmds[cmd]; ok {
		return subs[sub]
	}
	return false
}

// jsonFieldsFor returns sensible default --json fields for common commands.
func jsonFieldsFor(cmd string, args []string) string {
	if len(args) < 2 {
		return ""
	}
	sub := strings.ToLower(args[1])

	switch cmd {
	case "pr":
		switch sub {
		case "list":
			return "number,title,state,author,createdAt,url,headRefName"
		case "view":
			return "number,title,state,body,author,createdAt,url,headRefName,additions,deletions"
		case "checks":
			return "name,state,conclusion,startedAt,completedAt"
		}
	case "issue":
		switch sub {
		case "list":
			return "number,title,state,author,createdAt,url,labels"
		case "view":
			return "number,title,state,body,author,createdAt,url,labels,assignees"
		}
	case "run":
		switch sub {
		case "list":
			return "databaseId,displayTitle,status,conclusion,event,createdAt"
		case "view":
			return "databaseId,displayTitle,status,conclusion,event,createdAt,url"
		}
	case "repo":
		switch sub {
		case "list":
			return "name,description,isPrivate,updatedAt,url"
		case "view":
			return "name,description,isPrivate,stargazerCount,forkCount,url,defaultBranchRef"
		}
	case "release":
		switch sub {
		case "list":
			return "tagName,name,isPrerelease,isDraft,publishedAt"
		case "view":
			return "tagName,name,body,isPrerelease,isDraft,publishedAt,url"
		}
	}
	return ""
}
