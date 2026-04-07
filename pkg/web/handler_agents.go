package web

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/routing"
)

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		// Exclude the default/main agent — it is Sofia itself and is configured
		// via the Settings page, not the Sub-Agents page.
		subAgents := []config.AgentConfig{}
		for _, a := range s.cfg.Agents.List {
			if a.Default || routing.NormalizeAgentID(a.ID) == routing.DefaultAgentID {
				continue
			}
			subAgents = append(subAgents, a)
		}
		json.NewEncoder(w).Encode(subAgents)
		return
	}

	if r.Method == http.MethodPost {
		limitBody(r)
		var agent config.AgentConfig
		if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
			s.sendJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		if agent.ID == "" {
			s.sendJSONError(w, "Agent ID is required", http.StatusBadRequest)
			return
		}

		// Update or Add
		found := false
		for i, a := range s.cfg.Agents.List {
			if a.ID == agent.ID {
				s.cfg.Agents.List[i] = agent
				found = true
				break
			}
		}
		if !found {
			s.cfg.Agents.List = append(s.cfg.Agents.List, agent)
		}

		// Save config
		home, _ := os.UserHomeDir()
		configPath := os.Getenv("SOFIA_CONFIG")
		if configPath == "" {
			configPath = home + "/.sofia/config.json"
		}

		if err := config.SaveConfig(configPath, s.cfg); err != nil {
			s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Hot-reload agents so agent model changes apply immediately.
		s.agentLoop.ReloadAgents()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	if r.Method == http.MethodDelete {
		id := r.URL.Query().Get("id")
		if id == "" {
			s.sendJSONError(w, "Agent ID is required", http.StatusBadRequest)
			return
		}

		// Prevent deleting the default/main agent (Sofia itself).
		if routing.NormalizeAgentID(id) == routing.DefaultAgentID {
			s.sendJSONError(w, "Cannot delete the default agent", http.StatusBadRequest)
			return
		}
		// Also prevent deleting any agent explicitly marked as default.
		for _, a := range s.cfg.Agents.List {
			if a.ID == id && a.Default {
				s.sendJSONError(w, "Cannot delete the default agent", http.StatusBadRequest)
				return
			}
		}

		newList := []config.AgentConfig{}
		for _, a := range s.cfg.Agents.List {
			if a.ID != id {
				newList = append(newList, a)
			}
		}
		s.cfg.Agents.List = newList

		// Save config
		home, _ := os.UserHomeDir()
		configPath := os.Getenv("SOFIA_CONFIG")
		if configPath == "" {
			configPath = home + "/.sofia/config.json"
		}

		if err := config.SaveConfig(configPath, s.cfg); err != nil {
			s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Hot-reload agents so deletions apply immediately.
		s.agentLoop.ReloadAgents()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleAgentTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	templates, err := agent.ListPurposeTemplates()
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type tpl struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Skills      []string `json:"skills,omitempty"`
	}
	out := make([]tpl, 0, len(templates))
	for _, t := range templates {
		out = append(out, tpl{Name: t.Name, Description: t.Description, Skills: t.Skills})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *Server) handleAgentTemplateByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/agent-templates/")
	if name == "" {
		s.sendJSONError(w, "Template name is required", http.StatusBadRequest)
		return
	}

	t, err := agent.LoadPurposeTemplate(name)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}
