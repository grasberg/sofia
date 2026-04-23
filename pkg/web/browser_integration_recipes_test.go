//go:build integration

package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"

	"github.com/grasberg/sofia/pkg/config"
)

// Browser integration tests for the Recipes tab and broader nav smoke test.
// Run with: go test ./pkg/web/ -run TestBrowser_Recipe -tags integration -timeout 180s
//
// These tests drive a real Chromium via Playwright. /api/chat is mocked so we
// never pull in the agent loop — the tests verify that the UI wiring (list,
// search, parameter form, render, run, error surfacing) behaves correctly end
// to end and that no unexpected console errors appear during the flow.

// chatRecorder records every /api/chat invocation so tests can assert that the
// Run button actually POSTs the rendered prompt as a chat message.
type chatRecorder struct {
	mu       sync.Mutex
	messages []string
	fail     atomic.Bool // when set, /api/chat returns 500
}

func (c *chatRecorder) record(msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, msg)
}

func (c *chatRecorder) last() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.messages) == 0 {
		return ""
	}
	return c.messages[len(c.messages)-1]
}

func (c *chatRecorder) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.messages)
}

// recipeBrowserServer starts a real HTTP server with recipe and nav routes
// wired in, plus a mock /api/chat for Run testing. Returns the addr, the
// chat recorder, and a cleanup func.
func recipeBrowserServer(t *testing.T) (string, *chatRecorder, func()) {
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

	// Index + every nav-reachable partial used during the smoke test. The
	// layout lazy-loads settings children on first open, so we register the
	// full settings tree too — otherwise clicking nav-ai or nav-platform
	// fires HTMX requests that 404 and pollute the console log.
	mux.HandleFunc("/", s.handleIndex)
	uiRoutes := map[string][]byte{
		"/ui/chat":                     chatHTML,
		"/ui/agents":                   agentsHTML,
		"/ui/monitor":                  monitorHTML,
		"/ui/calendar":                 calendarHTML,
		"/ui/memory":                   memoryHTML,
		"/ui/goals":                    goalsHTML,
		"/ui/history":                  historyHTML,
		"/ui/eval":                     evalHTML,
		"/ui/files":                    filesHTML,
		"/ui/recipes":                  recipesHTML,
		"/ui/ai":                       settingsAIHTML,
		"/ui/platform":                 settingsPlatformHTML,
		"/ui/settings/models":          settingsModelsHTML,
		"/ui/settings/channels":        settingsChannelsHTML,
		"/ui/settings/tools":           settingsToolsHTML,
		"/ui/settings/integrations":    settingsIntegrationsHTML,
		"/ui/settings/skills":          settingsSkillsHTML,
		"/ui/settings/heartbeat":       settingsHeartbeatHTML,
		"/ui/settings/security":        settingsSecurityHTML,
		"/ui/settings/prompts":         settingsPromptsHTML,
		"/ui/settings/logs":            settingsLogsHTML,
		"/ui/settings/evolution":       settingsEvolutionHTML,
		"/ui/settings/autonomy":        settingsAutonomyHTML,
		"/ui/settings/github_autonomy": settingsGitHubAutonomyHTML,
		"/ui/settings/intelligence":    settingsIntelligenceHTML,
		"/ui/settings/budget":          settingsBudgetHTML,
		"/ui/settings/tts":             settingsTTSHTML,
		"/ui/settings/webhooks":        settingsWebhooksHTML,
		"/ui/settings/triggers":        settingsTriggersHTML,
		"/ui/settings/remote":          settingsRemoteHTML,
		"/ui/settings/cron":            settingsCronHTML,
		"/ui/settings/personas":        settingsPersonasHTML,
	}
	for path, data := range uiRoutes {
		d := data
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(d)
		})
	}

	// Real recipe handlers — they read bundled YAML directly, no agent loop needed.
	mux.HandleFunc("/api/recipes", s.handleRecipes)
	mux.HandleFunc("/api/recipes/", s.handleRecipes)

	// Status + config stubs so the sidebar / status panel don't error out.
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"version": "integration",
			"tools":   map[string]any{"count": 0},
			"skills":  map[string]any{"count": 0},
			"agents":  map[string]any{"list": []string{"sofia"}, "active": "sofia"},
		})
	})
	// Realistic enough config so layout.js's nested property access (.defaults,
	// .enabled, etc.) doesn't hit undefined and throw. We stub every top-level
	// section we know about with sane defaults.
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"webui":   map[string]any{"enabled": true, "host": "127.0.0.1", "port": 0},
			"session": map[string]any{"dm_scope": "per-channel-peer"},
			"agents": map[string]any{
				"defaults": map[string]any{"model": "sofia-test", "provider": "mock", "enabled": true},
				"list":     []any{},
			},
			"providers":   []any{},
			"channels":    map[string]any{"enabled": false},
			"webhooks":    map[string]any{"enabled": false},
			"evolution":   map[string]any{"enabled": false},
			"autonomy":    map[string]any{"enabled": false},
			"heartbeat":   map[string]any{"enabled": false},
			"security":    map[string]any{"enabled": false},
			"tts":         map[string]any{"enabled": false},
			"triggers":    map[string]any{"enabled": false},
			"remote":      map[string]any{"enabled": false},
			"cron":        map[string]any{"enabled": false},
			"personas":    map[string]any{"enabled": false},
			"intelligence": map[string]any{"enabled": false},
			"budget":      map[string]any{"enabled": false},
			"tools":       map[string]any{},
		})
	})
	// Silent stub WebSocket so nav clicks don't log "ws connection failed" errors.
	mux.HandleFunc("/ws/dashboard", func(w http.ResponseWriter, r *http.Request) {
		// Not a real upgrade — the browser will fail cleanly and the dashboard
		// JS suppresses the error if connection never establishes.
		w.WriteHeader(http.StatusNoContent)
	})

	// Mocked chat endpoint — records the message body so we can assert Run behaviour.
	rec := &chatRecorder{}
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &payload)
		rec.record(payload.Message)

		if rec.fail.Load() {
			http.Error(w, "mock chat failure", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"response": "Mocked Sofia response for: " + truncate(payload.Message, 40),
		})
	})

	// Explicit empty stubs for the many endpoints the layout polls in the
	// background. A single `/api/` catch-all would clobber the real recipe
	// routes on some Go versions' ServeMux precedence rules, so we register
	// each one by hand.
	emptyArray := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}
	emptyObject := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
	}
	for _, p := range []string{
		"/api/sessions", "/api/plan", "/api/plans", "/api/audit",
		"/api/approvals", "/api/cron", "/api/goals", "/api/agents",
		"/api/agent-templates", "/api/skills", "/api/models",
		"/api/memory/notes", "/api/memory/reflections",
		"/api/evolution/changelog", "/api/eval/runs", "/api/logs",
		"/api/workspace-docs",
	} {
		mux.HandleFunc(p, emptyArray)
	}
	for _, p := range []string{
		"/api/memory/graph", "/api/workspace/files",
		"/api/presence", "/api/evolution/status",
	} {
		mux.HandleFunc(p, emptyObject)
	}

	assetsDir := resolveAssetsDir()
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(assetsDir))))
	s.mux = mux

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)

	addr := fmt.Sprintf("http://%s", listener.Addr().String())
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}
	return addr, rec, cleanup
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// consoleCapture attaches listeners to a Playwright page and returns pointers
// to the accumulated console errors and failed network requests. The caller
// must call the returned detach func before inspecting the slices.
func consoleCapture(page playwright.Page) (errs *[]string, failed *[]string, detach func()) {
	e := []string{}
	f := []string{}
	var mu sync.Mutex

	onConsole := func(msg playwright.ConsoleMessage) {
		if msg.Type() == "error" {
			mu.Lock()
			e = append(e, msg.Text())
			mu.Unlock()
		}
	}
	onResponse := func(resp playwright.Response) {
		if resp.Status() >= 400 {
			mu.Lock()
			f = append(f, fmt.Sprintf("%d %s", resp.Status(), resp.URL()))
			mu.Unlock()
		}
	}

	page.On("console", onConsole)
	page.On("response", onResponse)

	errs = &e
	failed = &f
	detach = func() {
		page.RemoveListener("console", onConsole)
		page.RemoveListener("response", onResponse)
	}
	return
}

