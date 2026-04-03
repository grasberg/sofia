package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grasberg/sofia/pkg/config"
)

// --- helpers ---

// newTestServer creates a Server with a minimal config and nil agentLoop.
// UI template routes and middleware tests do not touch the agent loop.
func newTestServer(authToken string) *Server {
	cfg := &config.Config{
		WebUI: config.WebUIConfig{
			Enabled:   true,
			Host:      "127.0.0.1",
			Port:      0,
			AuthToken: authToken,
		},
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: "/tmp/sofia-test-ws",
			},
		},
	}
	return newTestServerWithConfig(cfg)
}

// newTestServerWithConfig constructs a Server without a real AgentLoop.
// It manually wires the mux with only the routes that do NOT require the loop.
func newTestServerWithConfig(cfg *config.Config) *Server {
	s := &Server{cfg: cfg}

	mux := http.NewServeMux()

	// Index
	mux.HandleFunc("/", s.handleIndex)

	// UI partials
	mux.HandleFunc("/ui/chat", templateHandler(chatHTML))
	mux.HandleFunc("/ui/agents", templateHandler(agentsHTML))
	mux.HandleFunc("/ui/monitor", templateHandler(monitorHTML))
	mux.HandleFunc("/ui/calendar", templateHandler(calendarHTML))
	mux.HandleFunc("/ui/memory", templateHandler(memoryHTML))
	mux.HandleFunc("/ui/pixels", templateHandler(pixelsHTML))
	mux.HandleFunc("/ui/goals", templateHandler(goalsHTML))
	mux.HandleFunc("/ui/history", templateHandler(historyHTML))
	mux.HandleFunc("/ui/settings/models", templateHandler(settingsModelsHTML))
	mux.HandleFunc("/ui/settings/channels", templateHandler(settingsChannelsHTML))
	mux.HandleFunc("/ui/settings/tools", templateHandler(settingsToolsHTML))
	mux.HandleFunc("/ui/settings/integrations", templateHandler(settingsIntegrationsHTML))
	mux.HandleFunc("/ui/settings/skills", templateHandler(settingsSkillsHTML))
	mux.HandleFunc("/ui/settings/heartbeat", templateHandler(settingsHeartbeatHTML))
	mux.HandleFunc("/ui/settings/security", templateHandler(settingsSecurityHTML))
	mux.HandleFunc("/ui/settings/prompts", templateHandler(settingsPromptsHTML))
	mux.HandleFunc("/ui/settings/logs", templateHandler(settingsLogsHTML))
	mux.HandleFunc("/ui/settings/evolution", templateHandler(settingsEvolutionHTML))
	mux.HandleFunc("/ui/settings/autonomy", templateHandler(settingsAutonomyHTML))
	mux.HandleFunc("/ui/settings/intelligence", templateHandler(settingsIntelligenceHTML))
	mux.HandleFunc("/ui/settings/budget", templateHandler(settingsBudgetHTML))
	mux.HandleFunc("/ui/settings/tts", templateHandler(settingsTTSHTML))
	mux.HandleFunc("/ui/settings/webhooks", templateHandler(settingsWebhooksHTML))
	mux.HandleFunc("/ui/settings/triggers", templateHandler(settingsTriggersHTML))
	mux.HandleFunc("/ui/settings/remote", templateHandler(settingsRemoteHTML))
	mux.HandleFunc("/ui/settings/cron", templateHandler(settingsCronHTML))
	mux.HandleFunc("/ui/settings/personas", templateHandler(settingsPersonasHTML))

	// API routes through auth middleware (for testing auth itself)
	mux.HandleFunc("/api/config", s.authMiddleware(s.handleConfig))
	mux.HandleFunc("GET /api/skills", s.authMiddleware(s.handleSkillsList))
	mux.HandleFunc("POST /api/skills/toggle", s.authMiddleware(s.handleSkillsToggle))

	s.mux = mux
	return s
}

func templateHandler(data []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	}
}

// --- maskSecrets tests ---

func TestMaskSecrets_SimpleMap(t *testing.T) {
	input := map[string]any{
		"name":    "test",
		"api_key": "sk-secret-123",
		"token":   "tok-abc",
	}
	result := maskSecrets(input).(map[string]any)

	if result["name"] != "test" {
		t.Errorf("expected name=test, got %v", result["name"])
	}
	if result["api_key"] != "********" {
		t.Errorf("expected api_key to be masked, got %v", result["api_key"])
	}
	if result["token"] != "********" {
		t.Errorf("expected token to be masked, got %v", result["token"])
	}
}

