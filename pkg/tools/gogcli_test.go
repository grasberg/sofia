package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGoogleCLITool_ValidateArgs(t *testing.T) {
	tool := NewGoogleCLITool("gog", 90, []string{"gmail"})

	result := tool.Execute(context.Background(), map[string]any{})
	if !result.IsError {
		t.Fatal("expected error for missing args")
	}
	if !strings.Contains(result.ForLLM, "args is required") {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	result = tool.Execute(context.Background(), map[string]any{"args": []any{"gmail", 123}})
	if !result.IsError {
		t.Fatal("expected error for non-string arg")
	}
	if !strings.Contains(result.ForLLM, "must be a string") {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	result = tool.Execute(context.Background(), map[string]any{
		"args":    []any{"gmail", "labels", "list"},
		"account": 123,
	})
	if !result.IsError {
		t.Fatal("expected error for non-string account")
	}
	if !strings.Contains(result.ForLLM, "account must be a string") {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	result = tool.Execute(context.Background(), map[string]any{
		"args":            []any{"gmail", "labels", "list"},
		"timeout_seconds": 1.5,
	})
	if !result.IsError {
		t.Fatal("expected error for non-integer timeout_seconds")
	}
	if !strings.Contains(result.ForLLM, "timeout_seconds must be a positive integer") {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
}

func TestGoogleCLITool_AllowlistBlocks(t *testing.T) {
	tool := NewGoogleCLITool("gog", 90, []string{"gmail"})

	result := tool.Execute(context.Background(), map[string]any{
		"args": []any{"drive", "ls"},
	})
	if !result.IsError {
		t.Fatal("expected allowlist block")
	}
	if !strings.Contains(result.ForLLM, "not in allowed_commands") {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
}

func TestGoogleCLITool_BinaryMissing(t *testing.T) {
	tool := NewGoogleCLITool("/definitely/not/found/gog", 90, []string{"gmail"})

	result := tool.Execute(context.Background(), map[string]any{
		"args": []any{"gmail", "labels", "list"},
	})
	if !result.IsError {
		t.Fatal("expected binary missing error")
	}
	if !strings.Contains(result.ForLLM, "binary not found") {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
}

func TestGoogleCLITool_SuccessInjectsFlags(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake gog helper uses POSIX shell script")
	}

	binary := createFakeGog(t, "#!/bin/sh\necho \"$@\"\n")
	tool := NewGoogleCLITool(binary, 90, []string{"gmail"})

	result := tool.Execute(context.Background(), map[string]any{
		"args":    []any{"gmail", "labels", "list"},
		"account": "me@example.com",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "--account me@example.com --json gmail labels list") {
		t.Fatalf("expected injected flags in output, got: %s", result.ForLLM)
	}
}

func TestGoogleCLITool_DoesNotDuplicateAccountFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake gog helper uses POSIX shell script")
	}

	binary := createFakeGog(t, "#!/bin/sh\necho \"$@\"\n")
	tool := NewGoogleCLITool(binary, 90, []string{"gmail"})

	result := tool.Execute(context.Background(), map[string]any{
		"args":    []any{"gmail", "--account=existing@example.com", "labels", "list"},
		"account": "me@example.com",
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.ForLLM)
	}
	if strings.Contains(result.ForLLM, "--account me@example.com") {
		t.Fatalf("did not expect duplicate --account injection, got: %s", result.ForLLM)
	}
}

func TestGoogleCLITool_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake gog helper uses POSIX shell script")
	}

	binary := createFakeGog(t, "#!/bin/sh\nsleep 2\necho done\n")
	tool := NewGoogleCLITool(binary, 1, []string{"gmail"})

	result := tool.Execute(context.Background(), map[string]any{
		"args": []any{"gmail", "labels", "list"},
	})
	if !result.IsError {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(result.ForLLM, "timed out") {
		t.Fatalf("expected timeout message, got: %s", result.ForLLM)
	}
}

func TestGoogleCLITool_BatchIDsInjectedBeforeFlags(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake gog helper uses POSIX shell script")
	}

	binary := createFakeGog(t, "#!/bin/sh\necho \"$@\"\n")
	tool := NewGoogleCLITool(binary, 90, []string{"gmail"})

	result := tool.Execute(context.Background(), map[string]any{
		"args":      []any{"gmail", "batch", "modify", "--add", "STARRED", "--remove", "INBOX"},
		"batch_ids": []any{"m1", "m2", "m2", "m3"},
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "gmail batch modify m1 m2 m3 --add STARRED --remove INBOX") {
		t.Fatalf("unexpected batched args: %s", result.ForLLM)
	}
}

func TestGoogleCLITool_BatchIDsInvalidCommand(t *testing.T) {
	tool := NewGoogleCLITool("gog", 90, []string{"gmail", "drive"})

	result := tool.Execute(context.Background(), map[string]any{
		"args":      []any{"drive", "ls"},
		"batch_ids": []any{"id1"},
	})
	if !result.IsError {
		t.Fatal("expected error for unsupported batch_ids command")
	}
	if !strings.Contains(result.ForLLM, "batch_ids requires a gmail batch command") {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
}

func createFakeGog(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gog")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("failed writing fake gog: %v", err)
	}
	return path
}

func BenchmarkGoogleCLITool_ValidateOnly(b *testing.B) {
	tool := NewGoogleCLITool("gog", 90, []string{"gmail"})
	ctx := context.Background()
	args := map[string]any{
		"args": []any{"drive", "ls"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := tool.Execute(ctx, args)
		if !result.IsError {
			b.Fatal("expected allowlist error in validate benchmark")
		}
	}
}

func BenchmarkGoogleCLITool_ExecuteFakeBinary(b *testing.B) {
	if runtime.GOOS == "windows" {
		b.Skip("fake gog helper uses POSIX shell script")
	}

	dir := b.TempDir()
	binary := filepath.Join(dir, "gog")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		b.Fatalf("failed writing fake gog: %v", err)
	}

	tool := NewGoogleCLITool(binary, 90, []string{"gmail"})
	ctx := context.Background()
	args := map[string]any{
		"args": []any{"gmail", "labels", "list"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := tool.Execute(ctx, args)
		if result.IsError {
			b.Fatalf("unexpected error: %s", result.ForLLM)
		}
	}
}
