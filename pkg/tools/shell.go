package tools

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
)

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
