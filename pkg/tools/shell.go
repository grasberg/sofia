package tools

import (
	"bytes"
	"context"
	crypto_rand "crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
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

// extraPATHDirs are common tool installation directories that may be absent
// from the minimal PATH inherited by "sh -c" subprocesses.
var extraPATHDirs = []string{
	"/opt/homebrew/bin",
	"/opt/homebrew/sbin",
	"/usr/local/bin",
	"/usr/local/sbin",
	"/usr/local/go/bin",
	"/home/linuxbrew/.linuxbrew/bin",
}

// computeEnrichedEnv builds the full environment variable set with extra PATH
// directories appended. Called once at ExecTool creation time to avoid
// repeated os.Stat calls on every command execution.
func computeEnrichedEnv() []string {
	currentPATH := os.Getenv("PATH")
	pathSet := make(map[string]bool)
	for _, p := range strings.Split(currentPATH, ":") {
		pathSet[p] = true
	}

	var extra []string
	for _, d := range extraPATHDirs {
		if !pathSet[d] {
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				extra = append(extra, d)
			}
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		for _, rel := range []string{"go/bin", ".local/bin", ".cargo/bin", ".pyenv/shims"} {
			d := filepath.Join(home, rel)
			if !pathSet[d] {
				if info, err := os.Stat(d); err == nil && info.IsDir() {
					extra = append(extra, d)
				}
			}
		}
	}

	env := os.Environ()
	if len(extra) == 0 {
		return env
	}

	newPATH := currentPATH + ":" + strings.Join(extra, ":")
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = "PATH=" + newPATH
			return env
		}
	}
	return append(env, "PATH="+newPATH)
}

// generateSecureToken produces a cryptographically random token with the given prefix.
func generateSecureToken(prefix string) string {
	b := make([]byte, 16)
	if _, err := crypto_rand.Read(b); err != nil {
		// Fallback to timestamp if crypto/rand fails (shouldn't happen).
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%x", prefix, b)
}

// commandAliases maps bare command names to their macOS/modern-Linux equivalents.
// These are tried automatically when the original command is not found.
var commandAliases = map[string]string{
	"pip":    "pip3",
	"python": "python3",
}

// rewriteCommand applies commandAliases to the first token of a shell command.
// Returns the original command unchanged if no alias matches.
func rewriteCommand(command string) string {
	trimmed := strings.TrimSpace(command)
	for old, replacement := range commandAliases {
		// Match "pip ..." or "pip" at start of command.
		if trimmed == old || strings.HasPrefix(trimmed, old+" ") {
			return replacement + trimmed[len(old):]
		}
	}
	return command
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
	elevated            bool
	mu                  sync.Mutex
	pendingTokens       map[string]time.Time
	sandboxConfig       *config.SandboxedExecConfig
	enrichedEnv         []string // cached enriched PATH environment, computed once at creation
}

// destructiveDenyPatterns are ALWAYS enforced, even in elevated mode.
// These protect against catastrophic system damage.
var destructiveDenyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\brm\s+-[rf]{1,2}\b`),
	regexp.MustCompile(`\bdel\s+/[fq]\b`),
	regexp.MustCompile(`\brmdir\s+/s\b`),
	regexp.MustCompile(`\b(format|mkfs|diskpart)\b\s`),
	regexp.MustCompile(`\bdd\s+if=`),
	regexp.MustCompile(`>\s*/dev/sd[a-z]\b`),
	regexp.MustCompile(`\b(shutdown|reboot|poweroff)\b`),
	regexp.MustCompile(`:\(\)\s*\{.*\};\s*:`),
	regexp.MustCompile(`\$\([^)]+\)`),
	regexp.MustCompile(`\$\{[^}]+\}`),
	regexp.MustCompile(`\$[A-Za-z_]\w*`),
	regexp.MustCompile("`[^`]+`"),
	regexp.MustCompile(`\|\s*sh\b`),
	regexp.MustCompile(`\|\s*bash\b`),
	regexp.MustCompile(`;\s*rm\s+-[rf]`),
	regexp.MustCompile(`&&\s*rm\s+-[rf]`),
	regexp.MustCompile(`\|\|\s*rm\s+-[rf]`),
	regexp.MustCompile(`>\s*/dev/null\s*>&?\s*\d?`),
	regexp.MustCompile(`<<-?\s*'?\w+'?`),
	regexp.MustCompile(`\$\(\s*cat\s+`),
	regexp.MustCompile(`\$\(\s*curl\s+`),
	regexp.MustCompile(`\$\(\s*wget\s+`),
	regexp.MustCompile(`\$\(\s*which\s+`),
	regexp.MustCompile(`\bsudo\b`),
	regexp.MustCompile(`\bpkill\b`),
	regexp.MustCompile(`\bkillall\b`),
	regexp.MustCompile(`\bkill\b`),
	regexp.MustCompile(`\bcurl\b.*\|\s*(sh|bash)`),
	regexp.MustCompile(`\bwget\b.*\|\s*(sh|bash)`),
	regexp.MustCompile(`\beval\b`),
	regexp.MustCompile(`\bsource\s+.*\.sh\b`),
}

