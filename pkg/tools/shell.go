package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
)

const (
	maxTimeoutRetries = 2
	timeoutMultiplier = 2
)

// commandNotFoundPatterns matches common "command not found" messages across shells/platforms.
var commandNotFoundPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)command not found`),
	regexp.MustCompile(`(?i)not recognized as an internal or external command`),
	regexp.MustCompile(`(?i)is not recognized`),
	regexp.MustCompile(`(?i)no such file or directory`),
	regexp.MustCompile(`(?i)cannot find the path`),
	regexp.MustCompile(`(?i)program not found`),
}

// isCommandNotFound returns true if stderr looks like a "command not found" error.
func isCommandNotFound(stderr string) bool {
	for _, p := range commandNotFoundPatterns {
		if p.MatchString(stderr) {
			return true
		}
	}
	return false
}

type ExecTool struct {
	workingDir          string
	timeout             time.Duration
	denyPatterns        []*regexp.Regexp
	allowPatterns       []*regexp.Regexp
	confirmPatterns     []*regexp.Regexp
	restrictToWorkspace bool
}

var defaultDenyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\brm\s+-[rf]{1,2}\b`),
	regexp.MustCompile(`\bdel\s+/[fq]\b`),
	regexp.MustCompile(`\brmdir\s+/s\b`),
	regexp.MustCompile(`\b(format|mkfs|diskpart)\b\s`), // Match disk wiping commands (must be followed by space/args)
	regexp.MustCompile(`\bdd\s+if=`),
	regexp.MustCompile(`>\s*/dev/sd[a-z]\b`), // Block writes to disk devices (but allow /dev/null)
	regexp.MustCompile(`\b(shutdown|reboot|poweroff)\b`),
	regexp.MustCompile(`:\(\)\s*\{.*\};\s*:`),
	regexp.MustCompile(`\$\([^)]+\)`),
	regexp.MustCompile(`\$\{[^}]+\}`),
	regexp.MustCompile("`[^`]+`"),
	regexp.MustCompile(`\|\s*sh\b`),
	regexp.MustCompile(`\|\s*bash\b`),
	regexp.MustCompile(`;\s*rm\s+-[rf]`),
	regexp.MustCompile(`&&\s*rm\s+-[rf]`),
	regexp.MustCompile(`\|\|\s*rm\s+-[rf]`),
	regexp.MustCompile(`>\s*/dev/null\s*>&?\s*\d?`),
	regexp.MustCompile(`<<\s*EOF`),
	regexp.MustCompile(`\$\(\s*cat\s+`),
	regexp.MustCompile(`\$\(\s*curl\s+`),
	regexp.MustCompile(`\$\(\s*wget\s+`),
	regexp.MustCompile(`\$\(\s*which\s+`),
	regexp.MustCompile(`\bsudo\b`),
	regexp.MustCompile(`\bchmod\s+[0-7]{3,4}\b`),
	regexp.MustCompile(`\bchown\b`),
	regexp.MustCompile(`\bpkill\b`),
	regexp.MustCompile(`\bkillall\b`),
	regexp.MustCompile(`\bkill\s+-[9]\b`),
	regexp.MustCompile(`\bcurl\b.*\|\s*(sh|bash)`),
	regexp.MustCompile(`\bwget\b.*\|\s*(sh|bash)`),
	regexp.MustCompile(`\bnpm\s+install\s+-g\b`),
	regexp.MustCompile(`\bpip\s+install\s+--user\b`),
	regexp.MustCompile(`\bapt\s+(install|remove|purge)\b`),
	regexp.MustCompile(`\byum\s+(install|remove)\b`),
	regexp.MustCompile(`\bdnf\s+(install|remove)\b`),
	regexp.MustCompile(`\bdocker\s+run\b`),
	regexp.MustCompile(`\bdocker\s+exec\b`),
	regexp.MustCompile(`\bgit\s+push\b`),
	regexp.MustCompile(`\bgit\s+force\b`),
	regexp.MustCompile(`\bssh\b.*@`),
	regexp.MustCompile(`\beval\b`),
	regexp.MustCompile(`\bsource\s+.*\.sh\b`),
}

func NewExecTool(workingDir string, restrict bool) *ExecTool {
	return NewExecToolWithConfig(workingDir, restrict, nil)
}

