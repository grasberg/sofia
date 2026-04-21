package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestShellTool_Success verifies successful command execution
func TestShellTool_Success(t *testing.T) {
	tool := NewExecTool("", false)

	ctx := context.Background()
	args := map[string]any{
		"command": "echo 'hello world'",
	}

	result := tool.Execute(ctx, args)

	// Success should not be an error
	if result.IsError {
		t.Errorf("Expected success, got IsError=true: %s", result.ForLLM)
	}

	// ForUser should contain command output
	if !strings.Contains(result.ForUser, "hello world") {
		t.Errorf("Expected ForUser to contain 'hello world', got: %s", result.ForUser)
	}

	// ForLLM should contain full output
	if !strings.Contains(result.ForLLM, "hello world") {
		t.Errorf("Expected ForLLM to contain 'hello world', got: %s", result.ForLLM)
	}
}

// TestShellTool_Failure verifies failed command execution
func TestShellTool_Failure(t *testing.T) {
	tool := NewExecTool("", false)

	ctx := context.Background()
	args := map[string]any{
		"command": "ls /nonexistent_directory_12345",
	}

	result := tool.Execute(ctx, args)

	// Failure should be marked as error
	if !result.IsError {
		t.Errorf("Expected error for failed command, got IsError=false")
	}

	// ForUser should contain error information
	if result.ForUser == "" {
		t.Errorf("Expected ForUser to contain error info, got empty string")
	}

	// ForLLM should contain exit code or error
	if !strings.Contains(result.ForLLM, "Exit code") && result.ForUser == "" {
		t.Errorf("Expected ForLLM to contain exit code or error, got: %s", result.ForLLM)
	}
}

// TestShellTool_Timeout verifies command timeout handling
func TestShellTool_Timeout(t *testing.T) {
	tool := NewExecTool("", false)
	tool.SetTimeout(100 * time.Millisecond)

	ctx := context.Background()
	args := map[string]any{
		"command": "sleep 10",
	}

	result := tool.Execute(ctx, args)

	// Timeout should be marked as error
	if !result.IsError {
		t.Errorf("Expected error for timeout, got IsError=false")
	}

	// Should mention timeout
	if !strings.Contains(result.ForLLM, "timed out") && !strings.Contains(result.ForUser, "timed out") {
		t.Errorf("Expected timeout message, got ForLLM: %s, ForUser: %s", result.ForLLM, result.ForUser)
	}
}

// TestShellTool_WorkingDir verifies custom working directory
func TestShellTool_WorkingDir(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0o644)

	tool := NewExecTool("", false)

	ctx := context.Background()
	args := map[string]any{
		"command":     "cat test.txt",
		"working_dir": tmpDir,
	}

	result := tool.Execute(ctx, args)

	if result.IsError {
		t.Errorf("Expected success in custom working dir, got error: %s", result.ForLLM)
	}

	if !strings.Contains(result.ForUser, "test content") {
		t.Errorf("Expected output from custom dir, got: %s", result.ForUser)
	}
}

// TestShellTool_DangerousCommand verifies safety guard blocks dangerous commands
func TestShellTool_DangerousCommand(t *testing.T) {
	tool := NewExecTool("", false)

	ctx := context.Background()
	args := map[string]any{
		"command": "rm -rf /",
	}

	result := tool.Execute(ctx, args)

	// Dangerous command should be blocked
	if !result.IsError {
		t.Errorf("Expected dangerous command to be blocked (IsError=true)")
	}

	if !strings.Contains(result.ForLLM, "blocked") && !strings.Contains(result.ForUser, "blocked") {
		t.Errorf("Expected 'blocked' message, got ForLLM: %s, ForUser: %s", result.ForLLM, result.ForUser)
	}
}

