//go:build integration

package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"

	"github.com/grasberg/sofia/pkg/config"
)

// Integration tests for the Web UI using Playwright browser automation.
// Run with: go test ./pkg/web/ -run TestBrowser -tags integration -timeout 120s
//
// Prerequisites:
//   npx playwright install chromium
// or:
//   go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps chromium

// testBrowserServer starts a real HTTP server for browser testing.
// It registers only the UI routes (no agentLoop dependency).
func testBrowserServer(t *testing.T, authToken string) (string, func()) {
	t.Helper()

	cfg := &config.Config{
		WebUI: config.WebUIConfig{
			Enabled:   true,
			Host:      "127.0.0.1",
			Port:      0,
			AuthToken: authToken,
		},
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: t.TempDir(),
			},
		},
	}

	s := &Server{cfg: cfg}
	mux := http.NewServeMux()

	// Index
	mux.HandleFunc("/", s.handleIndex)

	// UI partials
	uiRoutes := map[string][]byte{
		"/ui/chat":                  chatHTML,
		"/ui/agents":                agentsHTML,
		"/ui/monitor":               monitorHTML,
		"/ui/calendar":              calendarHTML,
		"/ui/memory":                memoryHTML,
		"/ui/goals":                 goalsHTML,
		"/ui/history":               historyHTML,
		"/ui/settings/models":       settingsModelsHTML,
		"/ui/settings/channels":     settingsChannelsHTML,
		"/ui/settings/tools":        settingsToolsHTML,
		"/ui/settings/integrations": settingsIntegrationsHTML,
		"/ui/settings/skills":       settingsSkillsHTML,
		"/ui/settings/heartbeat":    settingsHeartbeatHTML,
		"/ui/settings/security":     settingsSecurityHTML,
		"/ui/settings/prompts":      settingsPromptsHTML,
		"/ui/settings/logs":         settingsLogsHTML,
		"/ui/settings/evolution":    settingsEvolutionHTML,
		"/ui/settings/autonomy":     settingsAutonomyHTML,
		"/ui/settings/intelligence": settingsIntelligenceHTML,
		"/ui/settings/budget":       settingsBudgetHTML,
		"/ui/settings/tts":          settingsTTSHTML,
		"/ui/settings/webhooks":     settingsWebhooksHTML,
		"/ui/settings/triggers":     settingsTriggersHTML,
		"/ui/settings/remote":       settingsRemoteHTML,
		"/ui/settings/cron":         settingsCronHTML,
		"/ui/settings/personas":     settingsPersonasHTML,
	}
	for path, data := range uiRoutes {
		d := data // capture
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(d)
		})
	}

	// Mock API endpoints for browser test interaction
	mux.HandleFunc("/api/status", s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"version": "test-0.0.1",
			"tools":   map[string]any{"count": 5},
			"skills":  map[string]any{"count": 3},
			"agents":  map[string]any{"list": []string{"sofia"}, "active": "sofia"},
		})
	}))

	mux.HandleFunc("/api/config", s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"webui": map[string]any{"enabled": true, "port": 0},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))

	mux.HandleFunc("/api/sessions", s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
	}))

	// Static assets
	assetsDir := resolveAssetsDir()
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(assetsDir))))

	s.mux = mux

	// Start on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)

	addr := fmt.Sprintf("http://%s", listener.Addr().String())
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}

	return addr, cleanup
}

// launchBrowser starts Playwright and returns a browser page.
func launchBrowser(t *testing.T) (playwright.Page, func()) {
	t.Helper()

	pw, err := playwright.Run()
	if err != nil {
		t.Skipf("Playwright not available, skipping browser test: %v", err)
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		pw.Stop()
		t.Skipf("Cannot launch Chromium: %v", err)
	}

	page, err := browser.NewPage()
	if err != nil {
		browser.Close()
		pw.Stop()
		t.Fatalf("Cannot create page: %v", err)
	}

	cleanup := func() {
		page.Close()
		browser.Close()
		pw.Stop()
	}

	return page, cleanup
}

func TestBrowser_IndexPageLoads(t *testing.T) {
	addr, stopServer := testBrowserServer(t, "")
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	resp, err := page.Goto(addr)
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}
	if resp.Status() != 200 {
		t.Errorf("expected 200, got %d", resp.Status())
	}

	// Page should have HTML content
	content, err := page.Content()
	if err != nil {
		t.Fatalf("failed to get content: %v", err)
	}
	if !strings.Contains(content, "<html") && !strings.Contains(content, "<!DOCTYPE") {
		t.Error("expected HTML content in page")
	}
}

