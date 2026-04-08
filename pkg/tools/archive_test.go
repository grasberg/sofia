package tools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveTool_Name(t *testing.T) {
	tool := NewArchiveTool("/tmp", false)
	assert.Equal(t, "archive", tool.Name())
}

func TestArchiveTool_ZipRoundtrip(t *testing.T) {
	dir := t.TempDir()

	// Create source files
	srcDir := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file2.txt"), []byte("world"), 0o644))

	tool := NewArchiveTool(dir, false)
	archivePath := filepath.Join(dir, "test.zip")

	// Create zip
	result := tool.Execute(t.Context(), map[string]any{
		"action":       "create",
		"archive_path": archivePath,
		"format":       "zip",
		"files":        []any{srcDir},
	})
	assert.False(t, result.IsError, result.ForLLM)

	// List zip
	result = tool.Execute(t.Context(), map[string]any{
		"action":       "list",
		"archive_path": archivePath,
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "file1.txt")
	assert.Contains(t, result.ForLLM, "file2.txt")

	// Extract zip
	extractDir := filepath.Join(dir, "extracted")
	result = tool.Execute(t.Context(), map[string]any{
		"action":       "extract",
		"archive_path": archivePath,
		"dest":         extractDir,
	})
	assert.False(t, result.IsError)
}

func TestArchiveTool_DetectFormat(t *testing.T) {
	assert.Equal(t, "zip", detectArchiveFormat("file.zip"))
	assert.Equal(t, "tar", detectArchiveFormat("file.tar"))
	assert.Equal(t, "tar.gz", detectArchiveFormat("file.tar.gz"))
	assert.Equal(t, "tar.gz", detectArchiveFormat("file.tgz"))
	assert.Equal(t, "zip", detectArchiveFormat("unknown.xyz"))
}
