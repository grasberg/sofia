package web

import (
	"context"
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
	"github.com/grasberg/sofia/pkg/skills"
)

type Server struct {
	cfg            *config.Config
	agentLoop      *agent.AgentLoop
	Version        string
	server         *http.Server
	mu             sync.RWMutex
	skillInstaller *skills.SkillInstaller
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
	w.Write([]byte(indexHTML))
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
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	response, err := s.agentLoop.ProcessDirect(ctx, req.Message, "web:ui")
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"response": response})
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
	w.Header().Set("Access-Control-Allow-Origin", "*")

	logChan := logger.Subscribe()
	defer logger.Unsubscribe(logChan)

	// Send history
	history := logger.GetHistory()
	for _, line := range history {
		fmt.Fprintf(w, "data: %s\n\n", line)
	}

	// Send initial message if history is empty
	if len(history) == 0 {
		fmt.Fprintf(w, "data: {\"message\": \"Connected to log stream...\", \"level\": \"INFO\", \"component\": \"web\"}\n\n")
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
		json.NewEncoder(w).Encode(s.cfg.Agents.List)
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
			"identity": readOrEmpty(identityPath),
			"soul":     readOrEmpty(soulPath),
		})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Identity string `json:"identity"`
			Soul     string `json:"soul"`
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

