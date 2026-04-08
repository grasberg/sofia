package recipe

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleRecipeYAML = `
version: "1"
title: "Code Review"
description: "Automated code review for a given file"
author: "sofia"
instructions: "You are a senior code reviewer. Be thorough but constructive."
prompt: "Review the file {{filepath}} focusing on {{focus_area}}."
settings:
  model: "gpt-4o"
  temperature: 0.3
  max_turns: 5
parameters:
  - key: filepath
    description: "Path to the file to review"
    input_type: string
    requirement: true
  - key: focus_area
    description: "Area to focus the review on"
    input_type: select
    requirement: false
    default: "general"
    options:
      - general
      - security
      - performance
      - readability
extensions:
  - name: filesystem
response:
  json_schema:
    type: object
    properties:
      issues:
        type: array
        items:
          type: object
          properties:
            severity:
              type: string
            message:
              type: string
      summary:
        type: string
    required:
      - issues
      - summary
retry:
  max_retries: 2
  checks:
    - shell:
        command: "test -f /tmp/review_output.json"
  on_failure: "echo 'Review check failed' >> /tmp/recipe_log.txt"
sub_recipes:
  - name: lint-check
    path: ./lint.yaml
`

func TestParseRecipe(t *testing.T) {
	r, err := ParseRecipe([]byte(sampleRecipeYAML))
	require.NoError(t, err)

	assert.Equal(t, "1", r.Version)
	assert.Equal(t, "Code Review", r.Title)
	assert.Equal(t, "Automated code review for a given file", r.Description)
	assert.Equal(t, "sofia", r.Author)
	assert.Equal(t, "You are a senior code reviewer. Be thorough but constructive.", r.Instructions)
	assert.Contains(t, r.Prompt, "{{filepath}}")
	assert.Contains(t, r.Prompt, "{{focus_area}}")

	// Settings
	assert.Equal(t, "gpt-4o", r.Settings.Model)
	assert.InDelta(t, 0.3, r.Settings.Temperature, 0.001)
	assert.Equal(t, 5, r.Settings.MaxTurns)

	// Parameters
	require.Len(t, r.Parameters, 2)
	assert.Equal(t, "filepath", r.Parameters[0].Key)
	assert.Equal(t, "string", r.Parameters[0].InputType)
	assert.True(t, r.Parameters[0].Required)
	assert.Equal(t, "focus_area", r.Parameters[1].Key)
	assert.Equal(t, "select", r.Parameters[1].InputType)
	assert.False(t, r.Parameters[1].Required)
	assert.Equal(t, "general", r.Parameters[1].Default)
	assert.Equal(t, []string{"general", "security", "performance", "readability"}, r.Parameters[1].Options)

	// Extensions
	require.Len(t, r.Extensions, 1)
	assert.Equal(t, "filesystem", r.Extensions[0].Name)

	// Response schema
	require.NotNil(t, r.Response)
	assert.Equal(t, "object", r.Response.JSONSchema["type"])

	// Retry
	require.NotNil(t, r.Retry)
	assert.Equal(t, 2, r.Retry.MaxRetries)
	require.Len(t, r.Retry.Checks, 1)
	assert.Equal(t, "test -f /tmp/review_output.json", r.Retry.Checks[0].Shell.Command)
	assert.Contains(t, r.Retry.OnFailure, "recipe_log.txt")

	// Sub-recipes
	require.Len(t, r.SubRecipes, 1)
	assert.Equal(t, "lint-check", r.SubRecipes[0].Name)
	assert.Equal(t, "./lint.yaml", r.SubRecipes[0].Path)
}

