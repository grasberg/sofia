package openai_compat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestProviderChat_UsesMaxCompletionTokensForGLM(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]any{"content": "ok"},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	_, err := p.Chat(
		t.Context(),
		[]Message{{Role: "user", Content: "hi"}},
		nil,
		"glm-4.7",
		map[string]any{"max_tokens": 1234},
	)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if _, ok := requestBody["max_completion_tokens"]; !ok {
		t.Fatalf("expected max_completion_tokens in request body")
	}
	if _, ok := requestBody["max_tokens"]; ok {
		t.Fatalf("did not expect max_tokens key for glm model")
	}
}

func TestProviderChat_ParsesToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "",
						"tool_calls": []map[string]any{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]any{
									"name":      "get_weather",
									"arguments": "{\"city\":\"SF\"}",
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	out, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "hi"}}, nil, "gpt-4o", nil)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if len(out.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(out.ToolCalls))
	}
	if out.ToolCalls[0].Name != "get_weather" {
		t.Fatalf("ToolCalls[0].Name = %q, want %q", out.ToolCalls[0].Name, "get_weather")
	}
	if out.ToolCalls[0].Arguments["city"] != "SF" {
		t.Fatalf("ToolCalls[0].Arguments[city] = %v, want SF", out.ToolCalls[0].Arguments["city"])
	}
}

func TestProviderChat_ParsesReasoningContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content":           "The answer is 2",
						"reasoning_content": "Let me think step by step... 1+1=2",
						"tool_calls": []map[string]any{
							{
								"id":   "call_1",
								"type": "function",
								"function": map[string]any{
									"name":      "calculator",
									"arguments": "{\"expr\":\"1+1\"}",
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	out, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "1+1=?"}}, nil, "kimi-k2.5", nil)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if out.ReasoningContent != "Let me think step by step... 1+1=2" {
		t.Fatalf("ReasoningContent = %q, want %q", out.ReasoningContent, "Let me think step by step... 1+1=2")
	}
	if out.Content != "The answer is 2" {
		t.Fatalf("Content = %q, want %q", out.Content, "The answer is 2")
	}
	if len(out.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(out.ToolCalls))
	}
}

func TestProviderChat_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	_, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "hi"}}, nil, "gpt-4o", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestProviderChat_StripsMoonshotPrefixAndNormalizesKimiTemperature(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]any{"content": "ok"},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	_, err := p.Chat(
		t.Context(),
		[]Message{{Role: "user", Content: "hi"}},
		nil,
		"moonshot/kimi-k2.5",
		map[string]any{"temperature": 0.3},
	)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if requestBody["model"] != "kimi-k2.5" {
		t.Fatalf("model = %v, want kimi-k2.5", requestBody["model"])
	}
	if requestBody["temperature"] != 1.0 {
		t.Fatalf("temperature = %v, want 1.0", requestBody["temperature"])
	}
}

func TestProviderChat_StripsGroqAndOllamaPrefixes(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantModel string
	}{
		{
			name:      "strips groq prefix and keeps nested model",
			input:     "groq/openai/gpt-oss-120b",
			wantModel: "openai/gpt-oss-120b",
		},
		{
			name:      "strips ollama prefix",
			input:     "ollama/qwen2.5:14b",
			wantModel: "qwen2.5:14b",
		},
		{
			name:      "strips deepseek prefix",
			input:     "deepseek/deepseek-chat",
			wantModel: "deepseek-chat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requestBody map[string]any

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				resp := map[string]any{
					"choices": []map[string]any{
						{
							"message":       map[string]any{"content": "ok"},
							"finish_reason": "stop",
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			p := NewProvider("key", server.URL, "")
			_, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "hi"}}, nil, tt.input, nil)
			if err != nil {
				t.Fatalf("Chat() error = %v", err)
			}

			if requestBody["model"] != tt.wantModel {
				t.Fatalf("model = %v, want %s", requestBody["model"], tt.wantModel)
			}
		})
	}
}

