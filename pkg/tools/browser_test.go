package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Unit tests (no real browser required) ---

func TestWebBrowseTool_Name(t *testing.T) {
	tool := NewWebBrowseTool(BrowseToolOptions{})
	assert.Equal(t, "web_browse", tool.Name())
}

func TestWebBrowseTool_Description(t *testing.T) {
	tool := NewWebBrowseTool(BrowseToolOptions{})
	desc := tool.Description()
	assert.NotEmpty(t, desc)
	assert.Contains(t, desc, "Playwright")
}

func TestWebBrowseTool_Parameters(t *testing.T) {
	tool := NewWebBrowseTool(BrowseToolOptions{})
	params := tool.Parameters()

	assert.Equal(t, "object", params["type"])
	props, ok := params["properties"].(map[string]any)
	require.True(t, ok, "parameters.properties must be a map")
	assert.Contains(t, props, "url")
	assert.Contains(t, props, "actions")
	assert.Contains(t, props, "headless")

	req, ok := params["required"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"url"}, req)
}

func TestWebBrowseTool_Execute_MissingURL(t *testing.T) {
	tool := NewWebBrowseTool(BrowseToolOptions{})
	result := tool.Execute(context.Background(), map[string]any{})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "url is required")
}

func TestWebBrowseTool_Execute_EmptyURL(t *testing.T) {
	tool := NewWebBrowseTool(BrowseToolOptions{})
	result := tool.Execute(context.Background(), map[string]any{"url": ""})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "url is required")
}

func TestWebBrowseTool_Execute_InvalidActionType(t *testing.T) {
	tool := NewWebBrowseTool(BrowseToolOptions{})
	result := tool.Execute(context.Background(), map[string]any{
		"url": "https://example.com",
		"actions": []any{
			"not-an-object",
		},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "action[0] is not an object")
}

func TestWebBrowseTool_Execute_ActionMissingType(t *testing.T) {
	tool := NewWebBrowseTool(BrowseToolOptions{})
	result := tool.Execute(context.Background(), map[string]any{
		"url": "https://example.com",
		"actions": []any{
			map[string]any{"selector": "#foo"},
		},
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "action[0]")
}

// TestNewWebBrowseTool_Defaults verifies that zero-value options are filled with sensible defaults.
func TestNewWebBrowseTool_Defaults(t *testing.T) {
	tool := NewWebBrowseTool(BrowseToolOptions{})
	assert.Equal(t, 30, tool.opts.TimeoutSeconds)
	assert.Equal(t, "chromium", tool.opts.BrowserType)
}

func TestNewWebBrowseTool_WorkspaceScreenshotDir(t *testing.T) {
	tool := NewWebBrowseTool(BrowseToolOptions{Workspace: "/tmp/ws"})
	assert.Equal(t, filepath.Join("/tmp/ws", "screenshots"), tool.opts.ScreenshotDir)
}

func TestNewWebBrowseTool_ExplicitScreenshotDir(t *testing.T) {
	tool := NewWebBrowseTool(BrowseToolOptions{
		Workspace:     "/tmp/ws",
		ScreenshotDir: "/custom/dir",
	})
	assert.Equal(t, "/custom/dir", tool.opts.ScreenshotDir)
}

// --- parseAction unit tests ---

func TestParseAction_Basic(t *testing.T) {
	a, err := parseAction(map[string]any{"type": "click", "selector": "#btn"})
	require.NoError(t, err)
	assert.Equal(t, "click", a.Type)
	assert.Equal(t, "#btn", a.Selector)
}

func TestParseAction_AllFields(t *testing.T) {
	a, err := parseAction(map[string]any{
		"type":         "fill",
		"selector":     "#input",
		"value":        "hello",
		"url":          "https://example.com",
		"key":          "Enter",
		"script":       "document.title",
		"name":         "my-screenshot",
		"milliseconds": float64(1500),
	})
	require.NoError(t, err)
	assert.Equal(t, "fill", a.Type)
	assert.Equal(t, "#input", a.Selector)
	assert.Equal(t, "hello", a.Value)
	assert.Equal(t, "https://example.com", a.URL)
	assert.Equal(t, "Enter", a.Key)
	assert.Equal(t, "document.title", a.Script)
	assert.Equal(t, "my-screenshot", a.Name)
	assert.Equal(t, 1500, a.Milliseconds)
}

func TestParseAction_MissingType(t *testing.T) {
	_, err := parseAction(map[string]any{"selector": "#btn"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "'type'")
}

// --- urlSlug unit tests ---

func TestURLSlug_BasicHTTPS(t *testing.T) {
	s := urlSlug("https://example.com/page?q=1")
	assert.NotContains(t, s, "/")
	assert.NotContains(t, s, "?")
	assert.NotContains(t, s, "https://")
}

func TestURLSlug_Truncation(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("a", 100)
	s := urlSlug(long)
	assert.LessOrEqual(t, len(s), 60)
}

// --- buildLLMLog unit tests ---

func TestBuildLLMLog_NoActions(t *testing.T) {
	log := buildLLMLog("https://a.com", "https://a.com", nil, nil)
	assert.Contains(t, log, "https://a.com")
	assert.Contains(t, log, "No actions performed")
}

func TestBuildLLMLog_WithResults(t *testing.T) {
	results := []actionResult{
		{Step: 1, Action: "click(\"#btn\")", Result: "clicked"},
		{Step: 2, Action: "get_text(\"h1\")", Error: "element not found"},
	}
	log := buildLLMLog("https://a.com", "https://b.com", results, []string{"/tmp/screen.png"})
	assert.Contains(t, log, "clicked")
	assert.Contains(t, log, "ERROR: element not found")
	assert.Contains(t, log, "/tmp/screen.png")
	assert.Contains(t, log, "Current URL: https://b.com")
}

// --- truncateString unit tests ---

func TestTruncateString_ShortString(t *testing.T) {
	assert.Equal(t, "hello", truncateString("hello", 100))
}

func TestTruncateString_ExactLimit(t *testing.T) {
	assert.Equal(t, "hello", truncateString("hello", 5))
}

func TestTruncateString_Overflow(t *testing.T) {
	result := truncateString("hello world", 5)
	assert.Equal(t, "hello... [truncated]", result)
}
