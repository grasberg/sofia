package recipe

import (
	"embed"
	"fmt"
	"path"
	"sort"
	"strings"
)

//go:embed bundled/*.yaml
var bundledRecipes embed.FS

// BundledRecipes exposes the embedded recipe filesystem for external inspection.
func BundledRecipes() embed.FS { return bundledRecipes }

// ListBundledRecipes returns metadata for every recipe shipped inside the binary.
// Recipes are identified by their filename (without extension) and sorted by name.
func ListBundledRecipes() ([]RecipeMeta, error) {
	entries, err := bundledRecipes.ReadDir("bundled")
	if err != nil {
		return nil, fmt.Errorf("read bundled recipes: %w", err)
	}

	metas := make([]RecipeMeta, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(path.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		r, err := LoadBundledRecipe(strings.TrimSuffix(entry.Name(), ext))
		if err != nil {
			continue // skip broken recipes rather than failing listing
		}

		metas = append(metas, RecipeMeta{
			Name:        strings.TrimSuffix(entry.Name(), ext),
			Title:       r.Title,
			Description: r.Description,
			Path:        "bundled/" + entry.Name(),
			Source:      "bundled",
		})
	}

	sort.Slice(metas, func(i, j int) bool { return metas[i].Name < metas[j].Name })
	return metas, nil
}

// LoadBundledRecipe loads a single embedded recipe by its filename stem (e.g. "daily-reddit-digest").
func LoadBundledRecipe(name string) (*Recipe, error) {
	data, err := bundledRecipes.ReadFile("bundled/" + name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("read bundled recipe %q: %w", name, err)
	}
	return ParseRecipe(data)
}
