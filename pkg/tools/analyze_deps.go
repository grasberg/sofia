package tools

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
)

// depPatterns holds compiled regex patterns for dependency extraction per language.
var depPatterns = map[string][]*regexp.Regexp{
	"js": {
		regexp.MustCompile(`(?:import\s+.*?\s+from\s+['"](.+?)['"]|require\s*\(\s*['"](.+?)['"]\s*\))`),
	},
	"ts": {
		regexp.MustCompile(`(?:import\s+.*?\s+from\s+['"](.+?)['"]|require\s*\(\s*['"](.+?)['"]\s*\))`),
	},
	"python": {
		regexp.MustCompile(`^from\s+(\S+)\s+import`),
		regexp.MustCompile(`^import\s+(\S+)`),
	},
}

func (t *AnalyzeTool) dependencies(path, language string) *ToolResult {
	info, err := os.Stat(path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("cannot access path: %v", err))
	}
	if info.IsDir() {
		return ErrorResult("dependencies requires a file path, not a directory")
	}

	lang := detectLanguage(path, language)
	if lang == "" {
		return ErrorResult("unable to detect language; provide a language hint")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("cannot read file: %v", err))
	}

	var result string
	if lang == "go" {
		result, err = extractGoDeps(path, content)
		if err != nil {
			return ErrorResult(fmt.Sprintf("Go AST parse error: %v", err))
		}
	} else {
		result = extractRegexDeps(string(content), lang)
	}

	if result == "" {
		return NewToolResult("No dependencies found.")
	}
	return NewToolResult(result)
}

func extractGoDeps(path string, content []byte) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, content, parser.ImportsOnly)
	if err != nil {
		return "", err
	}

	if len(file.Imports) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("Imports:\n")
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if imp.Name != nil {
			sb.WriteString(fmt.Sprintf("  %s %s\n", imp.Name.Name, importPath))
		} else {
			sb.WriteString(fmt.Sprintf("  %s\n", importPath))
		}
	}
	return sb.String(), nil
}

func extractRegexDeps(content, lang string) string {
	patterns, ok := depPatterns[lang]
	if !ok {
		return ""
	}

	seen := make(map[string]bool)
	var deps []string

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, pat := range patterns {
			matches := pat.FindStringSubmatch(trimmed)
			if len(matches) < 2 {
				continue
			}
			// Pick the first non-empty capture group.
			for _, m := range matches[1:] {
				if m != "" && !seen[m] {
					seen[m] = true
					deps = append(deps, m)
					break
				}
			}
		}
	}

	if len(deps) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Dependencies:\n")
	for _, d := range deps {
		sb.WriteString(fmt.Sprintf("  %s\n", d))
	}
	return sb.String()
}
