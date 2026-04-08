package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrepTool_Name(t *testing.T) {
	tool := NewGrepTool("/tmp", false)
	assert.Equal(t, "grep", tool.Name())
}

func TestGrepTool_GoFallback(t *testing.T) {
	dir := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(dir, "hello.go"),
			[]byte("package main\nfunc Hello() string {\n\treturn \"hello world\"\n}\n"),
			0o644,
		),
	)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("no match here\n"), 0o644))

	tool := NewGrepTool(dir, false)

	t.Run("basic pattern match", func(t *testing.T) {
		result := tool.executeGoGrep(context.Background(), "Hello", dir, "", false, 0, 100, false)
		assert.False(t, result.IsError)
		assert.Contains(t, result.ForLLM, "hello.go")
		assert.Contains(t, result.ForLLM, "Hello")
	})

	t.Run("case insensitive", func(t *testing.T) {
		result := tool.executeGoGrep(context.Background(), "hello", dir, "", true, 0, 100, false)
		assert.False(t, result.IsError)
		assert.Contains(t, result.ForLLM, "hello.go")
	})

	t.Run("files only mode", func(t *testing.T) {
		result := tool.executeGoGrep(context.Background(), "Hello", dir, "", false, 0, 100, true)
		assert.False(t, result.IsError)
		assert.Contains(t, result.ForLLM, "hello.go")
		assert.NotContains(t, result.ForLLM, "func Hello")
	})

	t.Run("glob filter", func(t *testing.T) {
		result := tool.executeGoGrep(context.Background(), "no match", dir, "*.txt", false, 0, 100, false)
		assert.False(t, result.IsError)
		assert.Contains(t, result.ForLLM, "test.txt")
	})

	t.Run("no matches", func(t *testing.T) {
		result := tool.executeGoGrep(context.Background(), "nonexistent_string_xyz", dir, "", false, 0, 100, false)
		assert.False(t, result.IsError)
		assert.Contains(t, result.ForLLM, "No matches found")
	})

	t.Run("invalid regex", func(t *testing.T) {
		result := tool.executeGoGrep(context.Background(), "[invalid", dir, "", false, 0, 100, false)
		assert.True(t, result.IsError)
		assert.Contains(t, result.ForLLM, "invalid regex")
	})
}

func TestIsBinaryFile(t *testing.T) {
	dir := t.TempDir()

	// Text file
	textFile := filepath.Join(dir, "text.txt")
	require.NoError(t, os.WriteFile(textFile, []byte("hello world"), 0o644))
	assert.False(t, isBinaryFile(textFile))

	// Binary file (contains null byte)
	binFile := filepath.Join(dir, "binary.bin")
	require.NoError(t, os.WriteFile(binFile, []byte{0x00, 0x01, 0x02}, 0o644))
	assert.True(t, isBinaryFile(binFile))
}
