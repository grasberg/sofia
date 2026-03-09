package web

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/routing"
	"github.com/grasberg/sofia/pkg/skills"
)

//go:embed templates/layout.html
var layoutHTML []byte

//go:embed templates/chat.html
var chatHTML []byte

//go:embed templates/agents.html
var agentsHTML []byte

//go:embed templates/settings/models.html
var settingsModelsHTML []byte

//go:embed templates/settings/channels.html
var settingsChannelsHTML []byte

//go:embed templates/settings/tools.html
var settingsToolsHTML []byte

//go:embed templates/settings/skills.html
var settingsSkillsHTML []byte

//go:embed templates/settings/heartbeat.html
var settingsHeartbeatHTML []byte

//go:embed templates/settings/security.html
var settingsSecurityHTML []byte

//go:embed templates/settings/prompts.html
var settingsPromptsHTML []byte

//go:embed templates/settings/logs.html
var settingsLogsHTML []byte

//go:embed templates/history.html
var historyHTML []byte

type Server struct {
	cfg            *config.Config
	agentLoop      *agent.AgentLoop
	Version        string
	server         *http.Server
	mux            *http.ServeMux
	mu             sync.RWMutex
	skillInstaller *skills.SkillInstaller
}

// WebhookRegistrar is an interface for registering webhook HTTP handlers.
type WebhookRegistrar interface {
	RegisterWebhooks(mux *http.ServeMux)
}

// RegisterWebhooks registers webhook trigger handlers on the server's HTTP mux.
func (s *Server) RegisterWebhooks(registrar WebhookRegistrar) {
	if registrar != nil && s.mux != nil {
		registrar.RegisterWebhooks(s.mux)
	}
}

func NewServer(cfg *config.Config, agentLoop *agent.AgentLoop, version string) *Server {
	s := &Server{
		cfg:            cfg,
		agentLoop:      agentLoop,
		Version:        version,
		skillInstaller: skills.NewSkillInstaller(cfg.WorkspacePath()),
	}

	mux := http.NewServeMux()
	assetsDir := resolveAssetsDir()
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(assetsDir))))
	mux.HandleFunc("/", s.handleIndex)

	// HTMX Partials
	mux.HandleFunc("/ui/chat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(chatHTML)
	})
	mux.HandleFunc("/ui/agents", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(agentsHTML)
	})
	mux.HandleFunc("/ui/settings", func(w http.ResponseWriter, r *http.Request) {
		// Default settings view is models
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
<!-- SETTINGS TAB (HTMX Shell) -->
<div id="tab-settings" class="flex flex-col flex-grow min-h-0">
	<div id="subtab-content" class="flex flex-col flex-grow min-h-0" hx-get="/ui/settings/models" hx-trigger="load">
		<!-- HTMX will inject models.html here by default -->
	</div>
</div>
		`))
	})
	mux.HandleFunc("/ui/settings/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(settingsModelsHTML)
	})
	mux.HandleFunc("/ui/settings/channels", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(settingsChannelsHTML)
	})
	mux.HandleFunc("/ui/settings/tools", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(settingsToolsHTML)
	})
	mux.HandleFunc("/ui/settings/skills", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(settingsSkillsHTML)
	})
	mux.HandleFunc("/ui/settings/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(settingsHeartbeatHTML)
	})
	mux.HandleFunc("/ui/settings/security", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(settingsSecurityHTML)
	})
	mux.HandleFunc("/ui/settings/prompts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(settingsPromptsHTML)
	})
	mux.HandleFunc("/ui/settings/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(settingsLogsHTML)
	})
	mux.HandleFunc("/ui/history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(historyHTML)
	})

	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/skills/add", s.handleSkillAdd)
	mux.HandleFunc("/api/agents", s.handleAgents)
	mux.HandleFunc("/api/agent-templates", s.handleAgentTemplates)
	mux.HandleFunc("/api/agent-templates/", s.handleAgentTemplateByName)
	mux.HandleFunc("/api/workspace-docs", s.handleWorkspaceDocs)
	mux.HandleFunc("/api/restart", s.handleRestart)
	mux.HandleFunc("/api/update", s.handleUpdate)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/sessions/", s.handleSessionDetail)

	s.mux = mux
	s.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.WebUI.Host, cfg.WebUI.Port),
		Handler: mux,
	}

	return s
}

func (s *Server) Start(ctx context.Context) error {
	logger.InfoCF("web", "Starting Web UI", map[string]any{
		"host": s.cfg.WebUI.Host,
		"port": s.cfg.WebUI.Port,
	})

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.ErrorCF("web", "Web UI server error", map[string]any{"error": err.Error()})
		}
	}()

	<-ctx.Done()
	return s.Stop()
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.cfg)
		return
	}

	if r.Method == http.MethodPost {
		var newCfg config.Config
		if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
			s.sendJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Update internal config
		s.mu.Lock()
		*s.cfg = newCfg
		s.mu.Unlock()

		// Save to file (assuming default path for now)
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

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message    string `json:"message"`
		SessionKey string `json:"session_key"`
		Files      []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			IsText  bool   `json:"isText"`
			Content string `json:"content"` // plain text if isText, base64 data URL if image/binary
		} `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Determine session key: use provided key, or generate a new timestamped one.
	sessionKey := req.SessionKey
	if sessionKey == "" {
		sessionKey = "web:ui:" + time.Now().UTC().Format(time.RFC3339)
	}

	// Separate images (for native vision) from other files (inline text context)
	fullMessage := req.Message
	var imageDataURLs []string

	if len(req.Files) > 0 {
		var fileContext strings.Builder
		hasTextFiles := false

		for _, f := range req.Files {
			// Image files → pass as native vision data URLs
			if strings.HasPrefix(f.Type, "image/") {
				imageDataURLs = append(imageDataURLs, "data:"+f.Type+";base64,"+f.Content)
				continue
			}
			// Other files → embed as text context
			hasTextFiles = true
			fileContext.WriteString("=== FILE: " + f.Name + " ===\n")
			if f.IsText {
				fileContext.WriteString(f.Content)
			} else {
				fileContext.WriteString("[Binary file: " + f.Name + " (" + f.Type + ") — cannot display inline]\n")
			}
			fileContext.WriteString("\n=== END OF: " + f.Name + " ===\n\n")
		}

		if hasTextFiles {
			prefix := "The user has attached the following file(s) for you to read:\n\n" + fileContext.String()
			if fullMessage != "" {
				prefix += "User message: " + fullMessage
			}
			fullMessage = prefix
		}
	}

	ctx := r.Context()
	var response string
	var err error

	if len(imageDataURLs) > 0 {
		response, err = s.agentLoop.ProcessDirectWithImages(ctx, fullMessage, sessionKey, imageDataURLs)
	} else {
		response, err = s.agentLoop.ProcessDirect(ctx, fullMessage, sessionKey)
	}

	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"response": response, "session_key": sessionKey})
}

