package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ConfigHandler handles configuration-related endpoints.
type ConfigHandler struct {
	Srv *Server
}

// HandleConfigGET returns the current server configuration.
func (h *ConfigHandler) HandleConfigGET(w http.ResponseWriter, r *http.Request) {
	cfgJSON, err := configToMaskedJSON(h.Srv.Cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(cfgJSON)
}

// HandleConfigPOST saves updated configuration.
func (h *ConfigHandler) HandleConfigPOST(w http.ResponseWriter, r *http.Request) {
	limitBody(r)

	var newCfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Restore masked secrets from original config
	var origMap map[string]any
	if origJSON, err := configToMaskedJSON(h.Srv.Cfg); err == nil {
		json.Unmarshal(origJSON, &origMap)
	}

	var incomingMap map[string]any
	json.NewDecoder(r.Body).Decode(&incomingMap)
	restored := restoreMaskedSecrets(incomingMap, origMap)

	// Apply restored config
	restoredJSON, _ := json.Marshal(restored)
	json.Unmarshal(restoredJSON, &newCfg)

	// Save and reload configuration
	if err := saveConfig(&newCfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
