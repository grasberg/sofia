package web

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
)

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Write(layoutHTML)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	info := s.agentLoop.GetStartupInfo()

	// Inject the dynamic version directly here
	// to avoid cyclic dependencies in pkg/agent
	info["version"] = s.Version

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.RLock()
		cfgCopy := *s.cfg
		s.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		masked, err := configToMaskedJSON(&cfgCopy)
		if err != nil {
			s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(masked)
		return
	}

	if r.Method == http.MethodPost {
		limitBody(r)

		// Decode the incoming config into a generic map first so we
		// can restore any masked secrets before unmarshalling into
		// the typed struct.
		var incomingRaw any
		if err := json.NewDecoder(r.Body).Decode(&incomingRaw); err != nil {
			s.sendJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Build a generic map of the current config to use as the
		// source of truth for masked fields.
		s.mu.RLock()
		origBytes, err := json.Marshal(s.cfg)
		s.mu.RUnlock()
		if err != nil {
			s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var originalRaw any
		if err := json.Unmarshal(origBytes, &originalRaw); err != nil {
			s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Replace "********" placeholders with the real values.
		restoreMaskedSecrets(incomingRaw, originalRaw)

		// Re-encode and decode into the typed config struct.
		merged, err := json.Marshal(incomingRaw)
		if err != nil {
			s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var newCfg config.Config
		if err := json.Unmarshal(merged, &newCfg); err != nil {
			s.sendJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Sync model list to DB if present in the incoming config.
		// Models are stored in the database, not in config.json.
		if len(newCfg.ModelList) > 0 {
			if memDB := s.agentLoop.GetMemoryDB(); memDB != nil {
				catalogNames := make(map[string]bool)
				for _, m := range config.DefaultModelList() {
					catalogNames[m.ModelName] = true
				}
				if err := memDB.SyncModels(newCfg.ModelList, catalogNames); err != nil {
					logger.WarnCF("web", "Failed to sync models to DB", map[string]any{"error": err.Error()})
				}
				// Reload from DB to pick up any catalog entries the user didn't send.
				if err := memDB.LoadModelsIntoConfig(&newCfg); err != nil {
					logger.WarnCF("web", "Failed to reload models from DB", map[string]any{"error": err.Error()})
				}
			}
		}

		// Update internal config
		s.mu.Lock()
		*s.cfg = newCfg
		s.mu.Unlock()

		// Save to file (model_list is excluded — stored in DB).
		home, _ := os.UserHomeDir()
		configPath := os.Getenv("SOFIA_CONFIG")
		if configPath == "" {
			configPath = home + "/.sofia/config.json"
		}

		if err := config.SaveConfig(configPath, s.cfg); err != nil {
			s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Hot-reload agents so model changes take effect immediately
		s.agentLoop.ReloadAgents()

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
}
