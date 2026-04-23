package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grasberg/sofia/pkg/config"
)

// newRecipeTestServer returns a Server with just enough wiring to exercise the
// recipe routes. The agent loop stays nil because these handlers don't invoke it.
func newRecipeTestServer() *Server {
	s := &Server{cfg: &config.Config{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/recipes", s.handleRecipes)
	mux.HandleFunc("/api/recipes/", s.handleRecipes)
	s.mux = mux
	return s
}

func TestRecipes_List_ReturnsBundledLibrary(t *testing.T) {
	s := newRecipeTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/recipes", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var items []recipeListEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) < 42 {
		t.Fatalf("expected >= 42 recipes, got %d", len(items))
	}

	var found bool
	for _, it := range items {
		if it.Name == "daily-reddit-digest" && it.Source == "bundled" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'daily-reddit-digest' in bundled source")
	}
}

func TestRecipes_Detail_ReturnsFullRecipe(t *testing.T) {
	s := newRecipeTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/recipes/daily-reddit-digest", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var d recipeDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.Title == "" || d.Prompt == "" {
		t.Fatalf("detail missing title/prompt: %+v", d)
	}
	if d.Source != "bundled" {
		t.Fatalf("expected source=bundled, got %q", d.Source)
	}
	if len(d.Parameters) == 0 {
		t.Fatalf("expected parameters to be non-empty for daily-reddit-digest")
	}
}

func TestRecipes_Detail_NotFound(t *testing.T) {
	s := newRecipeTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/recipes/does-not-exist", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestRecipes_Render_UsesDefaultsWhenParamsOmitted(t *testing.T) {
	s := newRecipeTestServer()

	body := bytes.NewBufferString(`{"params": {}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/recipes/daily-reddit-digest/render", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Prompt == "" {
		t.Fatalf("expected non-empty rendered prompt")
	}
	if strings.Contains(res.Prompt, "{{subreddits}}") {
		t.Fatalf("defaults were not substituted: %q", res.Prompt)
	}
	if !strings.Contains(res.Prompt, "5pm") {
		t.Fatalf("expected default digest_time '5pm' in rendered prompt: %q", res.Prompt)
	}
}

func TestRecipes_Render_OverridesApplied(t *testing.T) {
	s := newRecipeTestServer()

	body := bytes.NewBufferString(`{"params": {"digest_time": "7am"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/recipes/daily-reddit-digest/render", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var res struct {
		Prompt string `json:"prompt"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if !strings.Contains(res.Prompt, "7am") {
		t.Fatalf("override 'digest_time=7am' not applied: %q", res.Prompt)
	}
}

func TestRecipes_MethodNotAllowed_OnListPost(t *testing.T) {
	s := newRecipeTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/recipes", nil)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