func TestMaskSecrets_NestedMap(t *testing.T) {
	input := map[string]any{
		"providers": map[string]any{
			"openai": map[string]any{
				"api_key": "sk-deep-nested",
				"model":   "gpt-4o",
			},
		},
	}
	result := maskSecrets(input).(map[string]any)
	providers := result["providers"].(map[string]any)
	openai := providers["openai"].(map[string]any)

	if openai["api_key"] != "********" {
		t.Errorf("expected nested api_key to be masked, got %v", openai["api_key"])
	}
	if openai["model"] != "gpt-4o" {
		t.Errorf("expected model to be unchanged, got %v", openai["model"])
	}
}

func TestMaskSecrets_SliceOfMaps(t *testing.T) {
	input := []any{
		map[string]any{"password": "secret1", "host": "localhost"},
		map[string]any{"password": "secret2", "host": "remote"},
	}
	result := maskSecrets(input).([]any)

	for i, item := range result {
		m := item.(map[string]any)
		if m["password"] != "********" {
			t.Errorf("item %d: expected password masked, got %v", i, m["password"])
		}
	}
}

func TestMaskSecrets_EmptyStringNotMasked(t *testing.T) {
	input := map[string]any{
		"api_key": "",
		"token":   "",
	}
	result := maskSecrets(input).(map[string]any)

	// Empty strings should NOT be masked (they are not set)
	if result["api_key"] != "" {
		t.Errorf("expected empty api_key to stay empty, got %v", result["api_key"])
	}
}

func TestMaskSecrets_AllSensitiveFields(t *testing.T) {
	input := map[string]any{
		"api_key":        "val1",
		"token":          "val2",
		"password":       "val3",
		"passphrase":     "val4",
		"secret_api_key": "val5",
		"secret":         "val6",
		"auth_token":     "val7",
		"api_token":      "val8",
	}
	result := maskSecrets(input).(map[string]any)
	for key, val := range result {
		if val != "********" {
			t.Errorf("expected %s to be masked, got %v", key, val)
		}
	}
}

func TestMaskSecrets_NonMapNonSlice(t *testing.T) {
	// Primitives should pass through unchanged
	if maskSecrets("hello") != "hello" {
		t.Error("string should pass through")
	}
	if maskSecrets(42) != 42 {
		t.Error("int should pass through")
	}
	if maskSecrets(nil) != nil {
		t.Error("nil should pass through")
	}
}

// --- configToMaskedJSON tests ---

func TestConfigToMaskedJSON(t *testing.T) {
	cfg := &config.Config{
		WebUI: config.WebUIConfig{
			Enabled:   true,
			Host:      "localhost",
			Port:      8080,
			AuthToken: "super-secret-token",
		},
	}

	data, err := configToMaskedJSON(cfg)
	if err != nil {
		t.Fatalf("configToMaskedJSON error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	webui := result["webui"].(map[string]any)
	if webui["auth_token"] != "********" {
		t.Errorf("expected auth_token masked, got %v", webui["auth_token"])
	}
	if webui["host"] != "localhost" {
		t.Errorf("expected host=localhost, got %v", webui["host"])
	}
}

// --- authMiddleware tests ---

func TestAuthMiddleware_NoToken_AllowsAll(t *testing.T) {
	s := newTestServer("")
	handler := s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// GET without any auth should pass when no token configured
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_WithToken_RejectsNoAuth(t *testing.T) {
	s := newTestServer("test-secret-token")
	handler := s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_WithToken_AcceptsValidBearer(t *testing.T) {
	s := newTestServer("test-secret-token")
	handler := s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_WithToken_RejectsWrongToken(t *testing.T) {
	s := newTestServer("correct-token")
	handler := s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_CSRF_PostWithoutHeader(t *testing.T) {
	s := newTestServer("") // No auth token — only CSRF check
	handler := s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for POST without X-Requested-With, got %d", w.Code)
	}
}

func TestAuthMiddleware_CSRF_PostWithHeader(t *testing.T) {
	s := newTestServer("")
	handler := s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader("{}"))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_CSRF_PutWithoutHeader(t *testing.T) {
	s := newTestServer("")
	handler := s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for PUT without X-Requested-With, got %d", w.Code)
	}
}

func TestAuthMiddleware_CSRF_DeleteWithoutHeader(t *testing.T) {
	s := newTestServer("")
	handler := s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/something", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for DELETE without X-Requested-With, got %d", w.Code)
	}
}

func TestAuthMiddleware_CSRF_GetDoesNotRequireHeader(t *testing.T) {
	s := newTestServer("")
	handler := s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for GET without X-Requested-With, got %d", w.Code)
	}
}

func TestAuthMiddleware_CombinedAuthAndCSRF(t *testing.T) {
	s := newTestServer("my-token")
	handler := s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// POST with valid auth but no CSRF → 403
	req := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer my-token")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 (missing CSRF), got %d", w.Code)
	}

	// POST with valid auth AND CSRF → 200
	req = httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer my-token")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w = httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- sendJSONError tests ---