// TestBrowser_RecipesTab_Smoke walks the recipes tab end-to-end: it loads the
// partial, verifies the list populated, runs search, selects a recipe, renders
// the prompt, runs it through the mocked chat endpoint, and finally asserts
// that no unexpected console errors surfaced along the way.
func TestBrowser_RecipesTab_Smoke(t *testing.T) {
	addr, chat, stopServer := recipeBrowserServer(t)
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	errs, failed, detach := consoleCapture(page)
	defer detach()

	if _, err := page.Goto(addr + "/ui/recipes"); err != nil {
		t.Fatalf("navigate: %v", err)
	}

	// Wait for the recipe count badge to settle (populated after /api/recipes resolves).
	countLoc := page.Locator("#recipes-count")
	if err := countLoc.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Fatalf("wait recipes-count: %v", err)
	}
	countText, _ := countLoc.TextContent()
	if !strings.Contains(countText, "total") {
		t.Fatalf("expected count badge to say 'N total', got %q", countText)
	}

	// 42 bundled recipes should render as 42 list buttons.
	items := page.Locator("#recipes-list .recipe-item")
	if err := items.First().WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(3000),
	}); err != nil {
		t.Fatalf("recipe list never rendered: %v", err)
	}
	n, _ := items.Count()
	if n < 42 {
		t.Fatalf("expected >= 42 recipe items, got %d", n)
	}

	// Search: "daily-reddit" is a slug fragment only the Reddit recipe matches.
	// Plain "reddit" also hits recipes that mention Reddit in their description
	// (e.g. the product-factory recipe), so we anchor on the slug instead.
	if err := page.Locator("#recipes-search").Fill("daily-reddit"); err != nil {
		t.Fatalf("fill search: %v", err)
	}
	page.WaitForTimeout(200)
	filtered, _ := page.Locator("#recipes-list .recipe-item").Count()
	if filtered != 1 {
		t.Fatalf("expected 1 item matching 'daily-reddit', got %d", filtered)
	}

	// Click the remaining item.
	if err := page.Locator("#recipes-list .recipe-item").First().Click(); err != nil {
		t.Fatalf("click recipe: %v", err)
	}

	// Detail panel should now be visible with title + prompt pre-rendered.
	detail := page.Locator("#recipe-detail")
	if err := detail.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(3000),
	}); err != nil {
		t.Fatalf("detail panel never shown: %v", err)
	}
	// Title is set asynchronously after the /api/recipes/<name> fetch resolves,
	// so poll until it lands rather than reading on a race.
	var title string
	for i := 0; i < 30; i++ {
		txt, _ := page.Locator("#recipe-title").TextContent()
		if strings.Contains(strings.ToLower(txt), "reddit") {
			title = txt
			break
		}
		page.WaitForTimeout(100)
	}
	if !strings.Contains(strings.ToLower(title), "reddit") {
		// Pull the /api/recipes/<name> response directly to distinguish API
		// failure from UI render failure.
		apiBody, _ := page.Evaluate(`async () => {
			const r = await fetch('/api/recipes/daily-reddit-digest', { credentials: 'same-origin' });
			return { status: r.status, body: await r.text() };
		}`)
		t.Fatalf("expected Reddit recipe title, got %q (api: %v)", title, apiBody)
	}

	// Wait for the auto-render to populate the prompt box.
	if err := page.Locator("#recipe-rendered").WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(3000),
	}); err != nil {
		t.Fatalf("rendered box missing: %v", err)
	}
	// Rendered prompt should contain default subreddit list.
	var renderedText string
	for i := 0; i < 20; i++ { // poll up to ~2s
		rt, _ := page.Locator("#recipe-rendered").TextContent()
		if rt != "" {
			renderedText = rt
			break
		}
		page.WaitForTimeout(100)
	}
	if !strings.Contains(renderedText, "LocalLLaMA") {
		t.Fatalf("expected default subreddit 'LocalLLaMA' in rendered prompt, got %q", renderedText)
	}

	// Edit a parameter (digest_time) then click Render — prompt should reflect change.
	if err := page.Locator("[data-param='digest_time']").Fill("06:30"); err != nil {
		t.Fatalf("fill digest_time: %v", err)
	}
	if err := page.Locator("#recipe-render-btn").Click(); err != nil {
		t.Fatalf("click render: %v", err)
	}
	for i := 0; i < 20; i++ {
		rt, _ := page.Locator("#recipe-rendered").TextContent()
		if strings.Contains(rt, "06:30") {
			renderedText = rt
			break
		}
		page.WaitForTimeout(100)
	}
	if !strings.Contains(renderedText, "06:30") {
		t.Fatalf("render did not pick up digest_time override, got %q", renderedText)
	}

	// Run — the rendered prompt should be posted to /api/chat and the response
	// panel should appear.
	if err := page.Locator("#recipe-run-btn").Click(); err != nil {
		t.Fatalf("click run: %v", err)
	}

	if err := page.Locator("#recipe-output-wrap").WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Fatalf("output wrap never shown: %v", err)
	}
	// Output is filled just after the wrap is unhidden — poll for the text.
	var out string
	for i := 0; i < 30; i++ {
		txt, _ := page.Locator("#recipe-output").TextContent()
		if strings.Contains(txt, "Mocked Sofia response") {
			out = txt
			break
		}
		page.WaitForTimeout(100)
	}
	if !strings.Contains(out, "Mocked Sofia response") {
		t.Fatalf("expected mocked chat response in output, got %q", out)
	}

	if chat.count() == 0 {
		t.Fatal("Run button did not POST to /api/chat")
	}
	if !strings.Contains(chat.last(), "06:30") {
		t.Fatalf("chat message missing override, got %q", chat.last())
	}

	// Assert no stray console errors or failed requests polluted the flow.
	// Same benign-patterns filter as the nav smoke — we only care about
	// application-level bugs, not mock-server gaps.
	benign := func(msg string) bool {
		for _, p := range []string{
			"Failed to load resource",
			"Refused to apply style",
			"WebSocket connection",
			"ws://",
			"/favicon",
			"/assets/",
			"Config fetch failed", // mock config stub doesn't carry every property the dashboard polls
		} {
			if strings.Contains(msg, p) {
				return true
			}
		}
		return false
	}
	for _, e := range *errs {
		if benign(e) {
			continue
		}
		t.Errorf("unexpected console error during smoke: %s", e)
	}
	for _, fr := range *failed {
		if benign(fr) {
			continue
		}
		t.Errorf("unexpected failed request: %s", fr)
	}
}

