package recipe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBundledRecipes_AllParse asserts that every recipe shipped inside the
// binary parses cleanly and has the required metadata. If this fails, the
// bundled library will not load at runtime either.
func TestBundledRecipes_AllParse(t *testing.T) {
	metas, err := ListBundledRecipes()
	require.NoError(t, err)
	require.NotEmpty(t, metas, "expected at least one bundled recipe")

	seen := make(map[string]bool, len(metas))
	for _, m := range metas {
		assert.NotEmpty(t, m.Name, "recipe is missing a filename stem")
		assert.NotEmpty(t, m.Title, "recipe %q is missing title", m.Name)
		assert.NotEmpty(t, m.Description, "recipe %q is missing description", m.Name)
		assert.Equal(t, "bundled", m.Source)
		assert.False(t, seen[m.Name], "duplicate recipe name: %s", m.Name)
		seen[m.Name] = true

		// Round-trip: loading the recipe by name must succeed and produce a usable prompt.
		r, err := LoadBundledRecipe(m.Name)
		require.NoError(t, err, "recipe %q failed to load", m.Name)
		assert.NotEmpty(t, r.Prompt, "recipe %q has empty prompt", m.Name)
	}
}

// TestBundledRecipes_Count documents the expected library size so an accidental
// deletion is caught in review.
func TestBundledRecipes_Count(t *testing.T) {
	metas, err := ListBundledRecipes()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(metas), 42, "bundled recipe library shrunk below its documented size")
}

// TestListRecipes_IncludesBundled verifies the bundled source is visible through
// the canonical discovery entry point that the CLI and web UI use.
func TestListRecipes_IncludesBundled(t *testing.T) {
	metas, err := ListRecipes("")
	require.NoError(t, err)

	var bundled int
	for _, m := range metas {
		if m.Source == "bundled" {
			bundled++
		}
	}
	assert.GreaterOrEqual(t, bundled, 42, "ListRecipes did not surface the bundled library")
}
