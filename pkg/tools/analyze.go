package tools

import (
	"bufio"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
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

// ---------------------------------------------------------------------------
// overview
// ---------------------------------------------------------------------------

func (t *AnalyzeTool) overview(root string, maxDepth int) *ToolResult {
	info, err := os.Stat(root)
	if err != nil {
		return ErrorResult(fmt.Sprintf("cannot access path: %v", err))
	}
	if !info.IsDir() {
		return ErrorResult("overview requires a directory path")
	}

	langCounts := make(map[string]int)
	totalLOC := 0
	var tree strings.Builder

	err = walkDir(root, root, 0, maxDepth, &tree, langCounts, &totalLOC)
	if err != nil {
		return ErrorResult(fmt.Sprintf("error walking directory: %v", err))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Directory: %s\n\n", root))
	sb.WriteString("Tree:\n")
	sb.WriteString(tree.String())
	sb.WriteString("\nFiles by language:\n")

	langs := sortedKeys(langCounts)
	for _, lang := range langs {
		sb.WriteString(fmt.Sprintf("  %-12s %d files\n", lang, langCounts[lang]))
	}
	sb.WriteString(fmt.Sprintf("\nTotal LOC: %d\n", totalLOC))

	return NewToolResult(sb.String())
}

func walkDir(
	dir, root string,
	depth, maxDepth int,
	tree *strings.Builder,
	langCounts map[string]int,
	totalLOC *int,
) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	indent := strings.Repeat("  ", depth)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
			continue
		}

		fullPath := filepath.Join(dir, name)

		if entry.IsDir() {
			tree.WriteString(fmt.Sprintf("%s%s/\n", indent, name))
			if depth < maxDepth {
				if err := walkDir(fullPath, root, depth+1, maxDepth, tree, langCounts, totalLOC); err != nil {
					return err
				}
			}
		} else {
			tree.WriteString(fmt.Sprintf("%s%s\n", indent, name))
			lang := detectLanguage(name, "")
			if lang != "" {
				langCounts[lang]++
				loc, _ := countLines(fullPath)
				*totalLOC += loc
			}
		}
	}
	return nil
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

// ---------------------------------------------------------------------------
// symbols
// ---------------------------------------------------------------------------

func (t *AnalyzeTool) symbols(path, language string) *ToolResult {
	info, err := os.Stat(path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("cannot access path: %v", err))
	}
	if info.IsDir() {
		return ErrorResult("symbols requires a file path, not a directory")
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
		result, err = extractGoSymbols(path, content)
		if err != nil {
			return ErrorResult(fmt.Sprintf("Go AST parse error: %v", err))
		}
	} else {
		result = extractRegexSymbols(string(content), lang)
	}

	if result == "" {
		return NewToolResult("No symbols found.")
	}
	return NewToolResult(result)
}

func extractGoSymbols(path string, content []byte) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Package: %s\n\n", file.Name.Name))

	// Types (structs, interfaces)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			pos := fset.Position(ts.Pos())
			switch ts.Type.(type) {
			case *ast.InterfaceType:
				sb.WriteString(fmt.Sprintf("interface %s (line %d)\n", ts.Name.Name, pos.Line))
			case *ast.StructType:
				sb.WriteString(fmt.Sprintf("struct %s (line %d)\n", ts.Name.Name, pos.Line))
			default:
				sb.WriteString(fmt.Sprintf("type %s (line %d)\n", ts.Name.Name, pos.Line))
			}
		}
	}

	// Functions and methods
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		pos := fset.Position(funcDecl.Pos())
		if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
			recv := exprString(funcDecl.Recv.List[0].Type)
			sb.WriteString(fmt.Sprintf("method (%s) %s (line %d)\n", recv, funcDecl.Name.Name, pos.Line))
		} else {
			sb.WriteString(fmt.Sprintf("func %s (line %d)\n", funcDecl.Name.Name, pos.Line))
		}
	}

	return sb.String(), nil
}

func exprString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + exprString(e.X)
	case *ast.SelectorExpr:
		return exprString(e.X) + "." + e.Sel.Name
	default:
		return "?"
	}
}