// TestBrowser_Recipe_RunShowsError asserts that when /api/chat fails, the UI
// surfaces the failure in the status area rather than silently eating it.
func TestBrowser_Recipe_RunShowsError(t *testing.T) {
	addr, chat, stopServer := recipeBrowserServer(t)
	defer stopServer()
	chat.fail.Store(true)

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	if _, err := page.Goto(addr + "/ui/recipes"); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	if err := page.Locator("#recipes-list .recipe-item").First().WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Fatalf("list never rendered: %v", err)
	}

	if err := page.Locator("#recipes-list .recipe-item").First().Click(); err != nil {
		t.Fatalf("click: %v", err)
	}
	if err := page.Locator("#recipe-detail").WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(3000),
	}); err != nil {
		t.Fatalf("detail: %v", err)
	}
	if err := page.Locator("#recipe-run-btn").Click(); err != nil {
		t.Fatalf("run: %v", err)
	}

	var status string
	for i := 0; i < 30; i++ {
		s, _ := page.Locator("#recipe-status").TextContent()
		if strings.HasPrefix(s, "Run failed") {
			status = s
			break
		}
		page.WaitForTimeout(100)
	}
	if !strings.HasPrefix(status, "Run failed") {
		t.Fatalf("expected 'Run failed:' status, got %q", status)
	}
}

