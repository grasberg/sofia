package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockFetch(_ context.Context, url string) (string, error) {
	return fmt.Sprintf("content from %s", url), nil
}

func TestEnrichMessageContent_FileRef(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "hello.txt")
	if err := os.WriteFile(f, []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}

	msg := fmt.Sprintf("check this out @%s", f)
	result := enrichMessageContent(context.Background(), msg, tmp, mockFetch)

	if !strings.Contains(result, "hello world") {
		t.Errorf("expected file content in result, got: %s", result)
	}
	if strings.Contains(result, "@"+f) {
		t.Errorf("expected @ref to be replaced, got: %s", result)
	}
}

func TestEnrichMessageContent_RelativeRef(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "notes.txt")
	if err := os.WriteFile(f, []byte("relative content"), 0o600); err != nil {
		t.Fatal(err)
	}

	msg := "check @./notes.txt please"
	result := enrichMessageContent(context.Background(), msg, tmp, mockFetch)

	if !strings.Contains(result, "relative content") {
		t.Errorf("expected file content in result, got: %s", result)
	}
}

func TestEnrichMessageContent_PathTraversal(t *testing.T) {
	tmp := t.TempDir()

	msg := "bad @../../etc/passwd"
	result := enrichMessageContent(context.Background(), msg, tmp, mockFetch)

	// The token should be left unchanged — traversal blocked
	if !strings.Contains(result, "@../../etc/passwd") {
		t.Errorf("expected path traversal token to be preserved, got: %s", result)
	}
	if strings.Contains(result, "root") {
		t.Errorf("should not have read /etc/passwd, got: %s", result)
	}
}

func TestEnrichMessageContent_URLRef(t *testing.T) {
	fetchCalled := false
	fetch := func(ctx context.Context, url string) (string, error) {
		fetchCalled = true
		if url != "https://example.com" {
			return "", fmt.Errorf("unexpected URL: %s", url)
		}
		return "example page content", nil
	}

	msg := "read @https://example.com for me"
	result := enrichMessageContent(context.Background(), msg, "", fetch)

	if !fetchCalled {
		t.Error("expected fetchURL to be called")
	}
	if !strings.Contains(result, "example page content") {
		t.Errorf("expected URL content in result, got: %s", result)
	}
}

func TestEnrichMessageContent_MaxRefs(t *testing.T) {
	tmp := t.TempDir()

	// Create 6 files
	for i := 1; i <= 6; i++ {
		f := filepath.Join(tmp, fmt.Sprintf("f%d.txt", i))
		if err := os.WriteFile(f, []byte(fmt.Sprintf("file%d", i)), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var parts []string
	for i := 1; i <= 6; i++ {
		parts = append(parts, fmt.Sprintf("@%s", filepath.Join(tmp, fmt.Sprintf("f%d.txt", i))))
	}
	msg := strings.Join(parts, " ")

	result := enrichMessageContent(context.Background(), msg, tmp, mockFetch)

	// Count expanded blocks — each expanded ref produces a ``` block
	expanded := strings.Count(result, "```")
	// 5 refs expanded → 10 backtick-fence occurrences (open+close each)
	if expanded != 10 {
		t.Errorf("expected 5 expanded refs (10 fence occurrences), got %d in: %s", expanded, result)
	}

	// The 6th file's content must NOT appear
	if strings.Contains(result, "file6") {
		t.Errorf("6th ref should not be expanded, got: %s", result)
	}
}

func TestEnrichMessageContent_Truncation(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "big.txt")

	// Write 51KB of data
	data := strings.Repeat("A", maxAtRefBytes+1024)
	if err := os.WriteFile(f, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	msg := fmt.Sprintf("@%s", f)
	result := enrichMessageContent(context.Background(), msg, tmp, mockFetch)

	if !strings.Contains(result, "... truncated") {
		t.Errorf("expected truncation marker, got: %s", result[:200])
	}

	// is_utf8 variant: write a file with multi-byte UTF-8 content (emoji-heavy)
	f2 := filepath.Join(tmp, "emoji.txt")
	emoji := strings.Repeat("🎉", (maxAtRefBytes/4)+100) // each emoji is 4 bytes
	if err := os.WriteFile(f2, []byte(emoji), 0o600); err != nil {
		t.Fatal(err)
	}
	msg2 := fmt.Sprintf("@%s", f2)
	result2 := enrichMessageContent(context.Background(), msg2, tmp, mockFetch)
	assert.True(t, utf8.ValidString(result2), "truncated result should be valid UTF-8")
	assert.Contains(t, result2, "... truncated")
}

func TestEnrichMessageContent_SiblingDirectoryTraversal(t *testing.T) {
	dir := t.TempDir()
	// Create a sibling directory with a secret file
	siblingDir := dir + "-evil"
	require.NoError(t, os.MkdirAll(siblingDir, 0o755))
	secretFile := filepath.Join(siblingDir, "secret.key")
	require.NoError(t, os.WriteFile(secretFile, []byte("secret"), 0o644))

	content := "@" + secretFile
	result := enrichMessageContent(context.Background(), content, dir, mockFetch)
	assert.Equal(t, content, result, "sibling directory escape should be blocked")

	os.RemoveAll(siblingDir)
}

func TestEnrichMessageContent_NoRefs(t *testing.T) {
	original := "hello, how are you?"
	result := enrichMessageContent(context.Background(), original, "/some/workspace", mockFetch)

	if result != original {
		t.Errorf("expected content unchanged, got: %s", result)
	}
}