func TestSendJSONError(t *testing.T) {
	s := newTestServer("")
	w := httptest.NewRecorder()
	s.sendJSONError(w, "something went wrong", http.StatusBadRequest)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if body["error"] != "something went wrong" {
		t.Errorf("expected error message, got %v", body)
	}
}

// --- handleIndex tests ---

func TestHandleIndex_Root(t *testing.T) {
	s := newTestServer("")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	s.handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("expected text/html, got %s", ct)
	}
	// layout.html should be served
	if w.Body.Len() == 0 {
		t.Error("expected non-empty body for index")
	}
}

func TestHandleIndex_NotFoundForOtherPaths(t *testing.T) {
	s := newTestServer("")
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	s.handleIndex(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for /nonexistent, got %d", w.Code)
	}
}

// --- UI template route tests ---

func TestUIRoutes_ServeHTMLContent(t *testing.T) {
	s := newTestServer("")

	routes := []string{
		"/ui/chat",
		"/ui/agents",
		"/ui/monitor",
		"/ui/calendar",
		"/ui/memory",
		"/ui/pixels",
		"/ui/goals",
		"/ui/history",
		"/ui/settings/models",
		"/ui/settings/channels",
		"/ui/settings/tools",
		"/ui/settings/integrations",
		"/ui/settings/skills",
		"/ui/settings/heartbeat",
		"/ui/settings/security",
		"/ui/settings/prompts",
		"/ui/settings/logs",
		"/ui/settings/evolution",
		"/ui/settings/autonomy",
		"/ui/settings/intelligence",
		"/ui/settings/budget",
		"/ui/settings/tts",
		"/ui/settings/webhooks",
		"/ui/settings/triggers",
		"/ui/settings/remote",
		"/ui/settings/cron",
		"/ui/settings/personas",
	}

	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, route, nil)
			w := httptest.NewRecorder()
			s.mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("route %s: expected 200, got %d", route, w.Code)
			}
			ct := w.Header().Get("Content-Type")
			if ct != "text/html; charset=utf-8" {
				t.Errorf("route %s: expected text/html, got %s", route, ct)
			}
			if w.Body.Len() == 0 {
				t.Errorf("route %s: expected non-empty body", route)
			}
		})
	}
}

// --- handleConfig GET test (with mux) ---

func TestHandleConfig_GET_MasksSecrets(t *testing.T) {
	s := newTestServer("")
	// Set secrets that should be masked in the response.
	// AuthToken is empty so auth middleware does not block.
	s.cfg.Providers.OpenAI.APIKey = "sk-openai-key"

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if strings.Contains(body, "sk-openai-key") {
		t.Error("response should not contain unmasked api_key")
	}
	if !strings.Contains(body, "********") {
		t.Error("response should contain masked values")
	}
}

func TestHandleConfig_POST_WithoutCSRF_Rejected(t *testing.T) {
	s := newTestServer("")

	body := `{"webui":{"enabled":true}}`
	req := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for POST without CSRF header, got %d", w.Code)
	}
}

// --- limitBody test ---

func TestLimitBody(t *testing.T) {
	// Create a request with a small body
	body := strings.NewReader("hello")
	req := httptest.NewRequest(http.MethodPost, "/", body)
	limitBody(req)

	data, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read limited body: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got '%s'", string(data))
	}
}

// --- restoreMaskedSecrets tests ---