func NewExecToolWithConfig(workingDir string, restrict bool, config *config.Config) *ExecTool {
	denyPatterns := make([]*regexp.Regexp, 0)

	if config != nil {
		execConfig := config.Tools.Exec
		enableDenyPatterns := execConfig.EnableDenyPatterns
		if enableDenyPatterns {
			denyPatterns = append(denyPatterns, defaultDenyPatterns...)
			if len(execConfig.CustomDenyPatterns) > 0 {
				fmt.Printf("Using custom deny patterns: %v\n", execConfig.CustomDenyPatterns)
				for _, pattern := range execConfig.CustomDenyPatterns {
					re, err := regexp.Compile(pattern)
					if err != nil {
						fmt.Printf("Invalid custom deny pattern %q: %v\n", pattern, err)
						continue
					}
					denyPatterns = append(denyPatterns, re)
				}
			}
		} else {
			// If deny patterns are disabled, we won't add any patterns, allowing all commands.
			fmt.Println("Warning: deny patterns are disabled. All commands will be allowed.")
		}
	} else {
		denyPatterns = append(denyPatterns, defaultDenyPatterns...)
	}

	var confirmPatterns []*regexp.Regexp
	if config != nil {
		for _, pattern := range config.Tools.Exec.ConfirmPatterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				fmt.Printf("Invalid confirm pattern %q: %v\n", pattern, err)
				continue
			}
			confirmPatterns = append(confirmPatterns, re)
		}
	}

	return &ExecTool{
		workingDir:          workingDir,
		timeout:             60 * time.Second,
		denyPatterns:        denyPatterns,
		allowPatterns:       nil,
		confirmPatterns:     confirmPatterns,
		restrictToWorkspace: restrict,
	}
}

func (t *ExecTool) Name() string {
	return "exec"
}

func (t *ExecTool) Description() string {
	return "Execute a shell command and return its output. Use with caution."
}

func (t *ExecTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"working_dir": map[string]any{
				"type":        "string",
				"description": "Optional working directory for the command",
			},
		},
		"required": []string{"command"},
	}
}

// runResult holds the outcome of a single command execution attempt.
type runResult struct {
	output   string
	timedOut bool
	err      error
}

// runOnce executes the command once with the given timeout and returns a runResult.
func (t *ExecTool) runOnce(ctx context.Context, command, cwd string, timeout time.Duration) runResult {
	var cmdCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		cmdCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	} else {
		cmd = exec.CommandContext(cmdCtx, "sh", "-c", command)
	}
	if cwd != "" {
		cmd.Dir = cwd
	}

	prepareCommandForTermination(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return runResult{
			output: fmt.Sprintf("failed to start command: %v", err),
			err:    err,
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case err = <-done:
	case <-cmdCtx.Done():
		_ = terminateProcessTree(cmd)
		select {
		case err = <-done:
		case <-time.After(2 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			err = <-done
		}
	}

	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		return runResult{timedOut: true, err: err}
	}

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}
	if err != nil {
		output += fmt.Sprintf("\nExit code: %v", err)
	}

	return runResult{output: output, err: err}
}

func (t *ExecTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	command, ok := args["command"].(string)
	if !ok {
		return ErrorResult("command is required")
	}

	cwd := t.workingDir
	if wd, ok := args["working_dir"].(string); ok && wd != "" {
		if t.restrictToWorkspace && t.workingDir != "" {
			resolvedWD, err := validatePath(wd, t.workingDir, true)
			if err != nil {
				return ErrorResult("Command blocked by safety guard (" + err.Error() + ")")
			}
			cwd = resolvedWD
		} else {
			cwd = wd
		}
	}

	if cwd == "" {
		wd, err := os.Getwd()
		if err == nil {
			cwd = wd
		}
	}

	if guardError := t.guardCommand(command, cwd); guardError != "" {
		return NonRetryableError(guardError)
	}

	// Check confirmation patterns
	if len(t.confirmPatterns) > 0 {
		lower := strings.ToLower(strings.TrimSpace(command))
		for _, pattern := range t.confirmPatterns {
			if pattern.MatchString(lower) {
				return ConfirmationResult(
					fmt.Sprintf("Command matches confirmation pattern. Allow execution?\nCommand: %s", command),
				)
			}
		}
	}

	// Auto-debug-loop: retry on timeout (up to maxTimeoutRetries times, doubling the timeout each time).
	effectiveTimeout := t.timeout
	for attempt := 0; ; attempt++ {
		res := t.runOnce(ctx, command, cwd, effectiveTimeout)

		if res.timedOut {
			if attempt < maxTimeoutRetries {
				newTimeout := effectiveTimeout * timeoutMultiplier
				logger.Info(fmt.Sprintf(
					"Command timed out (attempt %d/%d), retrying with increased timeout %v → %v: %s",
					attempt+1, maxTimeoutRetries, effectiveTimeout, newTimeout, command,
				))
				effectiveTimeout = newTimeout
				continue
			}
			// Exhausted retries — report final timeout to LLM with diagnostic context.
			msg := fmt.Sprintf(
				"[AUTO-DEBUG] Command timed out after %d attempts (final timeout: %v).\n"+
					"Consider: breaking the command into smaller steps, using a background process, "+
					"or increasing the timeout explicitly.\nCommand: %s",
				attempt+1, effectiveTimeout, command,
			)
			return RetryableError(msg, "Try breaking the command into smaller steps or increasing the timeout")
		}

		if res.err != nil {
			// Check for "command not found" — surface a rich diagnostic for the LLM.
			if isCommandNotFound(res.output) {
				msg := fmt.Sprintf(
					"[AUTO-DEBUG] Command not found or binary missing.\n"+
						"Diagnosis: the executable invoked by the command does not exist on this system or is not in PATH.\n"+
						"Suggestions: verify the tool is installed, check PATH, or use an alternative command.\n"+
						"Original output:\n%s",
					res.output,
				)
				logger.Info(fmt.Sprintf("Auto-debug: command not found for: %s", command))
				return NonRetryableError(msg)
			}

			// General failure — return as-is.
			output := res.output
			if output == "" {
				output = "(no output)"
			}
			if len(output) > 10000 {
				output = output[:10000] + fmt.Sprintf("\n... (truncated, %d more chars)", len(output)-10000)
			}
			return &ToolResult{
				ForLLM:    output + "\n[TOOL_STATUS: error, retryable: true]",
				ForUser:   output,
				IsError:   true,
				Retryable: true,
			}
		}

		// Success.
		output := res.output
		if output == "" {
			output = "(no output)"
		}
		if len(output) > 10000 {
			output = output[:10000] + fmt.Sprintf("\n... (truncated, %d more chars)", len(output)-10000)
		}
		return &ToolResult{ForLLM: output, ForUser: output, IsError: false}
	}
}