func TestProvider_ProxyConfigured(t *testing.T) {
	proxyURL := "http://127.0.0.1:8080"
	p := NewProvider("key", "https://example.com", proxyURL)

	transport, ok := p.httpClient.Transport.(*http.Transport)
	if !ok || transport == nil {
		t.Fatalf("expected http transport with proxy, got %T", p.httpClient.Transport)
	}

	req := &http.Request{URL: &url.URL{Scheme: "https", Host: "api.example.com"}}
	gotProxy, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("proxy function returned error: %v", err)
	}
	if gotProxy == nil || gotProxy.String() != proxyURL {
		t.Fatalf("proxy = %v, want %s", gotProxy, proxyURL)
	}
}

func TestProviderChat_AcceptsNumericOptionTypes(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]any{"content": "ok"},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	_, err := p.Chat(
		t.Context(),
		[]Message{{Role: "user", Content: "hi"}},
		nil,
		"gpt-4o",
		map[string]any{"max_tokens": float64(512), "temperature": 1},
	)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if requestBody["max_tokens"] != float64(512) {
		t.Fatalf("max_tokens = %v, want 512", requestBody["max_tokens"])
	}
	if requestBody["temperature"] != float64(1) {
		t.Fatalf("temperature = %v, want 1", requestBody["temperature"])
	}
}

func TestNormalizeModel_UsesAPIBase(t *testing.T) {
	if got := normalizeModel("deepseek/deepseek-chat", "https://api.deepseek.com/v1"); got != "deepseek-chat" {
		t.Fatalf("normalizeModel(deepseek) = %q, want %q", got, "deepseek-chat")
	}
	if got := normalizeModel("openrouter/auto", "https://openrouter.ai/api/v1"); got != "openrouter/auto" {
		t.Fatalf("normalizeModel(openrouter) = %q, want %q", got, "openrouter/auto")
	}
}

func TestProvider_RequestTimeoutDefault(t *testing.T) {
	p := NewProviderWithMaxTokensFieldAndTimeout("key", "https://example.com/v1", "", "", 0)
	if p.httpClient.Timeout != defaultRequestTimeout {
		t.Fatalf("http timeout = %v, want %v", p.httpClient.Timeout, defaultRequestTimeout)
	}
}

func TestProvider_RequestTimeoutOverride(t *testing.T) {
	p := NewProviderWithMaxTokensFieldAndTimeout("key", "https://example.com/v1", "", "", 300)
	if p.httpClient.Timeout != 300*time.Second {
		t.Fatalf("http timeout = %v, want %v", p.httpClient.Timeout, 300*time.Second)
	}
}

func TestProvider_FunctionalOptionMaxTokensField(t *testing.T) {
	p := NewProvider("key", "https://example.com/v1", "", WithMaxTokensField("max_completion_tokens"))
	if p.maxTokensField != "max_completion_tokens" {
		t.Fatalf("maxTokensField = %q, want %q", p.maxTokensField, "max_completion_tokens")
	}
}