func TestRestoreMaskedSecrets_RestoresPlaceholders(t *testing.T) {
	incoming := map[string]any{
		"providers": map[string]any{
			"openai": map[string]any{
				"api_key": "********",
				"model":   "gpt-4o",
			},
		},
	}
	original := map[string]any{
		"providers": map[string]any{
			"openai": map[string]any{
				"api_key": "sk-real-key-123",
				"model":   "gpt-3.5",
			},
		},
	}

	restoreMaskedSecrets(incoming, original)

	providers := incoming["providers"].(map[string]any)
	openai := providers["openai"].(map[string]any)
	if openai["api_key"] != "sk-real-key-123" {
		t.Errorf("expected restored api_key, got %v", openai["api_key"])
	}
	if openai["model"] != "gpt-4o" {
		t.Errorf("expected incoming model value gpt-4o, got %v", openai["model"])
	}
}

func TestRestoreMaskedSecrets_KeepsNewKeys(t *testing.T) {
	incoming := map[string]any{
		"api_key": "sk-brand-new-key",
	}
	original := map[string]any{
		"api_key": "sk-old-key",
	}

	restoreMaskedSecrets(incoming, original)

	if incoming["api_key"] != "sk-brand-new-key" {
		t.Errorf("should keep new key when not masked, got %v", incoming["api_key"])
	}
}

func TestRestoreMaskedSecrets_SliceOfMaps(t *testing.T) {
	incoming := []any{
		map[string]any{"api_key": "********", "name": "model-a"},
		map[string]any{"api_key": "sk-new", "name": "model-b"},
	}
	original := []any{
		map[string]any{"api_key": "sk-real-a", "name": "model-a"},
		map[string]any{"api_key": "sk-real-b", "name": "model-b"},
	}

	result := restoreMaskedSecrets(incoming, original)

	arr := result.([]any)
	first := arr[0].(map[string]any)
	if first["api_key"] != "sk-real-a" {
		t.Errorf("expected restored key in slice[0], got %v", first["api_key"])
	}
	second := arr[1].(map[string]any)
	if second["api_key"] != "sk-new" {
		t.Errorf("should keep new key in slice[1], got %v", second["api_key"])
	}
}

func TestRestoreMaskedSecrets_AllSensitiveFields(t *testing.T) {
	fields := []string{
		"api_key",
		"token",
		"password",
		"passphrase",
		"secret_api_key",
		"secret",
		"auth_token",
		"api_token",
	}
	for _, field := range fields {
		incoming := map[string]any{field: "********"}
		original := map[string]any{field: "real-value-" + field}

		restoreMaskedSecrets(incoming, original)

		if incoming[field] != "real-value-"+field {
			t.Errorf("field %s: expected restored value, got %v", field, incoming[field])
		}
	}
}

func TestRestoreMaskedSecrets_NilOriginal(t *testing.T) {
	incoming := map[string]any{
		"api_key": "********",
	}
	restoreMaskedSecrets(incoming, nil)

	if incoming["api_key"] != "********" {
		t.Errorf("expected placeholder to stay when no original, got %v", incoming["api_key"])
	}
}

func TestRestoreMaskedSecrets_DeeplyNested(t *testing.T) {
	incoming := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": map[string]any{
					"api_key": "********",
				},
			},
		},
	}
	original := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": map[string]any{
					"api_key": "sk-deep-secret",
				},
			},
		},
	}

	restoreMaskedSecrets(incoming, original)

	val := incoming["level1"].(map[string]any)["level2"].(map[string]any)["level3"].(map[string]any)["api_key"]
	if val != "sk-deep-secret" {
		t.Errorf("expected deeply nested key restored, got %v", val)
	}
}

// --- RegisterWebhooks test ---

type mockWebhookRegistrar struct {
	called bool
}

func (m *mockWebhookRegistrar) RegisterWebhooks(mux *http.ServeMux) {
	m.called = true
}

func TestRegisterWebhooks(t *testing.T) {
	s := newTestServer("")

	// nil registrar should not panic
	s.RegisterWebhooks(nil)

	// valid registrar should be called
	mock := &mockWebhookRegistrar{}
	s.RegisterWebhooks(mock)
	if !mock.called {
		t.Error("expected RegisterWebhooks to be called on registrar")
	}
}

func TestRegisterWebhooks_NilMux(t *testing.T) {
	s := &Server{cfg: &config.Config{}}
	// mux is nil — should not panic
	mock := &mockWebhookRegistrar{}
	s.RegisterWebhooks(mock)
	if mock.called {
		t.Error("should not call registrar when mux is nil")
	}
}

// --- handleSkillsList tests ---

