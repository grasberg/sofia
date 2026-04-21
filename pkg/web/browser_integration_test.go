//go:build integration

package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
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

	headless := os.Getenv("HEADED") == ""
	slowMo := 0.0
	if !headless {
		slowMo = 400 // slow down so you can see the actions
	}
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
		SlowMo:   playwright.Float(slowMo),
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

// --- Goals page E2E tests ---

// testBrowserServerWithGoalsAPI extends the test server with a mock goals API.
func testBrowserServerWithGoalsAPI(t *testing.T) (string, func()) {
	t.Helper()

	cfg := &config.Config{
		WebUI: config.WebUIConfig{
			Enabled: true,
			Host:    "127.0.0.1",
			Port:    0,
		},
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{Workspace: t.TempDir()},
		},
	}

	s := &Server{cfg: cfg}
	mux := http.NewServeMux()

	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/ui/goals", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(goalsHTML)
	})

	// Mock goals API with in-memory state
	var goals []map[string]any
	mux.HandleFunc("/api/goals", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if goals == nil {
				goals = []map[string]any{}
			}
			json.NewEncoder(w).Encode(goals)
		case http.MethodPost:
			var req map[string]any
			json.NewDecoder(r.Body).Decode(&req)
			goal := map[string]any{
				"id":          len(goals) + 1,
				"name":        req["name"],
				"description": req["description"],
				"status":      "active",
				"priority":    req["priority"],
				"phase":       "plan",
			}
			goals = append(goals, goal)
			json.NewEncoder(w).Encode(goal)
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"version": "test",
			"agents":  map[string]any{"list": []string{"main"}, "active": "main"},
		})
	})

	assetsDir := resolveAssetsDir()
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(assetsDir))))
	s.mux = mux

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

func TestBrowser_GoalsPageLoads(t *testing.T) {
	addr, stopServer := testBrowserServerWithGoalsAPI(t)
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	resp, err := page.Goto(addr + "/ui/goals")
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

	// Verify goals page structure
	if !strings.Contains(content, "Goals") {
		t.Error("expected 'Goals' heading in page")
	}
	if !strings.Contains(content, "goal-input") {
		t.Error("expected goal-input textarea in page")
	}
	if !strings.Contains(content, "goal-submit-btn") {
		t.Error("expected goal submit button in page")
	}
}

func TestBrowser_GoalsPageHasPrioritySelector(t *testing.T) {
	addr, stopServer := testBrowserServerWithGoalsAPI(t)
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	_, err := page.Goto(addr + "/ui/goals")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	content, err := page.Content()
	if err != nil {
		t.Fatalf("failed to get content: %v", err)
	}

	// Priority radio buttons
	for _, prio := range []string{"low", "medium", "high"} {
		if !strings.Contains(content, fmt.Sprintf(`value="%s"`, prio)) {
			t.Errorf("expected priority option %q in page", prio)
		}
	}

	// Agent count selector
	if !strings.Contains(content, "goal-agent-count") {
		t.Error("expected agent count selector in page")
	}
}

func TestBrowser_GoalsPageInputTextarea(t *testing.T) {
	addr, stopServer := testBrowserServerWithGoalsAPI(t)
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	_, err := page.Goto(addr + "/ui/goals")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	// Type into the goal input
	textarea := page.Locator("#goal-input")
	err = textarea.Fill("Build a REST API for user management")
	if err != nil {
		t.Fatalf("failed to fill textarea: %v", err)
	}

	// Verify the value was entered
	val, err := textarea.InputValue()
	if err != nil {
		t.Fatalf("failed to get input value: %v", err)
	}
	if val != "Build a REST API for user management" {
		t.Errorf("expected textarea value, got %q", val)
	}
}

func TestBrowser_GoalsPageEmptyState(t *testing.T) {
	addr, stopServer := testBrowserServerWithGoalsAPI(t)
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	_, err := page.Goto(addr + "/ui/goals")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	content, err := page.Content()
	if err != nil {
		t.Fatalf("failed to get content: %v", err)
	}

	// Empty state should exist in the DOM (may be shown/hidden via JS)
	if !strings.Contains(content, "goal-list-empty") {
		t.Error("expected empty state element in page")
	}
	if !strings.Contains(content, "No goals yet") {
		t.Error("expected 'No goals yet' empty state text")
	}
}

