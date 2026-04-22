package openai_compat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func normalizeModel(model, apiBase string) string {
	idx := strings.Index(model, "/")
	if idx == -1 {
		return model
	}

	if strings.Contains(strings.ToLower(apiBase), "openrouter.ai") ||
		strings.HasSuffix(model, ":free") || strings.HasSuffix(model, ":extended") {
		return model
	}

	prefix := strings.ToLower(model[:idx])
	switch prefix {
	case "moonshot", "nvidia", "groq", "ollama", "deepseek", "google", "openrouter", "zhipu", "mistral":
		return model[idx+1:]
	default:
		return model
	}
}

// inferMaxTokensField picks the request body field name for the output-token
// cap when the provider config doesn't set one explicitly. OpenAI's reasoning
// models (o1/o3/o4/…) and all GPT-5 variants reject the classic "max_tokens"
// key and require "max_completion_tokens"; Z.ai's GLM family behaves the same.
// Everything else uses the conventional "max_tokens".
//
// Matching is prefix-plus-separator so embedding a family name in an
// unrelated id (e.g. "kimi-o1-preview", "foo-glm-tuned") does not trigger the
// reasoning-model branch. The caller is responsible for passing a lowercased
// id — buildRequestBody lowers once and reuses the same value for the Kimi
// temperature check, so re-lowering here would be wasted work on every call.
func inferMaxTokensField(lowerModel string) string {
	for _, prefix := range maxCompletionTokensPrefixes {
		if lowerModel == prefix ||
			strings.HasPrefix(lowerModel, prefix+"-") ||
			strings.HasPrefix(lowerModel, prefix+".") ||
			strings.HasPrefix(lowerModel, prefix+":") {
			return "max_completion_tokens"
		}
	}
	return "max_tokens"
}

// maxCompletionTokensPrefixes is the set of model-id prefixes whose APIs
// reject the classic "max_tokens" request key. Additions should be API
// contracts, not marketing names — if a provider adds a new reasoning family
// that accepts "max_tokens", do not list it here.
var maxCompletionTokensPrefixes = []string{"o1", "o3", "o4", "o5", "gpt-5", "glm"}

func isOllamaEndpoint(apiBase string) bool {
	return strings.Contains(apiBase, "localhost:11434") ||
		strings.Contains(apiBase, "127.0.0.1:11434") ||
		strings.Contains(apiBase, "ollama.com")
}

// formatHTTPError turns a non-2xx HTTP response into a helpful error. The
// default format is terse ("Status: X / Body: ..."), but for 404 — which
// almost always means the URL or model id is wrong rather than a real
// "not found" — we add the requested URL, the model, and (when possible)
// a list of models actually hosted at the endpoint, pulled live from the
// /models API. That turns "404 page not found" from a dead end into a
// fix-it-in-30-seconds error.
func (p *Provider) formatHTTPError(statusCode int, body []byte, requestURL, model string) error {
	if statusCode == http.StatusNotFound {
		hint := "either the API base is wrong (should normally end in \"/v1\" for " +
			"OpenAI-compatible providers) or the model id is not hosted at this endpoint. " +
			"Check Settings → AI Models."
		// Probe /models on the same base with a tight timeout. If it
		// succeeds, surfacing even a handful of valid ids is usually enough
		// for the user to spot the typo (case mismatch, wrong org, etc.).
		if ids := p.sampleAvailableModels(); len(ids) > 0 {
			hint += "\n  Available at this endpoint: " + strings.Join(ids, ", ")
		}
		return fmt.Errorf(
			"API request failed:\n"+
				"  Status: 404\n"+
				"  URL:    %s\n"+
				"  Model:  %q\n"+
				"  Body:   %s\n"+
				"  Hint:   %s",
			requestURL, model, strings.TrimSpace(string(body)), hint)
	}
	return fmt.Errorf("API request failed:\n  Status: %d\n  Body:   %s",
		statusCode, string(body))
}

// sampleAvailableModels performs a best-effort GET /v1/models against the
// provider's base URL and returns up to 10 model IDs. Any error (timeout,
// non-200, parse failure) returns an empty slice — this is a diagnostic
// enhancement, never a blocker.
func (p *Provider) sampleAvailableModels() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", p.apiBase+"/models", nil)
	if err != nil {
		return nil
	}
	if key := p.resolveAPIKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil
	}

	var decoded struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	out := make([]string, 0, 10)
	for _, m := range decoded.Data {
		if m.ID == "" {
			continue
		}
		out = append(out, m.ID)
		if len(out) >= 10 {
			break
		}
	}
	return out
}

// hostRequiresAPIKey reports whether the apiBase is a remote endpoint that
// normally needs an Authorization header. Local development servers
// (localhost, 127.0.0.1, 0.0.0.0) are assumed keyless — this matches how
// Ollama and similar self-hosted runners behave.
func hostRequiresAPIKey(apiBase string) bool {
	lower := strings.ToLower(apiBase)
	if strings.Contains(lower, "://localhost") ||
		strings.Contains(lower, "://127.0.0.1") ||
		strings.Contains(lower, "://0.0.0.0") {
		return false
	}
	return true
}

// preflightAPIKey fails the request before it hits the network when the
// provider has no API key but the endpoint clearly needs one. The error
// body deliberately mimics an HTTP 401 so the fallback chain's error
// classifier treats it as an auth failure (same as a real 401), which
// triggers the user-friendly "update your keys in Settings" header.
func (p *Provider) preflightAPIKey(model string) error {
	if p.resolveAPIKey() != "" {
		return nil
	}
	if !hostRequiresAPIKey(p.apiBase) {
		return nil
	}
	return fmt.Errorf("API request failed:\n  Status: 401\n  Body:   "+
		"no api key found for %s (model %q) — set it in Settings → AI Models",
		p.apiBase, model)
}

func asInt(v any) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	case float32:
		return int(val), true
	default:
		return 0, false
	}
}

func asFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}
