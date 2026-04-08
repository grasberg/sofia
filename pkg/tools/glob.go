package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GlobTool provides pattern-based file finding using doublestar glob patterns.
// Supports ** for recursive directory matching (e.g., **/*.go, src/**/*.ts).
type GlobTool struct {
	workspace string
	restrict  bool
}

func NewGlobTool(workspace string, restrict bool) *GlobTool {
	return &GlobTool{workspace: workspace, restrict: restrict}
}

func (t *GlobTool) Name() string { return "glob" }
func (t *GlobTool) Description() string {
	return "Find files matching glob patterns. Supports ** for recursive matching (e.g., **/*.go, src/**/*.ts). Returns file paths sorted by modification time (newest first)."
}

func (t *GlobTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern to match files (e.g., **/*.go, src/**/*.ts, *.json)",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to search in. Defaults to workspace root.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return (default 100)",
			},
		},
		"required": []string{"pattern"},
	}
}

type globEntry struct {
	path    string
	modTime int64
}

func (t *GlobTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return ErrorResult("pattern is required")
	}

	searchDir := t.workspace
	if raw, ok := args["path"].(string); ok && raw != "" {
		searchDir = raw
	}

	if t.restrict {
		resolved, err := validatePath(searchDir, t.workspace, true)
		if err != nil {
			return ErrorResult(err.Error())
		}
		searchDir = resolved
	}

	limit := 100
	if raw, ok := args["limit"]; ok {
		if n, ok := parsePositiveInt(raw); ok {
			limit = n
		}
	}

	var entries []globEntry
	err := filepath.WalkDir(searchDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip hidden directories (but not hidden files if pattern explicitly matches them)
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
			return filepath.SkipDir
		}

		if d.IsDir() {
			return nil
		}

		// Get relative path for matching
		relPath, err := filepath.Rel(searchDir, path)
		if err != nil {
			return nil
		}

		matched, err := doublestarMatch(pattern, relPath)
		if err != nil {
			return nil
		}
		if !matched {
			return nil
		}

		modTime := int64(0)
		if info, err := d.Info(); err == nil {
			modTime = info.ModTime().UnixNano()
		}

		entries = append(entries, globEntry{path: relPath, modTime: modTime})
		return nil
	})
	if err != nil && err != context.Canceled {
		return ErrorResult(fmt.Sprintf("glob walk failed: %v", err))
	}

	// Sort by modification time (newest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].modTime > entries[j].modTime
	})

	if len(entries) > limit {
		entries = entries[:limit]
	}

	if len(entries) == 0 {
		return NewToolResult(fmt.Sprintf("No files match pattern %q in %s", pattern, searchDir))
	}

	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(e.path)
		sb.WriteByte('\n')
	}
	sb.WriteString(fmt.Sprintf("\n(%d files found)", len(entries)))

	return NewToolResult(sb.String())
}

// doublestarMatch implements ** glob matching in pure Go.
// Supports: *, ?, **, and character classes [abc].
func doublestarMatch(pattern, name string) (bool, error) {
	// Normalize separators
	pattern = filepath.ToSlash(pattern)
	name = filepath.ToSlash(name)

	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(patternParts, nameParts []string) (bool, error) {
	for len(patternParts) > 0 {
		if patternParts[0] == "**" {
			// ** can match zero or more path segments
			patternParts = patternParts[1:]
			if len(patternParts) == 0 {
				return true, nil // trailing ** matches everything
			}
			// Try matching the rest of pattern against every suffix of nameParts
			for i := 0; i <= len(nameParts); i++ {
				if matched, err := matchSegments(patternParts, nameParts[i:]); err != nil {
					return false, err
				} else if matched {
					return true, nil
				}
			}
			return false, nil
		}

		if len(nameParts) == 0 {
			return false, nil
		}

		matched, err := filepath.Match(patternParts[0], nameParts[0])
		if err != nil {
			return false, err
		}
		if !matched {
			return false, nil
		}

		patternParts = patternParts[1:]
		nameParts = nameParts[1:]
	}

	return len(nameParts) == 0, nil
}
