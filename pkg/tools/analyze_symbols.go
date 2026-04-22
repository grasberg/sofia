package tools

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
)

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