// langPatterns holds compiled regex patterns for symbol extraction per language.
var langPatterns = map[string][]*symbolPattern{
	"js": {
		{regexp.MustCompile(`(?:export\s+)?(?:async\s+)?function\s+(\w+)`), "func"},
		{regexp.MustCompile(`(?:export\s+)?class\s+(\w+)`), "class"},
		{regexp.MustCompile(`(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?\(`), "func"},
	},
	"ts": {
		{regexp.MustCompile(`(?:export\s+)?(?:async\s+)?function\s+(\w+)`), "func"},
		{regexp.MustCompile(`(?:export\s+)?class\s+(\w+)`), "class"},
		{regexp.MustCompile(`(?:export\s+)?interface\s+(\w+)`), "interface"},
		{regexp.MustCompile(`(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?\(`), "func"},
	},
	"python": {
		{regexp.MustCompile(`(?:async\s+)?def\s+(\w+)`), "func"},
		{regexp.MustCompile(`class\s+(\w+)`), "class"},
	},
	"java": {
		{regexp.MustCompile(`(?:public|private|protected)?\s*(?:static\s+)?(?:class|interface|enum)\s+(\w+)`), "type"},
		{regexp.MustCompile(`(?:public|private|protected)?\s*(?:static\s+)?[\w<>\[\]]+\s+(\w+)\s*\(`), "method"},
	},
	"rust": {
		{regexp.MustCompile(`(?:pub\s+)?fn\s+(\w+)`), "func"},
		{regexp.MustCompile(`(?:pub\s+)?struct\s+(\w+)`), "struct"},
		{regexp.MustCompile(`(?:pub\s+)?enum\s+(\w+)`), "enum"},
		{regexp.MustCompile(`(?:pub\s+)?trait\s+(\w+)`), "trait"},
		{regexp.MustCompile(`impl\s+(\w+)`), "impl"},
		{regexp.MustCompile(`(?:pub\s+)?type\s+(\w+)`), "type"},
		{regexp.MustCompile(`(?:pub\s+)?mod\s+(\w+)`), "mod"},
	},
	"ruby": {
		{regexp.MustCompile(`def\s+(\w+)`), "method"},
		{regexp.MustCompile(`class\s+(\w+)`), "class"},
		{regexp.MustCompile(`module\s+(\w+)`), "module"},
	},
}

type symbolPattern struct {
	re   *regexp.Regexp
	kind string
}

func extractRegexSymbols(content, lang string) string {
	patterns, ok := langPatterns[lang]
	if !ok {
		return ""
	}

	var sb strings.Builder
	lines := strings.Split(content, "\n")
	for lineNum, line := range lines {
		for _, pat := range patterns {
			matches := pat.re.FindStringSubmatch(line)
			if len(matches) >= 2 {
				sb.WriteString(fmt.Sprintf("%s %s (line %d)\n", pat.kind, matches[1], lineNum+1))
			}
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// dependencies
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// outline
// ---------------------------------------------------------------------------

func (t *AnalyzeTool) outline(path, language string) *ToolResult {
	info, err := os.Stat(path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("cannot access path: %v", err))
	}
	if info.IsDir() {
		return ErrorResult("outline requires a file path, not a directory")
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
		result, err = outlineGo(path, content)
		if err != nil {
			return ErrorResult(fmt.Sprintf("Go AST parse error: %v", err))
		}
	} else {
		result = outlineRegex(string(content), lang)
	}

	if result == "" {
		return NewToolResult("No outline entries found.")
	}
	return NewToolResult(result)
}

func outlineGo(path string, content []byte) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("package %s\n\n", file.Name.Name))

	if len(file.Imports) > 0 {
		sb.WriteString(fmt.Sprintf("imports (%d)\n", len(file.Imports)))
	}

	// Collect types with their methods.
	typeMethods := make(map[string][]string)

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					pos := fset.Position(ts.Pos())
					kind := "type"
					switch ts.Type.(type) {
					case *ast.InterfaceType:
						kind = "interface"
					case *ast.StructType:
						kind = "struct"
					}
					sb.WriteString(fmt.Sprintf("  %s %s (L%d)\n", kind, ts.Name.Name, pos.Line))
				}
			}
		case *ast.FuncDecl:
			pos := fset.Position(d.Pos())
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recv := exprString(d.Recv.List[0].Type)
				typeMethods[recv] = append(typeMethods[recv], fmt.Sprintf("    .%s (L%d)", d.Name.Name, pos.Line))
			} else {
				sb.WriteString(fmt.Sprintf("  func %s (L%d)\n", d.Name.Name, pos.Line))
			}
		}
	}

	// Append methods grouped by receiver.
	if len(typeMethods) > 0 {
		sb.WriteString("\nMethods:\n")
		for recv, methods := range typeMethods {
			sb.WriteString(fmt.Sprintf("  %s\n", recv))
			for _, m := range methods {
				sb.WriteString(m + "\n")
			}
		}
	}

	return sb.String(), nil
}

func outlineRegex(content, lang string) string {
	patterns, ok := langPatterns[lang]
	if !ok {
		return ""
	}

	var sb strings.Builder
	lines := strings.Split(content, "\n")
	for lineNum, line := range lines {
		for _, pat := range patterns {
			matches := pat.re.FindStringSubmatch(line)
			if len(matches) >= 2 {
				sb.WriteString(fmt.Sprintf("  %s %s (L%d)\n", pat.kind, matches[1], lineNum+1))
			}
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

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