// cautionDenyPatterns are skipped when elevated mode is active.
// These block package managers, docker, git push, chmod/chown, ssh, etc.
var cautionDenyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bchmod\s+[0-7]{3,4}\b`),
	regexp.MustCompile(`\bchown\b`),
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
}

func NewExecTool(workingDir string, restrict bool) *ExecTool {
	return NewExecToolWithConfig(workingDir, restrict, nil)
}

func NewExecToolWithConfig(workingDir string, restrict bool, cfg *config.Config) *ExecTool {
	denyPatterns := make([]*regexp.Regexp, 0)

	// Always enforce the critical destructive deny patterns regardless of config.
	// These protect against catastrophic system damage and cannot be disabled.
	denyPatterns = append(denyPatterns, destructiveDenyPatterns...)

	if cfg != nil {
		execConfig := cfg.Tools.Exec
		enableDenyPatterns := execConfig.EnableDenyPatterns
		if enableDenyPatterns {
			// Add caution patterns on top of the always-enforced destructive patterns
			denyPatterns = append(denyPatterns, cautionDenyPatterns...)
			if len(execConfig.CustomDenyPatterns) > 0 {
				logger.InfoCF("exec", "Using custom deny patterns",
					map[string]any{"patterns": execConfig.CustomDenyPatterns})
				for _, pattern := range execConfig.CustomDenyPatterns {
					re, err := regexp.Compile(pattern)
					if err != nil {
						logger.WarnCF("exec", "Invalid custom deny pattern",
							map[string]any{"pattern": pattern, "error": err.Error()})
						continue
					}
					denyPatterns = append(denyPatterns, re)
				}
			}
		} else {
			// Caution patterns are disabled, but destructive patterns are always enforced above.
			logger.WarnCF(
				"exec",
				"Caution deny patterns are disabled. Critical destructive patterns remain enforced.",
				nil,
			)
		}
	} else {
		denyPatterns = append(denyPatterns, cautionDenyPatterns...)
	}

	var confirmPatterns []*regexp.Regexp
	var sandboxCfg *config.SandboxedExecConfig
	if cfg != nil {
		for _, pattern := range cfg.Tools.Exec.ConfirmPatterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				fmt.Printf("Invalid confirm pattern %q: %v\n", pattern, err)
				continue
			}
			confirmPatterns = append(confirmPatterns, re)
		}
		sandboxCfg = &cfg.Guardrails.SandboxedExec
	}

	t := &ExecTool{
		workingDir:          workingDir,
		timeout:             60 * time.Second,
		denyPatterns:        denyPatterns,
		allowPatterns:       nil,
		confirmPatterns:     confirmPatterns,
		restrictToWorkspace: restrict,
		pendingTokens:       make(map[string]time.Time),
		sandboxConfig:       sandboxCfg,
	}
	// Pre-compute enriched PATH once at creation rather than on every exec call.
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		t.enrichedEnv = computeEnrichedEnv()
	}
	return t
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
			"approval_token": map[string]any{
				"type":        "string",
				"description": "Token provided by user to confirm execution",
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

	// Guardrail: Sandboxed Execution via Docker
	if t.sandboxConfig != nil && t.sandboxConfig.Enabled {
		image := t.sandboxConfig.DockerImage
		if image == "" {
			image = "alpine:latest" // Default
		}

		// Mount the workingDir as /workspace in the container
		mountDir := cwd
		if t.workingDir != "" && t.restrictToWorkspace {
			mountDir = t.workingDir // Always mount the root workspace if restricted
		}

		// Calculate relative path inside the container if we mounted a higher directory
		innerCwd := "/workspace"
		if mountDir != cwd && strings.HasPrefix(cwd, mountDir) {
			relPath, _ := filepath.Rel(mountDir, cwd)
			if relPath != "" && relPath != "." {
				innerCwd = filepath.Join(innerCwd, relPath)
			}
		}

		// docker run --rm --network none -v <mountDir>:/workspace -w <innerCwd> <image> sh -c <command>
		dockerArgs := []string{
			"run", "--rm",
			"--network", "none", // Prevent network access
			"-v", fmt.Sprintf("%s:/workspace", mountDir),
			"-w", innerCwd,
			image,
			"sh", "-c", command,
		}

		logger.Audit("Sandboxed Command Executed", map[string]any{
			"command": command,
			"image":   image,
			"cwd":     mountDir,
		})

		cmd = exec.CommandContext(cmdCtx, "docker", dockerArgs...)
	} else {
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
		} else {
			cmd = exec.CommandContext(cmdCtx, "sh", "-c", command)
		}
		if cwd != "" {
			cmd.Dir = cwd
		}
		// Apply pre-computed enriched PATH so Homebrew, pyenv, etc. are found.
		if t.enrichedEnv != nil {
			cmd.Env = t.enrichedEnv
		}
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

	// Clean up expired tokens
	t.mu.Lock()
	now := time.Now()
	for tok, exp := range t.pendingTokens {
		if now.After(exp) {
			delete(t.pendingTokens, tok)
		}
	}
	t.mu.Unlock()

	// Handle approval token if provided
	providedToken, hasToken := args["approval_token"].(string)

	// Check confirmation patterns
	if len(t.confirmPatterns) > 0 {
		lower := strings.ToLower(strings.TrimSpace(command))
		needsConfirm := false
		for _, pattern := range t.confirmPatterns {
			if pattern.MatchString(lower) {
				needsConfirm = true
				break
			}
		}

		if needsConfirm {
			if !hasToken || providedToken == "" {
				// Generate new token
				token := generateSecureToken("tok")

				t.mu.Lock()
				t.pendingTokens[token] = time.Now().Add(5 * time.Minute)
				t.mu.Unlock()

				return ConfirmationResult(
					fmt.Sprintf(
						"Command requires confirmation: `%s`\nUse tool again with approval_token: %q to proceed.",
						command,
						token,
					),
				)
			} else {
				// Verify token
				t.mu.Lock()
				expires, valid := t.pendingTokens[providedToken]
				if valid {
					delete(t.pendingTokens, providedToken) // One-time use
				}
				t.mu.Unlock()

				if !valid {
					return ErrorResult("Invalid or expired approval_token.")
				} else if time.Now().After(expires) {
					return ErrorResult("approval_token has expired. Please try again.")
				}
				// Valid! proceed.
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
			// check for "command not found" — surface a rich diagnostic for the LLM.
			if isCommandNotFound(res.output) {
				// Auto-fix common macOS/Linux aliases before giving up.
				if rewritten := rewriteCommand(command); rewritten != command {
					logger.Info(fmt.Sprintf("Auto-rewrite: %q → %q", command, rewritten))
					retryRes := t.runOnce(ctx, rewritten, cwd, effectiveTimeout)
					if retryRes.err == nil || !isCommandNotFound(retryRes.output) {
						// Rewritten command ran (may still have failed, but
						// it's a real error, not "not found").
						if retryRes.err != nil {
							return &ToolResult{ForLLM: retryRes.output, IsError: true, Err: retryRes.err}
						}
						return &ToolResult{ForLLM: retryRes.output, ForUser: retryRes.output}
					}
				}

				msg := fmt.Sprintf(
					"[AUTO-DEBUG] Command not found or binary missing.\n"+
						"Diagnosis: the executable invoked by the command does not exist on this system or is not in PATH.\n"+
						"Suggestions: verify the tool is installed, check PATH, or use an alternative command.\n"+
						"Original output:\n%s",
					res.output,
				)
				logger.Info(fmt.Sprintf("Auto-debug: command not found for: %s", command))
				logger.Audit("Command Execution Failed", map[string]any{
					"command": command,
					"cwd":     cwd,
					"error":   "command not found",
				})
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
			logger.Audit("Command Execution Error", map[string]any{
				"command":   command,
				"cwd":       cwd,
				"exit_code": res.err.Error(),
			})
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
		logger.Audit("Command Executed Successfully", map[string]any{
			"command": command,
			"cwd":     cwd,
		})
		return &ToolResult{ForLLM: output, ForUser: output, IsError: false}
	}
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