// TestInferMaxTokensField locks in the fallback routing for request bodies
// when a catalog entry or user config doesn't set MaxTokensField explicitly.
// OpenAI's reasoning models (o-series) and all GPT-5 variants fail with a
// 400 "Unsupported parameter" error if we send "max_tokens" instead of
// "max_completion_tokens"; Z.ai's GLM family behaves the same.
//
// The false-positive cases guard against name substrings leaking into the
// wrong branch — e.g. a fine-tune called "kimi-o1-preview" must not be
// routed to max_completion_tokens just because "o1" appears in the id.
func TestInferMaxTokensField(t *testing.T) {
	cases := []struct {
		model string
		want  string
	}{
		// OpenAI reasoning models — must use max_completion_tokens.
		{"o1", "max_completion_tokens"},
		{"o1-mini", "max_completion_tokens"},
		{"o1-preview", "max_completion_tokens"},
		{"o3", "max_completion_tokens"},
		{"o3-mini", "max_completion_tokens"},
		{"o3-pro", "max_completion_tokens"},
		{"o4-mini", "max_completion_tokens"},
		// GPT-5 family.
		{"gpt-5", "max_completion_tokens"},
		{"gpt-5-mini", "max_completion_tokens"},
		{"gpt-5.2", "max_completion_tokens"},
		{"gpt-5.2-codex", "max_completion_tokens"},
		// Z.ai GLM family.
		{"glm-5.1", "max_completion_tokens"},
		{"glm-4.7-flash", "max_completion_tokens"},
		// Models that still use the classic field.
		{"gpt-4o", "max_tokens"},
		{"gpt-4o-mini", "max_tokens"},
		{"gpt-4.1", "max_tokens"},
		{"claude-opus-4-6", "max_tokens"},
		{"llama-3.3-70b-versatile", "max_tokens"},
		{"deepseek-chat", "max_tokens"},
		// False-positive guards — an o-series prefix embedded in another name
		// must not trigger the reasoning-model branch.
		{"kimi-o1-preview", "max_tokens"},
		{"foo-o3", "max_tokens"},
	}
	for _, c := range cases {
		t.Run(c.model, func(t *testing.T) {
			if got := inferMaxTokensField(c.model); got != c.want {
				t.Errorf("inferMaxTokensField(%q) = %q, want %q", c.model, got, c.want)
			}
		})
	}
}

func TestProvider_FunctionalOptionRequestTimeout(t *testing.T) {
	p := NewProvider("key", "https://example.com/v1", "", WithRequestTimeout(45*time.Second))
	if p.httpClient.Timeout != 45*time.Second {
		t.Fatalf("http timeout = %v, want %v", p.httpClient.Timeout, 45*time.Second)
	}
}

func TestProvider_FunctionalOptionRequestTimeoutNonPositive(t *testing.T) {
	p := NewProvider("key", "https://example.com/v1", "", WithRequestTimeout(-1*time.Second))
	if p.httpClient.Timeout != defaultRequestTimeout {
		t.Fatalf("http timeout = %v, want %v", p.httpClient.Timeout, defaultRequestTimeout)
	}
}

func TestHostRequiresAPIKey(t *testing.T) {
	cases := []struct {
		name     string
		apiBase  string
		requires bool
	}{
		{"minimax", "https://api.minimax.io/v1", true},
		{"openai", "https://api.openai.com/v1", true},
		{"ollama-local", "http://localhost:11434/v1", false},
		{"ollama-127", "http://127.0.0.1:11434/v1", false},
		{"0.0.0.0 bind", "http://0.0.0.0:8080/v1", false},
		{"ollama-cloud", "https://ollama.com/v1", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hostRequiresAPIKey(c.apiBase); got != c.requires {
				t.Errorf("hostRequiresAPIKey(%q) = %v, want %v", c.apiBase, got, c.requires)
			}
		})
	}
}