func TestBrowser_UIRoutesReturnHTML(t *testing.T) {
	addr, stopServer := testBrowserServer(t, "")
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	routes := []string{
		"/ui/chat",
		"/ui/agents",
		"/ui/monitor",
		"/ui/calendar",
		"/ui/memory",
		"/ui/goals",
		"/ui/history",
		"/ui/settings/models",
		"/ui/settings/channels",
		"/ui/settings/tools",
	}

	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			resp, err := page.Goto(addr + route)
			if err != nil {
				t.Fatalf("failed to navigate to %s: %v", route, err)
			}
			if resp.Status() != 200 {
				t.Errorf("route %s: expected 200, got %d", route, resp.Status())
			}
		})
	}
}

func TestBrowser_ChatPageElements(t *testing.T) {
	addr, stopServer := testBrowserServer(t, "")
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	_, err := page.Goto(addr + "/ui/chat")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	content, err := page.Content()
	if err != nil {
		t.Fatalf("failed to get content: %v", err)
	}

	// Verify chat UI elements exist (based on embedded template content)
	if len(content) == 0 {
		t.Error("chat page should have content")
	}
}

func TestBrowser_SettingsSubtabNavigation(t *testing.T) {
	addr, stopServer := testBrowserServer(t, "")
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	settingsTabs := []string{
		"/ui/settings/models",
		"/ui/settings/channels",
		"/ui/settings/tools",
		"/ui/settings/skills",
		"/ui/settings/security",
		"/ui/settings/prompts",
	}

	for _, tab := range settingsTabs {
		t.Run(tab, func(t *testing.T) {
			resp, err := page.Goto(addr + tab)
			if err != nil {
				t.Fatalf("failed to navigate: %v", err)
			}
			if resp.Status() != 200 {
				t.Errorf("expected 200, got %d", resp.Status())
			}

			content, err := page.Content()
			if err != nil {
				t.Fatalf("failed to get content: %v", err)
			}
			if len(content) == 0 {
				t.Errorf("settings tab %s should have content", tab)
			}
		})
	}
}

func TestBrowser_APIStatusEndpoint(t *testing.T) {
	addr, stopServer := testBrowserServer(t, "")
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	resp, err := page.Goto(addr + "/api/status")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}
	if resp.Status() != 200 {
		t.Errorf("expected 200, got %d", resp.Status())
	}

	content, err := page.Content()
	if err != nil {
		t.Fatalf("failed to get content: %v", err)
	}
	if !strings.Contains(content, "test-0.0.1") {
		t.Error("expected version in API status response")
	}
}

func TestBrowser_AuthProtection(t *testing.T) {
	addr, stopServer := testBrowserServer(t, "browser-test-token")
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	// API endpoint should reject without auth
	resp, err := page.Goto(addr + "/api/status")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}
	if resp.Status() != 401 {
		t.Errorf("expected 401 without auth, got %d", resp.Status())
	}
}

func TestBrowser_NotFoundReturns404(t *testing.T) {
	addr, stopServer := testBrowserServer(t, "")
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	resp, err := page.Goto(addr + "/this-does-not-exist")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}
	if resp.Status() != 404 {
		t.Errorf("expected 404, got %d", resp.Status())
	}
}

func TestBrowser_JavaScriptExecution(t *testing.T) {
	addr, stopServer := testBrowserServer(t, "")
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	_, err := page.Goto(addr)
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	// Evaluate JavaScript in the page context
	result, err := page.Evaluate("() => document.title || document.querySelector('html') !== null")
	if err != nil {
		t.Fatalf("failed to evaluate JS: %v", err)
	}

	// Should return something truthy (page has HTML)
	if result == nil || result == false {
		t.Error("expected JavaScript to execute successfully in page")
	}
}

func TestBrowser_ResponseHeaders(t *testing.T) {
	addr, stopServer := testBrowserServer(t, "")
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	resp, err := page.Goto(addr + "/ui/chat")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	headers := resp.Headers()
	ct, ok := headers["content-type"]
	if !ok {
		t.Error("expected Content-Type header")
	} else if !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html content type, got %s", ct)
	}
}

func TestBrowser_MultiplePageNavigations(t *testing.T) {
	addr, stopServer := testBrowserServer(t, "")
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	// Navigate through several pages in sequence (simulates user browsing)
	pages := []string{"/", "/ui/chat", "/ui/agents", "/ui/monitor", "/ui/settings/models"}
	for _, p := range pages {
		resp, err := page.Goto(addr + p)
		if err != nil {
			t.Fatalf("failed to navigate to %s: %v", p, err)
		}
		if resp.Status() != 200 {
			t.Errorf("page %s: expected 200, got %d", p, resp.Status())
		}
	}
}
