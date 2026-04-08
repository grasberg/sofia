package tools

import (
	"context"
	"fmt"
	"time"
)

// SemgrepTool wraps Semgrep for static analysis and security scanning.
type SemgrepTool struct {
	binaryPath string
	timeout    time.Duration
}

func NewSemgrepTool(binaryPath string, timeoutSeconds int) *SemgrepTool {
	if binaryPath == "" {
		binaryPath = "semgrep"
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 120
	}
	return &SemgrepTool{
		binaryPath: binaryPath,
		timeout:    time.Duration(timeoutSeconds) * time.Second,
	}
}

func (t *SemgrepTool) Name() string { return "semgrep" }
func (t *SemgrepTool) Description() string {
	return "Run static analysis and security scanning with Semgrep. Scan code for vulnerabilities, bugs, and anti-patterns using predefined rulesets or custom patterns. Requires Semgrep CLI."
}

func (t *SemgrepTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"scan", "pattern"},
				"description": "scan: run ruleset scan, pattern: search with a pattern expression",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File or directory to scan (default: current workspace)",
			},
			"config": map[string]any{
				"type":        "string",
				"description": "Semgrep config/ruleset (e.g., auto, p/security-audit, p/owasp-top-ten, p/ci)",
			},
			"pattern": map[string]any{
				"type":        "string",
				"description": "Semgrep pattern expression (for pattern action). E.g., '$X == $X' to find tautologies.",
			},
			"language": map[string]any{
				"type":        "string",
				"description": "Language for pattern search (e.g., go, python, javascript, typescript, java, c, cpp, ruby)",
			},
			"severity": map[string]any{
				"type":        "string",
				"enum":        []string{"ERROR", "WARNING", "INFO"},
				"description": "Minimum severity level to report",
			},
			"json_output": map[string]any{
				"type":        "boolean",
				"description": "Output in JSON format (default false, uses text)",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Command timeout (default 120)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *SemgrepTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)
	if action == "" {
		return ErrorResult("action is required")
	}

	timeout := t.timeout
	if raw, ok := args["timeout_seconds"]; ok {
		if n, ok := parsePositiveInt(raw); ok {
			timeout = time.Duration(n) * time.Second
		}
	}

	switch action {
	case "scan":
		return t.scan(ctx, args, timeout)
	case "pattern":
		return t.patternSearch(ctx, args, timeout)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

func (t *SemgrepTool) scan(ctx context.Context, args map[string]any, timeout time.Duration) *ToolResult {
	sgArgs := []string{"scan"}

	config := "auto"
	if c, ok := args["config"].(string); ok && c != "" {
		config = c
	}
	sgArgs = append(sgArgs, "--config", config)

	if sev, ok := args["severity"].(string); ok && sev != "" {
		sgArgs = append(sgArgs, "--severity", sev)
	}

	if jsonOut, ok := args["json_output"].(bool); ok && jsonOut {
		sgArgs = append(sgArgs, "--json")
	}

	sgArgs = append(sgArgs, "--no-git-ignore")

	if path, ok := args["path"].(string); ok && path != "" {
		sgArgs = append(sgArgs, path)
	} else {
		sgArgs = append(sgArgs, ".")
	}

	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        sgArgs,
		Timeout:     timeout,
		ToolName:    "semgrep",
		InstallHint: "Install Semgrep: pip install semgrep",
	})
}

func (t *SemgrepTool) patternSearch(ctx context.Context, args map[string]any, timeout time.Duration) *ToolResult {
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return ErrorResult("pattern is required for pattern action")
	}

	lang, ok := args["language"].(string)
	if !ok || lang == "" {
		return ErrorResult("language is required for pattern action")
	}

	sgArgs := []string{"scan", "--pattern", pattern, "--lang", lang}

	if jsonOut, ok := args["json_output"].(bool); ok && jsonOut {
		sgArgs = append(sgArgs, "--json")
	}

	if path, ok := args["path"].(string); ok && path != "" {
		sgArgs = append(sgArgs, path)
	} else {
		sgArgs = append(sgArgs, ".")
	}

	return ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  t.binaryPath,
		Args:        sgArgs,
		Timeout:     timeout,
		ToolName:    "semgrep",
		InstallHint: "Install Semgrep: pip install semgrep",
	})
}