func (s *Server) sendJSONError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Restrict CORS to same-origin requests only (SSE endpoint)
	origin := r.Header.Get("Origin")
	if origin != "" {
		expectedOrigin := fmt.Sprintf("http://%s:%d", s.cfg.WebUI.Host, s.cfg.WebUI.Port)
		if s.cfg.WebUI.Host == "" || s.cfg.WebUI.Host == "0.0.0.0" {
			// Allow localhost variants when binding to all interfaces
			if origin == fmt.Sprintf("http://localhost:%d", s.cfg.WebUI.Port) ||
				origin == fmt.Sprintf("http://127.0.0.1:%d", s.cfg.WebUI.Port) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
		} else if origin == expectedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
	}

	logChan := logger.Subscribe()
	defer logger.Unsubscribe(logChan)

	// Send history
	history := logger.GetHistory()
	for _, line := range history {
		fmt.Fprintf(w, "data: %s\n\n", line)
	}

	// Send initial message if history is empty
	if len(history) == 0 {
		fmt.Fprintf(
			w,
			"data: {\"message\": \"Connected to log stream...\", \"level\": \"INFO\", \"component\": \"web\"}\n\n",
		)
	}

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	for {
		select {
		case msg := <-logChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleSkillAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

func resolveAssetsDir() string {
	if custom := os.Getenv("SOFIA_ASSETS_DIR"); custom != "" {
		if stat, err := os.Stat(custom); err == nil && stat.IsDir() {
			return custom
		}
	}

	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	candidates := []string{
		filepath.Join(wd, "assets"),
		filepath.Join(exeDir, "assets"),
		filepath.Join(exeDir, "..", "assets"),
		filepath.Join(exeDir, "..", "share", "sofia", "assets"),
	}

	for _, dir := range candidates {
		if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
			return dir
		}
	}

	return "assets"
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})

	go func() {
		time.Sleep(500 * time.Millisecond)
		argv0, err := os.Executable()
		if err != nil {
			logger.ErrorCF("web", "Failed to get executable for restart", map[string]any{"error": err.Error()})
			os.Exit(1)
		}
		logger.InfoCF("web", "Restarting Sofia via Web UI...", nil)
		err = syscall.Exec(argv0, os.Args, os.Environ())
		if err != nil {
			logger.ErrorCF("web", "Exec failed", map[string]any{"error": err.Error()})
			os.Exit(1)
		}
	}()
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logger.InfoCF("web", "Starting update process via Web UI...", nil)

	cmd := exec.Command("git", "pull")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logger.ErrorCF("web", "Git pull failed", map[string]any{"error": err.Error()})
		http.Error(w, "Failed to pull updates: "+err.Error(), http.StatusInternalServerError)
		return
	}

	cmd = exec.Command("make", "build")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logger.ErrorCF("web", "Make build failed", map[string]any{"error": err.Error()})
		http.Error(w, "Failed to build: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})

	go func() {
		time.Sleep(500 * time.Millisecond)
		argv0, err := os.Executable()
		if err != nil {
			logger.ErrorCF("web", "Failed to get executable for restart", map[string]any{"error": err.Error()})
			os.Exit(1)
		}
		logger.InfoCF("web", "Restarting Sofia after update...", nil)
		err = syscall.Exec(argv0, os.Args, os.Environ())
		if err != nil {
			logger.ErrorCF("web", "Exec failed after update", map[string]any{"error": err.Error()})
			os.Exit(1)
		}
	}()
}

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