func TestBrowser_GoalsPageTimelineViewExists(t *testing.T) {
	addr, stopServer := testBrowserServerWithGoalsAPI(t)
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	_, err := page.Goto(addr + "/ui/goals")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	content, err := page.Content()
	if err != nil {
		t.Fatalf("failed to get content: %v", err)
	}

	if !strings.Contains(content, "goals-timeline-view") {
		t.Error("expected timeline view element in page")
	}
	if !strings.Contains(content, "Back to goals") {
		t.Error("expected 'Back to goals' button in timeline view")
	}
}

func TestBrowser_GoalsPageExampleGoals(t *testing.T) {
	addr, stopServer := testBrowserServerWithGoalsAPI(t)
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	_, err := page.Goto(addr + "/ui/goals")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	content, err := page.Content()
	if err != nil {
		t.Fatalf("failed to get content: %v", err)
	}

	if !strings.Contains(content, "useExample") {
		t.Error("expected example goal buttons with useExample function")
	}
	if !strings.Contains(content, "Build a REST API for todos") {
		t.Error("expected 'Build a REST API for todos' example goal")
	}
	if !strings.Contains(content, "Set up CI/CD pipeline") {
		t.Error("expected 'Set up CI/CD pipeline' example goal")
	}
	if !strings.Contains(content, "Docker Compose setup") {
		t.Error("expected 'Docker Compose setup' example goal")
	}
}

func TestBrowser_GoalsPageToastContainer(t *testing.T) {
	addr, stopServer := testBrowserServerWithGoalsAPI(t)
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	_, err := page.Goto(addr + "/ui/goals")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	content, err := page.Content()
	if err != nil {
		t.Fatalf("failed to get content: %v", err)
	}

	if !strings.Contains(content, "goal-toast-container") {
		t.Error("expected toast notification container in page")
	}
}

func TestBrowser_GoalsPageTimelineProgressRing(t *testing.T) {
	addr, stopServer := testBrowserServerWithGoalsAPI(t)
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	_, err := page.Goto(addr + "/ui/goals")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	content, err := page.Content()
	if err != nil {
		t.Fatalf("failed to get content: %v", err)
	}

	if !strings.Contains(content, "tl-progress-ring") {
		t.Error("expected SVG progress ring in timeline view")
	}
	if !strings.Contains(content, "stroke-dasharray") {
		t.Error("expected progress ring animation attributes")
	}
}

func TestBrowser_GoalsPageResultCardSections(t *testing.T) {
	addr, stopServer := testBrowserServerWithGoalsAPI(t)
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	_, err := page.Goto(addr + "/ui/goals")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	content, err := page.Content()
	if err != nil {
		t.Fatalf("failed to get content: %v", err)
	}

	if !strings.Contains(content, "tl-result-summary") {
		t.Error("expected result summary section")
	}
	if !strings.Contains(content, "tl-result-artifacts") {
		t.Error("expected artifacts section")
	}
	if !strings.Contains(content, "tl-result-unmet") {
		t.Error("expected unmet criteria section")
	}
	if !strings.Contains(content, "tl-result-nextsteps") {
		t.Error("expected next steps section")
	}
	if !strings.Contains(content, "shareGoalResult") {
		t.Error("expected share button")
	}
}

func TestBrowser_GoalsPageAttentionBanner(t *testing.T) {
	addr, stopServer := testBrowserServerWithGoalsAPI(t)
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	_, err := page.Goto(addr + "/ui/goals")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	content, err := page.Content()
	if err != nil {
		t.Fatalf("failed to get content: %v", err)
	}

	if !strings.Contains(content, "tl-attention-banner") {
		t.Error("expected needs-attention banner in page")
	}
	if !strings.Contains(content, "This goal needs your help") {
		t.Error("expected attention banner text")
	}
}

func TestBrowser_GoalsPageCreatesGoalViaAPI(t *testing.T) {
	addr, stopServer := testBrowserServerWithGoalsAPI(t)
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	_, err := page.Goto(addr + "/ui/goals")
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	textarea := page.Locator("#goal-input")
	if err := textarea.Fill("Build a CI/CD pipeline"); err != nil {
		t.Fatalf("failed to fill textarea: %v", err)
	}

	submitBtn := page.Locator("#goal-submit-btn")
	if err := submitBtn.Click(); err != nil {
		t.Fatalf("failed to click submit: %v", err)
	}

	page.WaitForTimeout(1000)

	content, err := page.Content()
	if err != nil {
		t.Fatalf("failed to get content: %v", err)
	}

	if !strings.Contains(content, "CI/CD pipeline") || !strings.Contains(content, "goal-item-") {
		contentSnip := content
		if len(contentSnip) > 500 {
			contentSnip = contentSnip[:500]
		}
		t.Errorf("expected goal item in page after submit, got: %s...", contentSnip)
	}
}