// shellMetacharPatterns detects shell metacharacters that can bypass simple pattern matching.
var shellMetacharPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\\x[0-9a-fA-F]{2}`),           // hex escapes
	regexp.MustCompile(`\\u[0-9a-fA-F]{4}`),           // unicode escapes
	regexp.MustCompile(`\$'[^']*\\[^']*'`),            // $'...' ANSI-C quoting with escapes
	regexp.MustCompile(`\benv\b.*\b\w+=.*\b\w+\b`),    // env VAR=val cmd (command execution via env)
	regexp.MustCompile(`\bxargs\b.*\b(sh|bash|rm)\b`), // xargs piped to dangerous commands
	regexp.MustCompile(`\bfind\b.*-exec\b`),           // find -exec runs commands
	regexp.MustCompile(`\bawk\b.*\bsystem\s*\(`),      // awk system() calls
	regexp.MustCompile(`\bperl\b.*\s-e\s`),            // perl one-liners
	regexp.MustCompile(`\bruby\b.*\s-e\s`),            // ruby one-liners
	regexp.MustCompile(`\bexec\s+\d*[<>]`),            // exec with redirections
	regexp.MustCompile(`\|\s*while\b`),                // pipe to while loop
	regexp.MustCompile(`\bbase64\b.*\|\s*(sh|bash)`),  // base64 decode to shell
}

func (t *ExecTool) guardCommand(command, cwd string) string {
	cmd := strings.TrimSpace(command)
	lower := strings.ToLower(cmd)

	for _, pattern := range t.denyPatterns {
		if pattern.MatchString(lower) {
			return "Command blocked by safety guard (dangerous pattern detected)"
		}
	}

	// Check shell metacharacter bypass patterns
	for _, pattern := range shellMetacharPatterns {
		if pattern.MatchString(lower) {
			return "Command blocked by safety guard (shell metacharacter bypass detected)"
		}
	}

	if len(t.allowPatterns) > 0 {
		allowed := false
		for _, pattern := range t.allowPatterns {
			if pattern.MatchString(lower) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "Command blocked by safety guard (not in allowlist)"
		}
	}

	if t.restrictToWorkspace {
		if strings.Contains(cmd, "..\\") || strings.Contains(cmd, "../") {
			return "Command blocked by safety guard (path traversal detected)"
		}

		cwdPath, err := filepath.Abs(cwd)
		if err != nil {
			return ""
		}

		pathPattern := regexp.MustCompile(`[A-Za-z]:\\[^\\\"']+|/[^\s\"']+`)
		matches := pathPattern.FindAllString(cmd, -1)

		for _, raw := range matches {
			p, err := filepath.Abs(raw)
			if err != nil {
				continue
			}

			rel, err := filepath.Rel(cwdPath, p)
			if err != nil {
				continue
			}

			if strings.HasPrefix(rel, "..") {
				return "Command blocked by safety guard (path outside working dir)"
			}
		}
	}

	return ""
}

func (t *ExecTool) SetTimeout(timeout time.Duration) {
	t.timeout = timeout
}

func (t *ExecTool) SetRestrictToWorkspace(restrict bool) {
	t.restrictToWorkspace = restrict
}

func (t *ExecTool) SetAllowPatterns(patterns []string) error {
	t.allowPatterns = make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return fmt.Errorf("invalid allow pattern %q: %w", p, err)
		}
		t.allowPatterns = append(t.allowPatterns, re)
	}
	return nil
}
