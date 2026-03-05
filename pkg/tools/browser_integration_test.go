//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests that require real Playwright browser binaries.
// Run with: go test ./pkg/tools/ -run TestWebBrowseIntegration -tags integration

func TestWebBrowseIntegration_NavigateAndGetText(t *testing.T) {
	// Spin up a minimal local HTTP server to avoid external network dependency.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html><body>
  <h1 id="title">Hello Playwright</h1>
  <button id="btn" onclick="document.getElementById('out').textContent='clicked'">Click me</button>
  <p id="out"></p>
</body></html>`))
	}))
	defer srv.Close()

	tmpDir, err := os.MkdirTemp("", "browser-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tool := NewWebBrowseTool(BrowseToolOptions{
		Headless:       true,
		TimeoutSeconds: 30,
		BrowserType:    "chromium",
		ScreenshotDir:  tmpDir,
	})

	result := tool.Execute(context.Background(), map[string]any{
		"url": srv.URL,
		"actions": []any{
			map[string]any{"type": "get_text", "selector": "#title"},
			map[string]any{"type": "click", "selector": "#btn"},
			map[string]any{"type": "get_text", "selector": "#out"},
			map[string]any{"type": "screenshot", "name": "after-click"},
		},
	})

	require.False(t, result.IsError, "unexpected error: %s", result.ForLLM)
	assert.Contains(t, result.ForLLM, "Hello Playwright")
	assert.Contains(t, result.ForLLM, "clicked")

	// Verify screenshot was written.
	screenshotPath := filepath.Join(tmpDir, "after-click.png")
	_, statErr := os.Stat(screenshotPath)
	assert.NoError(t, statErr, "screenshot file should exist at %s", screenshotPath)

	// Verify JSON report structure.
	var report map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.ForUser), &report))
	assert.Equal(t, srv.URL, report["start_url"])
}

func TestWebBrowseIntegration_FillAndSubmit(t *testing.T) {
	submitted := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			submitted <- r.FormValue("query")
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body><p id="result">submitted</p></body></html>`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>
<form method="POST">
  <input id="q" name="query" type="text"/>
  <button type="submit" id="submit">Go</button>
</form>
</body></html>`))
	}))
	defer srv.Close()

	tool := NewWebBrowseTool(BrowseToolOptions{
		Headless:       true,
		TimeoutSeconds: 30,
		BrowserType:    "chromium",
	})

	result := tool.Execute(context.Background(), map[string]any{
		"url": srv.URL,
		"actions": []any{
			map[string]any{"type": "fill", "selector": "#q", "value": "playwright test"},
			map[string]any{"type": "click", "selector": "#submit"},
			map[string]any{"type": "wait_for", "selector": "#result"},
			map[string]any{"type": "get_text", "selector": "#result"},
		},
	})

	require.False(t, result.IsError, "unexpected error: %s", result.ForLLM)
	assert.Contains(t, result.ForLLM, "submitted")

	select {
	case val := <-submitted:
		assert.Equal(t, "playwright test", val)
	default:
		t.Error("form was not submitted to the server")
	}
}

func TestWebBrowseIntegration_EvaluateJavaScript(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>JS Test</title></head><body></body></html>`))
	}))
	defer srv.Close()

	tool := NewWebBrowseTool(BrowseToolOptions{
		Headless:       true,
		TimeoutSeconds: 30,
		BrowserType:    "chromium",
	})

	result := tool.Execute(context.Background(), map[string]any{
		"url": srv.URL,
		"actions": []any{
			map[string]any{"type": "evaluate", "script": "document.title"},
		},
	})

	require.False(t, result.IsError, "unexpected error: %s", result.ForLLM)
	assert.Contains(t, result.ForLLM, "JS Test")
}