func TestParseRecipe_MissingTitle(t *testing.T) {
	yaml := `
prompt: "Do something"
`
	_, err := ParseRecipe([]byte(yaml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title")
}

func TestParseRecipe_MissingPrompt(t *testing.T) {
	yaml := `
title: "Test Recipe"
`
	_, err := ParseRecipe([]byte(yaml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt")
}

func TestParseRecipe_InvalidYAML(t *testing.T) {
	_, err := ParseRecipe([]byte("{{not valid yaml"))
	require.Error(t, err)
}

func TestRenderPrompt_Basic(t *testing.T) {
	r := &Recipe{
		Prompt: "Analyse {{filepath}} for {{focus_area}} issues.",
		Parameters: []RecipeParam{
			{Key: "filepath", Required: true},
			{Key: "focus_area", Required: true},
		},
	}
	params := map[string]string{
		"filepath":   "main.go",
		"focus_area": "security",
	}

	result, err := RenderPrompt(r, params)
	require.NoError(t, err)
	assert.Equal(t, "Analyse main.go for security issues.", result)
}

func TestRenderPrompt_WithDefaults(t *testing.T) {
	r := &Recipe{
		Prompt: "Review {{filepath}} with style {{style}}.",
		Parameters: []RecipeParam{
			{Key: "filepath", Required: true},
			{Key: "style", Required: false, Default: "concise"},
		},
	}
	params := map[string]string{
		"filepath": "server.go",
	}

	result, err := RenderPrompt(r, params)
	require.NoError(t, err)
	assert.Equal(t, "Review server.go with style concise.", result)
}

func TestRenderPrompt_OverrideDefault(t *testing.T) {
	r := &Recipe{
		Prompt: "Deploy to {{env}}.",
		Parameters: []RecipeParam{
			{Key: "env", Required: false, Default: "staging"},
		},
	}
	params := map[string]string{
		"env": "production",
	}

	result, err := RenderPrompt(r, params)
	require.NoError(t, err)
	assert.Equal(t, "Deploy to production.", result)
}

func TestRenderPrompt_MissingRequired(t *testing.T) {
	r := &Recipe{
		Prompt: "Do {{action}} on {{target}}.",
		Parameters: []RecipeParam{
			{Key: "action", Required: true},
			{Key: "target", Required: true},
		},
	}
	params := map[string]string{
		"action": "deploy",
	}

	_, err := RenderPrompt(r, params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target")
}

func TestRenderPrompt_RequiredWithDefault(t *testing.T) {
	r := &Recipe{
		Prompt: "Run {{cmd}}.",
		Parameters: []RecipeParam{
			{Key: "cmd", Required: true, Default: "build"},
		},
	}

	result, err := RenderPrompt(r, map[string]string{})
	require.NoError(t, err)
	assert.Equal(t, "Run build.", result)
}

func TestRenderPrompt_NoParameters(t *testing.T) {
	r := &Recipe{
		Prompt: "Just do the thing.",
	}

	result, err := RenderPrompt(r, nil)
	require.NoError(t, err)
	assert.Equal(t, "Just do the thing.", result)
}

func TestRenderPrompt_UnmatchedPlaceholder(t *testing.T) {
	r := &Recipe{
		Prompt: "Hello {{name}}, welcome to {{place}}.",
		Parameters: []RecipeParam{
			{Key: "name", Required: true},
		},
	}
	params := map[string]string{
		"name": "Alice",
	}

	result, err := RenderPrompt(r, params)
	require.NoError(t, err)
	assert.Equal(t, "Hello Alice, welcome to {{place}}.", result)
}

func TestListRecipes_FromTempDir(t *testing.T) {
	tmpDir := t.TempDir()
	recipesDir := filepath.Join(tmpDir, "recipes")
	require.NoError(t, os.MkdirAll(recipesDir, 0o755))

	// Write two valid recipes.
	recipe1 := `
title: "Recipe One"
description: "First test recipe"
prompt: "Do thing one."
`
	recipe2 := `
title: "Recipe Two"
description: "Second test recipe"
prompt: "Do thing two."
`
	require.NoError(t, os.WriteFile(filepath.Join(recipesDir, "one.yaml"), []byte(recipe1), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(recipesDir, "two.yml"), []byte(recipe2), 0o644))

	// Write an invalid file that should be skipped.
	require.NoError(t, os.WriteFile(filepath.Join(recipesDir, "bad.yaml"), []byte("{{invalid"), 0o644))

	// Write a non-YAML file that should be ignored.
	require.NoError(t, os.WriteFile(filepath.Join(recipesDir, "readme.txt"), []byte("not a recipe"), 0o644))

	metas, err := ListRecipes(tmpDir)
	require.NoError(t, err)
	require.Len(t, metas, 2)

	names := make(map[string]bool)
	for _, m := range metas {
		names[m.Name] = true
		assert.Equal(t, "workspace", m.Source)
		assert.NotEmpty(t, m.Path)
	}
	assert.True(t, names["one"], "expected recipe 'one'")
	assert.True(t, names["two"], "expected recipe 'two'")
}

func TestListRecipes_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	recipesDir := filepath.Join(tmpDir, "recipes")
	require.NoError(t, os.MkdirAll(recipesDir, 0o755))

	metas, err := ListRecipes(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, metas)
}

func TestListRecipes_NoDir(t *testing.T) {
	metas, err := ListRecipes("/nonexistent/path")
	require.NoError(t, err)
	assert.Empty(t, metas)
}

func TestLoadRecipe_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.yaml")
	require.NoError(t, os.WriteFile(path, []byte(sampleRecipeYAML), 0o644))

	r, err := LoadRecipe(path)
	require.NoError(t, err)
	assert.Equal(t, "Code Review", r.Title)
}

func TestLoadRecipe_FileNotFound(t *testing.T) {
	_, err := LoadRecipe("/nonexistent/recipe.yaml")
	require.Error(t, err)
}

func TestValidateParams_AllPresent(t *testing.T) {
	r := &Recipe{
		Parameters: []RecipeParam{
			{Key: "a", Required: true},
			{Key: "b", Required: true},
		},
	}
	err := validateParams(r, map[string]string{"a": "1", "b": "2"})
	assert.NoError(t, err)
}

func TestValidateParams_MissingRequired(t *testing.T) {
	r := &Recipe{
		Parameters: []RecipeParam{
			{Key: "a", Required: true},
			{Key: "b", Required: true},
		},
	}
	err := validateParams(r, map[string]string{"a": "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "b")
}

func TestValidateParams_OptionalMissing(t *testing.T) {
	r := &Recipe{
		Parameters: []RecipeParam{
			{Key: "a", Required: true},
			{Key: "b", Required: false},
		},
	}
	err := validateParams(r, map[string]string{"a": "1"})
	assert.NoError(t, err)
}

func TestValidateParams_RequiredEmptyString(t *testing.T) {
	r := &Recipe{
		Parameters: []RecipeParam{
			{Key: "a", Required: true},
		},
	}
	err := validateParams(r, map[string]string{"a": ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a")
}

func TestValidateParams_NoParams(t *testing.T) {
	r := &Recipe{}
	err := validateParams(r, nil)
	assert.NoError(t, err)
}

func TestRecipeMeta_Fields(t *testing.T) {
	m := RecipeMeta{
		Name:        "test",
		Title:       "Test Recipe",
		Description: "A test",
		Path:        "/tmp/test.yaml",
		Source:      "workspace",
	}
	assert.Equal(t, "test", m.Name)
	assert.Equal(t, "Test Recipe", m.Title)
	assert.Equal(t, "A test", m.Description)
	assert.Equal(t, "/tmp/test.yaml", m.Path)
	assert.Equal(t, "workspace", m.Source)
}

func TestFormatResult_PlainContent(t *testing.T) {
	r := &RecipeResult{Content: "All good", Iterations: 3}
	out := FormatResult(r)
	assert.Equal(t, "All good", out)
}

func TestFormatResult_WithStructured(t *testing.T) {
	r := &RecipeResult{
		Content:    "Done",
		Structured: map[string]any{"status": "ok"},
		Iterations: 1,
	}
	out := FormatResult(r)
	assert.Contains(t, out, "Done")
	assert.Contains(t, out, "Structured output:")
	assert.Contains(t, out, `"status": "ok"`)
}