// TestShellTool_BypassAttempts verifies that various obfuscation techniques
// cannot bypass the deny patterns.
func TestShellTool_BypassAttempts(t *testing.T) {
	tool := NewExecTool("", false)
	ctx := context.Background()

	tests := []struct {
		name    string
		command string
	}{
		// Backslash-newline continuation: shell treats "rm \<newline>-rf /" as "rm -rf /"
		{"backslash-newline rm", "rm \\\n-rf /"},
		{"backslash-newline sudo", "su\\\ndo rm /tmp/x"},

		// Single-quote splitting: shell treats 'r'm as rm
		{"single-quote split rm", "'r'm -rf /"},
		{"single-quote split sudo", "'su'do rm /tmp/x"},
		{"single-quote split kill", "'ki'll -9 1"},

		// Double-quote splitting: shell treats r"m" as rm
		{"double-quote split rm", "r\"m\" -rf /"},
		{"double-quote split sudo", "su\"do\" rm /tmp/x"},
		{"double-quote split shutdown", "shut\"down\" now"},

		// Bare $variable references (not caught by $() or ${} patterns)
		{"bare dollar-var exec", "a=rm; $a -rf /"},

		// Heredoc with non-EOF delimiter
		{"heredoc DELIM", "cat << DELIM\nrm -rf /\nDELIM"},
		{"heredoc END", "cat << END\nrm -rf /\nEND"},
		{"heredoc quoted DELIM", "cat << 'DELIM'\nrm -rf /\nDELIM"},
		{"heredoc indented", "cat <<-INDENT\nrm -rf /\nINDENT"},

		// Tabs between command and args
		{"tab-separated rm", "rm\t-rf /"},
		// Multiple spaces
		{"multi-space rm", "rm   -rf /"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tool.Execute(ctx, map[string]any{"command": tt.command})
			if !result.IsError {
				t.Errorf("Expected bypass attempt %q to be blocked, but it was allowed", tt.command)
			}
			if !strings.Contains(result.ForLLM, "blocked") {
				t.Errorf("Expected 'blocked' in error for %q, got: %s", tt.command, result.ForLLM)
			}
		})
	}
}

