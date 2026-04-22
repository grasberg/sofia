package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// AnalyzeTool provides structural code understanding by extracting symbols,
// dependencies, and outlines from source files. For Go files it uses go/ast;
// for other languages it falls back to regex-based pattern matching.
type AnalyzeTool struct{}

func NewAnalyzeTool() *AnalyzeTool {
	return &AnalyzeTool{}
}

func (t *AnalyzeTool) Name() string { return "analyze" }

func (t *AnalyzeTool) Description() string {
	return "Analyze code structure: extract symbols, dependencies, outlines, and directory overviews. Supports Go (via AST), JS/TS, Python, Java, Rust, and Ruby (via regex)."
}

func (t *AnalyzeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Analysis action to perform",
				"enum":        []string{"overview", "symbols", "dependencies", "outline"},
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File or directory path to analyze",
			},
			"language": map[string]any{
				"type":        "string",
				"description": "Language hint (go, js, ts, python, java, rust, ruby). Auto-detected from extension if omitted.",
			},
			"depth": map[string]any{
				"type":        "number",
				"description": "Recursion depth for overview action (default 2)",
			},
		},
		"required": []string{"action", "path"},
	}
}

func (t *AnalyzeTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	action, ok := args["action"].(string)
	if !ok || action == "" {
		return ErrorResult("action is required")
	}

	path, ok := args["path"].(string)
	if !ok || path == "" {
		return ErrorResult("path is required")
	}

	language, _ := args["language"].(string)

	depth := 2
	if d, ok := args["depth"].(float64); ok && d > 0 {
		depth = int(d)
	}

	switch action {
	case "overview":
		return t.overview(path, depth)
	case "symbols":
		return t.symbols(path, language)
	case "dependencies":
		return t.dependencies(path, language)
	case "outline":
		return t.outline(path, language)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

// extensionToLang maps file extensions to language identifiers.
var extensionToLang = map[string]string{
	".go":   "go",
	".js":   "js",
	".jsx":  "js",
	".mjs":  "js",
	".cjs":  "js",
	".ts":   "ts",
	".tsx":  "ts",
	".mts":  "ts",
	".py":   "python",
	".pyw":  "python",
	".java": "java",
	".rs":   "rust",
	".rb":   "ruby",
}

func detectLanguage(path, hint string) string {
	if hint != "" {
		return hint
	}
	ext := strings.ToLower(filepath.Ext(path))
	return extensionToLang[ext]
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
