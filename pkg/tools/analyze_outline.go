package tools

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

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
