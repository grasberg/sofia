package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ReadFileTool struct {
	fs               fileSystem
	stalenessTracker *FileStalenessTracker
}

func newFS(workspace string, restrict bool) fileSystem {
	if restrict {
		return &sandboxFs{workspace: workspace}
	}
	return &hostFs{}
}

func NewReadFileTool(workspace string, restrict bool) *ReadFileTool {
	return &ReadFileTool{fs: newFS(workspace, restrict)}
}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "Read the contents of a file"
}

func (t *ReadFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	path, ok := args["path"].(string)
	if !ok {
		return ErrorResult("path is required")
	}

	// If running under a goal, try goal folder first for relative paths.
	if gf := goalFolderForSession(ctx, workspaceFromFS(t.fs)); gf != "" && !filepath.IsAbs(path) {
		candidate := filepath.Join(gf, path)
		if _, err := os.Stat(candidate); err == nil {
			path = candidate
		}
	}

	content, err := t.fs.ReadFile(path)
	if err != nil {
		return ErrorResult(err.Error())
	}
	if t.stalenessTracker != nil {
		t.stalenessTracker.RecordRead(path)
	}
	return NewToolResult(string(content))
}

// SetStalenessTracker sets the file staleness tracker for read recording.
func (t *ReadFileTool) SetStalenessTracker(tracker *FileStalenessTracker) {
	t.stalenessTracker = tracker
}

type WriteFileTool struct {
	fs               fileSystem
	stalenessTracker *FileStalenessTracker
}

func NewWriteFileTool(workspace string, restrict bool) *WriteFileTool {
	return &WriteFileTool{fs: newFS(workspace, restrict)}
}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "Write content to a file"
}

func (t *WriteFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to write",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	path, ok := args["path"].(string)
	if !ok {
		return ErrorResult("path is required")
	}

	content, ok := args["content"].(string)
	if !ok {
		return ErrorResult("content is required")
	}

	// If running under a goal, redirect relative paths into the goal folder.
	if gf := goalFolderForSession(ctx, workspaceFromFS(t.fs)); gf != "" && !filepath.IsAbs(path) {
		path = filepath.Join(gf, path)
	}

	// File staleness check: warn if file was modified since last read
	if t.stalenessTracker != nil {
		if warning := t.stalenessTracker.CheckBeforeWrite(path); warning != "" {
			return NewToolResult(warning)
		}
	}

	// Try to read existing content for diff (best-effort; ignore error if file doesn't exist).
	existingBytes, existingErr := t.fs.ReadFile(path)

	if err := t.fs.WriteFile(path, []byte(content)); err != nil {
		return ErrorResult(err.Error())
	}

	if t.stalenessTracker != nil {
		t.stalenessTracker.UpdateAfterWrite(path)
	}

	bare := fmt.Sprintf("File written: %s", path)

	if existingErr == nil {
		// File existed: show unified diff of old vs new.
		if diff := buildUnifiedDiff(string(existingBytes), content, path); diff != "" {
			return SilentResult(bare + "\n\n" + diff)
		}
		return SilentResult(bare)
	}

	// New file: show all-additions representation.
	if content == "" {
		return SilentResult(bare)
	}
	newFileDiff := buildNewFileDiff(content, path)
	if newFileDiff != "" {
		return SilentResult(bare + "\n\n" + newFileDiff)
	}
	return SilentResult(bare)
}

// SetStalenessTracker sets the file staleness tracker for write checking.
func (t *WriteFileTool) SetStalenessTracker(tracker *FileStalenessTracker) {
	t.stalenessTracker = tracker
}

type ListDirTool struct {
	fs fileSystem
}

func NewListDirTool(workspace string, restrict bool) *ListDirTool {
	return &ListDirTool{fs: newFS(workspace, restrict)}
}

func (t *ListDirTool) Name() string {
	return "list_dir"
}

func (t *ListDirTool) Description() string {
	return "List files and directories in a path"
}

func (t *ListDirTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to list",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ListDirTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	path, ok := args["path"].(string)
	if !ok {
		path = "."
	}

	// If running under a goal, default listing to the goal folder.
	if gf := goalFolderForSession(ctx, workspaceFromFS(t.fs)); gf != "" && (path == "." || path == "") {
		path = gf
	} else if gf != "" && !filepath.IsAbs(path) {
		path = filepath.Join(gf, path)
	}

	entries, err := t.fs.ReadDir(path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read directory: %v", err))
	}
	return formatDirEntries(entries)
}

func formatDirEntries(entries []os.DirEntry) *ToolResult {
	var result strings.Builder
	for _, entry := range entries {
		if entry.IsDir() {
			result.WriteString("DIR:  " + entry.Name() + "\n")
		} else {
			result.WriteString("FILE: " + entry.Name() + "\n")
		}
	}
	return NewToolResult(result.String())
}