// TestBrowser_Nav_Smoke walks through every sidebar nav link on the index page,
// asserts each UI partial loads into #main-content, and collects any console
// errors or non-asset 4xx/5xx responses along the way.
func TestBrowser_Nav_Smoke(t *testing.T) {
	addr, _, stopServer := recipeBrowserServer(t)
	defer stopServer()

	page, stopBrowser := launchBrowser(t)
	defer stopBrowser()

	errs, failed, detach := consoleCapture(page)
	defer detach()

	if _, err := page.Goto(addr + "/"); err != nil {
		t.Fatalf("index: %v", err)
	}

	// Each nav id must resolve. The layout ships nav-chat through nav-recipes.
	navIDs := []string{
		"nav-chat", "nav-goals", "nav-agents", "nav-history",
		"nav-memory", "nav-calendar", "nav-monitor", "nav-files",
		"nav-eval", "nav-recipes", "nav-ai", "nav-platform",
	}

	for _, id := range navIDs {
		t.Run(id, func(t *testing.T) {
			loc := page.Locator("#" + id)
			if err := loc.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateAttached,
				Timeout: playwright.Float(3000),
			}); err != nil {
				t.Fatalf("nav %s missing: %v", id, err)
			}
			if err := loc.Click(); err != nil {
				t.Fatalf("click %s: %v", id, err)
			}
			// Let HTMX swap the partial in; if #main-content never updates, the
			// test will still fail below via empty content.
			page.WaitForTimeout(250)
			html, _ := page.Locator("#main-content").InnerHTML()
			if strings.TrimSpace(html) == "" {
				t.Fatalf("nav %s: #main-content stayed empty", id)
			}
		})
	}

	// Filter out errors that only appear because our mock server doesn't
	// implement the full Sofia surface — we're after real JS bugs, not mock
	// gaps. Patterns here cover: resource-load failures from static assets,
	// WebSocket stubs, MIME-type quibbles from Go's FileServer when style.css
	// is served from a surrogate path, and partial-data errors from endpoints
	// that the catch-all `[]` response happens to be wrong shape for.
	benign := func(msg string) bool {
		patterns := []string{
			"Failed to load resource",
			"Refused to apply style",
			"WebSocket connection",
			"ws://",
			"/favicon",
			"/assets/",
			"Config fetch failed", // mock config stub doesn't carry every property the dashboard polls
		}
		for _, p := range patterns {
			if strings.Contains(msg, p) {
				return true
			}
		}
		return false
	}
	for _, e := range *errs {
		if benign(e) {
			continue
		}
		t.Errorf("console error during nav: %s", e)
	}
	for _, fr := range *failed {
		if benign(fr) {
			continue
		}
		t.Errorf("failed request during nav: %s", fr)
	}
}
