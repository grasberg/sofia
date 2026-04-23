package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/grasberg/sofia/pkg/recipe"
)

// recipeListEntry is the shape returned by GET /api/recipes.
type recipeListEntry struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// recipeDetail is the shape returned by GET /api/recipes/<name>.
type recipeDetail struct {
	Name         string                 `json:"name"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	Author       string                 `json:"author,omitempty"`
	Instructions string                 `json:"instructions,omitempty"`
	Prompt       string                 `json:"prompt"`
	Parameters   []recipeParameterJSON  `json:"parameters,omitempty"`
	Settings     *recipeSettingsJSON    `json:"settings,omitempty"`
	Source       string                 `json:"source"`
	Extensions   []string               `json:"extensions,omitempty"`
	Response     map[string]any         `json:"response,omitempty"`
}

type recipeParameterJSON struct {
	Key         string   `json:"key"`
	Description string   `json:"description,omitempty"`
	InputType   string   `json:"input_type"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"`
}

type recipeSettingsJSON struct {
	Provider    string  `json:"provider,omitempty"`
	Model       string  `json:"model,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	MaxTurns    int     `json:"max_turns,omitempty"`
}

// handleRecipes routes GET /api/recipes{,/<name>{,/render}}.
func (s *Server) handleRecipes(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/recipes")
	rest = strings.TrimPrefix(rest, "/")

	switch {
	case rest == "":
		if r.Method != http.MethodGet {
			s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleRecipesList(w, r)
	case strings.HasSuffix(rest, "/render"):
		if r.Method != http.MethodPost {
			s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimSuffix(rest, "/render")
		s.handleRecipeRender(w, r, name)
	default:
		if r.Method != http.MethodGet {
			s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleRecipeDetail(w, r, rest)
	}
}

func (s *Server) handleRecipesList(w http.ResponseWriter, r *http.Request) {
	workspace := ""
	if s.cfg != nil {
		workspace = s.cfg.WorkspacePath()
	}

	metas, err := recipe.ListRecipes(workspace)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out := make([]recipeListEntry, 0, len(metas))
	for _, m := range metas {
		out = append(out, recipeListEntry{
			Name:        m.Name,
			Title:       m.Title,
			Description: m.Description,
			Source:      m.Source,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleRecipeDetail(w http.ResponseWriter, r *http.Request, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		s.sendJSONError(w, "Recipe name required", http.StatusBadRequest)
		return
	}

	rec, source, err := s.loadRecipeByName(name)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	detail := recipeDetail{
		Name:         name,
		Title:        rec.Title,
		Description:  rec.Description,
		Author:       rec.Author,
		Instructions: rec.Instructions,
		Prompt:       rec.Prompt,
		Source:       source,
	}
	for _, p := range rec.Parameters {
		detail.Parameters = append(detail.Parameters, recipeParameterJSON{
			Key:         p.Key,
			Description: p.Description,
			InputType:   p.InputType,
			Required:    p.Required,
			Default:     p.Default,
			Options:     p.Options,
		})
	}
	if rec.Settings.Provider != "" || rec.Settings.Model != "" || rec.Settings.Temperature != 0 || rec.Settings.MaxTurns != 0 {
		detail.Settings = &recipeSettingsJSON{
			Provider:    rec.Settings.Provider,
			Model:       rec.Settings.Model,
			Temperature: rec.Settings.Temperature,
			MaxTurns:    rec.Settings.MaxTurns,
		}
	}
	for _, ext := range rec.Extensions {
		detail.Extensions = append(detail.Extensions, ext.Name)
	}
	if rec.Response != nil {
		detail.Response = rec.Response.JSONSchema
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}

func (s *Server) handleRecipeRender(w http.ResponseWriter, r *http.Request, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		s.sendJSONError(w, "Recipe name required", http.StatusBadRequest)
		return
	}

	limitBody(r)
	var req struct {
		Params map[string]string `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	rec, _, err := s.loadRecipeByName(name)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	prompt, err := recipe.RenderPrompt(rec, req.Params)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"prompt":       prompt,
		"instructions": rec.Instructions,
	})
}

// loadRecipeByName resolves a recipe by name across workspace, global, and
// bundled sources, returning the recipe and the source it came from.
func (s *Server) loadRecipeByName(name string) (*recipe.Recipe, string, error) {
	workspace := ""
	if s.cfg != nil {
		workspace = s.cfg.WorkspacePath()
	}

	metas, err := recipe.ListRecipes(workspace)
	if err != nil {
		return nil, "", err
	}
	for _, m := range metas {
		if m.Name != name {
			continue
		}
		if m.Source == "bundled" {
			rec, err := recipe.LoadBundledRecipe(name)
			return rec, m.Source, err
		}
		rec, err := recipe.LoadRecipe(m.Path)
		return rec, m.Source, err
	}
	return nil, "", &recipeNotFoundError{name: name}
}

type recipeNotFoundError struct{ name string }

func (e *recipeNotFoundError) Error() string { return "recipe not found: " + e.name }
