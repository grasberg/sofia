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

// CLICommandInput holds parameters for executing a CLI-based tool.
type CLICommandInput struct {
	Ctx         context.Context
	BinaryPath  string
	Args        []string
	Timeout     time.Duration
	ToolName    string // e.g. "gh", "vercel", "gog" -- for error messages
	InstallHint string // e.g. "Install GitHub CLI: brew install gh"
}

// ExecuteCLICommand runs a CLI command with timeout, captures stdout/stderr,
// truncates output, and returns a ToolResult with standardized error handling.
// This eliminates duplication across github.go, vercel.go, gogcli.go, etc.
func ExecuteCLICommand(input CLICommandInput) *ToolResult {
	runCtx, cancel := context.WithTimeout(input.Ctx, input.Timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, input.BinaryPath, input.Args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Merge stdout and stderr
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

	// Truncate long output
	const maxLen = 12000
	if len(output) > maxLen {
		output = output[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(output)-maxLen)
	}

	// Error handling: binary not found
	if isBinaryNotFound(err) {
		return ErrorResult(fmt.Sprintf("%s binary not found at %q. %s",
			input.ToolName, input.BinaryPath, input.InstallHint))
	}

	// Error handling: timeout
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		msg := fmt.Sprintf("%s command timed out after %v", input.ToolName, input.Timeout)
		if output != "(no output)" {
			msg += "\n\n" + output
		}
		return &ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	// Error handling: exit error
	if err != nil {
		output += fmt.Sprintf("\n\nExit error: %v", err)
		return &ToolResult{ForLLM: output, ForUser: output, IsError: true}
	}

	// Success
	return &ToolResult{ForLLM: output, ForUser: output, IsError: false}
}

// isBinaryNotFound checks if the error indicates the binary doesn't exist.
func isBinaryNotFound(err error) bool {
	if err == nil {
		return false
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return errors.Is(execErr.Err, exec.ErrNotFound)
	}
	// Fallback: check error message for various of "not found" patterns
	errMsg := err.Error()
	return strings.Contains(errMsg, "executable file not found") ||
		strings.Contains(errMsg, "no such file or directory") ||
		strings.Contains(errMsg, "not found")
}

// NormalizeCLICommands converts a slice of command strings into a deduplicated set.
func NormalizeCLICommands(commands []string) map[string]struct{} {
	allow := make(map[string]struct{}, len(commands))
	for _, cmd := range commands {
		normalized := strings.ToLower(strings.TrimSpace(cmd))
		if normalized != "" {
			allow[normalized] = struct{}{}
		}
	}
	return allow
}
