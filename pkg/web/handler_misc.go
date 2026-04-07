package web

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/cron"
	"github.com/grasberg/sofia/pkg/search"
)

// handleSearch handles GET /api/search?q=<query>&limit=10 — searches conversation history.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if strings.TrimSpace(query) == "" {
		s.sendJSONError(w, "query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	memDB := s.agentLoop.GetMemoryDB()
	if memDB == nil {
		s.sendJSONError(w, "memory database not available", http.StatusServiceUnavailable)
		return
	}

	dbRows, err := memDB.SearchMessages(query, limit*5)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to search entries and run keyword ranking
	entries := make([]search.MessageEntry, len(dbRows))
	for i, row := range dbRows {
		entries[i] = search.MessageEntry{
			SessionKey: row.SessionKey,
			Content:    row.Content,
			Role:       row.Role,
			Timestamp:  row.CreatedAt,
		}
	}

	results := search.KeywordSearch(query, entries, limit)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// handleEvolutionStatus returns the evolution engine's current status as JSON.
func (s *Server) handleEvolutionStatus(w http.ResponseWriter, _ *http.Request) {
	engine := s.agentLoop.GetEvolutionEngine()
	if engine == nil {
		http.Error(w, `{"error":"evolution engine not enabled"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": engine.FormatStatus(),
	})
}

// handleEvolutionChangelog returns recent evolution changelog entries as JSON.
func (s *Server) handleEvolutionChangelog(w http.ResponseWriter, r *http.Request) {
	engine := s.agentLoop.GetEvolutionEngine()
	if engine == nil {
		http.Error(w, `{"error":"evolution engine not enabled"}`, http.StatusNotFound)
		return
	}
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	entries, err := engine.RecentHistory(limit)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// handleCron handles GET /api/cron (list jobs), POST /api/cron (add job),
// and DELETE /api/cron?name=<name> (remove job).
func (s *Server) handleCron(w http.ResponseWriter, r *http.Request) {
	if s.cronService == nil {
		s.sendJSONError(w, "Cron service not available", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		jobs := s.cronService.ListJobs(true)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jobs)

	case http.MethodPost:
		limitBody(r)
		var req struct {
			Name     string `json:"name"`
			Schedule string `json:"schedule"`
			Message  string `json:"message"`
			AgentID  string `json:"agent_id"`
			Enabled  bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.sendJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Schedule == "" || req.Message == "" {
			s.sendJSONError(
				w,
				"name, schedule, and message are required",
				http.StatusBadRequest,
			)
			return
		}
		schedule := cron.CronSchedule{Kind: "cron", Expr: req.Schedule}
		job, err := s.cronService.AddJob(req.Name, schedule, req.Message, false, "", "")
		if err != nil {
			s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(job)

	case http.MethodDelete:
		name := r.URL.Query().Get("name")
		if name == "" {
			s.sendJSONError(w, "name parameter is required", http.StatusBadRequest)
			return
		}
		jobs := s.cronService.ListJobs(true)
		removed := false
		for _, j := range jobs {
			if j.Name == name {
				removed = s.cronService.RemoveJob(j.ID)
				break
			}
		}
		if !removed {
			s.sendJSONError(w, "job not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))

	default:
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleCronToggle handles POST /api/cron/toggle — enables or disables a cron job.
func (s *Server) handleCronToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cronService == nil {
		s.sendJSONError(w, "Cron service not available", http.StatusServiceUnavailable)
		return
	}

	limitBody(r)
	var req struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jobs := s.cronService.ListJobs(true)
	for _, j := range jobs {
		if j.Name == req.Name {
			s.cronService.EnableJob(j.ID, req.Enabled)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
			return
		}
	}
	s.sendJSONError(w, "job not found", http.StatusNotFound)
}

// handleBackupExport creates a ZIP archive of Sofia's config, memory database,
// workspace files, cron jobs, and plans — and streams it as a download.
func (s *Server) handleBackupExport(w http.ResponseWriter, _ *http.Request) {
	home, _ := os.UserHomeDir()
	sofiaDir := filepath.Join(home, ".sofia")

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=sofia-backup-%s.zip", time.Now().Format("20060102-150405")))

	zw := zip.NewWriter(w)
	defer zw.Close()

	// Add files from ~/.sofia/ (config, memory.db, cron jobs, plans, skills)
	addFileToZip := func(zw *zip.Writer, fsPath, zipPath string) {
		data, err := os.ReadFile(fsPath)
		if err != nil {
			return
		}
		f, err := zw.Create(zipPath)
		if err != nil {
			return
		}
		f.Write(data)
	}

	// Config
	addFileToZip(zw, filepath.Join(sofiaDir, "config.json"), "config.json")

	// Memory database
	addFileToZip(zw, filepath.Join(sofiaDir, "memory.db"), "memory.db")

	// Workspace files
	workspace := s.cfg.WorkspacePath()
	walkFiles := []string{"plans.json", "AGENT.md", "USER.md", "IDENTITY.md", "SOUL.md", "HEARTBEAT.md"}
	for _, name := range walkFiles {
		addFileToZip(zw, filepath.Join(workspace, name), "workspace/"+name)
	}

	// Cron jobs
	addFileToZip(zw, filepath.Join(workspace, "cron", "jobs.json"), "cron/jobs.json")

	// Skills directory
	skillsDir := filepath.Join(workspace, "skills")
	if entries, err := os.ReadDir(skillsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				skillFile := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
				addFileToZip(zw, skillFile, "skills/"+entry.Name()+"/SKILL.md")
			}
		}
	}

	// Write the current config as a separate JSON for easy reading
	cfgData, err := json.MarshalIndent(s.cfg, "", "  ")
	if err == nil {
		if f, err := zw.Create("config-current.json"); err == nil {
			f.Write(cfgData)
		}
	}
}
