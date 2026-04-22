package tools

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