// TestShellTool_NormalizeCommand verifies the normalizeCommand helper.
func TestShellTool_NormalizeCommand(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"rm \\\n-rf /", "rm -rf /"},
		{"'r'm -rf /", "rm -rf /"},
		{"r\"m\" -rf /", "rm -rf /"},
		{"rm   -rf   /", "rm -rf /"},
		{"rm\t-rf\t/", "rm -rf /"},
		{"  rm -rf /  ", "rm -rf /"},
		{"echo hello", "echo hello"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeCommand(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeCommand(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestShellTool_GuardCommand_DenyPatterns verifies deny pattern blocking
func TestShellTool_GuardCommand_DenyPatterns(t *testing.T) {
	tool := NewExecTool("", false)

	// These should be blocked by default deny patterns.
	blocked := []struct {
		name string
		cmd  string
	}{
		{"rm -rf /", "rm -rf /"},
		{"mkfs", "mkfs /dev/sda"},
		{"dd if=/dev/zero", "dd if=/dev/zero of=/dev/sda"},
		{"sudo", "sudo apt install foo"},
		{"kill", "kill -9 1"},
		{"eval", "eval dangerous_cmd"},
	}

	for _, tt := range blocked {
		t.Run(tt.name, func(t *testing.T) {
			result := tool.guardCommand(tt.cmd, "/tmp")
			if result == "" {
				t.Errorf("expected guardCommand to block %q", tt.cmd)
			}
		})
	}

	// These should be allowed.
	allowed := []struct {
		name string
		cmd  string
	}{
		{"echo", "echo hello"},
		{"ls", "ls -la"},
		{"go test", "go test ./..."},
		{"cat", "cat README.md"},
	}

	for _, tt := range allowed {
		t.Run("allow:"+tt.name, func(t *testing.T) {
			result := tool.guardCommand(tt.cmd, "/tmp")
			if result != "" {
				t.Errorf("expected guardCommand to allow %q, got: %s", tt.cmd, result)
			}
		})
	}
}

// TestShellTool_Elevation verifies the two-phase elevation flow
func TestShellTool_Elevation(t *testing.T) {
	tool := NewExecTool("", false)

	// Request a token
	token := tool.RequestElevation()
	if token == "" {
		t.Fatal("expected non-empty elevation token")
	}

	// Confirm with the token
	msg, ok := tool.ConfirmElevation(token)
	if !ok {
		t.Fatalf("expected ConfirmElevation to succeed, got: %s", msg)
	}
	if !tool.elevated {
		t.Fatal("expected elevated to be true after confirmation")
	}

	// Same token should not work twice
	msg, ok = tool.ConfirmElevation(token)
	if ok {
		t.Fatal("expected ConfirmElevation to fail with reused token")
	}
}

// TestShellTool_Elevation_InvalidToken verifies rejection of bad tokens
func TestShellTool_Elevation_InvalidToken(t *testing.T) {
	tool := NewExecTool("", false)

	msg, ok := tool.ConfirmElevation("invalid-token")
	if ok {
		t.Fatalf("expected invalid token to be rejected, got: %s", msg)
	}
}

// TestShellTool_SetElevated verifies direct elevation setting
func TestShellTool_SetElevated(t *testing.T) {
	tool := NewExecTool("", false)

	tool.SetElevated(true)
	if !tool.elevated {
		t.Fatal("expected elevated to be true")
	}

	tool.SetElevated(false)
	if tool.elevated {
		t.Fatal("expected elevated to be false")
	}
}

// TestShellTool_MissingCommand verifies error handling for missing command
func TestShellTool_MissingCommand(t *testing.T) {
	tool := NewExecTool("", false)

	ctx := context.Background()
	args := map[string]any{}

	result := tool.Execute(ctx, args)

	// Should return error result
	if !result.IsError {
		t.Errorf("Expected error when command is missing")
	}
}

// TestShellTool_StderrCapture verifies stderr is captured and included
func TestShellTool_StderrCapture(t *testing.T) {
	tool := NewExecTool("", false)

	ctx := context.Background()
	args := map[string]any{
		"command": "sh -c 'echo stdout; echo stderr >&2'",
	}

	result := tool.Execute(ctx, args)

	// Both stdout and stderr should be in output
	if !strings.Contains(result.ForLLM, "stdout") {
		t.Errorf("Expected stdout in output, got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "stderr") {
		t.Errorf("Expected stderr in output, got: %s", result.ForLLM)
	}
}

// TestShellTool_OutputTruncation verifies long output is truncated
func TestShellTool_OutputTruncation(t *testing.T) {
	tool := NewExecTool("", false)

	ctx := context.Background()
	// Generate long output (>10000 chars)
	args := map[string]any{
		"command": "python3 -c \"print('x' * 20000)\" || echo " + strings.Repeat("x", 20000),
	}

	result := tool.Execute(ctx, args)

	// Should have truncation message or be truncated
	if len(result.ForLLM) > 15000 {
		t.Errorf("Expected output to be truncated, got length: %d", len(result.ForLLM))
	}
}

// TestShellTool_WorkingDir_OutsideWorkspace verifies that working_dir cannot escape the workspace directly
func TestShellTool_WorkingDir_OutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	outsideDir := filepath.Join(root, "outside")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}

	tool := NewExecTool(workspace, true)
	result := tool.Execute(context.Background(), map[string]any{
		"command":     "pwd",
		"working_dir": outsideDir,
	})

	if !result.IsError {
		t.Fatalf("expected working_dir outside workspace to be blocked, got output: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "blocked") {
		t.Errorf("expected 'blocked' in error, got: %s", result.ForLLM)
	}
}

// TestShellTool_WorkingDir_SymlinkEscape verifies that a symlink inside the workspace
// pointing outside cannot be used as working_dir to escape the sandbox.
func TestShellTool_WorkingDir_SymlinkEscape(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	secretDir := filepath.Join(root, "secret")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	if err := os.MkdirAll(secretDir, 0o755); err != nil {
		t.Fatalf("failed to create secret dir: %v", err)
	}
	os.WriteFile(filepath.Join(secretDir, "secret.txt"), []byte("top secret"), 0o644)

	// symlink lives inside the workspace but resolves to secretDir outside it
	link := filepath.Join(workspace, "escape")
	if err := os.Symlink(secretDir, link); err != nil {
		t.Skipf("symlinks not supported in this environment: %v", err)
	}

	tool := NewExecTool(workspace, true)
	result := tool.Execute(context.Background(), map[string]any{
		"command":     "cat secret.txt",
		"working_dir": link,
	})

	if !result.IsError {
		t.Fatalf("expected symlink working_dir escape to be blocked, got output: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "blocked") {
		t.Errorf("expected 'blocked' in error, got: %s", result.ForLLM)
	}
}

// TestIsSafeSystemPath verifies safe path detection
func TestIsSafeSystemPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/usr/bin/python3", true},
		{"/bin/sh", true},
		{"/sbin/ifconfig", true},
		{"/opt/homebrew/bin/go", true},
		{"/tmp/scratch", true},
		{"/var/log/syslog", true},
		{"/dev/null", true},
		{"/proc/1/status", true},
		{"/nix/store/abc", true},
		{"/Library/Frameworks/Python.framework", true},
		{"/System/Library/CoreServices", true},
		{"/Applications/Xcode.app", true},
		{"/Volumes/External/project", true},
		{"/private/tmp/test", true},
		// Home dot-directories are safe (tool configs)
		{"/Users/alice/.cargo/bin/rustc", true},
		{"/home/bob/.local/bin/pip", true},
		// Non-dot home paths are NOT safe (user data)
		{"/Users/alice/Documents/secret.txt", false},
		{"/home/bob/projects/app/main.go", false},
		// Random paths are not safe
		{"/workspace/project/file.go", false},
		{"/data/db/production.db", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isSafeSystemPath(tt.path)
			if got != tt.want {
				t.Errorf("isSafeSystemPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestShellMetacharPatterns verifies obfuscation detection
func TestShellMetacharPatterns(t *testing.T) {
	dangerous := []struct {
		name string
		cmd  string
	}{
		{"hex escape", `\x72\x6d -rf /`},
		{"unicode escape", `\u0072\u006d -rf /`},
		{"base64 to shell", "echo cm0gLXJmIC8= | base64 -d | bash"},
		{"xargs to rm", "find . | xargs rm -rf"},
		{"awk system", "awk '{system(\"rm -rf /\")}'"},
		{"perl one-liner", "perl -e 'system(\"rm -rf /\")'"},
		{"ruby one-liner", "ruby -e 'system(\"rm -rf /\")'"},
	}

	for _, tt := range dangerous {
		t.Run(tt.name, func(t *testing.T) {
			lower := strings.ToLower(tt.cmd)
			matched := false
			for _, p := range shellMetacharPatterns {
				if p.MatchString(lower) {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("expected shellMetacharPatterns to match %q", tt.cmd)
			}
		})
	}

	safe := []string{
		"echo hello world",
		"ls -la /tmp",
		"cat /etc/hostname",
		"go test ./...",
	}

	for _, cmd := range safe {
		t.Run("safe:"+cmd, func(t *testing.T) {
			lower := strings.ToLower(cmd)
			for _, p := range shellMetacharPatterns {
				if p.MatchString(lower) {
					t.Errorf("shellMetacharPatterns should NOT match safe command %q (pattern: %s)", cmd, p.String())
				}
			}
		})
	}
}

// TestShellTool_RestrictToWorkspace verifies workspace restriction
func TestShellTool_RestrictToWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	tool := NewExecTool(tmpDir, false)
	tool.SetRestrictToWorkspace(true)

	ctx := context.Background()
	args := map[string]any{
		"command": "cat ../../etc/passwd",
	}

	result := tool.Execute(ctx, args)

	// Path traversal should be blocked
	if !result.IsError {
		t.Errorf("Expected path traversal to be blocked with restrictToWorkspace=true")
	}

	if !strings.Contains(result.ForLLM, "blocked") && !strings.Contains(result.ForUser, "blocked") {
		t.Errorf(
			"Expected 'blocked' message for path traversal, got ForLLM: %s, ForUser: %s",
			result.ForLLM,
			result.ForUser,
		)
	}
}
