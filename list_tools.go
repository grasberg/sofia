package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"sort"
)

func main() {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "./pkg/tools", nil, 0)
	if err != nil {
		panic(err)
	}

	type Tool struct {
		Name string
		Desc string
	}
	tools := make(map[string]*Tool)

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv != nil {
					recvName := ""
					switch expr := fd.Recv.List[0].Type.(type) {
					case *ast.StarExpr:
						if ident, ok := expr.X.(*ast.Ident); ok {
							recvName = ident.Name
						}
					case *ast.Ident:
						recvName = expr.Name
					}
					
					if recvName == "" {
						continue
					}

					if tools[recvName] == nil {
						tools[recvName] = &Tool{}
					}

					if fd.Name.Name == "Name" {
						for _, stmt := range fd.Body.List {
							if ret, ok := stmt.(*ast.ReturnStmt); ok && len(ret.Results) > 0 {
								if basicLit, ok := ret.Results[0].(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
									tools[recvName].Name = strings.Trim(basicLit.Value, "\"")
								} else if ident, ok := ret.Results[0].(*ast.Ident); ok {
									tools[recvName].Name = ident.Name // e.g., variable return
								}
							}
						}
					}
					
					if fd.Name.Name == "Description" {
						for _, stmt := range fd.Body.List {
							if ret, ok := stmt.(*ast.ReturnStmt); ok && len(ret.Results) > 0 {
								if basicLit, ok := ret.Results[0].(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
									tools[recvName].Desc = strings.Trim(basicLit.Value, "\"`\n")
									tools[recvName].Desc = strings.Split(tools[recvName].Desc, "\n")[0] // Just first line
								}
							}
						}
					}
				}
			}
		}
	}

	var results []string
	for _, t := range tools {
		if t.Name != "" && t.Name != "c.name" && t.Name != "t.name" && t.Name != "m.name" && t.Name != "finalOutputToolName" && !strings.Contains(t.Name,"mock") && t.Desc != "" {
			results = append(results, fmt.Sprintf("| `%s` | %s |", t.Name, t.Desc))
		}
	}
	sort.Strings(results)
	for _, res := range results {
		fmt.Println(res)
	}
}