func TestHandleSkillsList_EmptyWithNilAgentLoop(t *testing.T) {
	s := newTestServer("")
	req := httptest.NewRequest("GET", "/api/skills", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty list, got %d items", len(result))
	}
}

// --- handleSkillsToggle tests ---

func newTestServerWithAgents(agents ...config.AgentConfig) *Server {
	cfg := &config.Config{
		WebUI: config.WebUIConfig{Enabled: true, Host: "127.0.0.1"},
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{Workspace: "/tmp/sofia-test-ws"},
			List:     agents,
		},
	}
	return newTestServerWithConfig(cfg)
}

func TestHandleSkillsToggle_MissingSkillName(t *testing.T) {
	s := newTestServerWithAgents(config.AgentConfig{ID: "main", Default: true})
	body := `{"enabled": false}`
	req := httptest.NewRequest("POST", "/api/skills/toggle", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleSkillsToggle_NoDefaultAgent(t *testing.T) {
	s := newTestServerWithAgents(config.AgentConfig{ID: "sub1"})
	body := `{"skill": "coding", "enabled": false}`
	req := httptest.NewRequest("POST", "/api/skills/toggle", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHandleSkillsToggle_DisableSkill_WithExistingFilter(t *testing.T) {
	agents := []config.AgentConfig{
		{ID: "main", Default: true, Skills: []string{"coding", "debug", "web"}},
	}
	s := newTestServerWithAgents(agents...)

	// Use a temp config file so SaveConfig doesn't touch the real one.
	tmpDir := t.TempDir()
	t.Setenv("SOFIA_CONFIG", tmpDir+"/config.json")

	body := `{"skill": "debug", "enabled": false}`
	req := httptest.NewRequest("POST", "/api/skills/toggle", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	// Verify "debug" was removed from the skills filter.
	for _, a := range s.cfg.Agents.List {
		if a.Default {
			for _, sk := range a.Skills {
				if sk == "debug" {
					t.Error("expected 'debug' to be removed from skills filter")
				}
			}
			if len(a.Skills) != 2 {
				t.Errorf("expected 2 skills, got %d: %v", len(a.Skills), a.Skills)
			}
		}
	}
}

func TestHandleSkillsToggle_EnableSkill(t *testing.T) {
	agents := []config.AgentConfig{
		{ID: "main", Default: true, Skills: []string{"coding", "web"}},
	}
	s := newTestServerWithAgents(agents...)

	tmpDir := t.TempDir()
	t.Setenv("SOFIA_CONFIG", tmpDir+"/config.json")

	body := `{"skill": "debug", "enabled": true}`
	req := httptest.NewRequest("POST", "/api/skills/toggle", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	for _, a := range s.cfg.Agents.List {
		if a.Default {
			found := false
			for _, sk := range a.Skills {
				if sk == "debug" {
					found = true
				}
			}
			if !found {
				t.Errorf("expected 'debug' in skills filter, got: %v", a.Skills)
			}
		}
	}
}

func TestHandleSkillsToggle_EnableDuplicate(t *testing.T) {
	agents := []config.AgentConfig{
		{ID: "main", Default: true, Skills: []string{"coding", "web"}},
	}
	s := newTestServerWithAgents(agents...)

	tmpDir := t.TempDir()
	t.Setenv("SOFIA_CONFIG", tmpDir+"/config.json")

	// Enable a skill that's already in the list.
	body := `{"skill": "coding", "enabled": true}`
	req := httptest.NewRequest("POST", "/api/skills/toggle", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	for _, a := range s.cfg.Agents.List {
		if a.Default {
			count := 0
			for _, sk := range a.Skills {
				if sk == "coding" {
					count++
				}
			}
			if count != 1 {
				t.Errorf("expected 'coding' once, got %d times in: %v", count, a.Skills)
			}
		}
	}
}

func TestHandleSkillsToggle_InvalidJSON(t *testing.T) {
	s := newTestServerWithAgents(config.AgentConfig{ID: "main", Default: true})
	req := httptest.NewRequest("POST", "/api/skills/toggle", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleSkillsToggle_CSRFRequired(t *testing.T) {
	s := newTestServerWithAgents(config.AgentConfig{ID: "main", Default: true})
	body := `{"skill": "coding", "enabled": true}`
	req := httptest.NewRequest("POST", "/api/skills/toggle", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No X-Requested-With header — should be blocked by CSRF check.
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing CSRF header, got %d", w.Code)
	}
}