// When the user hasn't entered an API key for a remote provider, Chat must
// fail fast with a helpful 401-shaped error — not fire off an unauthenticated
// request and bubble back upstream's cryptic "Please carry the API secret
// key" message.
func TestProviderChat_PreflightsEmptyKeyOnRemote(t *testing.T) {
	var requestsReceived int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsReceived++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Use an explicitly remote-looking URL (not the httptest's 127.0.0.1 base
	// — that would bypass the preflight). We never actually hit the network
	// because the preflight short-circuits.
	p := NewProvider("", "https://api.minimax.io/v1", "")

	_, err := p.Chat(context.Background(), nil, nil, "MiniMax-M2.7", nil)
	if err == nil {
		t.Fatal("expected preflight error for empty key on remote endpoint")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Status: 401") {
		t.Errorf("expected 401-shaped error (for auth classifier), got: %s", msg)
	}
	if !strings.Contains(msg, "Settings") {
		t.Errorf("expected pointer to Settings, got: %s", msg)
	}
	if !strings.Contains(msg, "MiniMax-M2.7") {
		t.Errorf("expected model name in error, got: %s", msg)
	}
	if requestsReceived != 0 {
		t.Errorf("expected zero network calls when preflight fires, got %d", requestsReceived)
	}
}

// Localhost endpoints (Ollama, self-hosted servers) routinely run without
// an API key — preflight must NOT interfere there.
func TestProviderChat_PreflightSkipsLocalhost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	// httptest.Server URL is 127.0.0.1:<port> which our preflight treats as
	// local and therefore keyless-OK.
	p := NewProvider("", server.URL, "")

	resp, err := p.Chat(context.Background(), nil, nil, "local-model", nil)
	if err != nil {
		t.Fatalf("expected no preflight error on localhost, got: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
}

// When the key is set, preflight is a no-op and we hit the server normally.
func TestProviderChat_PreflightPassesWhenKeySet(t *testing.T) {
	var seenAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	p := NewProvider("secret-key-123", server.URL, "")
	if _, err := p.Chat(context.Background(), nil, nil, "model", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seenAuth != "Bearer secret-key-123" {
		t.Errorf("expected Authorization: Bearer secret-key-123, got %q", seenAuth)
	}
}

// 404s on chat/completions almost always mean "wrong API base" or "the model
// id isn't hosted here" — neither of which is obvious from the default nginx
// "404 page not found" body. The helpful error must surface the URL and
// model so the user can fix their config without guessing.
func TestProviderChat_404ErrorIncludesURLAndModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r) // writes "404 page not found"
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	_, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "hi"}}, nil, "fake-model-xyz", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Status: 404") {
		t.Errorf("expected status 404 in message, got: %s", msg)
	}
	if !strings.Contains(msg, server.URL+"/chat/completions") {
		t.Errorf("expected requested URL in message, got: %s", msg)
	}
	if !strings.Contains(msg, "fake-model-xyz") {
		t.Errorf("expected model id in message, got: %s", msg)
	}
	if !strings.Contains(msg, "/v1") {
		t.Errorf("expected '/v1' hint in message, got: %s", msg)
	}
}

// When /chat/completions returns 404, the provider probes /models to show
// the user which model ids are actually hosted. This turns a dead-end
// "404 page not found" into an actionable error.
func TestProviderChat_404ListsAvailableModelsFromProbe(t *testing.T) {
	var chatHits, modelsHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			chatHits++
			http.NotFound(w, r)
		case "/models":
			modelsHits++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[
                {"id":"meta/llama-3.3-70b-instruct"},
                {"id":"nvidia/llama-3.1-nemotron-70b-instruct"},
                {"id":"minimaxai/minimax-m2"}
            ]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	_, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "hi"}}, nil, "fake", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if chatHits != 1 {
		t.Errorf("expected 1 chat call, got %d", chatHits)
	}
	if modelsHits != 1 {
		t.Errorf("expected probe to hit /models once, got %d", modelsHits)
	}
	msg := err.Error()
	if !strings.Contains(msg, "Available at this endpoint") {
		t.Errorf("expected available-models section, got: %s", msg)
	}
	for _, id := range []string{"meta/llama-3.3-70b-instruct", "minimaxai/minimax-m2"} {
		if !strings.Contains(msg, id) {
			t.Errorf("expected model id %q in error, got: %s", id, msg)
		}
	}
}

// If the /models probe itself fails (auth, network), we still produce a
// useful 404 error — just without the available-models list.
func TestProviderChat_404WithFailedProbeStillHelpful(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Both endpoints 404: simulates a completely wrong api base.
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	_, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "hi"}}, nil, "x", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Status: 404") {
		t.Errorf("expected 404 status in message, got: %s", msg)
	}
	// No "Available at this endpoint:" line should appear when the probe fails.
	if strings.Contains(msg, "Available at this endpoint") {
		t.Errorf("should not list models when probe failed, got: %s", msg)
	}
}

// Non-404 errors keep the original terse format to avoid noise on real
// upstream errors that already come back structured.
func TestProviderChat_Non404ErrorStaysTerse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad things"}`, http.StatusBadRequest)
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	_, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "hi"}}, nil, "m", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Status: 400") {
		t.Errorf("expected 400 in message, got: %s", msg)
	}
	if strings.Contains(msg, "Hint:") {
		t.Errorf("hint should only be added for 404, got: %s", msg)
	}
}
