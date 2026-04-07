package web

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/routing"
	"github.com/grasberg/sofia/pkg/skills"
)

func (s *Server) handleSkillAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitBody(r)
	var req struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Content == "" {
		s.sendJSONError(w, "Name and Content are required", http.StatusBadRequest)
		return
	}

	if err := s.skillInstaller.InstallFromMarkdown(req.Name, []byte(req.Content)); err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleSkillsList(w http.ResponseWriter, r *http.Request) {
	var allSkills []skills.SkillInfo
	if s.agentLoop != nil {
		info := s.agentLoop.GetStartupInfo()
		skillsInfo, _ := info["skills"].(map[string]any)
		allSkills, _ = skillsInfo["list"].([]skills.SkillInfo)
	}

	// Find the default agent's skills filter from config.
	var enabledSkills []string
	for _, a := range s.cfg.Agents.List {
		if a.Default || routing.NormalizeAgentID(a.ID) == routing.DefaultAgentID {
			enabledSkills = a.Skills
			break
		}
	}

	// Build response with enabled status.
	type skillResponse struct {
		Name        string `json:"name"`
		Path        string `json:"path"`
		Source      string `json:"source"`
		Description string `json:"description"`
		Enabled     bool   `json:"enabled"`
	}

	enabledSet := make(map[string]bool, len(enabledSkills))
	for _, name := range enabledSkills {
		enabledSet[name] = true
	}

	result := make([]skillResponse, 0, len(allSkills))
	for _, sk := range allSkills {
		// Skills are disabled by default; only explicitly listed skills are enabled.
		enabled := enabledSet[sk.Name]
		result = append(result, skillResponse{
			Name:        sk.Name,
			Path:        sk.Path,
			Source:      sk.Source,
			Description: sk.Description,
			Enabled:     enabled,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleSkillsToggle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	limitBody(r)
	var req struct {
		Skill   string `json:"skill"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Skill == "" {
		s.sendJSONError(w, "Skill name is required", http.StatusBadRequest)
		return
	}

	// Find the default agent config.
	defaultIdx := -1
	for i, a := range s.cfg.Agents.List {
		if a.Default || routing.NormalizeAgentID(a.ID) == routing.DefaultAgentID {
			defaultIdx = i
			break
		}
	}
	if defaultIdx == -1 {
		s.sendJSONError(w, "Default agent not found", http.StatusInternalServerError)
		return
	}

	agentCfg := &s.cfg.Agents.List[defaultIdx]

	// Skills are disabled by default. The skills list contains only enabled skills.
	if req.Enabled {
		// Add skill to the list if not already present.
		found := false
		for _, name := range agentCfg.Skills {
			if name == req.Skill {
				found = true
				break
			}
		}
		if !found {
			agentCfg.Skills = append(agentCfg.Skills, req.Skill)
		}
	} else {
		// Remove skill from the list.
		newSkills := make([]string, 0, len(agentCfg.Skills))
		for _, name := range agentCfg.Skills {
			if name != req.Skill {
				newSkills = append(newSkills, name)
			}
		}
		agentCfg.Skills = newSkills
	}

	// Save config.
	home, _ := os.UserHomeDir()
	configPath := os.Getenv("SOFIA_CONFIG")
	if configPath == "" {
		configPath = home + "/.sofia/config.json"
	}
	if err := config.SaveConfig(configPath, s.cfg); err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Reload agents so the skills filter takes effect.
	if s.agentLoop != nil {
		s.agentLoop.ReloadAgents()
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
