package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobTool_Name(t *testing.T) {
	tool := NewGlobTool("/tmp", false)
	assert.Equal(t, "glob", tool.Name())
}

func TestGlobTool_Execute(t *testing.T) {
	// Create temp workspace with test files
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "pkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "app.go"), []byte("package src"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "pkg", "util.go"), []byte("package pkg"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# readme"), 0o644))

	tool := NewGlobTool(dir, false)

	t.Run("match go files", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]any{
			"pattern": "**/*.go",
		})
		assert.False(t, result.IsError)
		assert.Contains(t, result.ForLLM, "main.go")
		assert.Contains(t, result.ForLLM, "app.go")
		assert.Contains(t, result.ForLLM, "util.go")
		assert.NotContains(t, result.ForLLM, "README.md")
	})

	t.Run("match specific directory", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]any{
			"pattern": "src/**/*.go",
		})
		assert.False(t, result.IsError)
		assert.Contains(t, result.ForLLM, "app.go")
		assert.Contains(t, result.ForLLM, "util.go")
		assert.NotContains(t, result.ForLLM, "main.go")
	})

	t.Run("no matches", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]any{
			"pattern": "**/*.rs",
		})
		assert.False(t, result.IsError)
		assert.Contains(t, result.ForLLM, "No files match")
	})

	t.Run("missing pattern", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]any{})
		assert.True(t, result.IsError)
	})

	t.Run("limit results", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]any{
			"pattern": "**/*.go",
			"limit":   float64(1),
		})
		assert.False(t, result.IsError)
		assert.Contains(t, result.ForLLM, "(1 files found)")
	})
}

func TestDoublestarMatch(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "main.rs", false},
		{"**/*.go", "main.go", true},
		{"**/*.go", "src/main.go", true},
		{"**/*.go", "src/pkg/main.go", true},
		{"src/**/*.go", "src/main.go", true},
		{"src/**/*.go", "src/pkg/main.go", true},
		{"src/**/*.go", "main.go", false},
		{"**", "anything", true},
		{"**", "a/b/c", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.name, func(t *testing.T) {
			got, err := doublestarMatch(tt.pattern, tt.name)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