const indexHTML = `
<!DOCTYPE html>
<html lang="sv">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Sofia - Command Center</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <!-- Lucide Icons -->
    <script src="https://unpkg.com/lucide@latest"></script>
    <script>
        // Check for saved theme or system preference
        if (localStorage.getItem('theme') === 'dark' || (!('theme' in localStorage) && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
            document.documentElement.classList.add('dark');
        } else {
            document.documentElement.classList.remove('dark');
        }

        tailwind.config = {
            darkMode: "class",
            theme: {
                extend: {
                    colors: {
                        sofia: {
                            DEFAULT: "#ff4d4d",
                            hover: "#e63e3e",
                            light: "#ff6666",
                            dark: "#cc3d3d",
                            bg: "#1a0a0a"
                        },
                        zinc: {
                            950: "#050505",
                        }
                    },
                    animation: {
                        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
                        'slide-up': 'slideUp 0.3s ease-out',
                        'fade-in': 'fadeIn 0.5s ease-out',
                    },
                    keyframes: {
                        slideUp: {
                            '0%': { transform: 'translateY(10px)', opacity: '0' },
                            '100%': { transform: 'translateY(0)', opacity: '1' },
                        },
                        fadeIn: {
                            '0%': { opacity: '0' },
                            '100%': { opacity: '1' },
                        }
                    }
                }
            }
        }
    </script>
    <style>
        @import url('https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=JetBrains+Mono:wght@400;500&display=swap');
        
        :root {
            --sofia-red: #ff4d4d;
            --bg-main: #f9f9f9;
            --bg-card: #ffffff;
            --bg-sidebar: #f3f3f3;
            --border-color: #e5e7eb;
            --text-main: #18181b;
            --text-muted: #71717a;
            --glass-bg: rgba(255, 255, 255, 0.7);
            --nav-hover: rgba(0, 0, 0, 0.05);
        }

        .dark {
            --bg-main: #080808;
            --bg-card: #111111;
            --bg-sidebar: #050505;
            --border-color: #222222;
            --text-main: #e0e0e0;
            --text-muted: #888888;
            --glass-bg: rgba(17, 17, 17, 0.7);
            --nav-hover: rgba(255, 255, 255, 0.05);
        }

        body { 
            background-color: var(--bg-main); 
            color: var(--text-main); 
            font-family: 'Inter', sans-serif;
            overflow: hidden;
            transition: background-color 0.3s ease, color 0.3s ease;
        }

        .glass-panel {
            background: var(--glass-bg);
            backdrop-filter: blur(12px);
            border: 1px solid var(--border-color);
            transition: all 0.3s ease;
        }

        .sofia-border { border: 1px solid var(--border-color); }
        .sofia-border-active { border: 1px solid var(--sofia-red); }

        .nav-item {
            display: flex;
            align-items: center;
            gap: 12px;
            padding: 10px 16px;
            border-radius: 8px;
            color: var(--text-muted);
            transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
            font-weight: 500;
        }

        .nav-item:hover {
            color: var(--text-main);
            background: var(--nav-hover);
        }

        .nav-item.active {
            color: var(--text-main);
            background: rgba(255, 77, 77, 0.1);
            border-left: 3px solid var(--sofia-red);
            border-radius: 0 8px 8px 0;
        }

        .tab-content { display: none; height: 100%; opacity: 0; transform: translateY(10px); }
        .tab-content.active { display: flex; flex-direction: column; opacity: 1; transform: translateY(0); transition: all 0.3s ease-out; }

        /* Custom Scrollbar */
        ::-webkit-scrollbar { width: 6px; }
        ::-webkit-scrollbar-track { background: transparent; }
        ::-webkit-scrollbar-thumb { background: var(--border-color); border-radius: 10px; }
        ::-webkit-scrollbar-thumb:hover { background: var(--text-muted); }

        .chat-bubble-user {
            background: var(--bg-card);
            border: 1px solid var(--border-color);
            border-bottom-right-radius: 2px;
        }

        .chat-bubble-sofia {
            background: rgba(255, 77, 77, 0.05);
            border: 1px solid rgba(255, 77, 77, 0.2);
            border-bottom-left-radius: 2px;
        }

        .mono { font-family: 'JetBrains Mono', monospace; }

        /* Scanning effect for the logo */
        .logo-container {
            position: relative;
            overflow: hidden;
        }
        .logo-container::after {
            content: "";
            position: absolute;
            top: -100%;
            left: -100%;
            width: 300%;
            height: 300%;
            background: linear-gradient(45deg, transparent 45%, rgba(255, 77, 77, 0.1) 50%, transparent 55%);
            animation: scan 4s infinite linear;
        }
        @keyframes scan {
            0% { transform: translate(-30%, -30%); }
            100% { transform: translate(30%, 30%); }
        }

        @keyframes spin-slow {
            from { transform: rotate(0deg); }
            to { transform: rotate(360deg); }
        }
        .animate-spin-slow {
            animation: spin-slow 3s linear infinite;
        }

        .glow-red {
            box-shadow: 0 0 20px rgba(255, 77, 77, 0.15);
        }

        .agent-log-box {
            transition: all 0.4s cubic-bezier(0.175, 0.885, 0.32, 1.275);
            animation: popIn 0.4s ease-out;
        }

        @keyframes popIn {
            0% { transform: scale(0.9); opacity: 0; }
            100% { transform: scale(1); opacity: 1; }
        }

        .agent-log-line {
            border-left: 1px solid rgba(255, 255, 255, 0.1);
            padding-left: 8px;
            margin-bottom: 2px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 9px;
        }
    </style>
</head>
<body class="h-screen flex overflow-hidden bg-[var(--bg-main)] text-[var(--text-main)] transition-colors duration-300">
    <!-- SIDEBAR -->
    <aside class="w-64 min-w-[16rem] shrink-0 border-r border-[var(--border-color)] flex flex-col bg-[var(--bg-sidebar)] z-20 transition-colors duration-300">
        <div class="p-6 flex items-center gap-3">
            <div class="logo-container w-10 h-10 rounded-lg bg-black/5 dark:bg-sofia-bg flex items-center justify-center border border-sofia/30 glow-red transition-colors duration-300">
                <img src="/assets/sofiamantis.png" alt="S" class="w-7 h-7 object-contain">
            </div>
            <div>
                <h1 class="text-xl font-bold tracking-tight text-[var(--text-main)] transition-colors duration-300">Sofia</h1>
            </div>
        </div>

        <nav class="flex-grow px-4 mt-4 space-y-1">
            <a href="#" onclick="showTab('chat')" id="nav-chat" class="nav-item active">
                <i data-lucide="message-square" class="w-5 h-5"></i>
                <span>Chat</span>
            </a>
            <a href="#" onclick="showTab('agents')" id="nav-agents" class="nav-item">
                <i data-lucide="users" class="w-5 h-5"></i>
                <span>Agents</span>
            </a>




            <a href="#" onclick="showTab('settings')" id="nav-settings" class="nav-item">
                <i data-lucide="settings" class="w-5 h-5"></i>
                <span>Settings</span>
            </a>
            <a href="#" onclick="showTab('logs')" id="nav-logs" class="nav-item">
                <i data-lucide="terminal" class="w-5 h-5"></i>
                <span>Logs</span>
            </a>
        </nav>

        <div class="p-4 border-t border-[var(--border-color)] space-y-4">
            <!-- System Activity (Moved from Chat) -->
            <div class="bg-[var(--bg-main)] rounded-xl p-4 border border-[var(--border-color)]">
                <h3 class="text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-3">System Resources</h3>
                <div class="space-y-3">
                    <div class="flex items-center gap-3">
                        <div class="w-7 h-7 rounded-lg bg-blue-500/10 flex items-center justify-center text-blue-500">
                            <i data-lucide="activity" class="w-3.5 h-3.5"></i>
                        </div>
                        <div class="overflow-hidden">
                            <div class="text-[10px] font-semibold text-[var(--text-main)]">CPU</div>
                            <div class="text-[9px] text-zinc-500 uppercase">Normal</div>
                        </div>
                    </div>
                    <div class="flex items-center gap-3">
                        <div class="w-7 h-7 rounded-lg bg-green-500/10 flex items-center justify-center text-green-500">
                            <i data-lucide="database" class="w-3.5 h-3.5"></i>
                        </div>
                        <div class="overflow-hidden">
                            <div class="text-[10px] font-semibold text-[var(--text-main)]">Memory</div>
                            <div class="text-[9px] text-zinc-500 uppercase">Stable</div>
                        </div>
                    </div>
                </div>
            </div>

            <div class="bg-[var(--bg-main)] rounded-xl p-4 border border-[var(--border-color)]">
                <div class="flex items-center justify-between mb-2">
                    <span class="text-[10px] font-bold uppercase text-zinc-500">System Status</span>
                    <span class="w-2 h-2 rounded-full bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.5)]"></span>
                </div>
                <div id="mini-status" class="text-xs text-zinc-400 space-y-1">
                    <div class="flex justify-between"><span>Version:</span> <span id="stat-version" class="text-[var(--text-main)] font-mono">-</span></div>
                    <div class="flex justify-between"><span>Tools:</span> <span id="stat-tools" class="text-[var(--text-main)] font-mono">-</span></div>
                    <div class="flex justify-between"><span>Skills:</span> <span id="stat-skills" class="text-[var(--text-main)] font-mono">-</span></div>
                </div>
            </div>
        </div>
    </aside>

    <!-- MAIN CONTENT -->
    <main class="flex-grow flex flex-col relative bg-[var(--bg-main)]">
        <!-- Top Header -->
        <header class="min-h-16 border-b border-[var(--border-color)] flex items-center justify-between px-8 py-3 bg-[var(--bg-sidebar)]/50 backdrop-blur-md z-10 transition-colors duration-300">
            <div class="flex flex-col items-start">
                <h2 id="view-title" class="text-sm font-semibold text-zinc-400 uppercase tracking-wider">Direct Chat</h2>
                <div id="settings-header-tabs" class="hidden items-center gap-2 mt-2">
                    <button id="settings-tab-prompts" onclick="showSettingsSubTab('prompts')" class="px-3 py-1.5 rounded-lg text-xs border border-[var(--border-color)] bg-transparent">SOUL.md & IDENTITY.md</button>
                    <button id="settings-tab-heartbeat" onclick="showSettingsSubTab('heartbeat')" class="px-3 py-1.5 rounded-lg text-xs border border-[var(--border-color)] bg-transparent">Heartbeat</button>
                    <button id="settings-tab-models" onclick="showSettingsSubTab('models')" class="px-3 py-1.5 rounded-lg text-xs border border-[var(--border-color)] bg-transparent">Models</button>
                    <button id="settings-tab-channels" onclick="showSettingsSubTab('channels')" class="px-3 py-1.5 rounded-lg text-xs border border-[var(--border-color)] bg-transparent">Channels</button>
                    <button id="settings-tab-tools" onclick="showSettingsSubTab('tools')" class="px-3 py-1.5 rounded-lg text-xs border border-[var(--border-color)] bg-transparent">Tools</button>
                    <button id="settings-tab-skills" onclick="showSettingsSubTab('skills')" class="px-3 py-1.5 rounded-lg text-xs border border-[var(--border-color)] bg-transparent">Skills</button>
                    <button id="settings-tab-security" onclick="showSettingsSubTab('security')" class="px-3 py-1.5 rounded-lg text-xs border border-[var(--border-color)] bg-transparent">Security</button>
                </div>
            </div>
            <div class="flex items-center gap-4">
                <div id="status-badge" class="flex items-center gap-2 px-3 py-1 rounded-full bg-[var(--bg-main)] border border-[var(--border-color)] text-[11px] font-medium text-zinc-400 transition-colors duration-300">
                    <span class="w-1.5 h-1.5 rounded-full bg-green-500"></span>
                    Gateway Online
                </div>
                <!-- Update & Restart Buttons -->
                <div class="flex items-center gap-2 border-l border-[var(--border-color)] pl-4 ml-2">
                    <button onclick="updateSofia()" class="flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium bg-[var(--nav-hover)] hover:bg-sofia/20 text-[var(--sofia-red)] transition-colors border border-[var(--border-color)]">
                        <i data-lucide="download-cloud" class="w-3.5 h-3.5"></i>
                        Update
                    </button>
                    <button onclick="restartSofia()" class="flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium bg-[var(--nav-hover)] hover:bg-sofia/20 text-[var(--sofia-red)] transition-colors border border-[var(--border-color)]">
                        <i data-lucide="refresh-cw" class="w-3.5 h-3.5"></i>
                        Restart
                    </button>
                </div>
                <button id="theme-toggle" onclick="toggleTheme()" class="w-8 h-8 flex items-center justify-center rounded-lg hover:bg-[var(--nav-hover)] transition text-zinc-500">
                    <i data-lucide="moon" class="w-4 h-4"></i>
                </button>
                <button class="w-8 h-8 flex items-center justify-center rounded-lg hover:bg-[var(--nav-hover)] transition text-zinc-500">
                    <i data-lucide="bell" class="w-4 h-4"></i>
                </button>
            </div>
        </header>

        <!-- Content Area -->
        <div class="flex-grow overflow-hidden p-6">
            
            <!-- CHAT TAB -->
            <div id="tab-chat" class="tab-content active">
                <div class="flex-grow flex gap-6 overflow-hidden">
                    <!-- Chat Area -->
                    <div class="flex-grow flex flex-col glass-panel rounded-2xl border border-[var(--border-color)] overflow-hidden shadow-2xl transition-all duration-300">
                        <div id="chat-history" class="flex-grow overflow-y-auto p-6 space-y-6">
                            <div class="flex gap-4 animate-fade-in">
                                <div class="w-8 h-8 rounded-lg bg-sofia/10 border border-sofia/20 flex items-center justify-center shrink-0">
                                    <img src="/assets/sofiamantis.png" class="w-5 h-5 opacity-80">
                                </div>
                                <div class="chat-bubble-sofia px-4 py-3 rounded-2xl text-sm leading-relaxed max-w-[85%] text-[var(--text-main)] transition-colors duration-300">
                                    Welcome User. System is ready for instructions. How can I assist you today?
                                </div>
                            </div>
                        </div>
                        
                        <!-- Input Area -->
                        <div class="p-4 bg-[var(--bg-sidebar)]/50 border-t border-[var(--border-color)] transition-colors duration-300">
                            <div class="relative flex items-center">
                                <input type="text" id="chat-input" placeholder="Send a command to Sofia..." 
                                    class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl pl-12 pr-24 py-4 text-sm focus:outline-none focus:border-sofia/50 focus:ring-1 focus:ring-sofia/20 transition-all text-[var(--text-main)]">
                                <div class="absolute left-4 text-zinc-500">
                                    <i data-lucide="command" class="w-5 h-5"></i>
                                </div>
                                <div class="absolute right-3 flex gap-2">
                                    <button onclick="sendChat()" class="bg-sofia hover:bg-sofia-hover text-white px-4 py-2 rounded-lg text-xs font-bold transition-all flex items-center gap-2">
                                        <span>Send</span>
                                        <i data-lucide="send" class="w-3 h-3"></i>
                                    </button>
                                </div>
                            </div>
                            <div class="mt-2 px-2 flex justify-between">
                                <span class="text-[10px] text-zinc-600 font-medium">Press <kbd class="px-1 bg-[var(--bg-main)] border border-[var(--border-color)] rounded">Enter</kbd> to send</span>
                                <span id="typing-indicator" class="text-[10px] text-sofia/60 font-medium hidden animate-pulse">Sofia is typing...</span>
                            </div>
                        </div>
                    </div>

                    <!-- Right Sidebar: Agent Monitor -->
                    <div id="agent-monitor-sidebar" class="w-80 shrink-0 flex flex-col">
                        <div class="glass-panel rounded-2xl border border-[var(--border-color)] flex flex-col h-full overflow-hidden shadow-2xl transition-all duration-300">
                            <div class="p-4 border-b border-[var(--border-color)] bg-[var(--bg-sidebar)]/50 flex items-center justify-between transition-colors duration-300">
                                <h3 class="text-[10px] font-bold uppercase tracking-widest text-zinc-500">Agent Monitor</h3>
                                <div class="flex gap-1">
                                    <span class="w-1.5 h-1.5 rounded-full bg-sofia animate-pulse"></span>
                                </div>
                            </div>
                            <div id="agent-activity-monitor" class="flex-grow p-4 overflow-y-auto space-y-4 bg-black/5 dark:bg-black/20 transition-colors duration-300">
                                <!-- Agent boxes will be injected here -->
                            </div>
                            <!-- Live Status Panel (Moved from left sidebar) -->
                            <div id="live-activity-panel" class="p-4 border-t border-[var(--border-color)] bg-sofia/5 hidden transition-colors duration-300">
                                <div class="flex items-center justify-between mb-3">
                                    <span class="text-[10px] font-bold uppercase text-sofia/80 tracking-widest">Global Status</span>
                                    <div class="w-2 h-2 rounded-full bg-sofia animate-pulse shadow-[0_0_8px_rgba(255,77,77,0.5)]"></div>
                                </div>
                                <div class="grid grid-cols-2 gap-2">
                                    <div class="flex items-center gap-2 bg-black/5 dark:bg-black/20 p-2 rounded-lg border border-[var(--border-color)] overflow-hidden transition-colors duration-300">
                                        <div class="w-6 h-6 rounded bg-sofia/10 flex items-center justify-center text-sofia shrink-0">
                                            <i data-lucide="bot" class="w-3 h-3"></i>
                                        </div>
                                        <div class="overflow-hidden">
                                            <div class="text-[8px] text-zinc-500 font-bold uppercase leading-tight">Agent</div>
                                            <div id="active-agent-name" class="text-[10px] text-[var(--text-main)] font-mono truncate">-</div>
                                        </div>
                                    </div>
                                    <div class="flex items-center gap-2 bg-black/5 dark:bg-black/20 p-2 rounded-lg border border-[var(--border-color)] overflow-hidden transition-colors duration-300">
                                        <div class="w-6 h-6 rounded bg-zinc-200 dark:bg-zinc-800 flex items-center justify-center text-zinc-400 shrink-0">
                                            <i data-lucide="activity" class="w-3 h-3 animate-spin-slow"></i>
                                        </div>
                                        <div class="overflow-hidden">
                                            <div class="text-[8px] text-zinc-500 font-bold uppercase leading-tight">Status</div>
                                            <div id="active-agent-status" class="text-[10px] text-zinc-500 dark:text-zinc-300 truncate italic">Thinking...</div>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- LOGS TAB -->
            <div id="tab-logs" class="tab-content">
                <div class="glass-panel rounded-2xl border border-[var(--border-color)] p-6 flex flex-col h-full overflow-hidden shadow-2xl transition-all duration-300">
                    <div class="flex items-center justify-between mb-6">
                        <div>
                            <h2 class="text-xl font-bold text-[var(--text-main)]">System Logs</h2>
                            <p class="text-sm text-zinc-500">Real-time monitoring of system events and agent activity.</p>
                        </div>
                        <div class="flex gap-2">
                            <button onclick="clearLogs()" class="px-4 py-2 bg-[var(--bg-main)] hover:bg-[var(--nav-hover)] border border-[var(--border-color)] rounded-xl text-xs font-medium text-zinc-400 transition-all flex items-center gap-2">
                                <i data-lucide="trash-2" class="w-3.5 h-3.5"></i>
                                Clear
                            </button>
                        </div>
                    </div>
                    <div id="log-view" class="flex-grow bg-black/5 dark:bg-black/40 rounded-2xl p-6 font-mono text-xs overflow-y-auto text-zinc-500 dark:text-zinc-400 border border-[var(--border-color)] leading-relaxed transition-colors duration-300">
                        <div class="text-zinc-600 italic">Connecting to stream...</div>
                    </div>
                </div>
            </div>

            <!-- AGENTS TAB -->
            <div id="tab-agents" class="tab-content">
                <div class="grid grid-cols-1 md:grid-cols-3 gap-6 h-full overflow-y-auto pr-2">
                    <!-- Add/Edit Agent -->
                    <div class="md:col-span-1 glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl h-fit sticky top-0 transition-all duration-300">
                        <div class="flex items-center gap-3 mb-6">
                            <div class="w-10 h-10 rounded-xl bg-sofia/10 flex items-center justify-center text-sofia">
                                <i data-lucide="user-plus" class="w-5 h-5"></i>
                            </div>
                            <h2 id="agent-form-title" class="text-lg font-bold text-[var(--text-main)]">New Agent</h2>
                        </div>
                        <div class="space-y-4">
                            <div>
                                <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">Identifier (Slug)</label>
                                <input type="text" id="agent-id" placeholder="e.g. coder" 
                                    class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm focus:outline-none focus:border-sofia/50 transition-all text-[var(--text-main)]">
                            </div>
                            <div>
                                <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">Display Name</label>
                                <input type="text" id="agent-name" placeholder="e.g. Coder"
                                    class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm focus:outline-none focus:border-sofia/50 transition-all text-[var(--text-main)]">
                            </div>
                            <div>
                                <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">AI Model</label>
                                <select id="agent-model"
                                    class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm focus:outline-none focus:border-sofia/50 transition-all text-[var(--text-main)]">
                                    <option value="">Default (System Default)</option>
                                </select>
                            </div>
							<div>
								<label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">Purpose Template (Antigravity)</label>
								<select id="agent-template" onchange="onTemplateSelected()"
									class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm focus:outline-none focus:border-sofia/50 transition-all text-[var(--text-main)]">
									<option value="">No template</option>
								</select>
								<div id="agent-template-missing-skills" class="mt-2 hidden text-[11px] text-yellow-500"></div>
							</div>
							<div>
								<label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">Purpose &amp; Instructions</label>
								<div id="agent-template-missing-skills" class="mb-2 hidden text-[11px] text-yellow-500"></div>
								<div id="agent-template-hint" class="hidden mb-1.5 text-[10px] text-sofia/70">Pre-filled from template — you can edit freely.</div>
								<textarea id="agent-instructions" rows="7" placeholder="Describe what this agent should do and how it should behave..."
									class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm focus:outline-none focus:border-sofia/50 transition-all text-[var(--text-main)] resize-none"></textarea>
							</div>
                            <div>
                                <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">Custom Skills</label>
                                <div id="agent-custom-skills-tags" class="flex flex-wrap gap-1.5 mb-2 min-h-[24px]"></div>
                                <select id="agent-custom-skills-picker" onchange="addCustomSkill()" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-3 py-2.5 text-xs focus:outline-none focus:border-sofia/50 transition-all text-[var(--text-main)]">
                                    <option value="">+ Add a skill...</option>
                                </select>
                                <div class="mt-1 text-[10px] text-zinc-500">Skills added here are always available to this agent, regardless of template.</div>
                            </div>
                            <div class="pt-4 flex flex-col gap-2">
                                <button onclick="saveAgent()" class="w-full bg-sofia hover:bg-sofia-hover text-white font-bold py-3 rounded-xl transition shadow-lg shadow-sofia/10">Save Agent</button>
                                <button onclick="resetAgentForm()" class="w-full py-2 text-zinc-500 hover:text-zinc-300 text-xs font-medium transition">Reset form</button>
                            </div>
                        </div>
                    </div>

                    <!-- Agent List -->
                    <div class="md:col-span-2 space-y-4">
                        <div class="flex items-center justify-between mb-2">
                            <h2 class="text-lg font-bold text-[var(--text-main)]">Configured Agents</h2>
                            <span class="text-xs text-zinc-500" id="agent-count">0 agents</span>
                        </div>
                        <div id="agents-list" class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                            <!-- Filled by JS -->
                        </div>
                    </div>
                </div>
            </div>

            <!-- SKILLS TAB -->
            <div id="settings-subtab-skills" class="settings-subtab hidden h-full">
                <div class="grid grid-cols-1 md:grid-cols-2 gap-8 h-full">
                    <!-- Skills List -->
                    <div class="flex flex-col h-full overflow-hidden">
                        <div class="flex items-center justify-between mb-4">
                            <h2 class="text-lg font-bold text-[var(--text-main)]">Installed Skills</h2>
                            <div class="relative w-48">
                                <input type="text" id="skill-search" onkeyup="filterSkills()" placeholder="Search skills..." 
                                    class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-lg px-3 py-1.5 text-xs focus:outline-none focus:border-sofia/50 transition-all text-[var(--text-main)]">
                                <i data-lucide="search" class="absolute right-3 top-2 w-3.5 h-3.5 text-zinc-600"></i>
                            </div>
                        </div>
                        <div id="skills-list" class="flex-grow overflow-y-auto pr-2 space-y-4">
                            <p class="text-zinc-500 italic text-sm">Loading library...</p>
                        </div>
                    </div>

                    <!-- Tools & Add Skill -->
                    <div class="flex flex-col gap-6 overflow-hidden">
                        <div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl shrink-0 transition-all duration-300">
                            <h2 class="text-lg font-bold text-[var(--text-main)] mb-4">Install New Skill</h2>
                            <div class="space-y-4">
                                <input type="text" id="new-skill-name" placeholder="Skill name (e.g. web-search)" 
                                    class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:border-sofia/50 text-[var(--text-main)]">
                                <textarea id="new-skill-content" rows="4" placeholder="Markdown content (SKILL.md)..." 
                                    class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm font-mono focus:outline-none focus:border-sofia/50 text-[var(--text-main)]"></textarea>
                                <button onclick="addSkill()" class="w-full bg-[var(--bg-sidebar)] hover:bg-[var(--nav-hover)] text-[var(--text-main)] border border-[var(--border-color)] font-bold py-2.5 rounded-xl transition text-sm">Install Skill</button>
                            </div>
                        </div>
                    </div>
                </div>
            </div>


			<!-- TOOLS TAB -->
			<div id="settings-subtab-tools" class="settings-subtab hidden h-full">
				<div class="h-full overflow-y-auto pr-2 space-y-6">
					<div class="flex items-center justify-between mb-4">
						<h2 class="text-xl font-bold text-[var(--text-main)]">Native Tools</h2>
						<p class="text-sm text-zinc-500">Built-in tools available to all agents.</p>
					</div>
			<div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl transition-all duration-300">
						<div id="tools-list" class="space-y-3">
							<!-- Filled by JS -->
						</div>
					</div>

					<!-- Google CLI Tool (gog) -->
					<div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl transition-all duration-300">
						<div class="flex items-center justify-between mb-4">
							<div class="flex items-center gap-3">
								<div class="w-9 h-9 rounded-xl bg-blue-500/10 flex items-center justify-center">
									<i data-lucide="mail" class="w-5 h-5 text-blue-400"></i>
								</div>
								<div>
									<h3 class="text-sm font-bold text-[var(--text-main)]">Google Tools (gog CLI)</h3>
									<p class="text-[10px] text-zinc-500">Gmail, Google Drive, Google Calendar integration</p>
								</div>
							</div>
							<label class="relative inline-flex items-center cursor-pointer">
								<input type="checkbox" id="cfg-google-enabled" class="sr-only peer" onchange="saveToolsConfig()">
								<div class="w-9 h-5 bg-zinc-700 rounded-full peer peer-checked:bg-sofia transition-colors after:content-[''] after:absolute after:top-0.5 after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:after:translate-x-full"></div>
							</label>
						</div>

						<div id="google-tool-config" class="space-y-3 mb-4">
							<div class="grid grid-cols-2 gap-3">
								<div>
									<label class="block text-[10px] uppercase tracking-widest text-zinc-500 mb-1">Binary Path</label>
									<input type="text" id="cfg-google-binary" placeholder="gog"
										class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-lg px-3 py-2 text-xs text-[var(--text-main)]">
								</div>
								<div>
									<label class="block text-[10px] uppercase tracking-widest text-zinc-500 mb-1">Timeout (seconds)</label>
									<input type="number" id="cfg-google-timeout" placeholder="90"
										class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-lg px-3 py-2 text-xs text-[var(--text-main)]">
								</div>
							</div>
							<div>
								<label class="block text-[10px] uppercase tracking-widest text-zinc-500 mb-1">Allowed Commands</label>
								<input type="text" id="cfg-google-commands" placeholder="gmail, drive, calendar"
									class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-lg px-3 py-2 text-xs text-[var(--text-main)]">
							</div>
							<button onclick="saveToolsConfig()" class="px-4 py-2 rounded-lg bg-sofia hover:bg-sofia-hover text-white text-xs font-bold transition shadow-lg shadow-sofia/20 flex items-center gap-2">
								<i data-lucide="save" class="w-3 h-3"></i> Save Google Settings
							</button>
						</div>

						<div class="mt-3 p-3 rounded-xl bg-[var(--bg-main)] border border-[var(--border-color)]">
							<h4 class="text-[10px] uppercase tracking-widest text-zinc-500 mb-2 font-bold">Setup Instructions</h4>
							<ol class="text-[11px] text-zinc-400 leading-relaxed space-y-1.5 list-decimal list-inside">
								<li>Clone the repo: <code class="px-1 py-0.5 rounded bg-zinc-800 text-sofia text-[10px]">git clone https://github.com/steipete/gogcli.git</code> then <code class="px-1 py-0.5 rounded bg-zinc-800 text-sofia text-[10px]">cd gogcli &amp;&amp; go build -o gog . &amp;&amp; mv gog /usr/local/bin/</code></li>
								<li>Run <code class="px-1 py-0.5 rounded bg-zinc-800 text-sofia text-[10px]">gog auth login</code> to authenticate with Google</li>
								<li>Enable the toggle above and save. Restart Sofia after saving.</li>
								<li>Sofia will now have access to Gmail, Drive and Calendar tools.</li>
							</ol>
						</div>
					</div>

					<!-- GitHub CLI Tool -->
					<div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl transition-all duration-300">
						<div class="flex items-center justify-between mb-4">
							<div class="flex items-center gap-3">
								<div class="w-9 h-9 rounded-xl bg-purple-500/10 flex items-center justify-center">
									<i data-lucide="github" class="w-5 h-5 text-purple-400"></i>
								</div>
								<div>
									<h3 class="text-sm font-bold text-[var(--text-main)]">GitHub Integration</h3>
									<p class="text-[10px] text-zinc-500">Access GitHub repositories, issues, and pull requests</p>
								</div>
							</div>
							<span class="px-2 py-0.5 rounded bg-zinc-800 text-[10px] text-zinc-500 border border-zinc-700/50">Coming Soon</span>
						</div>

						<div class="p-3 rounded-xl bg-[var(--bg-main)] border border-[var(--border-color)]">
							<h4 class="text-[10px] uppercase tracking-widest text-zinc-500 mb-2 font-bold">Preparation</h4>
							<ol class="text-[11px] text-zinc-400 leading-relaxed space-y-1.5 list-decimal list-inside">
								<li>Install GitHub CLI: <code class="px-1 py-0.5 rounded bg-zinc-800 text-sofia text-[10px]">brew install gh</code> (macOS) or <a href="https://cli.github.com/" target="_blank" class="text-sofia hover:underline">cli.github.com</a></li>
								<li>Authenticate: <code class="px-1 py-0.5 rounded bg-zinc-800 text-sofia text-[10px]">gh auth login</code></li>
								<li>GitHub tool support is under development. Once available, you can enable it here.</li>
							</ol>
						</div>
					</div>
				</div>
			</div>

			<!-- MODELS TAB -->
			<div id="settings-subtab-models" class="settings-subtab hidden h-full">
				<div class="h-full overflow-y-auto pr-2 space-y-6">
                        <input type="hidden" id="cfg-model" value="">
                        

                            <!-- Add/Edit Model Form (Progressive) -->
                            <div id="model-config-form" class="hidden mb-6 glass-panel p-6 rounded-2xl border border-sofia/30 shadow-xl transition-all duration-300 bg-sofia/5">
                                <div class="flex items-center justify-between mb-6">
                                    <h3 id="model-form-title" class="text-sm font-bold text-sofia uppercase tracking-widest">Add New Model</h3>
                                    <button onclick="closeModelForm()" class="text-zinc-500 hover:text-[var(--text-main)]">
                                        <i data-lucide="x" class="w-4 h-4"></i>
                                    </button>
                                </div>
                                <input type="hidden" id="edit-model-index" value="-1">

                                <div class="space-y-5">
                                    <!-- Step 1: Provider -->
                                    <div>
                                        <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">1. Select Provider</label>
                                        <select id="form-provider" onchange="onProviderChange()" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm focus:outline-none focus:border-sofia/50 text-[var(--text-main)]">
                                            <option value="">-- Choose a Provider --</option>
                                            <option value="Google Gemini">Google Gemini</option>
                                            <option value="OpenAI">OpenAI</option>
                                            <option value="Anthropic">Anthropic</option>
                                            <option value="DeepSeek">DeepSeek</option>
                                            <option value="Groq">Groq</option>
                                            <option value="Mistral">Mistral</option>
                                            <option value="OpenRouter">OpenRouter</option>
                                            <option value="Qwen">Qwen</option>
                                            <option value="Moonshot">Moonshot</option>
                                            <option value="xAI (Grok)">xAI (Grok)</option>
                                            <option value="Z.ai">Z.ai</option>
                                            <option value="MiniMax">MiniMax</option>
                                            <option value="Custom">Custom / Other</option>
                                        </select>
                                    </div>

                                    <!-- Step 2: Model (hidden until provider selected) -->
                                    <div id="form-step-model" class="hidden animate-fade-in">
                                        <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">2. Select Model</label>
                                        <select id="form-model-select" onchange="onModelChange()" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm focus:outline-none focus:border-sofia/50 text-[var(--text-main)]">
                                            <!-- Filled by JS -->
                                        </select>
                                        <div id="form-model-custom-wrapper" class="hidden mt-2">
                                            <input type="text" id="form-model-custom" placeholder="e.g. custom/my-model-1" onchange="onModelChange()" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm focus:outline-none focus:border-sofia/50 text-[var(--text-main)]">
                                            <p class="text-[10px] text-zinc-500 mt-1 ml-1 text-right">Enter protocol/model id (e.g. openai/my-model)</p>
                                        </div>
                                    </div>

                                    <!-- Step 3: Config (hidden until model selected) -->
                                    <div id="form-step-config" class="hidden space-y-4 pt-5 border-t border-[var(--border-color)] animate-fade-in">
                                        <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 ml-1">3. Configuration Details</label>
                                        
                                        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                                            <div>
                                                <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">Alias / Display Name</label>
                                                <input type="text" id="form-model-alias" placeholder="e.g. GPT-4o (Personal)" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm text-[var(--text-main)] focus:outline-none focus:border-sofia/50 transition-all w-full">
                                            </div>
                                            <div>
                                                <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">API Key</label>
                                                <input type="password" id="form-model-key" placeholder="sk-..." class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm text-[var(--text-main)] focus:outline-none focus:border-sofia/50 transition-all">
                                            </div>
                                            <div>
                                                <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">API Base URL (Optional)</label>
                                                <input type="text" id="form-model-base" placeholder="leave blank for default" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm text-[var(--text-main)] focus:outline-none focus:border-sofia/50 transition-all opacity-80 focus:opacity-100">
                                            </div>
                                            <div>
                                                <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">Workspace ID (Optional)</label>
                                                <input type="text" id="form-model-workspace" placeholder="Optional identifier" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm text-[var(--text-main)] focus:outline-none focus:border-sofia/50 transition-all opacity-80 focus:opacity-100">
                                            </div>
                                        </div>

                                        <details class="text-sm bg-black/5 dark:bg-zinc-900/40 border border-[var(--border-color)] rounded-xl mt-4 group">
                                            <summary class="text-[11px] font-bold uppercase tracking-widest text-zinc-500 cursor-pointer p-4 hover:text-[var(--text-main)] focus:outline-none flex justify-between items-center group-open:border-b group-open:border-[var(--border-color)]">
                                                Advanced Model Settings 
                                                <i data-lucide="chevron-down" class="w-4 h-4 transition-transform group-open:rotate-180"></i>
                                            </summary>
                                            <div class="grid grid-cols-1 md:grid-cols-2 gap-4 p-4">
                                                <div>
                                                    <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">RPM Limit</label>
                                                    <input type="number" id="form-model-rpm" placeholder="0 = infinite" min="0" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-3 py-2 text-xs text-[var(--text-main)]">
                                                </div>
                                                <div>
                                                    <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">Timeout (seconds)</label>
                                                    <input type="number" id="form-model-timeout" placeholder="Client default" min="0" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-3 py-2 text-xs text-[var(--text-main)]">
                                                </div>
                                                <div>
                                                    <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">Proxy URL</label>
                                                    <input type="text" id="form-model-proxy" placeholder="http://..." class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-3 py-2 text-xs text-[var(--text-main)]">
                                                </div>
                                                <div>
                                                    <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">Auth Method</label>
                                                    <input type="text" id="form-model-auth" placeholder="Bearer" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-3 py-2 text-xs text-[var(--text-main)]">
                                                </div>
                                                <div>
                                                    <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">Max Tokens Field</label>
                                                    <input type="text" id="form-model-tokens-field" placeholder="max_tokens" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-3 py-2 text-xs text-[var(--text-main)]">
                                                </div>
                                                <div>
                                                    <label class="block text-[100px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">Connect Mode</label>
                                                    <input type="text" id="form-model-connect" placeholder="default" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-3 py-2 text-xs text-[var(--text-main)]">
                                                </div>
                                            </div>
                                        </details>

                                        <div class="pt-4 flex gap-3 justify-end items-center mt-2">
                                            <button onclick="closeModelForm()" class="px-6 py-2.5 bg-transparent border border-[var(--border-color)] hover:bg-[var(--bg-main)] text-[var(--text-main)] rounded-xl text-sm font-medium transition">
                                                Cancel
                                            </button>
                                            <button onclick="saveModelForm()" class="px-8 bg-sofia hover:bg-sofia-hover text-white font-bold py-2.5 rounded-xl transition shadow-lg shadow-sofia/20 flex items-center gap-2">
                                                <i data-lucide="check" class="w-4 h-4"></i> Save Model
                                            </button>
                                        </div>
                                    </div>
                                </div>
                            </div>

                        <!-- Configured Models List -->
                        <div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl transition-all duration-300">
                            <div class="flex items-center justify-between mb-4">
                                <div>
                                    <h3 class="text-xs font-bold uppercase tracking-widest text-zinc-500">Configured Models</h3>
                                    <p class="text-[10px] text-zinc-500 mt-1">Select the standard model for system operations</p>
                                </div>
                                <button onclick="openModelForm()" class="px-4 py-2 bg-[var(--bg-main)] border border-[var(--border-color)] hover:bg-[var(--nav-hover)] text-[var(--text-main)] rounded-xl text-xs font-bold transition flex items-center gap-2">
                                    <i data-lucide="plus" class="w-3.5 h-3.5"></i> Add Model
                                </button>
                            </div>

                            <div id="provider-model-list" class="space-y-2 max-h-96 overflow-y-auto pr-2">
                                <!-- Filled by JS -->
                                <div class="text-sm text-zinc-500 italic p-4 text-center border border-dashed border-[var(--border-color)] rounded-xl">No models configured.</div>
                            </div>
                        </div>
                    </div>
				</div>
			</div>

			<!-- CHANNELS TAB -->
<div id="settings-subtab-channels" class="settings-subtab hidden h-full">
	<div class="h-full overflow-y-auto pr-2 space-y-6">
                        <div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl transition-colors duration-300">
                            <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-3 ml-1">Channel Setup</label>
                            <div class="grid grid-cols-1 xl:grid-cols-2 gap-4">
                                <div class="p-4 rounded-xl border border-[var(--border-color)] bg-[var(--bg-main)] space-y-3">
                                    <label class="flex items-center gap-3 cursor-pointer">
                                        <input type="checkbox" id="cfg-telegram" class="w-4 h-4 rounded border-zinc-700 bg-zinc-800 text-sofia focus:ring-sofia/20">
                                        <span class="text-sm font-semibold text-[var(--text-main)]">Telegram</span>
                                    </label>
                                    <input type="password" id="cfg-telegram-token" placeholder="Bot token"
                                        class="w-full bg-transparent border border-[var(--border-color)] rounded-lg px-3 py-2 text-xs text-[var(--text-main)]">
                                    <input type="text" id="cfg-telegram-proxy" placeholder="Proxy URL (optional)"
                                        class="w-full bg-transparent border border-[var(--border-color)] rounded-lg px-3 py-2 text-xs text-[var(--text-main)]">
                                    <textarea id="cfg-telegram-allow-from" rows="3" placeholder="Allowed user IDs (comma or newline separated)"
                                        class="w-full bg-transparent border border-[var(--border-color)] rounded-lg px-3 py-2 text-xs text-[var(--text-main)]"></textarea>
                                    <div id="cfg-telegram-warning" class="hidden text-[11px] text-yellow-500">Telegram is enabled but token is empty.</div>
                                </div>

                                <div class="p-4 rounded-xl border border-[var(--border-color)] bg-[var(--bg-main)] space-y-3">
                                    <label class="flex items-center gap-3 cursor-pointer">
                                        <input type="checkbox" id="cfg-discord" class="w-4 h-4 rounded border-zinc-700 bg-zinc-800 text-sofia focus:ring-sofia/20">
                                        <span class="text-sm font-semibold text-[var(--text-main)]">Discord</span>
                                    </label>
                                    <input type="password" id="cfg-discord-token" placeholder="Bot token"
                                        class="w-full bg-transparent border border-[var(--border-color)] rounded-lg px-3 py-2 text-xs text-[var(--text-main)]">
                                    <textarea id="cfg-discord-allow-from" rows="3" placeholder="Allowed user IDs (comma or newline separated)"
                                        class="w-full bg-transparent border border-[var(--border-color)] rounded-lg px-3 py-2 text-xs text-[var(--text-main)]"></textarea>
                                    <label class="flex items-center gap-2 text-xs text-zinc-500">
                                        <input type="checkbox" id="cfg-discord-mention-only" class="w-4 h-4 rounded border-zinc-700 bg-zinc-800 text-sofia focus:ring-sofia/20">
                                        Respond only when bot is mentioned
                                    </label>
                                    <div id="cfg-discord-warning" class="hidden text-[11px] text-yellow-500">Discord is enabled but token is empty.</div>
                                </div>
                            </div>

                            <div id="cfg-channels-warning" class="hidden mt-3 text-[11px] text-yellow-500">One or more enabled channels are missing tokens.</div>

                            <button onclick="saveConfig()" class="mt-4 bg-sofia hover:bg-sofia-hover text-white font-bold px-8 py-3 rounded-xl transition shadow-lg shadow-sofia/20 flex items-center gap-2">
                            <button onclick="saveConfig()" class="mt-4 bg-sofia hover:bg-sofia-hover text-white font-bold px-8 py-3 rounded-xl transition shadow-lg shadow-sofia/20 flex items-center gap-2">
                                <i data-lucide="save" class="w-4 h-4"></i>
                                Save changes
                            </button>
                        </div>



	</div>
</div>

            <!-- SETTINGS TAB -->
            <div id="tab-settings" class="tab-content h-full">
                <div class="h-full overflow-y-auto pr-2 space-y-6">
                    
                    <div id="settings-subtab-security" class="settings-subtab hidden space-y-4">
                        <div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl transition-colors duration-300">
                            <h3 class="text-xs font-bold uppercase tracking-widest text-zinc-500 border-b border-[var(--border-color)] pb-3 mb-3">Workspace Security</h3>

                            <div class="p-4 rounded-xl border border-[var(--border-color)] bg-[var(--bg-main)]">
                                <div class="flex items-start justify-between">
                                    <div class="space-y-1 pr-6">
                                        <h4 class="text-sm font-semibold text-[var(--text-main)]">Restrict to Workspace</h4>
                                        <p class="text-[11px] leading-relaxed text-zinc-500">
                                            When enabled, Sofia's file and command tools are strictly sandboxed to the configured workspace path.
                                            This prevents accidental modification of system files or parent directories.
                                        </p>
                                    </div>
                                    <label class="relative inline-flex items-center cursor-pointer shrink-0 mt-1">
                                        <input type="checkbox" id="cfg-restrict-workspace" class="sr-only peer" onchange="saveConfig()">
                                        <div class="w-9 h-5 bg-zinc-700 rounded-full peer peer-checked:bg-sofia transition-colors after:content-[''] after:absolute after:top-0.5 after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:after:translate-x-full"></div>
                                    </label>
                                </div>
                            </div>
                        </div>
                    </div>

                    <div id="settings-subtab-heartbeat" class="settings-subtab hidden space-y-4">
                        <div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl transition-colors duration-300">
                            <h3 class="text-xs font-bold uppercase tracking-widest text-zinc-500 border-b border-[var(--border-color)] pb-3 mb-3">Heartbeat Background Agent</h3>
                            
                            <div class="flex items-start justify-between mb-4 pb-4 border-b border-[var(--border-color)]">
                                <div>
                                    <h4 class="text-sm font-semibold text-[var(--text-main)]">Enable Heartbeat</h4>
                                    <p class="text-[11px] leading-relaxed text-zinc-500 mt-1">Sofia will automatically spawn background agents on a schedule.</p>
                                </div>
                                <label class="relative inline-flex items-center cursor-pointer mt-1">
                                    <input type="checkbox" id="cfg-heartbeat-enabled" class="sr-only peer" onchange="saveConfig()">
                                    <div class="w-9 h-5 bg-zinc-700 rounded-full peer peer-checked:bg-sofia transition-colors after:content-[''] after:absolute after:top-0.5 after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:after:translate-x-full relative"></div>
                                </label>
                            </div>

                            <div class="grid grid-cols-2 gap-4 mb-4">
                                <div>
                                    <label class="block text-[10px] uppercase tracking-widest text-zinc-500 mb-1">Interval (minutes)</label>
                                    <input type="number" id="cfg-heartbeat-interval" min="5" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-3 py-2 text-xs text-[var(--text-main)] transition-colors focus:border-sofia focus:ring-1 focus:ring-sofia/20" onchange="saveConfig()">
                                </div>
                                <div>
                                    <label class="block text-[10px] uppercase tracking-widest text-zinc-500 mb-1">Active Hours</label>
                                    <input type="text" id="cfg-heartbeat-hours" placeholder="09:00-17:00 (Leave empty for 24/7)" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-3 py-2 text-xs text-[var(--text-main)] transition-colors focus:border-sofia focus:ring-1 focus:ring-sofia/20" onchange="saveConfig()">
                                </div>
                            </div>

                            <label class="block text-[10px] uppercase tracking-widest text-zinc-500 mb-2">Active Days (Leave empty for every day)</label>
                            <div class="flex flex-wrap gap-2 mb-6" id="cfg-heartbeat-days-container">
                                <!-- Days will be injected here via JS -->
                            </div>
                        </div>
                    </div>

                    <div id="settings-subtab-prompts" class="settings-subtab hidden space-y-4">
                        <div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl transition-colors duration-300">
                            <h3 class="text-xs font-bold uppercase tracking-widest text-zinc-500 border-b border-[var(--border-color)] pb-3 mb-3">Prompt Files</h3>
                            <label class="block text-[10px] uppercase tracking-widest text-zinc-500 mb-1">IDENTITY.md</label>
                            <textarea id="cfg-identity-md" rows="16" class="w-full min-h-[28rem] bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-3 py-2 text-xs font-mono text-[var(--text-main)] mb-3"></textarea>
                            <label class="block text-[10px] uppercase tracking-widest text-zinc-500 mb-1">SOUL.md</label>
                            <textarea id="cfg-soul-md" rows="16" class="w-full min-h-[28rem] bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-3 py-2 text-xs font-mono text-[var(--text-main)]"></textarea>
                            <button onclick="saveWorkspaceDocs()" class="mt-3 px-4 py-2 rounded-lg bg-[var(--bg-main)] border border-[var(--border-color)] text-xs hover:bg-[var(--nav-hover)]">Save prompt files</button>
                        </div>
                    </div>

                </div>
            </div>
        </div>
    </main>

    <script>
        let currentTab = 'chat';
        let skillsData = [];
        let activeAgentID = null;
        let purposeTemplates = [];
        let purposeTemplateMap = {};
        let currentConfig = null;
        let currentSettingsSubTab = 'models';

        // Provider → list of {label, model_id, api_base} presets
        const PROVIDER_MODELS = {
            "Google Gemini": [
                { label: "Gemini 3.1 Pro (Preview)", model_id: "gemini/gemini-3.1-pro-preview", api_base: "https://generativelanguage.googleapis.com/v1beta/openai" },
                { label: "Gemini 3.1 Flash-Lite (Preview)", model_id: "gemini/gemini-3.1-flash-lite-preview", api_base: "https://generativelanguage.googleapis.com/v1beta/openai" },
                { label: "Gemini 3 Flash (Preview)", model_id: "gemini/gemini-3-flash-preview", api_base: "https://generativelanguage.googleapis.com/v1beta/openai" },
                { label: "Gemini 2.5 Pro", model_id: "gemini/gemini-2.5-pro", api_base: "https://generativelanguage.googleapis.com/v1beta/openai" },
                { label: "Gemini 2.5 Flash", model_id: "gemini/gemini-2.5-flash", api_base: "https://generativelanguage.googleapis.com/v1beta/openai" },
                { label: "Gemini 2.5 Flash-Lite", model_id: "gemini/gemini-2.5-flash-lite", api_base: "https://generativelanguage.googleapis.com/v1beta/openai" },
                { label: "Gemini 2.0 Flash", model_id: "gemini/gemini-2.0-flash", api_base: "https://generativelanguage.googleapis.com/v1beta/openai" },
            ],
            "OpenAI": [
                { label: "GPT-5.2", model_id: "openai/gpt-5.2", api_base: "https://api.openai.com/v1" },
                { label: "GPT-5.2 Pro", model_id: "openai/gpt-5.2-pro", api_base: "https://api.openai.com/v1" },
                { label: "GPT-5.2 Codex", model_id: "openai/gpt-5.2-codex", api_base: "https://api.openai.com/v1" },
                { label: "GPT-5", model_id: "openai/gpt-5", api_base: "https://api.openai.com/v1" },
                { label: "GPT-5 Mini", model_id: "openai/gpt-5-mini", api_base: "https://api.openai.com/v1" },
                { label: "GPT-5 Nano", model_id: "openai/gpt-5-nano", api_base: "https://api.openai.com/v1" },
                { label: "GPT-4.1", model_id: "openai/gpt-4.1", api_base: "https://api.openai.com/v1" },
                { label: "GPT-4o", model_id: "openai/gpt-4o", api_base: "https://api.openai.com/v1" },
                { label: "GPT-4o Mini", model_id: "openai/gpt-4o-mini", api_base: "https://api.openai.com/v1" },
                { label: "o3", model_id: "openai/o3", api_base: "https://api.openai.com/v1" },
                { label: "o3 Pro", model_id: "openai/o3-pro", api_base: "https://api.openai.com/v1" },
                { label: "o3 Mini", model_id: "openai/o3-mini", api_base: "https://api.openai.com/v1" },
                { label: "o4 Mini", model_id: "openai/o4-mini", api_base: "https://api.openai.com/v1" },
            ],
            "Anthropic": [
                { label: "Claude Opus 4.6", model_id: "anthropic/claude-opus-4-6", api_base: "https://api.anthropic.com/v1" },
                { label: "Claude Sonnet 4.6", model_id: "anthropic/claude-sonnet-4-6", api_base: "https://api.anthropic.com/v1" },
                { label: "Claude Opus 4.5", model_id: "anthropic/claude-opus-4-5", api_base: "https://api.anthropic.com/v1" },
                { label: "Claude Sonnet 4.5", model_id: "anthropic/claude-sonnet-4-5", api_base: "https://api.anthropic.com/v1" },
                { label: "Claude Haiku 4.5", model_id: "anthropic/claude-haiku-4-5", api_base: "https://api.anthropic.com/v1" },
            ],
            "DeepSeek": [
                { label: "DeepSeek V3 (Chat)", model_id: "deepseek/deepseek-chat", api_base: "https://api.deepseek.com/v1" },
                { label: "DeepSeek R1 (Reasoner)", model_id: "deepseek/deepseek-reasoner", api_base: "https://api.deepseek.com/v1" },
            ],
            "Groq": [
                { label: "Llama 3.3 70b", model_id: "groq/llama-3.3-70b-versatile", api_base: "https://api.groq.com/openai/v1" },
                { label: "Mixtral 8x7b", model_id: "groq/mixtral-8x7b-32768", api_base: "https://api.groq.com/openai/v1" },
            ],
            "Mistral": [
                { label: "Mistral Large (Latest)", model_id: "mistral/mistral-large-latest", api_base: "https://api.mistral.ai/v1" },
                { label: "Mistral Medium 3.1", model_id: "mistral/mistral-medium-latest", api_base: "https://api.mistral.ai/v1" },
                { label: "Mistral Small 3.2", model_id: "mistral/mistral-small-latest", api_base: "https://api.mistral.ai/v1" },
                { label: "Codestral (Latest)", model_id: "mistral/codestral-latest", api_base: "https://api.mistral.ai/v1" },
                { label: "Devstral 2", model_id: "mistral/devstral-latest", api_base: "https://api.mistral.ai/v1" },
                { label: "Pixtral Large", model_id: "mistral/pixtral-large-latest", api_base: "https://api.mistral.ai/v1" },
            ],
            "Qwen": [
                { label: "Qwen3.5 Plus", model_id: "qwen/qwen3.5-plus", api_base: "https://dashscope.aliyuncs.com/compatible-mode/v1" },
                { label: "Qwen3 Max", model_id: "qwen/qwen3-max", api_base: "https://dashscope.aliyuncs.com/compatible-mode/v1" },
                { label: "Qwen Plus", model_id: "qwen/qwen-plus-latest", api_base: "https://dashscope.aliyuncs.com/compatible-mode/v1" },
                { label: "Qwen Turbo", model_id: "qwen/qwen-turbo-latest", api_base: "https://dashscope.aliyuncs.com/compatible-mode/v1" },
                { label: "Qwen3 Coder", model_id: "qwen/qwen3-coder-next", api_base: "https://dashscope.aliyuncs.com/compatible-mode/v1" },
            ],
            "Moonshot": [
                { label: "Kimi K2.5", model_id: "moonshot/kimi-k2.5", api_base: "https://api.moonshot.cn/v1" },
            ],
            "xAI (Grok)": [
                { label: "Grok 4", model_id: "grok/grok-4-0709", api_base: "https://api.x.ai/v1" },
                { label: "Grok 4.1 Fast", model_id: "grok/grok-4-1-fast-reasoning", api_base: "https://api.x.ai/v1" },
                { label: "Grok 3", model_id: "grok/grok-3", api_base: "https://api.x.ai/v1" },
                { label: "Grok 3 Mini", model_id: "grok/grok-3-mini", api_base: "https://api.x.ai/v1" },
                { label: "Grok 2", model_id: "grok/grok-2-1212", api_base: "https://api.x.ai/v1" },
            ],
            "Z.ai": [
                { label: "GLM-5", model_id: "zai/glm-5", api_base: "https://api.z.ai/api/paas/v4" },
                { label: "GLM-4.7 FlashX", model_id: "zai/glm-4.7-flashx", api_base: "https://api.z.ai/api/paas/v4" },
                { label: "GLM-4.5", model_id: "zai/glm-4.5", api_base: "https://api.z.ai/api/paas/v4" },
                { label: "GLM-4.5 Air", model_id: "zai/glm-4.5-air", api_base: "https://api.z.ai/api/paas/v4" },
            ],
            "MiniMax": [
                { label: "MiniMax M2.5", model_id: "minimax/MiniMax-M2.5", api_base: "https://api.minimax.io/v1" },
                { label: "MiniMax M1 80k", model_id: "minimax/MiniMax-M1-80k", api_base: "https://api.minimax.io/v1" },
                { label: "abab7", model_id: "minimax/abab7", api_base: "https://api.minimax.io/v1" },
                { label: "abab6.5s", model_id: "minimax/abab6.5s", api_base: "https://api.minimax.io/v1" },
            ],
            "OpenRouter": [],
            "Custom": [],
        };

        // In-memory model list and standard model (populated from config)
        let configuredModels = [];
        let standardModel = "";

        function renderConfiguredModels() {
            const list = document.getElementById("provider-model-list");
            const agentModelSelect = document.getElementById("agent-model");
            const prevAgent = agentModelSelect ? agentModelSelect.value : "";
            agentModelSelect.innerHTML = "<option value=''>Default (System Default)</option>";

            if (configuredModels.length === 0) {
                list.innerHTML = "<div class='text-sm text-zinc-500 italic p-4 text-center border border-dashed border-[var(--border-color)] rounded-xl'>No models configured. Click Add Model to get started.</div>";
                return;
            }

            list.innerHTML = configuredModels.map(function(m, i) {
                const isStandard = m.model_name === standardModel;
                const borderClass = isStandard ? "border-sofia bg-sofia/5" : "border-[var(--border-color)] bg-[var(--bg-main)]";
                const iconBgClass = isStandard ? "bg-sofia/15 text-sofia" : "bg-[var(--bg-sidebar)] text-zinc-400";
                const iconName = isStandard ? "star" : "cpu";
                const standardBadge = isStandard ? "<span class='px-1.5 py-0.5 rounded text-[9px] font-bold uppercase tracking-widest bg-sofia text-white'>Standard</span>" : "";
                const modelStr = m.model || "";
                const keyBadge = m.api_key ? "\u00b7 \ud83d\udd11" : "\u00b7 no key";
                const standardBtn = !isStandard ? "<button onclick=\"setStandardModel('" + m.model_name + "')\" title=\"Set as Standard\" class=\"px-2.5 py-1.5 rounded-lg text-[10px] font-bold border border-sofia/30 text-sofia hover:bg-sofia/10 transition whitespace-nowrap flex items-center gap-1\"><i data-lucide=\"star\" class=\"w-3 h-3\"></i> Standard</button>" : "";
                return "<div class=\"flex items-center justify-between px-4 py-3 rounded-xl border " + borderClass + " transition-all\">" +
                    "<div class=\"flex items-center gap-3 overflow-hidden\">" +
                        "<div class=\"w-8 h-8 rounded-lg " + iconBgClass + " flex items-center justify-center shrink-0\">" +
                            "<i data-lucide=\"" + iconName + "\" class=\"w-4 h-4\"></i>" +
                        "</div>" +
                        "<div class=\"overflow-hidden\">" +
                            "<div class=\"text-sm font-semibold text-[var(--text-main)] flex items-center gap-2\">" +
                                m.model_name + " " + standardBadge +
                            "</div>" +
                            "<div class=\"text-[10px] text-zinc-500 font-mono truncate\">" + modelStr + " " + keyBadge + "</div>" +
                        "</div>" +
                    "</div>" +
                    "<div class=\"flex gap-1 shrink-0 ml-3\">" +
                        standardBtn +
                        "<button onclick=\"openModelForm(" + i + ")\" class=\"p-1.5 rounded-lg hover:bg-[var(--nav-hover)] text-zinc-500 hover:text-[var(--text-main)] transition\"><i data-lucide=\"edit-3\" class=\"w-3.5 h-3.5\"></i></button>" +
                        "<button onclick=\"removeConfiguredModel(" + i + ")\" class=\"p-1.5 rounded-lg hover:bg-red-500/10 text-zinc-500 hover:text-red-400 transition\"><i data-lucide=\"trash-2\" class=\"w-3.5 h-3.5\"></i></button>" +
                    "</div>" +
                "</div>";
            }).join("");




            configuredModels.forEach(m => {
                if (m.api_key) {
                    const opt = document.createElement("option");
                    opt.value = m.model_name;
                    opt.textContent = m.model_name;
                    agentModelSelect.appendChild(opt);
                }
            });
            if (prevAgent === "" || Array.from(agentModelSelect.options).some(o => o.value === prevAgent)) {
                agentModelSelect.value = prevAgent;
            }

            // Sync hidden cfg-model input
            document.getElementById("cfg-model").value = standardModel;
            refreshIcons();
        }

        function setStandardModel(modelName) {
            standardModel = modelName;
            renderConfiguredModels();
            saveConfig();
        }

        function removeConfiguredModel(index) {
            const m = configuredModels[index];
            if (!confirm("Remove model " + (m ? m.model_name : "") + "?")) return;
            configuredModels.splice(index, 1);
            if (standardModel === (m && m.model_name)) {
                standardModel = configuredModels.length > 0 ? configuredModels[0].model_name : "";
            }
            renderConfiguredModels();
            saveConfig();
        }

        function openModelForm(editIndex) {
            const form = document.getElementById("model-config-form");
            const title = document.getElementById("model-form-title");
            document.getElementById("edit-model-index").value = editIndex !== undefined ? editIndex : -1;

            // Reset form fields
            document.getElementById("form-provider").value = "";
            document.getElementById("form-model-select").innerHTML = "";
            document.getElementById("form-step-model").classList.add("hidden");
            document.getElementById("form-step-config").classList.add("hidden");
            document.getElementById("form-model-custom-wrapper").classList.add("hidden");
            document.getElementById("form-model-alias").value = "";
            document.getElementById("form-model-key").value = "";
            document.getElementById("form-model-base").value = "";
            document.getElementById("form-model-workspace").value = "";
            document.getElementById("form-model-rpm").value = "";
            document.getElementById("form-model-timeout").value = "";
            document.getElementById("form-model-proxy").value = "";
            document.getElementById("form-model-auth").value = "";
            document.getElementById("form-model-tokens-field").value = "";
            document.getElementById("form-model-connect").value = "";

            if (editIndex !== undefined && editIndex >= 0) {
                const m = configuredModels[editIndex];
                title.textContent = "Edit Model";
                // Detect provider from model string prefix
                const prefix = m.model ? m.model.split("/")[0] : "";
                const providerMap = { gemini: "Google Gemini", openai: "OpenAI", anthropic: "Anthropic", deepseek: "DeepSeek", groq: "Groq" };
                const detectedProvider = providerMap[prefix] || "Custom";
                document.getElementById("form-provider").value = detectedProvider;
                onProviderChange();
                // Try to select model in dropdown
                const sel = document.getElementById("form-model-select");
                let found = false;
                for (let i = 0; i < sel.options.length; i++) {
                    if (sel.options[i].value === m.model) { sel.value = m.model; found = true; break; }
                }
                if (!found) {
                    // custom entry
                    if (detectedProvider === "Custom") {
                        document.getElementById("form-model-custom-wrapper").classList.remove("hidden");
                        document.getElementById("form-model-custom").value = m.model || "";
                    }
                }
                document.getElementById("form-step-config").classList.remove("hidden");
                document.getElementById("form-model-alias").value = m.model_name || "";
                document.getElementById("form-model-key").value = m.api_key || "";
                document.getElementById("form-model-base").value = m.api_base || "";
                document.getElementById("form-model-workspace").value = m.workspace || "";
                document.getElementById("form-model-rpm").value = m.rpm || "";
                document.getElementById("form-model-timeout").value = m.request_timeout || "";
                document.getElementById("form-model-proxy").value = m.proxy || "";
                document.getElementById("form-model-auth").value = m.auth_method || "";
                document.getElementById("form-model-tokens-field").value = m.max_tokens_field || "";
                document.getElementById("form-model-connect").value = m.connect_mode || "";
            } else {
                title.textContent = "Add New Model";
            }

            form.classList.remove("hidden");
            form.scrollIntoView({ behavior: "smooth", block: "nearest" });
            refreshIcons();
        }

        function closeModelForm() {
            document.getElementById("model-config-form").classList.add("hidden");
        }

        function onProviderChange() {
            const provider = document.getElementById("form-provider").value;
            const stepModel = document.getElementById("form-step-model");
            const stepConfig = document.getElementById("form-step-config");
            const sel = document.getElementById("form-model-select");
            const customWrapper = document.getElementById("form-model-custom-wrapper");

            if (!provider) {
                stepModel.classList.add("hidden");
                stepConfig.classList.add("hidden");
                return;
            }

            stepModel.classList.remove("hidden");
            stepConfig.classList.add("hidden");

            if (provider === "Custom" || provider === "OpenRouter") {
                sel.classList.add("hidden");
                customWrapper.classList.remove("hidden");
                document.getElementById("form-model-custom").value = provider === "OpenRouter" ? "openrouter/" : "";
                if (provider === "OpenRouter") {
                    document.getElementById("form-model-base").value = "https://openrouter.ai/api/v1";
                }
            } else {
                sel.classList.remove("hidden");
                customWrapper.classList.add("hidden");
                const models = PROVIDER_MODELS[provider] || [];
                sel.innerHTML = "<option value=''>-- Select Model --</option>" +
                    models.map(function(m) { return "<option value=\"" + m.model_id + "\" data-base=\"" + m.api_base + "\">" + m.label + "</option>"; }).join("");

            }
            refreshIcons();
        }

        function onModelChange() {
            const provider = document.getElementById("form-provider").value;
            let modelId = "";
            let apiBase = "";
            if (provider === "Custom" || provider === "OpenRouter") {
                modelId = document.getElementById("form-model-custom").value.trim();
            } else {
                const sel = document.getElementById("form-model-select");
                modelId = sel.value;
                const opt = sel.options[sel.selectedIndex];
                apiBase = opt ? (opt.getAttribute("data-base") || "") : "";
            }

            const stepConfig = document.getElementById("form-step-config");
            if (!modelId) { stepConfig.classList.add("hidden"); return; }
            stepConfig.classList.remove("hidden");

            // Auto-fill API base and alias if empty
            if (!document.getElementById("form-model-base").value) {
                document.getElementById("form-model-base").value = apiBase;
            }
            if (!document.getElementById("form-model-alias").value) {
                const parts = modelId.split("/");
                document.getElementById("form-model-alias").value = parts[parts.length - 1];
            }

            // Try to inherit API key from same provider
            if (!document.getElementById("form-model-key").value) {
                const prefix = modelId.split("/")[0];
                const existing = configuredModels.find(m => m.model && m.model.startsWith(prefix + "/") && m.api_key);
                if (existing) document.getElementById("form-model-key").value = existing.api_key;
            }
            refreshIcons();
        }

        function saveModelForm() {
            const provider = document.getElementById("form-provider").value;
            let modelId = "";
            if (provider === "Custom" || provider === "OpenRouter") {
                modelId = document.getElementById("form-model-custom").value.trim();
            } else {
                modelId = document.getElementById("form-model-select").value;
            }
            const alias = document.getElementById("form-model-alias").value.trim();
            if (!alias || !modelId) {
                alert("Please select a model and provide an alias name.");
                return;
            }
            const entry = {
                model_name: alias,
                model: modelId,
            };
            const apiBase = document.getElementById("form-model-base").value.trim();
            const apiKey = document.getElementById("form-model-key").value.trim();
            const workspace = document.getElementById("form-model-workspace").value.trim();
            const rpm = parseInt(document.getElementById("form-model-rpm").value, 10);
            const timeout = parseInt(document.getElementById("form-model-timeout").value, 10);
            const proxy = document.getElementById("form-model-proxy").value.trim();
            const auth = document.getElementById("form-model-auth").value.trim();
            const tokensField = document.getElementById("form-model-tokens-field").value.trim();
            const connectMode = document.getElementById("form-model-connect").value.trim();

            if (apiBase) entry.api_base = apiBase;
            if (apiKey) entry.api_key = apiKey;
            if (workspace) entry.workspace = workspace;
            if (!isNaN(rpm) && rpm > 0) entry.rpm = rpm;
            if (!isNaN(timeout) && timeout > 0) entry.request_timeout = timeout;
            if (proxy) entry.proxy = proxy;
            if (auth) entry.auth_method = auth;
            if (tokensField) entry.max_tokens_field = tokensField;
            if (connectMode) entry.connect_mode = connectMode;

            const editIndex = parseInt(document.getElementById("edit-model-index").value, 10);
            if (editIndex >= 0) {
                configuredModels[editIndex] = entry;
            } else {
                configuredModels.push(entry);
                standardModel = alias; // Auto-set new model as standard
            }
            closeModelForm();
            renderConfiguredModels();
            saveConfig();
        }

        function getProviderModelsFromForm() {
            return configuredModels;
        }

        function refreshDefaultModelOptions() {
            const agentModelSelect = document.getElementById("agent-model");
            const prevAgent = agentModelSelect ? agentModelSelect.value : "";
            agentModelSelect.innerHTML = "<option value=''>Default (System Default)</option>";
            configuredModels.forEach(m => {
                if (m.api_key) {
                    const opt = document.createElement("option");
                    opt.value = m.model_name;
                    opt.textContent = m.model_name;
                    agentModelSelect.appendChild(opt);
                }
            });
            if (prevAgent === "" || Array.from(agentModelSelect.options).some(o => o.value === prevAgent)) {
                agentModelSelect.value = prevAgent;
            }
            document.getElementById("cfg-model").value = standardModel;
        }




        // Initialize Lucide Icons
        function refreshIcons() {
            lucide.createIcons();
        }

        async function updateSofia() {
            if (!confirm("This will pull the latest code from Git, recompile Sofia, and restart. Continue?")) return;
            try {
                const res = await fetch("/api/update", { method: "POST" });
                if (res.ok) {
                    alert("Update successful! Sofia is restarting. The page will reload shortly.");
                    setTimeout(() => window.location.reload(), 3000);
                } else {
                    const text = await res.text();
                    alert("Update failed: " + text);
                }
            } catch (err) {
                alert("Error calling update: " + err);
            }
        }

        async function restartSofia() {
            if (!confirm("Are you sure you want to restart Sofia?")) return;
            try {
                const res = await fetch("/api/restart", { method: "POST" });
                if (res.ok) {
                    alert("Sofia is restarting. The page will reload shortly.");
                    setTimeout(() => window.location.reload(), 2000);
                } else {
                    alert("Failed to restart");
                }
            } catch (err) {
                alert("Error calling restart: " + err);
            }
        }

        function toggleTheme() {
            const html = document.documentElement;
            if (html.classList.contains('dark')) {
                html.classList.remove('dark');
                localStorage.setItem('theme', 'light');
            } else {
                html.classList.add('dark');
                localStorage.setItem('theme', 'dark');
            }
            updateThemeIcons();
        }

        function updateThemeIcons() {
            const isDark = document.documentElement.classList.contains('dark');
            const btn = document.getElementById('theme-toggle');
            if (btn) {
                btn.innerHTML = isDark ? '<i data-lucide="sun" class="w-4 h-4"></i>' : '<i data-lucide="moon" class="w-4 h-4"></i>';
                refreshIcons();
            }
        }

        async function addSkill() {
            const name = document.getElementById("new-skill-name").value;
            const content = document.getElementById("new-skill-content").value;
            
            if (!name || !content) {
                alert("Name and content are required!");
                return;
            }

            try {
                const res = await fetch("/api/skills/add", {
                    method: "POST",
                    headers: {"Content-Type": "application/json"},
                    body: JSON.stringify({name, content})
                });

                if (res.ok) {
                    alert("Skill installed!");
                    document.getElementById("new-skill-name").value = "";
                    document.getElementById("new-skill-content").value = "";
                    fetchStatus();
                } else {
                    const err = await res.text();
                    alert("Error: " + err);
                }
            } catch (e) {
                alert("Could not connect to the server.");
            }
        }

        function showTab(tabId) {
            currentTab = tabId;
            document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.nav-item').forEach(l => l.classList.remove('active'));
            const settingsHeaderTabs = document.getElementById('settings-header-tabs');
            
            document.getElementById('tab-' + tabId).classList.add('active');
            document.getElementById('nav-' + tabId).classList.add('active');
            
            // Update Title
            const titles = {
                'chat': 'Direct Chat',
                'logs': 'System Logs',
                'agents': 'Agent Management',
                'settings': 'System Settings'
            };
            document.getElementById('view-title').innerText = titles[tabId];

            if (tabId === 'agents') {
                fetchAgents();
                fetchConfig(); // Ensure model dropdown is populated
            }

            if (tabId === 'settings') {
                settingsHeaderTabs.classList.remove('hidden');
                settingsHeaderTabs.classList.add('flex');
                fetchConfig();
                fetchStatus();
                showSettingsSubTab(currentSettingsSubTab);
            } else {
                settingsHeaderTabs.classList.add('hidden');
                settingsHeaderTabs.classList.remove('flex');
            }
            
            refreshIcons();
        }

        function showSettingsSubTab(tabId) {
            currentSettingsSubTab = tabId;
            const tabs = ['prompts', 'heartbeat', 'security', 'models', 'channels', 'tools', 'skills'];
            tabs.forEach(t => {
                const panel = document.getElementById('settings-subtab-' + t);
                const button = document.getElementById('settings-tab-' + t);
                if (!panel || !button) return;
                if (t === tabId) {
                    panel.classList.remove('hidden');
                    button.classList.add('bg-[var(--bg-main)]', 'text-[var(--text-main)]');
                } else {
                    panel.classList.add('hidden');
                    button.classList.remove('bg-[var(--bg-main)]', 'text-[var(--text-main)]');
                }
            });
        }

        async function fetchStatus() {
            try {
                const res = await fetch("/api/status");
                if (!res.ok) return;
                const data = await res.json();
                
                // Update Mini Status in Sidebar
                document.getElementById("stat-version").innerText = data.version || "dev";
                document.getElementById("stat-tools").innerText = data.tools.count;
                document.getElementById("stat-skills").innerText = data.skills.total;

                // Update Live Activity
                const livePanel = document.getElementById("live-activity-panel");
                const sidebar = document.getElementById("agent-monitor-sidebar");
                const monitor = document.getElementById("agent-activity-monitor");

                if (data.agents.active && data.agents.active.id) {
                    activeAgentID = data.agents.active.id;
                    livePanel.classList.remove("hidden");
                    sidebar.classList.remove("hidden");
                    document.getElementById("active-agent-name").innerText = data.agents.active.id;
                    document.getElementById("active-agent-status").innerText = data.agents.active.status;
                } else {
                    activeAgentID = null;
                    livePanel.classList.add("hidden");
                    // User requested sidebar to always be visible
                    // if (monitor.children.length === 0) {
                    //     sidebar.classList.add("hidden");
                    // }
                }

                // If on agents tab, we might want to refresh to show active status
                if (currentTab === 'agents' && activeAgentID) {
                    // We don't want to full refresh every 2s as it's jarring, 
                    // but we can update the borders
                    document.querySelectorAll('[data-agent-id]').forEach(el => {
                        if (el.getAttribute('data-agent-id') === activeAgentID) {
                            el.classList.add('border-sofia', 'bg-sofia/5');
                            el.classList.remove('border-zinc-800');
                        } else {
                            el.classList.remove('border-sofia', 'bg-sofia/5');
                            el.classList.add('border-zinc-800');
                        }
                    });
                }

                // Update System Details in Settings
                const details = document.getElementById("system-details");
                if (details) {
                    details.innerHTML = 
                        "<div class='flex justify-between items-center'><span class='text-zinc-500'>Active Agents:</span> <span class='text-zinc-200 font-medium'>" + data.agents.ids.length + "</span></div>" +
                        "<div class='flex flex-wrap gap-1 mt-1'>" + data.agents.ids.map(id => "<span class='px-2 py-0.5 rounded bg-zinc-800 text-[10px] text-zinc-400 border border-zinc-700/50'>" + id + "</span>").join("") + "</div>" +
                        "<div class='flex justify-between items-center mt-4'><span class='text-zinc-500'>Loaded Tools:</span> <span class='text-zinc-200 font-medium'>" + data.tools.count + "</span></div>" +
                        "<div class='flex justify-between items-center'><span class='text-zinc-500'>Loaded Skills:</span> <span class='text-zinc-200 font-medium'>" + data.skills.total + "</span></div>";
                }

                // Update Skills List
                skillsData = data.skills.list || [];
                renderSkills(skillsData);

                // Update Tools List
                const toolsList = document.getElementById("tools-list");
                if (data.tools.list) {
                    toolsList.innerHTML = data.tools.list.map(t => 
                        "<div class='p-3 bg-zinc-900/30 rounded-xl border border-zinc-800/50 hover:border-zinc-700 transition-colors'>" +
                        "<div class='font-bold text-xs text-sofia flex items-center gap-2'><i data-lucide='box' class='w-3 h-3'></i>" + t.name + "</div>" +
                        "<div class='text-[10px] text-zinc-500 mt-1 leading-relaxed'>" + (t.description || "No description.") + "</div>" +
                        "</div>"
                    ).join("");
                    refreshIcons();
                }
            } catch (e) {
                console.error("Status fetch failed", e);
            }
        }

        function renderSkills(list) {
            const skillsList = document.getElementById("skills-list");
            if (!list || list.length === 0) {
                skillsList.innerHTML = "<p class='text-zinc-500 italic text-sm'>No skills found.</p>";
                return;
            }

            const grouped = list.reduce((acc, s) => {
                acc[s.source] = acc[s.source] || [];
                acc[s.source].push(s);
                return acc;
            }, {});

            let html = "";
            for (const source in grouped) {
                html += "<div class='mb-6'><h3 class='text-[10px] font-bold uppercase tracking-widest text-zinc-600 mb-3 ml-1'>" + source + "</h3>";
                grouped[source].forEach(s => {
                    html += "<div class='group p-4 bg-zinc-900/40 rounded-2xl border border-zinc-800/50 hover:border-sofia/30 transition-all cursor-default'>" +
                            "<div class='flex justify-between items-start'>" +
                            "<div class='font-bold text-sm text-zinc-200 group-hover:text-sofia transition-colors'>" + s.name + "</div>" +
                            "<span class='px-1.5 py-0.5 rounded bg-zinc-800 text-[9px] text-zinc-500 border border-zinc-700'>SKILL</span>" +
                            "</div>" +
                            "<div class='text-xs text-zinc-500 mt-1 line-clamp-2 leading-relaxed'>" + (s.description || "No description available.") + "</div>" +
                            "<div class='text-[9px] font-mono text-zinc-700 mt-3 truncate opacity-50'>" + s.path + "</div>" +
                            "</div>";
                });
                html += "</div>";
            }
            skillsList.innerHTML = html;
            populateSkillPicker();
        }

        function filterSkills() {
            const query = document.getElementById("skill-search").value.toLowerCase();
            const filtered = skillsData.filter(s => 
                s.name.toLowerCase().includes(query) || 
                (s.description && s.description.toLowerCase().includes(query))
            );
            renderSkills(filtered);
        }

        async function fetchPurposeTemplates() {
            try {
                const res = await fetch("/api/agent-templates");
                if (!res.ok) return;
                purposeTemplates = await res.json();
                purposeTemplateMap = {};

                const select = document.getElementById("agent-template");
                select.innerHTML = "<option value=''>No template</option>";

                purposeTemplates.forEach(t => {
                    purposeTemplateMap[t.name] = t;
                    const opt = document.createElement("option");
                    opt.value = t.name;
                    opt.textContent = t.name;
                    select.appendChild(opt);
                });
            } catch (e) {
                console.error("Purpose templates fetch failed", e);
            }
        }

        async function onTemplateSelected() {
            const selected = document.getElementById("agent-template").value;
            const warning = document.getElementById("agent-template-missing-skills");
            const hint = document.getElementById("agent-template-hint");
            const textarea = document.getElementById("agent-instructions");

            warning.classList.add("hidden");
            warning.innerText = "";
            hint.classList.add("hidden");

            if (!selected) {
                return;
            }

            const idInput = document.getElementById("agent-id");
            const nameInput = document.getElementById("agent-name");
            if (!idInput.value) idInput.value = selected;
            if (!nameInput.value) nameInput.value = selected;

            try {
                const res = await fetch("/api/agent-templates/" + encodeURIComponent(selected));
                if (!res.ok) return;
                const data = await res.json();
                textarea.value = data.instructions || "";
                hint.classList.remove("hidden");

                const availableSkills = new Set((skillsData || []).map(s => s.name));
                const missing = (data.skills || []).filter(s => !availableSkills.has(s));
                if (missing.length > 0) {
                    warning.innerText = "Missing skills: " + missing.join(", ");
                    warning.classList.remove("hidden");
                }
            } catch (e) {
                console.error("Template load failed", e);
            }
        }

        async function fetchAgents() {
            try {
                const res = await fetch("/api/agents");
                if (!res.ok) return;
                const agents = await res.json();
                const list = document.getElementById("agents-list");
                document.getElementById("agent-count").innerText = (agents ? agents.length : 0) + " agents";
                
                if (!agents || agents.length === 0) {
                    list.innerHTML = "<div class='col-span-full py-12 text-center border-2 border-dashed border-zinc-900 rounded-3xl text-zinc-600 italic'>No sub-agents configured.</div>";
                    return;
                }

				list.innerHTML = agents.map(function(a) {
					const isActive = a.id === activeAgentID;
					const modelName = a.model && a.model.primary ? a.model.primary : (a.model || "");
					const templateName = a.template || "";
					const templateSkillsMode = a.template_skills_mode || "fallback";
					const agentSkillsJson = JSON.stringify(a.skills || []).replace(/"/g, "&quot;");
					const skillCount = a.skills && a.skills.length ? a.skills.length : 0;
					return '<div data-agent-id="' + a.id + '" data-agent-name="' + (a.name || '').replace(/'/g, "&#39;") + '" data-agent-model="' + modelName.replace(/'/g, "&#39;") + '" data-agent-template="' + templateName.replace(/'/g, "&#39;") + '" data-agent-tsm="' + templateSkillsMode + '" data-agent-skills="' + agentSkillsJson + '" class="group p-5 bg-zinc-900/40 border ' + (isActive ? 'border-sofia bg-sofia/5' : 'border-zinc-800') + ' rounded-2xl hover:border-sofia/30 transition-all shadow-lg">' +
                        '<div class="flex justify-between items-start mb-4">' +
                            '<div class="w-10 h-10 rounded-xl bg-zinc-800 flex items-center justify-center text-sofia group-hover:bg-sofia/10 transition-colors">' +
                                '<i data-lucide="bot" class="w-5 h-5"></i>' +
                            '</div>' +
                            '<div class="flex gap-1">' +
							'<button onclick="editAgentFromCard(this)" data-card-id="' + a.id + '" class="p-2 text-zinc-500 hover:text-white hover:bg-zinc-800 rounded-lg transition"><i data-lucide="edit-3" class="w-3.5 h-3.5"></i></button>' +
                                '<button onclick="deleteAgent(\'' + a.id + '\')" class="p-2 text-zinc-500 hover:text-red-400 hover:bg-red-900/20 rounded-lg transition"><i data-lucide="trash-2" class="w-3.5 h-3.5"></i></button>' +
                            '</div>' +
                        '</div>' +
                        '<div>' +
                            '<div class="font-bold text-zinc-200">' + (a.name || a.id) + '</div>' +
                            '<div class="text-[10px] font-mono text-zinc-600 uppercase tracking-tighter mt-0.5">' + a.id + '</div>' +
                            (templateName ? '<div class="mt-2 text-[10px] text-sofia/80 font-medium">Purpose: ' + templateName + '</div>' : '') +
                            (skillCount > 0 ? '<div class="mt-1 text-[10px] text-zinc-500 flex items-center gap-1"><i data-lucide=\"zap\" class=\"w-2.5 h-2.5\"></i> ' + skillCount + ' custom skill' + (skillCount !== 1 ? 's' : '') + '</div>' : '') +
                            '<div class="mt-4 pt-4 border-t border-zinc-800/50 flex items-center justify-between">' +
                                '<div class="text-[10px] text-zinc-500 font-medium flex items-center gap-1.5"><i data-lucide="cpu" class="w-3 h-3"></i> ' + (modelName || 'Default') + '</div>' +
                                '<span class="w-1.5 h-1.5 rounded-full ' + (isActive ? 'bg-sofia animate-pulse' : 'bg-green-500/50') + '"></span>' +
                            '</div>' +
                        '</div>' +
                    '</div>';
				}).join("");
                refreshIcons();
            } catch (e) {
                console.error("Agents fetch failed", e);
            }
        }

		async function saveAgent() {
			const id = document.getElementById("agent-id").value;
			const name = document.getElementById("agent-name").value;
			const modelStr = document.getElementById("agent-model").value;
			const template = document.getElementById("agent-template").value;
            
            if (!id) {
                alert("ID is required!");
                return;
            }

            const agent = { id, name };
            if (modelStr) agent.model = modelStr;
            if (agentCustomSkills.length > 0) agent.skills = agentCustomSkills.slice();
            const instructions = document.getElementById("agent-instructions").value.trim();
            if (instructions) agent.instructions = instructions;
			if (template) {
				agent.template = template;
			}

            try {
                const res = await fetch("/api/agents", {
                    method: "POST",
                    headers: {"Content-Type": "application/json"},
                    body: JSON.stringify(agent)
                });

                if (res.ok) {
                    resetAgentForm();
                    fetchAgents();
                } else {
                    alert("Could not save agent.");
                }
            } catch (e) {
                alert("Network error while saving.");
            }
        }

        async function deleteAgent(id) {
            if (!confirm("Are you sure you want to delete agent " + id + "?")) return;
            
            try {
                const res = await fetch("/api/agents?id=" + id, { method: "DELETE" });
                if (res.ok) fetchAgents();
                else alert("Could not delete agent.");
            } catch (e) {
                alert("Network error while deleting.");
            }
        }

		function editAgentFromCard(btn) {
			const cardId = btn.getAttribute("data-card-id");
			const el = document.querySelector('[data-agent-id="' + cardId + '"]');
			editAgentFromEl(el);
		}

		function editAgentFromEl(el) {
			if (!el) return;
			const id = el.getAttribute("data-agent-id") || "";
			const name = el.getAttribute("data-agent-name") || "";
			const model = el.getAttribute("data-agent-model") || "";
			const template = el.getAttribute("data-agent-template") || "";
			const tsm = el.getAttribute("data-agent-tsm") || "fallback";
			const instr = el.getAttribute("data-agent-instructions") || "";
			let skills = [];
			try { skills = JSON.parse(el.getAttribute("data-agent-skills") || "[]"); } catch(e) {}
			editAgent(id, name, model, template, tsm, skills, instr);
		}

		function editAgent(id, name, model, template, templateSkillsMode, skills, instructions) {
			document.getElementById("agent-id").value = id;
			document.getElementById("agent-id").disabled = true;
			document.getElementById("agent-id").classList.add("opacity-50");
			document.getElementById("agent-name").value = name;
			
            const modelSelect = document.getElementById("agent-model");
            // Check if model exists in options
            let exists = false;
            for (let i = 0; i < modelSelect.options.length; i++) {
                if (modelSelect.options[i].value === model) {
                    exists = true;
                    break;
                }
            }
            if (!exists && model) {
                const opt = document.createElement("option");
                opt.value = model;
                opt.textContent = model + " (Not in list)";
                modelSelect.appendChild(opt);
            }
            modelSelect.value = model || "";

			document.getElementById("agent-template").value = template || "";
            agentCustomSkills = (Array.isArray(skills) ? skills : []);
            renderSkillTags();
            document.getElementById("agent-instructions").value = instructions || "";
			onTemplateSelected();
			document.getElementById("agent-form-title").innerText = "Edit Agent";
			refreshIcons();
		}

        // ── Custom Skills Tag UI ──────────────────────────────────────────
        let agentCustomSkills = [];

        function renderSkillTags() {
            const container = document.getElementById("agent-custom-skills-tags");
            if (!container) return;
            if (agentCustomSkills.length === 0) {
                container.innerHTML = "<span class='text-[10px] text-zinc-600 italic'>No custom skills added</span>";
                return;
            }
            container.innerHTML = agentCustomSkills.map(function(s) {
                return "<span class='inline-flex items-center gap-1 px-2 py-1 rounded-lg bg-sofia/10 border border-sofia/20 text-[10px] font-medium text-sofia'>" + s +
                    "<button type='button' onclick='removeCustomSkill(\"" + s + "\")' class='ml-0.5 text-sofia/60 hover:text-red-400 transition leading-none'>&times;</button></span>";
            }).join("");
        }

        function addCustomSkill() {
            const picker = document.getElementById("agent-custom-skills-picker");
            const skill = picker.value;
            if (!skill) return;
            if (agentCustomSkills.indexOf(skill) === -1) {
                agentCustomSkills.push(skill);
                renderSkillTags();
            }
            picker.value = "";
        }

        function removeCustomSkill(skill) {
            agentCustomSkills = agentCustomSkills.filter(function(s) { return s !== skill; });
            renderSkillTags();
        }

        function populateSkillPicker() {
            const picker = document.getElementById("agent-custom-skills-picker");
            if (!picker) return;
            const current = picker.value;
            picker.innerHTML = "<option value=''>+ Add a skill...</option>";
            (skillsData || []).forEach(function(s) {
                const opt = document.createElement("option");
                opt.value = s.name;
                opt.textContent = s.name;
                picker.appendChild(opt);
            });
            picker.value = current;
        }
        // ─────────────────────────────────────────────────────────────────

        function resetAgentForm() {
            document.getElementById("agent-id").value = "";
            document.getElementById("agent-id").disabled = false;
            document.getElementById("agent-id").classList.remove("opacity-50");
			document.getElementById("agent-name").value = "";
			document.getElementById("agent-model").value = "";
			document.getElementById("agent-template").value = "";

            agentCustomSkills = [];
            renderSkillTags();
            document.getElementById("agent-instructions").value = "";
			onTemplateSelected();
			document.getElementById("agent-form-title").innerText = "New Agent";
			refreshIcons();
		}




        function formatAllowFrom(values) {
            if (!Array.isArray(values) || values.length === 0) return "";
            return values.join(", ");
        }

        function parseAllowFromInput(raw) {
            return raw
                .split(/[\n,]/)
                .map(v => v.trim())
                .filter(v => v.length > 0);
        }

        function validateChannelSettings() {
            const telegramEnabled = document.getElementById("cfg-telegram").checked;
            const telegramToken = document.getElementById("cfg-telegram-token").value.trim();
            const discordEnabled = document.getElementById("cfg-discord").checked;
            const discordToken = document.getElementById("cfg-discord-token").value.trim();

            const telegramWarn = document.getElementById("cfg-telegram-warning");
            const discordWarn = document.getElementById("cfg-discord-warning");
            const globalWarn = document.getElementById("cfg-channels-warning");

            const telegramInvalid = telegramEnabled && !telegramToken;
            const discordInvalid = discordEnabled && !discordToken;

            telegramWarn.classList.toggle("hidden", !telegramInvalid);
            discordWarn.classList.toggle("hidden", !discordInvalid);
            globalWarn.classList.toggle("hidden", !(telegramInvalid || discordInvalid));

            return !(telegramInvalid || discordInvalid);
        }

        async function fetchWorkspaceDocs() {
            try {
                const res = await fetch("/api/workspace-docs");
                if (!res.ok) return;
                const docs = await res.json();
                document.getElementById("cfg-identity-md").value = docs.identity || "";
                document.getElementById("cfg-soul-md").value = docs.soul || "";
            } catch (e) {
                console.error("Workspace docs fetch failed", e);
            }
        }

        async function saveWorkspaceDocs() {
            try {
                const payload = {
                    identity: document.getElementById("cfg-identity-md").value,
                    soul: document.getElementById("cfg-soul-md").value,
                };
                const res = await fetch("/api/workspace-docs", {
                    method: "POST",
                    headers: {"Content-Type": "application/json"},
                    body: JSON.stringify(payload),
                });
                if (res.ok) {
                    alert("Prompt files saved!");
                } else {
                    const msg = await res.text();
                    alert("Could not save prompt files: " + msg);
                }
            } catch (e) {
                alert("Network error while saving prompt files.");
            }
        }

        async function saveToolsConfig() {
            try {
                let cfg = currentConfig;
                if (!cfg) {
                    const res = await fetch("/api/config");
                    cfg = await res.json();
                }

                if (!cfg.tools) cfg.tools = {};
                if (!cfg.tools.google) cfg.tools.google = {};

                cfg.tools.google.enabled = document.getElementById("cfg-google-enabled").checked;
                cfg.tools.google.binary_path = document.getElementById("cfg-google-binary").value.trim() || "gog";
                cfg.tools.google.timeout_seconds = parseInt(document.getElementById("cfg-google-timeout").value) || 90;
                const cmdsStr = document.getElementById("cfg-google-commands").value.trim();
                cfg.tools.google.allowed_commands = cmdsStr ? cmdsStr.split(",").map(s => s.trim()).filter(Boolean) : ["gmail", "drive", "calendar"];

                const saveRes = await fetch("/api/config", {
                    method: "POST",
                    headers: {"Content-Type": "application/json"},
                    body: JSON.stringify(cfg)
                });

                if (saveRes.ok) {
                    currentConfig = cfg;
                    alert("Google tool settings saved! Restart Sofia to apply changes.");
                } else {
                    alert("Could not save settings.");
                }
            } catch (e) {
                alert("An error occurred while saving tool settings.");
            }
        }

        async function fetchConfig() {
            try {
                const res = await fetch("/api/config");
                if (!res.ok) return;
                const cfg = await res.json();
                currentConfig = cfg;

                document.getElementById("cfg-telegram").checked = cfg.channels.telegram.enabled;
                document.getElementById("cfg-discord").checked = cfg.channels.discord.enabled;
                document.getElementById("cfg-telegram-token").value = cfg.channels.telegram.token || "";
                document.getElementById("cfg-telegram-proxy").value = cfg.channels.telegram.proxy || "";
                document.getElementById("cfg-telegram-allow-from").value = formatAllowFrom(cfg.channels.telegram.allow_from);
                document.getElementById("cfg-discord-token").value = cfg.channels.discord.token || "";
                document.getElementById("cfg-discord-allow-from").value = formatAllowFrom(cfg.channels.discord.allow_from);
                document.getElementById("cfg-discord-mention-only").checked = !!cfg.channels.discord.mention_only;
                document.getElementById("cfg-restrict-workspace").checked = !!cfg.agents.defaults.restrict_to_workspace;
                
                // Heartbeat mapping
                if (cfg.heartbeat) {
                    document.getElementById("cfg-heartbeat-enabled").checked = !!cfg.heartbeat.enabled;
                    document.getElementById("cfg-heartbeat-interval").value = cfg.heartbeat.interval || 30;
                    document.getElementById("cfg-heartbeat-hours").value = cfg.heartbeat.active_hours || "";
                    renderHeartbeatDays(cfg.heartbeat.active_days || []);
                } else {
                    renderHeartbeatDays([]);
                }
                
                validateChannelSettings();


                configuredModels = cfg.model_list ? cfg.model_list.slice() : [];
                standardModel = cfg.agents.defaults.model_name || cfg.agents.defaults.model || "";
                if (!standardModel && configuredModels.length > 0) {
                    standardModel = configuredModels[0].model_name;
                }
                renderConfiguredModels();

                // Populate Google tool settings
                if (cfg.tools && cfg.tools.google) {
                    document.getElementById("cfg-google-enabled").checked = !!cfg.tools.google.enabled;
                    document.getElementById("cfg-google-binary").value = cfg.tools.google.binary_path || "gog";
                    document.getElementById("cfg-google-timeout").value = cfg.tools.google.timeout_seconds || 90;
                    document.getElementById("cfg-google-commands").value = (cfg.tools.google.allowed_commands || []).join(", ");
                }

                fetchWorkspaceDocs();
            } catch (e) {
                console.error("Config fetch failed", e);
            }
        }

        let agentActivity = {};

        function clearLogs() {
            const logView = document.getElementById("log-view");
            logView.innerHTML = "<div class='text-zinc-600 italic'>Logs cleared.</div>";
        }

        function setupLogStream() {
            const logView = document.getElementById("log-view");
            const eventSource = new EventSource("/api/logs");

            eventSource.onmessage = function(event) {
                try {
                    const entry = JSON.parse(event.data);
                    
                    // 1. Update Global Log View
                    const div = document.createElement("div");
                    div.className = "mb-1 border-l-2 border-zinc-800 pl-2 py-0.5 animate-fade-in";
                    
                    const timeStr = entry.timestamp ? entry.timestamp.split('T')[1].split('Z')[0] : "";
                    const compStr = entry.component ? "[" + entry.component + "] " : "";
                    div.textContent = timeStr + " " + compStr + entry.message;
                    
                    if (entry.level === "ERROR" || entry.level === "FATAL") div.classList.add("text-red-500");
                    if (entry.level === "WARN") div.classList.add("text-yellow-500");

                    logView.appendChild(div);
                    while (logView.childNodes.length > 200) logView.removeChild(logView.firstChild);
                    logView.scrollTop = logView.scrollHeight;

                    // 2. Handle Agent Specific Logs
                    if (entry.component && entry.component.startsWith("agent:")) {
                        handleAgentLog(entry);
                    }
                } catch (e) {
                    // Fallback for non-json logs if any
                    const div = document.createElement("div");
                    div.className = "mb-1 border-l-2 border-zinc-800 pl-2 py-0.5 text-zinc-600 italic";
                    div.textContent = event.data;
                    logView.appendChild(div);
                }
            };

            eventSource.onerror = function() {
                eventSource.close();
                setTimeout(setupLogStream, 5000);
            };
        }

        function handleAgentLog(entry) {
            const agentId = entry.component.replace("agent:", "");
            const monitor = document.getElementById("agent-activity-monitor");
            const sidebar = document.getElementById("agent-monitor-sidebar");
            
            if (!agentActivity[agentId]) {
                agentActivity[agentId] = { lastSeen: Date.now(), timer: null };
                sidebar.classList.remove("hidden");
                
                const box = document.createElement("div");
                box.id = "agent-box-" + agentId;
                box.className = "agent-log-box w-full bg-zinc-900/80 border border-sofia/20 rounded-xl overflow-hidden flex flex-col shadow-lg transition-all duration-300";
                box.innerHTML =
                    '<div class="px-3 py-2 bg-sofia/10 border-b border-sofia/10 flex items-center justify-between">' +
                    '  <div class="flex items-center gap-2">' +
                    '    <div id="dot-' + agentId + '" class="w-2 h-2 rounded-full bg-sofia animate-pulse shadow-[0_0_6px_rgba(255,77,77,0.6)]"></div>' +
                    '    <span class="text-[10px] font-bold uppercase tracking-widest text-white">' + agentId + '</span>' +
                    '  </div>' +
                    '  <span id="status-' + agentId + '" class="text-[8px] text-sofia/80 font-mono animate-pulse">WORKING</span>' +
                    '</div>' +
                    '<div id="logs-' + agentId + '" class="p-2 h-48 overflow-y-auto bg-black/40 space-y-0.5 font-mono text-[10px] leading-relaxed"></div>' +
                    '<div id="typing-' + agentId + '" class="px-3 py-1.5 border-t border-zinc-800/50 flex items-center gap-1.5">' +
                    '  <span class="w-1 h-1 rounded-full bg-sofia/60 animate-bounce" style="animation-delay:0ms"></span>' +
                    '  <span class="w-1 h-1 rounded-full bg-sofia/60 animate-bounce" style="animation-delay:150ms"></span>' +
                    '  <span class="w-1 h-1 rounded-full bg-sofia/60 animate-bounce" style="animation-delay:300ms"></span>' +
                    '  <span class="text-[9px] text-zinc-600 ml-1">processing...</span>' +
                    '</div>';
                monitor.appendChild(box);
                refreshIcons();
            }

            // Append log line with timestamp + color coding
            const logsDiv = document.getElementById("logs-" + agentId);
            const line = document.createElement("div");
            const timeStr = entry.timestamp ? entry.timestamp.split("T")[1].replace("Z","").substring(0,8) : "";
            const msg = entry.message || "";

            // Color-code by log type
            let textColor = "text-zinc-400";
            let prefix = "";
            if (msg.startsWith("Tool call:")) {
                textColor = "text-amber-400";
                prefix = "⚡ ";
            } else if (msg.startsWith("Response:") || msg.startsWith("LLM response")) {
                textColor = "text-green-400";
                prefix = "✓ ";
            } else if (msg.startsWith("LLM requested")) {
                textColor = "text-sky-400";
                prefix = "→ ";
            } else if (entry.level === "ERROR" || entry.level === "FATAL") {
                textColor = "text-red-400";
                prefix = "✗ ";
            } else if (entry.level === "WARN") {
                textColor = "text-yellow-400";
                prefix = "⚠ ";
            }

            line.className = "flex gap-1.5 py-0.5 border-b border-zinc-800/30 " + textColor;
            line.innerHTML =
                '<span class="text-zinc-600 shrink-0">' + timeStr + '</span>' +
                '<span class="truncate">' + prefix + escapeHtml(msg) + '</span>';
            logsDiv.appendChild(line);

            // Auto-scroll (but only if already near bottom)
            if (logsDiv.scrollHeight - logsDiv.scrollTop < logsDiv.clientHeight + 60) {
                logsDiv.scrollTop = logsDiv.scrollHeight;
            }

            // Reset done timer
            if (agentActivity[agentId].timer) clearTimeout(agentActivity[agentId].timer);
            agentActivity[agentId].lastSeen = Date.now();
            agentActivity[agentId].timer = setTimeout(() => {
                completeAgentTask(agentId);
            }, 8000);
        }

        function completeAgentTask(agentId) {
            const statusLabel = document.getElementById("status-" + agentId);
            const box = document.getElementById("agent-box-" + agentId);
            const dot = document.getElementById("dot-" + agentId);
            const typing = document.getElementById("typing-" + agentId);
            
            if (statusLabel) {
                statusLabel.innerText = "DONE";
                statusLabel.className = "text-[8px] text-green-400 font-mono";
            }
            if (dot) {
                dot.classList.remove("bg-sofia", "animate-pulse");
                dot.classList.add("bg-green-500");
                dot.style.boxShadow = "0 0 6px rgba(34,197,94,0.5)";
            }
            if (typing) typing.remove();
            if (box) {
                box.classList.add("border-green-500/20");
                box.classList.remove("border-sofia/20");
            }

            // Fade and remove after delay
            setTimeout(() => {
                if (!box) return;
                box.style.transition = "opacity 0.5s, transform 0.5s";
                box.style.opacity = "0";
                box.style.transform = "translateY(-8px)";
                setTimeout(() => {
                    box.remove();
                    delete agentActivity[agentId];
                }, 500);
            }, 15000);
        }

        function escapeHtml(text) {
            return (text || "")
                .replace(/&/g, "&amp;")
                .replace(/</g, "&lt;")
                .replace(/>/g, "&gt;")
                .replace(/\"/g, "&quot;")
                .replace(/'/g, "&#39;");
        }

        function formatAssistantMessage(text) {
            let safe = escapeHtml(text);

            // Code fences
            safe = safe.replace(new RegExp("\\x60\\x60\\x60([\\w-]+)?\\n([\\s\\S]*?)\\x60\\x60\\x60", "g"), (_, lang, code) => {
                const language = lang ? "<div class='text-[10px] text-zinc-400 mb-1 uppercase tracking-wider'>" + lang + "</div>" : "";
                return "<div class='mt-3 mb-2 p-3 rounded-xl bg-black/20 border border-[var(--border-color)] overflow-x-auto'>" +
                    language +
                    "<pre class='text-xs text-zinc-200 whitespace-pre'><code>" + code + "</code></pre>" +
                "</div>";
            });

            // Basic emphasis
            safe = safe.replace(/\*\*(.*?)\*\*/g, "<strong class='text-[var(--text-main)]'>$1</strong>");
            safe = safe.replace(new RegExp("\\x60([^\\x60]+)\\x60", "g"), "<code class='px-1 py-0.5 rounded bg-black/20 border border-[var(--border-color)] text-xs'>$1</code>");

            // Preserve line breaks
            safe = safe.replace(/\n/g, "<br>");
            return safe;
        }

        async function sendChat() {
            const input = document.getElementById("chat-input");
            const msg = input.value;
            if (!msg) return;

            const history = document.getElementById("chat-history");
            const indicator = document.getElementById("typing-indicator");
            
            // User message
            history.innerHTML += 
                "<div class='flex flex-col items-end gap-1 animate-slide-up'>" +
                    "<div class='chat-bubble-user px-4 py-3 rounded-2xl text-sm max-w-[85%] text-white'>" + msg + "</div>" +
                    "<div class='text-[9px] text-zinc-600 mr-1 font-medium'>MAGNUS</div>" +
                "</div>";
            
            input.value = "";
            indicator.classList.remove("hidden");
            history.scrollTop = history.scrollHeight;

            try {
                const res = await fetch("/api/chat", {
                    method: "POST",
                    headers: {"Content-Type": "application/json"},
                    body: JSON.stringify({message: msg})
                });
                
                const contentType = res.headers.get("content-type");
                let responseData;
                
                if (contentType && contentType.includes("application/json")) {
                    responseData = await res.json();
                } else {
                    const text = await res.text();
                    responseData = { error: text || "An unexpected error occurred (non-JSON response)" };
                }

                if (!res.ok) {
                    throw new Error(responseData.error || "Unknown error");
                }
                
                indicator.classList.add("hidden");
                
                // Sofia message
                history.innerHTML += 
                    "<div class='flex gap-4 animate-slide-up'>" +
                        "<div class='w-8 h-8 rounded-lg bg-sofia/10 border border-sofia/20 flex items-center justify-center shrink-0'>" +
                            "<img src='/assets/sofiamantis.png' class='w-5 h-5 opacity-80'>" +
                        "</div>" +
                        "<div>" +
                            "<div class='chat-bubble-sofia px-4 py-3 rounded-2xl text-sm leading-relaxed max-w-[85%] text-zinc-300 whitespace-pre-wrap break-words'>" + formatAssistantMessage(responseData.response) + "</div>" +
                            "<div class='text-[9px] text-zinc-600 ml-1 mt-1 font-bold uppercase tracking-widest'>Sofia System</div>" +
                        "</div>" +
                    "</div>";
            } catch (err) {
                indicator.classList.add("hidden");
                history.innerHTML += 
                    "<div class='flex flex-col items-center py-4'>" +
                        "<div class='px-4 py-2 rounded-xl bg-red-900/10 border border-red-900/30 text-[11px] text-red-400 flex items-center gap-2'>" +
                            "<i data-lucide='alert-circle' class='w-3.5 h-3.5'></i>" +
                            "Critical error: " + err.message +
                        "</div>" +
                    "</div>";
                refreshIcons();
            }
            history.scrollTop = history.scrollHeight;
        }

        async function saveConfig() {
            try {
                let cfg = currentConfig;
                if (!cfg) {
                    const res = await fetch("/api/config");
                    cfg = await res.json();
                }

                cfg.model_list = getProviderModelsFromForm();
                cfg.agents.defaults.model_name = document.getElementById("cfg-model").value;
                cfg.agents.defaults.model = "";
                cfg.agents.defaults.provider = ""; // Clear legacy provider field
                
                cfg.channels.telegram.enabled = document.getElementById("cfg-telegram").checked;
                cfg.channels.discord.enabled = document.getElementById("cfg-discord").checked;
                cfg.channels.telegram.token = document.getElementById("cfg-telegram-token").value.trim();
                cfg.channels.telegram.proxy = document.getElementById("cfg-telegram-proxy").value.trim();
                cfg.channels.telegram.allow_from = parseAllowFromInput(document.getElementById("cfg-telegram-allow-from").value);
                cfg.channels.discord.token = document.getElementById("cfg-discord-token").value.trim();
                cfg.channels.discord.allow_from = parseAllowFromInput(document.getElementById("cfg-discord-allow-from").value);
                cfg.channels.discord.mention_only = document.getElementById("cfg-discord-mention-only").checked;
                cfg.agents.defaults.restrict_to_workspace = document.getElementById("cfg-restrict-workspace").checked;
                
                // Save Heartbeat settings if block exists
                if (document.getElementById("cfg-heartbeat-enabled")) {
                    if (!cfg.heartbeat) cfg.heartbeat = {};
                    cfg.heartbeat.enabled = document.getElementById("cfg-heartbeat-enabled").checked;
                    cfg.heartbeat.interval = parseInt(document.getElementById("cfg-heartbeat-interval").value) || 30;
                    cfg.heartbeat.active_hours = document.getElementById("cfg-heartbeat-hours").value.trim();
                    cfg.heartbeat.active_days = getCheckedHeartbeatDays();
                }

                validateChannelSettings();

                const saveRes = await fetch("/api/config", {
                    method: "POST",
                    headers: {"Content-Type": "application/json"},
                    body: JSON.stringify(cfg)
                });
                
                if (saveRes.ok) {
                    currentConfig = cfg;
                    alert("Settings saved!");
                } else {
                    alert("Could not save settings.");
                }
            } catch (e) {
                alert("An error occurred while communicating with the server.");
            }
        }

        // Initial Load
        fetchStatus();
        fetchConfig();
        fetchPurposeTemplates();
        onTemplateSelected();
        showSettingsSubTab('prompts');
        setupLogStream();
        updateThemeIcons();
        refreshIcons();

        [
            "cfg-telegram",
            "cfg-telegram-token",
            "cfg-discord",
            "cfg-discord-token",
        ].forEach(id => {
            const el = document.getElementById(id);
            if (el) {
                el.addEventListener('blur', saveConfig);
            }
        });

        // Heartbeat Day Checkbox Handlers
        function renderHeartbeatDays(activeDays) {
            const days = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"];
            const container = document.getElementById("cfg-heartbeat-days-container");
            if (!container) return;

            container.innerHTML = "";
            days.forEach(day => {
                const isChecked = activeDays.includes(day);
                container.innerHTML += 
                    "<label class=\"flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-[var(--border-color)] bg-[var(--bg-main)] cursor-pointer hover:border-sofia/50 transition-colors\">" +
                        "<input type=\"checkbox\" value=\"" + day + "\" class=\"hb-day-checkbox w-3.5 h-3.5 text-sofia bg-zinc-800 border-zinc-600 rounded focus:ring-sofia\" " + (isChecked ? "checked" : "") + " onchange=\"saveConfig()\">" +
                        "<span class=\"text-[11px] text-[var(--text-main)]\">" + day + "</span>" +
                    "</label>";
            });
        }

        function getCheckedHeartbeatDays() {
            const checkboxes = document.querySelectorAll('.hb-day-checkbox:checked');
            return Array.from(checkboxes).map(box => box.value);
        }

        [
            "cfg-telegram",
            "cfg-telegram-token",
            "cfg-discord",
            "cfg-discord-token",
        ].forEach(id => {
            const el = document.getElementById(id);
            if (!el) return;
            el.addEventListener("change", validateChannelSettings);
            el.addEventListener("input", validateChannelSettings);
        });
        
        // Auto-refresh status
        setInterval(fetchStatus, 2000);

        document.getElementById("chat-input").addEventListener("keypress", (e) => {
            if (e.key === "Enter") sendChat();
        });
    </script>
</body>
</html>
`

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
