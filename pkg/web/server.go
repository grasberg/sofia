package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/skills"
)

type Server struct {
	cfg            *config.Config
	agentLoop      *agent.AgentLoop
	server         *http.Server
	mu             sync.RWMutex
	skillInstaller *skills.SkillInstaller
}

func NewServer(cfg *config.Config, agentLoop *agent.AgentLoop) *Server {
	s := &Server{
		cfg:            cfg,
		agentLoop:      agentLoop,
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
                <p class="text-[10px] uppercase tracking-widest text-zinc-500 font-bold">Alpha v2.5</p>
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
            <a href="#" onclick="showTab('skills')" id="nav-skills" class="nav-item">
                <i data-lucide="zap" class="w-5 h-5"></i>
                <span>Skills & Tools</span>
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
                    <button id="settings-tab-models" onclick="showSettingsSubTab('models')" class="px-3 py-1.5 rounded-lg text-xs border border-[var(--border-color)] bg-[var(--bg-main)]">Models</button>
                    <button id="settings-tab-prompts" onclick="showSettingsSubTab('prompts')" class="px-3 py-1.5 rounded-lg text-xs border border-[var(--border-color)] bg-transparent">SOUL.md & IDENTITY.md</button>
                    <button id="settings-tab-channels" onclick="showSettingsSubTab('channels')" class="px-3 py-1.5 rounded-lg text-xs border border-[var(--border-color)] bg-transparent">Channels</button>
                </div>
            </div>
            <div class="flex items-center gap-4">
                <div id="status-badge" class="flex items-center gap-2 px-3 py-1 rounded-full bg-[var(--bg-main)] border border-[var(--border-color)] text-[11px] font-medium text-zinc-400 transition-colors duration-300">
                    <span class="w-1.5 h-1.5 rounded-full bg-green-500"></span>
                    Gateway Online
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
                                    Welcome Magnus. System is ready for instructions. How can I assist you today?
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
                    <div id="agent-monitor-sidebar" class="w-80 shrink-0 flex flex-col hidden">
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

                    <!-- Add/Edit Agent -->
                    <div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl h-fit sticky top-0 transition-all duration-300">
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
                                <input type="text" id="agent-model" placeholder="gemini-2.0-flash"
                                    class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm focus:outline-none focus:border-sofia/50 transition-all text-[var(--text-main)]">
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
								<label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1.5 ml-1">Template Skills Mode</label>
								<select id="agent-template-skills-mode"
									class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm focus:outline-none focus:border-sofia/50 transition-all text-[var(--text-main)]">
									<option value="fallback">Fallback (recommended)</option>
									<option value="overwrite">Overwrite template skills</option>
								</select>
								<div class="mt-1 text-[10px] text-zinc-500">Fallback uses template skills only if the agent has no explicit skills.</div>
							</div>
							<div class="bg-black/5 dark:bg-zinc-900/40 border border-[var(--border-color)] rounded-xl p-3 transition-colors duration-300">
                                <div id="agent-template-desc" class="text-xs text-zinc-500 dark:text-zinc-400 mb-2">Select a template to preview purpose and instructions.</div>
                                <pre id="agent-template-preview" class="max-h-44 overflow-y-auto whitespace-pre-wrap text-[11px] leading-relaxed text-zinc-500">No template selected.</pre>
                            </div>
                            <div class="pt-4 flex flex-col gap-2">
                                <button onclick="saveAgent()" class="w-full bg-sofia hover:bg-sofia-hover text-white font-bold py-3 rounded-xl transition shadow-lg shadow-sofia/10">Save Agent</button>
                                <button onclick="resetAgentForm()" class="w-full py-2 text-zinc-500 hover:text-zinc-300 text-xs font-medium transition">Reset form</button>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- SKILLS TAB -->
            <div id="tab-skills" class="tab-content">
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

                        <div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl flex-grow overflow-hidden flex flex-col transition-all duration-300">
                            <h2 class="text-lg font-bold text-[var(--text-main)] mb-4">Tools (Native Tools)</h2>
                            <div id="tools-list" class="flex-grow overflow-y-auto pr-2 space-y-3">
                                <!-- Filled by JS -->
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- SETTINGS TAB -->
            <div id="tab-settings" class="tab-content h-full">
                <div class="h-full overflow-y-auto pr-2 space-y-6">
                    <div id="settings-subtab-models" class="settings-subtab space-y-6">
                        <div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl transition-all duration-300">
                            <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-2 ml-1">Default Model (LLM)</label>
                            <select id="cfg-model" class="w-full bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl px-4 py-3 text-sm focus:outline-none focus:border-sofia/50 transition-all text-[var(--text-main)]"></select>
                        </div>

                        <div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl transition-all duration-300">
                            <div class="flex items-center justify-between mb-3">
                                <h3 class="text-xs font-bold uppercase tracking-widest text-zinc-500">Providers & Models</h3>
                                <button onclick="addProviderModelRow()" class="px-3 py-1 rounded-lg bg-[var(--bg-main)] border border-[var(--border-color)] text-xs hover:bg-[var(--nav-hover)]">Add Model</button>
                            </div>
                            <div id="provider-model-list" class="space-y-2"></div>
                            <div class="mt-2 text-[10px] text-zinc-500">Add model aliases and provider endpoints (supports OpenAI-compatible APIs and other protocols).</div>
                            <button onclick="saveConfig()" class="mt-4 bg-sofia hover:bg-sofia-hover text-white font-bold px-8 py-3 rounded-xl transition shadow-lg shadow-sofia/20 flex items-center gap-2">
                                <i data-lucide="save" class="w-4 h-4"></i>
                                Save changes
                            </button>
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

                    <div id="settings-subtab-channels" class="settings-subtab hidden space-y-4">
                        <div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl transition-colors duration-300">
                            <label class="block text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-3 ml-1">Enabled Channels</label>
                            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                                <label class="flex items-center gap-3 p-3 bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl cursor-pointer hover:bg-[var(--nav-hover)] transition-all">
                                    <input type="checkbox" id="cfg-telegram" class="w-4 h-4 rounded border-zinc-700 bg-zinc-800 text-sofia focus:ring-sofia/20">
                                    <span class="text-sm font-medium text-[var(--text-main)]">Telegram</span>
                                </label>
                                <label class="flex items-center gap-3 p-3 bg-[var(--bg-main)] border border-[var(--border-color)] rounded-xl cursor-pointer hover:bg-[var(--nav-hover)] transition-all">
                                    <input type="checkbox" id="cfg-discord" class="w-4 h-4 rounded border-zinc-700 bg-zinc-800 text-sofia focus:ring-sofia/20">
                                    <span class="text-sm font-medium text-[var(--text-main)]">Discord</span>
                                </label>
                            </div>

                            <button onclick="saveConfig()" class="mt-4 bg-sofia hover:bg-sofia-hover text-white font-bold px-8 py-3 rounded-xl transition shadow-lg shadow-sofia/20 flex items-center gap-2">
                                <i data-lucide="save" class="w-4 h-4"></i>
                                Save changes
                            </button>
                        </div>

                        <div class="glass-panel p-6 rounded-2xl border border-[var(--border-color)] shadow-xl flex flex-col gap-4 transition-colors duration-300">
                            <h3 class="text-xs font-bold uppercase tracking-widest text-zinc-500 border-b border-[var(--border-color)] pb-3">System Information</h3>
                            <div id="system-details" class="text-sm space-y-4">
                                <!-- Filled by JS -->
                            </div>
                            <div class="mt-auto pt-4 flex gap-2">
                                <div class="px-3 py-1 rounded bg-[var(--bg-main)] border border-[var(--border-color)] text-[10px] font-mono text-zinc-500">v2.5.0-stable</div>
                                <div class="px-3 py-1 rounded bg-[var(--bg-main)] border border-[var(--border-color)] text-[10px] font-mono text-zinc-500">GO-1.26</div>
                            </div>
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

        // Initialize Lucide Icons
        function refreshIcons() {
            lucide.createIcons();
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
                'skills': 'Library & Tools',
                'settings': 'System Settings'
            };
            document.getElementById('view-title').innerText = titles[tabId];

            if (tabId === 'agents') fetchAgents();
            if (tabId === 'skills') fetchStatus();
            if (tabId === 'settings') {
                settingsHeaderTabs.classList.remove('hidden');
                settingsHeaderTabs.classList.add('flex');
                fetchConfig();
                showSettingsSubTab(currentSettingsSubTab);
            } else {
                settingsHeaderTabs.classList.add('hidden');
                settingsHeaderTabs.classList.remove('flex');
            }
            
            refreshIcons();
        }

        function showSettingsSubTab(tabId) {
            currentSettingsSubTab = tabId;
            const tabs = ['models', 'prompts', 'channels'];
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
                    // Only hide sidebar if no sub-agent activity and live panel is hidden
                    if (monitor.children.length === 0) {
                        sidebar.classList.add("hidden");
                    }
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
                details.innerHTML = 
                    "<div class='flex justify-between items-center'><span class='text-zinc-500'>Active Agents:</span> <span class='text-zinc-200 font-medium'>" + data.agents.ids.length + "</span></div>" +
                    "<div class='flex flex-wrap gap-1 mt-1'>" + data.agents.ids.map(id => "<span class='px-2 py-0.5 rounded bg-zinc-800 text-[10px] text-zinc-400 border border-zinc-700/50'>" + id + "</span>").join("") + "</div>" +
                    "<div class='flex justify-between items-center mt-4'><span class='text-zinc-500'>Loaded Tools:</span> <span class='text-zinc-200 font-medium'>" + data.tools.count + "</span></div>" +
                    "<div class='flex justify-between items-center'><span class='text-zinc-500'>Loaded Skills:</span> <span class='text-zinc-200 font-medium'>" + data.skills.total + "</span></div>";

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
            const preview = document.getElementById("agent-template-preview");
            const desc = document.getElementById("agent-template-desc");
            const warning = document.getElementById("agent-template-missing-skills");

            warning.classList.add("hidden");
            warning.innerText = "";

            if (!selected) {
                preview.textContent = "No template selected.";
                desc.textContent = "Select a template to preview purpose and instructions.";
                return;
            }

            const idInput = document.getElementById("agent-id");
            const nameInput = document.getElementById("agent-name");
            if (!idInput.value) idInput.value = selected;
            if (!nameInput.value) nameInput.value = selected;

            const meta = purposeTemplateMap[selected];
            desc.textContent = meta && meta.description ? meta.description : "No description available.";

            try {
                const res = await fetch("/api/agent-templates/" + encodeURIComponent(selected));
                if (!res.ok) {
                    preview.textContent = "Could not load template instructions.";
                    return;
                }
                const data = await res.json();
                preview.textContent = data.instructions || "No instructions available.";

                const availableSkills = new Set((skillsData || []).map(s => s.name));
                const missing = (data.skills || []).filter(s => !availableSkills.has(s));
                if (missing.length > 0) {
                    warning.innerText = "Missing skills: " + missing.join(", ");
                    warning.classList.remove("hidden");
                }
            } catch (e) {
                preview.textContent = "Network error while loading template.";
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

				list.innerHTML = agents.map(a => {
					const isActive = a.id === activeAgentID;
					const modelName = a.model && a.model.primary ? a.model.primary : (a.model || "");
					const templateName = a.template || "";
					const templateSkillsMode = a.template_skills_mode || "fallback";
					return '<div data-agent-id="' + a.id + '" class="group p-5 bg-zinc-900/40 border ' + (isActive ? 'border-sofia bg-sofia/5' : 'border-zinc-800') + ' rounded-2xl hover:border-sofia/30 transition-all shadow-lg">' +
                        '<div class="flex justify-between items-start mb-4">' +
                            '<div class="w-10 h-10 rounded-xl bg-zinc-800 flex items-center justify-center text-sofia group-hover:bg-sofia/10 transition-colors">' +
                                '<i data-lucide="bot" class="w-5 h-5"></i>' +
                            '</div>' +
                            '<div class="flex gap-1">' +
							'<button onclick="editAgent(\'' + a.id + '\', \'' + (a.name || '') + '\', \'' + modelName + '\', \'' + templateName + '\', \'' + templateSkillsMode + '\')" class="p-2 text-zinc-500 hover:text-white hover:bg-zinc-800 rounded-lg transition"><i data-lucide="edit-3" class="w-3.5 h-3.5"></i></button>' +
                                '<button onclick="deleteAgent(\'' + a.id + '\')" class="p-2 text-zinc-500 hover:text-red-400 hover:bg-red-900/20 rounded-lg transition"><i data-lucide="trash-2" class="w-3.5 h-3.5"></i></button>' +
                            '</div>' +
                        '</div>' +
                        '<div>' +
                            '<div class="font-bold text-zinc-200">' + (a.name || a.id) + '</div>' +
                            '<div class="text-[10px] font-mono text-zinc-600 uppercase tracking-tighter mt-0.5">' + a.id + '</div>' +
                            (templateName ? '<div class="mt-2 text-[10px] text-sofia/80 font-medium">Purpose: ' + templateName + '</div>' : '') +
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
			const templateSkillsMode = document.getElementById("agent-template-skills-mode").value;
            
            if (!id) {
                alert("ID is required!");
                return;
            }

            const agent = { id, name };
            if (modelStr) agent.model = modelStr;
			if (template) {
				agent.template = template;
				agent.template_skills_mode = templateSkillsMode || "fallback";
				const meta = purposeTemplateMap[template];
				if (meta && meta.skills && meta.skills.length > 0) {
					if ((templateSkillsMode || "fallback") === "overwrite") {
						agent.skills = meta.skills;
					}
				}
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

		function editAgent(id, name, model, template, templateSkillsMode) {
			document.getElementById("agent-id").value = id;
			document.getElementById("agent-id").disabled = true;
			document.getElementById("agent-id").classList.add("opacity-50");
			document.getElementById("agent-name").value = name;
			document.getElementById("agent-model").value = model;
			document.getElementById("agent-template").value = template || "";
			document.getElementById("agent-template-skills-mode").value = templateSkillsMode || "fallback";
			onTemplateSelected();
			document.getElementById("agent-form-title").innerText = "Edit Agent";
			refreshIcons();
		}

        function resetAgentForm() {
            document.getElementById("agent-id").value = "";
            document.getElementById("agent-id").disabled = false;
            document.getElementById("agent-id").classList.remove("opacity-50");
			document.getElementById("agent-name").value = "";
			document.getElementById("agent-model").value = "";
			document.getElementById("agent-template").value = "";
			document.getElementById("agent-template-skills-mode").value = "fallback";
			onTemplateSelected();
			document.getElementById("agent-form-title").innerText = "New Agent";
			refreshIcons();
		}

        function addProviderModelRow(seed) {
            const list = document.getElementById("provider-model-list");
            const row = document.createElement("div");
            row.className = "provider-model-row p-3 rounded-xl border border-[var(--border-color)] bg-[var(--bg-main)]";
            row.innerHTML =
                "<div class='grid grid-cols-12 gap-2'>" +
                    "<input data-key='model_name' placeholder='model_name (alias)' class='col-span-3 bg-transparent border border-[var(--border-color)] rounded px-2 py-1 text-xs text-[var(--text-main)]'>" +
                    "<input data-key='model' placeholder='protocol/model-id' class='col-span-4 bg-transparent border border-[var(--border-color)] rounded px-2 py-1 text-xs text-[var(--text-main)]'>" +
                    "<input data-key='api_base' placeholder='api_base (optional)' class='col-span-3 bg-transparent border border-[var(--border-color)] rounded px-2 py-1 text-xs text-[var(--text-main)]'>" +
                    "<input data-key='api_key' placeholder='api_key' type='password' class='col-span-2 bg-transparent border border-[var(--border-color)] rounded px-2 py-1 text-xs text-[var(--text-main)]'>" +
                "</div>" +
                "<div class='grid grid-cols-12 gap-2 mt-2'>" +
                    "<input data-key='proxy' placeholder='proxy (optional)' class='col-span-3 bg-transparent border border-[var(--border-color)] rounded px-2 py-1 text-xs text-[var(--text-main)]'>" +
                    "<input data-key='auth_method' placeholder='auth_method' class='col-span-2 bg-transparent border border-[var(--border-color)] rounded px-2 py-1 text-xs text-[var(--text-main)]'>" +
                    "<input data-key='connect_mode' placeholder='connect_mode' class='col-span-2 bg-transparent border border-[var(--border-color)] rounded px-2 py-1 text-xs text-[var(--text-main)]'>" +
                    "<input data-key='workspace' placeholder='workspace' class='col-span-3 bg-transparent border border-[var(--border-color)] rounded px-2 py-1 text-xs text-[var(--text-main)]'>" +
                    "<button onclick='removeProviderModelRow(this)' class='col-span-2 text-xs border border-[var(--border-color)] rounded hover:bg-[var(--nav-hover)]'>Remove</button>" +
                "</div>" +
                "<div class='grid grid-cols-12 gap-2 mt-2'>" +
                    "<input data-key='rpm' placeholder='rpm' type='number' min='0' class='col-span-2 bg-transparent border border-[var(--border-color)] rounded px-2 py-1 text-xs text-[var(--text-main)]'>" +
                    "<input data-key='request_timeout' placeholder='request_timeout' type='number' min='0' class='col-span-3 bg-transparent border border-[var(--border-color)] rounded px-2 py-1 text-xs text-[var(--text-main)]'>" +
                    "<input data-key='max_tokens_field' placeholder='max_tokens_field' class='col-span-4 bg-transparent border border-[var(--border-color)] rounded px-2 py-1 text-xs text-[var(--text-main)]'>" +
                    "<div class='col-span-3 text-[10px] text-zinc-500 flex items-center'>Advanced model settings</div>" +
                "</div>";
            list.appendChild(row);

            if (seed) {
                row.querySelector("[data-key='model_name']").value = seed.model_name || "";
                row.querySelector("[data-key='model']").value = seed.model || "";
                row.querySelector("[data-key='api_base']").value = seed.api_base || "";
                row.querySelector("[data-key='api_key']").value = seed.api_key || "";
                row.querySelector("[data-key='proxy']").value = seed.proxy || "";
                row.querySelector("[data-key='auth_method']").value = seed.auth_method || "";
                row.querySelector("[data-key='connect_mode']").value = seed.connect_mode || "";
                row.querySelector("[data-key='workspace']").value = seed.workspace || "";
                row.querySelector("[data-key='rpm']").value = seed.rpm || "";
                row.querySelector("[data-key='request_timeout']").value = seed.request_timeout || "";
                row.querySelector("[data-key='max_tokens_field']").value = seed.max_tokens_field || "";
            }

            row.querySelector("[data-key='model_name']").addEventListener("input", refreshDefaultModelOptions);
            refreshDefaultModelOptions();
        }

        function removeProviderModelRow(btn) {
            const row = btn.closest(".provider-model-row");
            if (row) row.remove();
            refreshDefaultModelOptions();
        }

        function getProviderModelsFromForm() {
            const rows = Array.from(document.querySelectorAll(".provider-model-row"));
            const models = [];
            rows.forEach(row => {
                const modelName = row.querySelector("[data-key='model_name']").value.trim();
                const model = row.querySelector("[data-key='model']").value.trim();
                const apiBase = row.querySelector("[data-key='api_base']").value.trim();
                const apiKey = row.querySelector("[data-key='api_key']").value.trim();
                const proxy = row.querySelector("[data-key='proxy']").value.trim();
                const authMethod = row.querySelector("[data-key='auth_method']").value.trim();
                const connectMode = row.querySelector("[data-key='connect_mode']").value.trim();
                const workspace = row.querySelector("[data-key='workspace']").value.trim();
                const rpmRaw = row.querySelector("[data-key='rpm']").value.trim();
                const requestTimeoutRaw = row.querySelector("[data-key='request_timeout']").value.trim();
                const maxTokensField = row.querySelector("[data-key='max_tokens_field']").value.trim();
                if (!modelName || !model) return;
                const entry = { model_name: modelName, model: model };
                if (apiBase) entry.api_base = apiBase;
                if (apiKey) entry.api_key = apiKey;
                if (proxy) entry.proxy = proxy;
                if (authMethod) entry.auth_method = authMethod;
                if (connectMode) entry.connect_mode = connectMode;
                if (workspace) entry.workspace = workspace;
                if (maxTokensField) entry.max_tokens_field = maxTokensField;
                const rpm = Number.parseInt(rpmRaw, 10);
                if (!Number.isNaN(rpm) && rpm > 0) entry.rpm = rpm;
                const requestTimeout = Number.parseInt(requestTimeoutRaw, 10);
                if (!Number.isNaN(requestTimeout) && requestTimeout > 0) entry.request_timeout = requestTimeout;
                models.push(entry);
            });
            return models;
        }

        function refreshDefaultModelOptions() {
            const select = document.getElementById("cfg-model");
            const previous = select.value;
            const models = getProviderModelsFromForm();

            select.innerHTML = "";
            if (models.length === 0) {
                const opt = document.createElement("option");
                opt.value = "";
                opt.textContent = "No models configured";
                select.appendChild(opt);
                return;
            }

            models.forEach(m => {
                const opt = document.createElement("option");
                opt.value = m.model_name;
                opt.textContent = m.model_name + " (" + m.model + ")";
                select.appendChild(opt);
            });

            if (models.some(m => m.model_name === previous)) {
                select.value = previous;
            }
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

        async function fetchConfig() {
            try {
                const res = await fetch("/api/config");
                if (!res.ok) return;
                const cfg = await res.json();
                currentConfig = cfg;

                document.getElementById("cfg-telegram").checked = cfg.channels.telegram.enabled;
                document.getElementById("cfg-discord").checked = cfg.channels.discord.enabled;

                const list = document.getElementById("provider-model-list");
                list.innerHTML = "";
                (cfg.model_list || []).forEach(m => addProviderModelRow(m));
                if (!cfg.model_list || cfg.model_list.length === 0) {
                    addProviderModelRow();
                }

                refreshDefaultModelOptions();
                const defaultModel = cfg.agents.defaults.model_name || cfg.agents.defaults.model || "";
                if (defaultModel) {
                    document.getElementById("cfg-model").value = defaultModel;
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
                // Create new box
                agentActivity[agentId] = {
                    lastSeen: Date.now(),
                    timer: null
                };
                
                sidebar.classList.remove("hidden");
                
                const box = document.createElement("div");
                box.id = "agent-box-" + agentId;
                box.className = "agent-log-box w-full bg-zinc-900/80 border border-sofia/20 rounded-xl overflow-hidden flex flex-col shadow-lg";
                box.innerHTML = "<div class=\"px-3 py-2 bg-sofia/10 border-b border-sofia/10 flex items-center justify-between\">" +
                    "<div class=\"flex items-center gap-2\">" +
                    "<div class=\"w-2 h-2 rounded-full bg-sofia animate-pulse\"></div>" +
                    "<span class=\"text-[10px] font-bold uppercase tracking-widest text-white\">" + agentId + "</span>" +
                    "</div>" +
                    "<span id=\"status-" + agentId + "\" class=\"text-[8px] text-sofia/80 font-mono\">WORKING</span>" +
                    "</div>" +
                    "<div id=\"logs-" + agentId + "\" class=\"p-2 h-32 overflow-y-auto bg-black/40 space-y-1\"></div>";
                monitor.appendChild(box);
                refreshIcons();
            }

            // Update existing box
            const logsDiv = document.getElementById("logs-" + agentId);
            const line = document.createElement("div");
            line.className = "agent-log-line text-zinc-400";
            line.textContent = entry.message;
            logsDiv.appendChild(line);
            logsDiv.scrollTop = logsDiv.scrollHeight;
            
            // Reset completion timer
            if (agentActivity[agentId].timer) clearTimeout(agentActivity[agentId].timer);
            
            agentActivity[agentId].lastSeen = Date.now();
            agentActivity[agentId].timer = setTimeout(() => {
                completeAgentTask(agentId);
            }, 5000); // Consider "done" after 5s of silence
        }

        function completeAgentTask(agentId) {
            const statusLabel = document.getElementById("status-" + agentId);
            const box = document.getElementById("agent-box-" + agentId);
            const dot = box.querySelector('.rounded-full');
            
            if (statusLabel) {
                statusLabel.innerText = "COMPLETED";
                statusLabel.className = "text-[8px] text-green-500 font-mono";
            }
            if (dot) {
                dot.classList.remove("bg-sofia", "animate-pulse");
                dot.classList.add("bg-green-500");
            }
            
            box.classList.add("border-green-500/20", "bg-green-500/5");
            box.classList.remove("border-sofia/20", "bg-zinc-900/80");

            // Remove after a while
            setTimeout(() => {
                if (!box) return;
                box.style.opacity = "0";
                box.style.transform = "translateY(-10px)";
                setTimeout(() => {
                    box.remove();
                    delete agentActivity[agentId];
                    
                    const monitor = document.getElementById("agent-activity-monitor");
                    const sidebar = document.getElementById("agent-monitor-sidebar");
                    const livePanel = document.getElementById("live-activity-panel");
                    if (monitor.children.length === 0 && livePanel.classList.contains("hidden")) {
                        sidebar.classList.add("hidden");
                    }
                }, 500);
            }, 10000);
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
                            "<div class='chat-bubble-sofia px-4 py-3 rounded-2xl text-sm leading-relaxed max-w-[85%] text-zinc-300'>" + responseData.response + "</div>" +
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
                
                cfg.channels.telegram.enabled = document.getElementById("cfg-telegram").checked;
                cfg.channels.discord.enabled = document.getElementById("cfg-discord").checked;

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
        showSettingsSubTab('models');
        setupLogStream();
        updateThemeIcons();
        refreshIcons();
        
        // Auto-refresh status
        setInterval(fetchStatus, 2000);

        document.getElementById("chat-input").addEventListener("keypress", (e) => {
            if (e.key === "Enter") sendChat();
        });
    </script>
</body>
</html>
`
