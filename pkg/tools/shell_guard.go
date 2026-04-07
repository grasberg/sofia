package tools

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

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

// pathPatternRe is pre-compiled to avoid recompilation on every guardCommand call.
var pathPatternRe = regexp.MustCompile(`[A-Za-z]:\\[^\\"']+|/[^\s"']+`)

// normalizeCommand prepares a command string for deny-pattern matching by removing
// shell constructs that could bypass simple regex patterns:
//   - Strips backslash-newline continuations (shell line continuation)
//   - Removes single and double quotes used to split/obfuscate command names
//   - Collapses runs of whitespace into a single space
func normalizeCommand(cmd string) string {
	// Remove backslash-newline continuations (shell line continuation).
	// In sh -c, a backslash followed by a newline joins the two lines.
	cmd = strings.ReplaceAll(cmd, "\\\n", "")

	// Remove single and double quotes that can be used to obfuscate commands.
	// e.g. 'r'm becomes rm, r"m" becomes rm.
	cmd = strings.ReplaceAll(cmd, "'", "")
	cmd = strings.ReplaceAll(cmd, "\"", "")

	// Collapse all whitespace runs (spaces, tabs, etc.) into a single space.
	fields := strings.Fields(cmd)
	cmd = strings.Join(fields, " ")

	return cmd
}

func (t *ExecTool) guardCommand(command, cwd string) string {
	cmd := strings.TrimSpace(command)

	// Normalize the command to defeat obfuscation: strip backslash-newline
	// continuations, remove shell quotes, and collapse whitespace.
	normalized := normalizeCommand(cmd)
	lower := strings.ToLower(normalized)

	// Also check the raw (un-normalized) lowercase form so that patterns
	// looking for literal quote/escape constructs still work.
	rawLower := strings.ToLower(cmd)

	// When elevated, only check destructive patterns from the configured deny list.
	// When not elevated, check the full deny list.
	patternsToCheck := t.denyPatterns
	if t.elevated && len(t.denyPatterns) > 0 {
		patternsToCheck = destructiveDenyPatterns
	}

	for _, pattern := range patternsToCheck {
		if pattern.MatchString(lower) || pattern.MatchString(rawLower) {
			return "Command blocked by safety guard (dangerous pattern detected)"
		}
	}

	// Check shell metacharacter bypass patterns against the raw form
	// (normalization strips quotes that these patterns may look for).
	for _, pattern := range shellMetacharPatterns {
		if pattern.MatchString(rawLower) || pattern.MatchString(lower) {
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

		matches := pathPatternRe.FindAllString(cmd, -1)

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

// SetElevated sets the elevated flag. When true, only destructive deny patterns
// are enforced; caution patterns (package managers, docker, git push, etc.) are
// allowed through. Requires a two-phase confirmation: first call returns a token,
// second call with the token actually enables elevation.
func (t *ExecTool) SetElevated(v bool) {
	t.elevated = v
}

// RequestElevation implements a confirmation flow for enabling elevated mode.
// Returns a confirmation token on first call. Pass the token back to ConfirmElevation
// to actually enable it.
func (t *ExecTool) RequestElevation() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	token := fmt.Sprintf("elevate_%d", time.Now().UnixNano()/1000)
	t.pendingTokens[token] = time.Now().Add(5 * time.Minute)
	return token
}

// ConfirmElevation verifies the token and enables elevated mode if valid.
// Returns a warning message on success or an error message on failure.
func (t *ExecTool) ConfirmElevation(token string) (string, bool) {
	t.mu.Lock()
	expires, valid := t.pendingTokens[token]
	if valid {
		delete(t.pendingTokens, token)
	}
	t.mu.Unlock()

	if !valid || time.Now().After(expires) {
		return "Invalid or expired elevation token.", false
	}

	t.elevated = true
	return "WARNING: Elevated mode is now ACTIVE. Caution-level deny patterns (package managers, " +
		"docker, git push, chmod, ssh, etc.) are bypassed. Critical destructive patterns " +
		"(rm -rf /, mkfs, dd, fork bombs, sudo, kill) remain enforced. " +
		"Elevated mode increases the risk of unintended system changes.", true
}
