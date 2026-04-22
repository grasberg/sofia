package tools

import (
	crypto_rand "crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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
