package templates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleTemplate = `---
name: code-review
description: Review code for quality and bugs
variables: [language, code]
tags: [development]
---
Review the following {{.language}} code for bugs, quality issues, and improvements:

` + "```{{.language}}" + `
{{.code}}
` + "```" + `

Provide specific, actionable feedback.
`

const minimalTemplate = `---
name: summarize
description: Summarize text
variables: [text]
---
Please summarize the following text:

{{.text}}
`

func writeTemplate(t *testing.T, dir, filename, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644))
}

func TestTemplateManager_Load(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "code-review.md", sampleTemplate)
	writeTemplate(t, dir, "summarize.md", minimalTemplate)

	tm := NewTemplateManager(dir)
	require.NoError(t, tm.Load())

	// Both templates should be loaded.
	tpl, ok := tm.Get("code-review")
	require.True(t, ok)
	assert.Equal(t, "code-review", tpl.Name)
	assert.Equal(t, "Review code for quality and bugs", tpl.Description)
	assert.Equal(t, []string{"language", "code"}, tpl.Variables)
	assert.Equal(t, []string{"development"}, tpl.Tags)
	assert.Contains(t, tpl.Content, "{{.language}}")
	assert.Equal(t, filepath.Join(dir, "code-review.md"), tpl.FilePath)

	tpl2, ok := tm.Get("summarize")
	require.True(t, ok)
	assert.Equal(t, "summarize", tpl2.Name)
	assert.Equal(t, []string{"text"}, tpl2.Variables)
}

func TestTemplateManager_Load_SkipNonMd(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "code-review.md", sampleTemplate)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a template"), 0o644))

	tm := NewTemplateManager(dir)
	require.NoError(t, tm.Load())

	assert.Equal(t, 1, len(tm.List()))
}

func TestTemplateManager_Load_MissingDir(t *testing.T) {
	tm := NewTemplateManager("/nonexistent/path")
	// Load should not error when directories are missing — it just skips them.
	require.NoError(t, tm.Load())
	assert.Empty(t, tm.List())
}

func TestTemplateManager_Load_Priority(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	writeTemplate(t, dir1, "tpl.md", `---
name: shared
description: from dir1
variables: []
---
Content from dir1
`)
	writeTemplate(t, dir2, "tpl.md", `---
name: shared
description: from dir2
variables: []
---
Content from dir2
`)

	tm := NewTemplateManager(dir1, dir2)
	require.NoError(t, tm.Load())

	tpl, ok := tm.Get("shared")
	require.True(t, ok)
	assert.Equal(t, "from dir1", tpl.Description, "first directory should win")
}

func TestTemplateManager_Load_FallbackName(t *testing.T) {
	dir := t.TempDir()
	// Template file without a name field in frontmatter — should use filename.
	writeTemplate(t, dir, "my-template.md", `---
description: no name field
variables: [x]
---
Hello {{.x}}
`)

	tm := NewTemplateManager(dir)
	require.NoError(t, tm.Load())

	tpl, ok := tm.Get("my-template")
	require.True(t, ok)
	assert.Equal(t, "my-template", tpl.Name)
}

func TestTemplateManager_Render(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "code-review.md", sampleTemplate)

	tm := NewTemplateManager(dir)
	require.NoError(t, tm.Load())

	result, err := tm.Render("code-review", map[string]string{
		"language": "go",
		"code":     "func main() {}",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "go")
	assert.Contains(t, result, "func main() {}")
	assert.Contains(t, result, "Review the following go code")
}

func TestTemplateManager_Render_ExtraVars(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "summarize.md", minimalTemplate)

	tm := NewTemplateManager(dir)
	require.NoError(t, tm.Load())

	// Extra variables beyond required should not cause errors.
	result, err := tm.Render("summarize", map[string]string{
		"text":  "Hello world",
		"extra": "ignored",
	})
	require.NoError(t, err)
	assert.Contains(t, result, "Hello world")
}

func TestTemplateManager_MissingVariable(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "code-review.md", sampleTemplate)

	tm := NewTemplateManager(dir)
	require.NoError(t, tm.Load())

	_, err := tm.Render("code-review", map[string]string{
		"language": "go",
		// "code" is missing
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required variables")
	assert.Contains(t, err.Error(), "code")
}

func TestTemplateManager_MissingVariable_Multiple(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "code-review.md", sampleTemplate)

	tm := NewTemplateManager(dir)
	require.NoError(t, tm.Load())

	_, err := tm.Render("code-review", map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "language")
	assert.Contains(t, err.Error(), "code")
}

func TestTemplateManager_Render_NotFound(t *testing.T) {
	tm := NewTemplateManager()
	require.NoError(t, tm.Load())

	_, err := tm.Render("nonexistent", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestTemplateManager_List(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "code-review.md", sampleTemplate)
	writeTemplate(t, dir, "summarize.md", minimalTemplate)

	tm := NewTemplateManager(dir)
	require.NoError(t, tm.Load())

	list := tm.List()
	require.Len(t, list, 2)
	// Should be sorted alphabetically.
	assert.Equal(t, "code-review", list[0].Name)
	assert.Equal(t, "summarize", list[1].Name)
}

func TestTemplateManager_List_Empty(t *testing.T) {
	dir := t.TempDir()
	tm := NewTemplateManager(dir)
	require.NoError(t, tm.Load())

	list := tm.List()
	assert.Empty(t, list)
}

func TestParseYAMLList(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"[a, b, c]", []string{"a", "b", "c"}},
		{"[single]", []string{"single"}},
		{"[]", nil},
		{"", nil},
		{"standalone", []string{"standalone"}},
		{"[  spaced , items ]", []string{"spaced", "items"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseYAMLList(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseSimpleYAML(t *testing.T) {
	input := `name: test
description: "a description"
variables: [a, b]`

	result := parseSimpleYAML(input)
	assert.Equal(t, "test", result["name"])
	assert.Equal(t, "a description", result["description"])
	assert.Equal(t, "[a, b]", result["variables"])
}

func TestExtractFrontmatter(t *testing.T) {
	input := "---\nname: test\n---\nBody content"
	fm := extractFrontmatter(input)
	assert.Equal(t, "name: test", fm)
}

func TestStripFrontmatter(t *testing.T) {
	input := "---\nname: test\n---\nBody content"
	body := stripFrontmatter(input)
	assert.Equal(t, "Body content", body)
}

func TestTemplateManager_Load_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "bare.md", "Just plain content, no frontmatter")

	tm := NewTemplateManager(dir)
	require.NoError(t, tm.Load())

	// File without frontmatter should be skipped.
	_, ok := tm.Get("bare")
	assert.False(t, ok)
}

func TestTemplateManager_Reload(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "tpl.md", `---
name: v1
description: version 1
variables: []
---
Version 1
`)

	tm := NewTemplateManager(dir)
	require.NoError(t, tm.Load())
	tpl, _ := tm.Get("v1")
	assert.Equal(t, "version 1", tpl.Description)

	// Overwrite with v2.
	writeTemplate(t, dir, "tpl.md", `---
name: v2
description: version 2
variables: []
---
Version 2
`)
	require.NoError(t, tm.Load())

	_, ok := tm.Get("v1")
	assert.False(t, ok, "v1 should be gone after reload")
	tpl2, ok := tm.Get("v2")
	require.True(t, ok)
	assert.Equal(t, "version 2", tpl2.Description)
}
