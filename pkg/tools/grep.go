package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// GrepTool provides structured code search, preferring ripgrep (rg) when available
// with a pure Go regexp fallback.
type GrepTool struct {
	workspace string
	restrict  bool
}

func NewGrepTool(workspace string, restrict bool) *GrepTool {
	return &GrepTool{workspace: workspace, restrict: restrict}
}

func (t *GrepTool) Name() string { return "grep" }
func (t *GrepTool) Description() string {
	return "Search file contents using regex patterns. Uses ripgrep (rg) when available for speed, falls back to pure Go. Returns matching lines with file paths and line numbers."
}

func (t *GrepTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Regex pattern to search for",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File or directory to search in. Defaults to workspace root.",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "File glob filter (e.g., *.go, *.ts). Maps to rg --glob.",
			},
			"case_insensitive": map[string]any{
				"type":        "boolean",
				"description": "Case insensitive search (default false)",
			},
			"context_lines": map[string]any{
				"type":        "integer",
				"description": "Number of context lines before and after each match (default 0)",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of matching lines to return (default 100)",
			},
			"files_only": map[string]any{
				"type":        "boolean",
				"description": "Only return file paths that contain matches (default false)",
			},
		},
		"required": []string{"pattern"},
	}
}

func (t *GrepTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return ErrorResult("pattern is required")
	}

	searchPath := t.workspace
	if raw, ok := args["path"].(string); ok && raw != "" {
		searchPath = raw
	}

	if t.restrict {
		resolved, err := validatePath(searchPath, t.workspace, true)
		if err != nil {
			return ErrorResult(err.Error())
		}
		searchPath = resolved
	}

	globFilter := ""
	if raw, ok := args["glob"].(string); ok {
		globFilter = raw
	}

	caseInsensitive := false
	if raw, ok := args["case_insensitive"].(bool); ok {
		caseInsensitive = raw
	}

	contextLines := 0
	if raw, ok := args["context_lines"]; ok {
		if n, ok := parsePositiveInt(raw); ok {
			contextLines = n
		}
	}

	maxResults := 100
	if raw, ok := args["max_results"]; ok {
		if n, ok := parsePositiveInt(raw); ok {
			maxResults = n
		}
	}

	filesOnly := false
	if raw, ok := args["files_only"].(bool); ok {
		filesOnly = raw
	}

	// Try ripgrep first, fall back to Go implementation
	if rgPath, err := exec.LookPath("rg"); err == nil {
		return t.executeRipgrep(
			ctx,
			rgPath,
			pattern,
			searchPath,
			globFilter,
			caseInsensitive,
			contextLines,
			maxResults,
			filesOnly,
		)
	}
	return t.executeGoGrep(ctx, pattern, searchPath, globFilter, caseInsensitive, contextLines, maxResults, filesOnly)
}

func (t *GrepTool) executeRipgrep(
	ctx context.Context, rgPath, pattern, searchPath, globFilter string,
	caseInsensitive bool, contextLines, maxResults int, filesOnly bool,
) *ToolResult {
	rgArgs := []string{"--no-heading", "--line-number", "--color", "never"}

	if filesOnly {
		rgArgs = append(rgArgs, "--files-with-matches")
	}
	if caseInsensitive {
		rgArgs = append(rgArgs, "-i")
	}
	if contextLines > 0 {
		rgArgs = append(rgArgs, "-C", fmt.Sprintf("%d", contextLines))
	}
	if globFilter != "" {
		rgArgs = append(rgArgs, "--glob", globFilter)
	}
	rgArgs = append(rgArgs, fmt.Sprintf("--max-count=%d", maxResults))
	rgArgs = append(rgArgs, pattern, searchPath)

	result := ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  rgPath,
		Args:        rgArgs,
		Timeout:     30 * time.Second,
		ToolName:    "rg",
		InstallHint: "Install ripgrep: brew install ripgrep",
	})

	// rg returns exit code 1 for "no matches" — not an error
	if result.IsError && strings.Contains(result.ForLLM, "Exit error: exit status 1") {
		return NewToolResult("No matches found.")
	}
	return result
}

type grepMatch struct {
	file string
	line int
	text string
}

func (t *GrepTool) executeGoGrep(
	ctx context.Context, pattern, searchPath, globFilter string,
	caseInsensitive bool, contextLines, maxResults int, filesOnly bool,
) *ToolResult {
	flags := ""
	if caseInsensitive {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + pattern)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid regex: %v", err))
	}

	var matches []grepMatch
	matchedFiles := make(map[string]bool)

	err = filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Apply glob filter
		if globFilter != "" {
			matched, _ := filepath.Match(globFilter, d.Name())
			if !matched {
				return nil
			}
		}

		// Skip binary files (check first 512 bytes)
		if isBinaryFile(path) {
			return nil
		}

		relPath, _ := filepath.Rel(searchPath, path)
		if relPath == "" {
			relPath = path
		}

		if filesOnly {
			if matchedFiles[relPath] {
				return nil
			}
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if re.MatchString(line) {
				if filesOnly {
					matchedFiles[relPath] = true
					matches = append(matches, grepMatch{file: relPath})
					break
				}
				matches = append(matches, grepMatch{file: relPath, line: lineNum, text: line})
				if len(matches) >= maxResults {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil && err != context.Canceled && err != filepath.SkipAll {
		return ErrorResult(fmt.Sprintf("grep walk failed: %v", err))
	}

	if len(matches) == 0 {
		return NewToolResult("No matches found.")
	}

	var sb strings.Builder
	if filesOnly {
		for _, m := range matches {
			sb.WriteString(m.file)
			sb.WriteByte('\n')
		}
	} else {
		for _, m := range matches {
			sb.WriteString(fmt.Sprintf("%s:%d: %s\n", m.file, m.line, m.text))
		}
	}
	sb.WriteString(fmt.Sprintf("\n(%d matches)", len(matches)))
	return NewToolResult(sb.String())
}

// isBinaryFile checks if a file appears to be binary by reading the first 512 bytes.
func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return false
	}
	for _, b := range buf[:n] {
		if b == 0 {
			return true
		}
	}
	return false
}

// rgJSONMatch represents a ripgrep JSON match output line.
type rgJSONMatch struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}
