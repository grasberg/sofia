package recipe

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var templateVarRe = regexp.MustCompile(`\{\{(\w+)\}\}`)

// LoadRecipe parses a YAML recipe file at the given path.
func LoadRecipe(path string) (*Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read recipe %s: %w", path, err)
	}

	return ParseRecipe(data)
}

// ParseRecipe unmarshals a YAML byte slice into a Recipe.
func ParseRecipe(data []byte) (*Recipe, error) {
	var r Recipe
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse recipe YAML: %w", err)
	}

	if r.Title == "" {
		return nil, fmt.Errorf("recipe is missing required field: title")
	}
	if r.Prompt == "" {
		return nil, fmt.Errorf("recipe is missing required field: prompt")
	}

	return &r, nil
}

// ListRecipes discovers recipes from workspace/recipes/, ~/.sofia/recipes/,
// and the recipes bundled inside the Sofia binary. Earlier sources win when
// names collide: user customisations override global recipes, which override
// built-ins.
func ListRecipes(workspacePath string) ([]RecipeMeta, error) {
	seen := make(map[string]bool)
	var metas []RecipeMeta

	// 1. Project-local: workspace/recipes/
	if workspacePath != "" {
		localDir := filepath.Join(workspacePath, "recipes")
		found, err := discoverRecipes(localDir, "workspace")
		if err == nil {
			for _, m := range found {
				if !seen[m.Name] {
					seen[m.Name] = true
					metas = append(metas, m)
				}
			}
		}
	}

	// 2. Global: ~/.sofia/recipes/
	home, err := os.UserHomeDir()
	if err == nil {
		globalDir := filepath.Join(home, ".sofia", "recipes")
		found, gErr := discoverRecipes(globalDir, "global")
		if gErr == nil {
			for _, m := range found {
				if !seen[m.Name] {
					seen[m.Name] = true
					metas = append(metas, m)
				}
			}
		}
	}

	// 3. Bundled: compiled into the binary.
	if found, bErr := ListBundledRecipes(); bErr == nil {
		for _, m := range found {
			if !seen[m.Name] {
				seen[m.Name] = true
				metas = append(metas, m)
			}
		}
	}

	return metas, nil
}

// discoverRecipes scans a directory for .yaml/.yml files and extracts metadata.
func discoverRecipes(dir, source string) ([]RecipeMeta, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var metas []RecipeMeta
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		r, err := LoadRecipe(path)
		if err != nil {
			continue // skip invalid recipes
		}

		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		metas = append(metas, RecipeMeta{
			Name:        name,
			Title:       r.Title,
			Description: r.Description,
			Path:        path,
			Source:      source,
		})
	}

	return metas, nil
}

// RenderPrompt substitutes {{param}} placeholders in the recipe prompt with the
// supplied parameter values. Missing required parameters produce an error.
func RenderPrompt(r *Recipe, params map[string]string) (string, error) {
	if err := validateParams(r, params); err != nil {
		return "", err
	}

	// Build lookup: explicit params + defaults.
	lookup := make(map[string]string, len(r.Parameters))
	for _, p := range r.Parameters {
		if p.Default != "" {
			lookup[p.Key] = p.Default
		}
	}
	for k, v := range params {
		lookup[k] = v
	}

	result := templateVarRe.ReplaceAllStringFunc(r.Prompt, func(match string) string {
		key := match[2 : len(match)-2] // strip {{ and }}
		if val, ok := lookup[key]; ok {
			return val
		}
		return match // leave unmatched placeholders as-is
	})

	return result, nil
}

// validateParams checks that all required parameters are present.
func validateParams(r *Recipe, params map[string]string) error {
	for _, p := range r.Parameters {
		if !p.Required {
			continue
		}
		val, ok := params[p.Key]
		if !ok || val == "" {
			if p.Default != "" {
				continue // has a default value
			}
			return fmt.Errorf("missing required parameter: %s", p.Key)
		}
	}
	return nil
}
