package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) handleWorkspaceDocs(w http.ResponseWriter, r *http.Request) {
	workspace := s.cfg.WorkspacePath()
	identityPath := filepath.Join(workspace, "IDENTITY.md")
	soulPath := filepath.Join(workspace, "SOUL.md")
	heartbeatPath := filepath.Join(workspace, "HEARTBEAT.md")

	if r.Method == http.MethodGet {
		readOrEmpty := func(path string) string {
			b, err := os.ReadFile(path)
			if err != nil {
				return ""
			}
			return string(b)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"identity":  readOrEmpty(identityPath),
			"soul":      readOrEmpty(soulPath),
			"heartbeat": readOrEmpty(heartbeatPath),
		})
		return
	}

	if r.Method == http.MethodPost {
		limitBody(r)
		var req struct {
			Identity  string `json:"identity"`
			Soul      string `json:"soul"`
			Heartbeat string `json:"heartbeat"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.sendJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := os.MkdirAll(workspace, 0o755); err != nil {
			s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := os.WriteFile(identityPath, []byte(req.Identity), 0o644); err != nil {
			s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(soulPath, []byte(req.Soul), 0o644); err != nil {
			s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if req.Heartbeat != "" {
			if err := os.WriteFile(heartbeatPath, []byte(req.Heartbeat), 0o644); err != nil {
				s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// handleWorkspaceFiles lists files and directories under the workspace path.
// Query param "path" is relative to workspace root; defaults to ".".
func (s *Server) handleWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	workspace := s.cfg.WorkspacePath()
	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		relPath = "."
	}

	// Resolve and validate the path stays within workspace.
	target := filepath.Join(workspace, filepath.Clean(relPath))
	if !strings.HasPrefix(target, workspace) {
		s.sendJSONError(w, "Path outside workspace", http.StatusBadRequest)
		return
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	type fileEntry struct {
		Name    string `json:"name"`
		IsDir   bool   `json:"is_dir"`
		Size    int64  `json:"size"`
		ModTime string `json:"mod_time"`
	}

	result := make([]fileEntry, 0, len(entries))
	for _, e := range entries {
		info, infoErr := e.Info()
		if infoErr != nil {
			continue
		}
		result = append(result, fileEntry{
			Name:    e.Name(),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"workspace": workspace,
		"path":      relPath,
		"entries":   result,
	})
}

// handleWorkspaceFile reads and returns the content of a single workspace file.
// Query param "path" is relative to workspace root.
func (s *Server) handleWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	workspace := s.cfg.WorkspacePath()
	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		s.sendJSONError(w, "path parameter is required", http.StatusBadRequest)
		return
	}

	// Resolve and validate the path stays within workspace.
	target := filepath.Join(workspace, filepath.Clean(relPath))
	if !strings.HasPrefix(target, workspace) {
		s.sendJSONError(w, "Path outside workspace", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(target)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusNotFound)
		return
	}
	if info.IsDir() {
		s.sendJSONError(w, "Path is a directory, use /api/workspace/files", http.StatusBadRequest)
		return
	}

	// Limit readable file size to 1 MB.
	const maxFileSize = 1 << 20
	if info.Size() > maxFileSize {
		s.sendJSONError(w, "File too large (max 1MB)", http.StatusRequestEntityTooLarge)
		return
	}

	content, err := os.ReadFile(target)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"path":     relPath,
		"name":     filepath.Base(target),
		"size":     info.Size(),
		"mod_time": info.ModTime().Format("2006-01-02 15:04:05"),
		"content":  string(content),
	})
}

// handlePlan returns the active plan status as JSON.
func (s *Server) handlePlan(w http.ResponseWriter, _ *http.Request) {
	plan := s.agentLoop.GetActivePlan()
	w.Header().Set("Content-Type", "application/json")
	if plan == nil {
		w.Write([]byte("null"))
		return
	}
	json.NewEncoder(w).Encode(plan)
}
