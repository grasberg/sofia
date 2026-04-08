package tools

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

const diffMaxLines = 100

// buildUnifiedDiff returns a unified diff string for original→modified content,
// capped at diffMaxLines total lines. Returns "" if there is no diff.
func buildUnifiedDiff(original, modified, path string) string {
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(original),
		B:        difflib.SplitLines(modified),
		FromFile: "a/" + path,
		ToFile:   "b/" + path,
		Context:  3,
	})
	if err != nil || diff == "" {
		return ""
	}
	return capDiffLines(diff)
}

// buildNewFileDiff returns a simple all-additions diff representation for a new file,
// capped at diffMaxLines total lines.
func buildNewFileDiff(content, path string) string {
	if content == "" {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "+++ (new file) %s\n", path)
	for _, line := range strings.SplitAfter(content, "\n") {
		if line != "" {
			sb.WriteString("+" + line)
		}
	}
	return capDiffLines(sb.String())
}

// capDiffLines truncates a diff string to diffMaxLines lines with a trailer if needed.
func capDiffLines(diff string) string {
	lines := strings.Split(diff, "\n")
	// Strip trailing empty element produced by a terminal "\n"
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= diffMaxLines {
		return diff
	}
	omitted := len(lines) - diffMaxLines
	truncated := strings.Join(lines[:diffMaxLines], "\n")
	return truncated + fmt.Sprintf("\n... %d more lines omitted", omitted)
}

// EditFileTool edits a file by replacing old_text with new_text.
// The old_text must exist exactly in the file.
type EditFileTool struct {
	fs               fileSystem
	stalenessTracker *FileStalenessTracker
}

// NewEditFileTool creates a new EditFileTool with optional directory restriction.
func NewEditFileTool(workspace string, restrict bool) *EditFileTool {
	var fs fileSystem
	if restrict {
		fs = &sandboxFs{workspace: workspace}
	} else {
		fs = &hostFs{}
	}
	return &EditFileTool{fs: fs}
}

func (t *EditFileTool) Name() string {
	return "edit_file"
}

func (t *EditFileTool) Description() string {
	return "Edit a file by replacing old_text with new_text. The old_text must exist exactly in the file."
}

func (t *EditFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The file path to edit",
			},
			"old_text": map[string]any{
				"type":        "string",
				"description": "The exact text to find and replace",
			},
			"new_text": map[string]any{
				"type":        "string",
				"description": "The text to replace with",
			},
		},
		"required": []string{"path", "old_text", "new_text"},
	}
}

func (t *EditFileTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	path, ok := args["path"].(string)
	if !ok {
		return ErrorResult("path is required")
	}

	oldText, ok := args["old_text"].(string)
	if !ok {
		return ErrorResult("old_text is required")
	}

	newText, ok := args["new_text"].(string)
	if !ok {
		return ErrorResult("new_text is required")
	}

	// If running under a goal, redirect relative paths into the goal folder.
	if gf := goalFolderForSession(ctx, workspaceFromFS(t.fs)); gf != "" && !filepath.IsAbs(path) {
		path = filepath.Join(gf, path)
	}

	if t.stalenessTracker != nil {
		if warning := t.stalenessTracker.CheckBeforeWrite(path); warning != "" {
			return NewToolResult(warning)
		}
	}

	// Read original content for diff generation (best-effort; failures fall back to bare result).
	originalBytes, readErr := t.fs.ReadFile(path)

	if err := editFile(t.fs, path, oldText, newText); err != nil {
		return ErrorResult(err.Error())
	}

	if t.stalenessTracker != nil {
		t.stalenessTracker.UpdateAfterWrite(path)
	}

	bare := fmt.Sprintf("File edited: %s", path)
	if readErr != nil {
		return SilentResult(bare)
	}
	originalContent := string(originalBytes)
	modifiedContent := strings.Replace(originalContent, oldText, newText, 1)
	if diff := buildUnifiedDiff(originalContent, modifiedContent, path); diff != "" {
		return SilentResult(bare + "\n\n" + diff)
	}
	return SilentResult(bare)
}

// SetStalenessTracker sets the file staleness tracker for write checking.
func (t *EditFileTool) SetStalenessTracker(tracker *FileStalenessTracker) {
	t.stalenessTracker = tracker
}

type AppendFileTool struct {
	fs               fileSystem
	stalenessTracker *FileStalenessTracker
}

func NewAppendFileTool(workspace string, restrict bool) *AppendFileTool {
	var fs fileSystem
	if restrict {
		fs = &sandboxFs{workspace: workspace}
	} else {
		fs = &hostFs{}
	}
	return &AppendFileTool{fs: fs}
}

func (t *AppendFileTool) Name() string {
	return "append_file"
}

func (t *AppendFileTool) Description() string {
	return "Append content to the end of a file"
}

func (t *AppendFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The file path to append to",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The content to append",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *AppendFileTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
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

	if t.stalenessTracker != nil {
		if warning := t.stalenessTracker.CheckBeforeWrite(path); warning != "" {
			return NewToolResult(warning)
		}
	}

	if err := appendFile(t.fs, path, content); err != nil {
		return ErrorResult(err.Error())
	}

	if t.stalenessTracker != nil {
		t.stalenessTracker.UpdateAfterWrite(path)
	}

	return SilentResult(fmt.Sprintf("Appended to %s", path))
}

// SetStalenessTracker sets the file staleness tracker for write checking.
func (t *AppendFileTool) SetStalenessTracker(tracker *FileStalenessTracker) {
	t.stalenessTracker = tracker
}

// editFile reads the file via sysFs, performs the replacement, and writes back.
// It uses a fileSystem interface, allowing the same logic for both restricted and unrestricted modes.
func editFile(sysFs fileSystem, path, oldText, newText string) error {
	content, err := sysFs.ReadFile(path)
	if err != nil {
		return err
	}

	newContent, err := replaceEditContent(content, oldText, newText)
	if err != nil {
		return err
	}

	return sysFs.WriteFile(path, newContent)
}

// appendFile reads the existing content (if any) via sysFs, appends new content, and writes back.
func appendFile(sysFs fileSystem, path, appendContent string) error {
	content, err := sysFs.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	newContent := append(content, []byte(appendContent)...)
	return sysFs.WriteFile(path, newContent)
}

// replaceEditContent handles the core logic of finding and replacing a single occurrence of oldText.
func replaceEditContent(content []byte, oldText, newText string) ([]byte, error) {
	contentStr := string(content)

	if !strings.Contains(contentStr, oldText) {
		return nil, fmt.Errorf("old_text not found in file. Make sure it matches exactly")
	}

	count := strings.Count(contentStr, oldText)
	if count > 1 {
		return nil, fmt.Errorf("old_text appears %d times. Please provide more context to make it unique", count)
	}

	newContent := strings.Replace(contentStr, oldText, newText, 1)
	return []byte(newContent), nil
}
