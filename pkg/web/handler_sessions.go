package web

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleSessions handles GET /api/sessions — returns a list of session metadata
// sorted by most recently updated.
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sm := s.agentLoop.GetDefaultSessionManager()
	if sm == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}
	metas := sm.ListSessions()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metas)
}

// handleSessionDetail handles GET /api/sessions/<key> and DELETE /api/sessions/<key>.
func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	// Extract session key from URL path: /api/sessions/<key>
	key := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if key == "" {
		s.sendJSONError(w, "Missing session key", http.StatusBadRequest)
		return
	}
	// Validate session key format to prevent path traversal or injection
	if strings.ContainsAny(key, "/\\..") || strings.Contains(key, "..") {
		s.sendJSONError(w, "Invalid session key", http.StatusBadRequest)
		return
	}

	sm := s.agentLoop.GetDefaultSessionManager()
	if sm == nil {
		s.sendJSONError(w, "Session manager unavailable", http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		messages := sm.GetHistory(key)
		// Filter to user and assistant messages only (hide tool calls/results).
		type msgView struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		visible := make([]msgView, 0, len(messages))
		for _, m := range messages {
			if m.Role == "user" || m.Role == "assistant" {
				visible = append(visible, msgView{Role: m.Role, Content: m.Content})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(visible)

	case http.MethodDelete:
		if err := sm.DeleteSession(key); err != nil {
			s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))

	default:
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
