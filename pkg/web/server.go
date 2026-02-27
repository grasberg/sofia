package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/sipeed/sofia/pkg/agent"
	"github.com/sipeed/sofia/pkg/config"
	"github.com/sipeed/sofia/pkg/logger"
	"github.com/sipeed/sofia/pkg/skills"
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
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/skills/add", s.handleSkillAdd)
	mux.HandleFunc("/api/agents", s.handleAgents)

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
			http.Error(w, err.Error(), http.StatusBadRequest)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	response, err := s.agentLoop.ProcessDirect(ctx, req.Message, "web:ui")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"response": response})
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
		fmt.Fprintf(w, "data: %s\n\n", "Ansluten till logg-strömmen...")
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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Content == "" {
		http.Error(w, "Name and Content are required", http.StatusBadRequest)
		return
	}

	if err := s.skillInstaller.InstallFromMarkdown(req.Name, []byte(req.Content)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if agent.ID == "" {
			http.Error(w, "Agent ID is required", http.StatusBadRequest)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	if r.Method == http.MethodDelete {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Agent ID is required", http.StatusBadRequest)
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

const indexHTML = `
<!DOCTYPE html>
<html lang="sv" class="dark">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Sofia 🦞 - Dashboard</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <script>
        tailwind.config = {
            darkMode: "class",
            theme: {
                extend: {
                    colors: {
                        sofia: "#ff4d4d",
                    }
                }
            }
        }
    </script>
    <style>
        body { background-color: #0f0f0f; color: #e0e0e0; }
        .sofia-card { background-color: #1a1a1a; border: 1px solid #333; }
        .nav-link { border-bottom: 2px solid transparent; transition: all 0.3s; }
        .nav-link.active { border-bottom: 2px solid #ff4d4d; color: #ff4d4d; }
        .tab-content { display: none; }
        .tab-content.active { display: block; }
    </style>
</head>
<body class="p-4 md:p-8">
    <div class="max-w-6xl mx-auto">
        <header class="flex flex-col md:flex-row justify-between items-center mb-8 border-b border-sofia pb-4 gap-4">
            <div class="flex items-center gap-4">
                <h1 class="text-4xl font-bold text-sofia">Sofia 🦞</h1>
                <nav class="flex gap-6 ml-8">
                    <a href="#" onclick="showTab('chat')" id="nav-chat" class="nav-link active font-bold py-2">Chatt</a>
                    <a href="#" onclick="showTab('agents')" id="nav-agents" class="nav-link font-bold py-2">Agenter</a>
                    <a href="#" onclick="showTab('settings')" id="nav-settings" class="nav-link font-bold py-2">Inställningar</a>
                </nav>
            </div>
            <div id="status-badge" class="px-3 py-1 rounded-full bg-green-900 text-green-300 text-sm">Online</div>
        </header>

        <!-- CHAT TAB -->
        <div id="tab-chat" class="tab-content active">
            <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
                <!-- Chat Card -->
                <div class="sofia-card p-6 rounded-xl shadow-lg md:col-span-2 flex flex-col h-[600px]">
                    <h2 class="text-xl font-bold mb-4 text-sofia">Direktchatt</h2>
                    <div id="chat-history" class="flex-grow overflow-y-auto mb-4 space-y-4 p-2">
                        <div class="text-gray-500 italic text-sm">Välkommen Magnus. Vad kan jag göra för dig?</div>
                    </div>
                    <div class="flex gap-2">
                        <input type="text" id="chat-input" placeholder="Skriv ett kommando..." class="flex-grow bg-zinc-800 border border-zinc-700 rounded-lg px-4 py-2 focus:outline-none focus:border-sofia">
                        <button onclick="sendChat()" class="bg-sofia hover:bg-red-600 text-white font-bold px-6 py-2 rounded-lg transition">Skicka</button>
                    </div>
                </div>

                <!-- Status Card -->
                <div class="sofia-card p-6 rounded-xl shadow-lg">
                    <h2 class="text-xl font-bold mb-4 text-sofia">Systemstatus</h2>
                    <div id="status-info" class="space-y-2">
                        <p>Laddar status...</p>
                    </div>
                    <div class="mt-6">
                        <h3 class="text-sm font-bold mb-2 uppercase text-gray-500">Loggar</h3>
                        <div id="log-view" class="bg-black p-3 rounded font-mono text-xs h-48 overflow-y-auto text-green-500 border border-zinc-800">
                            Laddar loggar...
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <!-- AGENTS TAB -->
        <div id="tab-agents" class="tab-content">
            <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
                <!-- Agent List -->
                <div class="sofia-card p-6 rounded-xl shadow-lg md:col-span-2">
                    <h2 class="text-xl font-bold mb-4 text-sofia">Mina Agenter</h2>
                    <div id="agents-list" class="space-y-4">
                        <p class="text-gray-500">Laddar agenter...</p>
                    </div>
                </div>

                <!-- Add/Edit Agent -->
                <div class="sofia-card p-6 rounded-xl shadow-lg">
                    <h2 id="agent-form-title" class="text-xl font-bold mb-4 text-sofia">Lägg till Agent</h2>
                    <div class="space-y-4">
                        <div>
                            <label class="block text-sm font-medium mb-1">ID (t.ex. coder)</label>
                            <input type="text" id="agent-id" class="w-full bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm">
                        </div>
                        <div>
                            <label class="block text-sm font-medium mb-1">Namn</label>
                            <input type="text" id="agent-name" class="w-full bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm">
                        </div>
                        <div>
                            <label class="block text-sm font-medium mb-1">Modell</label>
                            <input type="text" id="agent-model" class="w-full bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm" placeholder="gemini-3-flash-preview">
                        </div>
                        <div class="pt-4">
                            <button onclick="saveAgent()" class="w-full bg-sofia hover:bg-red-600 text-white font-bold px-6 py-2 rounded transition">Spara Agent</button>
                            <button onclick="resetAgentForm()" class="w-full mt-2 bg-zinc-700 hover:bg-zinc-600 text-white font-bold px-6 py-2 rounded transition text-xs">Rensa</button>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <!-- SETTINGS TAB -->
        <div id="tab-settings" class="tab-content">
            <div class="grid grid-cols-1 gap-8">
                <!-- Config Card -->
                <div class="sofia-card p-6 rounded-xl shadow-lg">
                    <h2 class="text-xl font-bold mb-6 text-sofia">Konfiguration</h2>
                    <div class="grid grid-cols-1 md:grid-cols-2 gap-8">
                        <div>
                            <label class="block text-sm font-medium mb-1">Aktiv Modell</label>
                            <input type="text" id="cfg-model" class="w-full bg-zinc-800 border border-zinc-700 rounded px-3 py-2 mb-4">
                            
                            <div class="flex items-center gap-4 mb-4">
                                <label class="flex items-center gap-2 cursor-pointer">
                                    <input type="checkbox" id="cfg-telegram" class="accent-sofia"> Telegram
                                </label>
                                <label class="flex items-center gap-2 cursor-pointer">
                                    <input type="checkbox" id="cfg-discord" class="accent-sofia"> Discord
                                </label>
                            </div>
                            <button onclick="saveConfig()" class="bg-sofia hover:bg-red-600 text-white font-bold px-6 py-2 rounded transition">Spara inställningar</button>
                        </div>
                        <div class="bg-zinc-900/50 p-4 rounded-lg border border-zinc-800">
                            <h3 class="text-sm font-bold mb-2 uppercase text-gray-500">Systeminfo</h3>
                            <div id="system-details" class="text-sm space-y-1">
                                <!-- Filled by JS -->
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Skills & Tools -->
                <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                    <!-- Add Skill Card -->
                    <div class="sofia-card p-6 rounded-xl shadow-lg">
                        <h2 class="text-xl font-bold mb-4 text-sofia">Lägg till ny Skill</h2>
                        <div class="space-y-4">
                            <div>
                                <label class="block text-sm font-medium mb-1">Skill-namn (t.ex. my-skill)</label>
                                <input type="text" id="new-skill-name" class="w-full bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm" placeholder="hyphen-case-name">
                            </div>
                            <div>
                                <label class="block text-sm font-medium mb-1">Markdown-innehåll (SKILL.md)</label>
                                <textarea id="new-skill-content" rows="8" class="w-full bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm font-mono" placeholder="---
name: my-skill
description: ...
---
# My Skill ..."></textarea>
                            </div>
                            <button onclick="addSkill()" class="w-full bg-sofia hover:bg-red-600 text-white font-bold px-6 py-2 rounded transition">Installera Skill</button>
                        </div>
                    </div>

                    <!-- Skills -->
                    <div class="sofia-card p-6 rounded-xl shadow-lg">
                        <h2 class="text-xl font-bold mb-4 text-sofia">Installerade Skills</h2>
                        <div id="skills-list" class="space-y-4 max-h-[500px] overflow-y-auto pr-2">
                            <p class="text-gray-500">Laddar skills...</p>
                        </div>
                    </div>
                </div>

                <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                    <!-- Tools -->
                    <div class="sofia-card p-6 rounded-xl shadow-lg">
                        <h2 class="text-xl font-bold mb-4 text-sofia">Verktyg (Tools)</h2>
                        <div id="tools-list" class="space-y-4 max-h-[500px] overflow-y-auto pr-2">
                            <p class="text-gray-500">Laddar verktyg...</p>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <script>
        let currentTab = 'chat';

        async function addSkill() {
            const name = document.getElementById("new-skill-name").value;
            const content = document.getElementById("new-skill-content").value;
            
            if (!name || !content) {
                alert("Namn och innehåll krävs!");
                return;
            }

            const res = await fetch("/api/skills/add", {
                method: "POST",
                headers: {"Content-Type": "application/json"},
                body: JSON.stringify({name, content})
            });

            if (res.ok) {
                alert("Skill installerad!");
                document.getElementById("new-skill-name").value = "";
                document.getElementById("new-skill-content").value = "";
                fetchStatus();
            } else {
                const err = await res.text();
                alert("Fel: " + err);
            }
        }

        function showTab(tabId) {
            currentTab = tabId;
            document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.nav-link').forEach(l => l.classList.remove('active'));
            
            document.getElementById('tab-' + tabId).classList.add('active');
            document.getElementById('nav-' + tabId).classList.add('active');
            
            if (tabId === 'agents') {
                fetchAgents();
            }
        }

        async function fetchStatus() {
            const res = await fetch("/api/status");
            const data = await res.json();
            
            // Update Mini Status
            const info = document.getElementById("status-info");
            info.innerHTML = "<div>Verktyg: <span class='text-sofia font-bold'>" + data.tools.count + "</span></div>" +
                             "<div>Skills: <span class='text-sofia font-bold'>" + data.skills.available + "/" + data.skills.total + "</span></div>" +
                             "<div class='mt-4 text-xs text-gray-500'>Uppdaterad: " + new Date().toLocaleTimeString() + "</div>";

            // Update System Details in Settings
            const details = document.getElementById("system-details");
            details.innerHTML = "<div>Agenter: <span class='text-gray-300'>" + data.agents.ids.join(", ") + "</span></div>" +
                                "<div>Totalt antal verktyg: <span class='text-gray-300'>" + data.tools.count + "</span></div>" +
                                "<div>Totalt antal skills: <span class='text-gray-300'>" + data.skills.total + "</span></div>";

            // Update Skills List
            const skillsList = document.getElementById("skills-list");
            if (data.skills.list) {
                const grouped = data.skills.list.reduce((acc, s) => {
                    acc[s.source] = acc[s.source] || [];
                    acc[s.source].push(s);
                    return acc;
                }, {});

                let skillsHtml = "";
                for (const source in grouped) {
                    skillsHtml += "<div class='mb-4'><h3 class='text-xs font-bold uppercase text-gray-500 mb-2 border-b border-zinc-800 pb-1'>" + source + "</h3>";
                    grouped[source].forEach(s => {
                        skillsHtml += "<div class='mb-3 p-3 bg-zinc-900/30 rounded border border-zinc-800/50'>" +
                                      "<div class='font-bold text-sofia'>" + s.name + "</div>" +
                                      "<div class='text-sm text-gray-400 mt-1'>" + (s.description || "Ingen beskrivning.") + "</div>" +
                                      "<div class='text-[10px] text-gray-600 mt-2 truncate'>" + s.path + "</div>" +
                                      "</div>";
                    });
                    skillsHtml += "</div>";
                }
                skillsList.innerHTML = skillsHtml;
            }

            // Update Tools List
            const toolsList = document.getElementById("tools-list");
            if (data.tools.list) {
                toolsList.innerHTML = data.tools.list.map(t => 
                    "<div class='mb-3 p-3 bg-zinc-900/30 rounded border border-zinc-800/50'>" +
                    "<div class='font-bold text-sofia'>" + t.name + "</div>" +
                    "<div class='text-sm text-gray-400 mt-1'>" + (t.description || "Ingen beskrivning.") + "</div>" +
                    "</div>"
                ).join("");
            }
        }

        async function fetchAgents() {
            const res = await fetch("/api/agents");
            const agents = await res.json();
            const list = document.getElementById("agents-list");
            
            if (!agents || agents.length === 0) {
                list.innerHTML = "<p class='text-gray-500 italic'>Inga sub-agenter konfigurerade.</p>";
                return;
            }

            list.innerHTML = agents.map(a => 
                '<div class="p-4 bg-zinc-900/50 border border-zinc-800 rounded-lg flex justify-between items-center">' +
                    '<div>' +
                        '<div class="font-bold text-sofia">' + (a.name || a.id) + ' <span class="text-xs font-normal text-gray-500 ml-2">(' + a.id + ')</span></div>' +
                        '<div class="text-sm text-gray-400">Modell: ' + (a.model || 'Standard') + '</div>' +
                    '</div>' +
                    '<div class="flex gap-2">' +
                        '<button onclick="editAgent(\'' + a.id + '\', \'' + (a.name || '') + '\', \'' + (a.model || '') + '\')" class="text-xs bg-zinc-800 hover:bg-zinc-700 px-3 py-1 rounded">Redigera</button>' +
                        '<button onclick="deleteAgent(\'' + a.id + '\')" class="text-xs bg-red-900/30 hover:bg-red-900/50 text-red-400 px-3 py-1 rounded">Ta bort</button>' +
                    '</div>' +
                '</div>'
            ).join("");
        }

        async function saveAgent() {
            const id = document.getElementById("agent-id").value;
            const name = document.getElementById("agent-name").value;
            const modelStr = document.getElementById("agent-model").value;
            
            if (!id) {
                alert("ID krävs!");
                return;
            }

            const agent = { id, name };
            if (modelStr) {
                agent.model = modelStr;
            }

            const res = await fetch("/api/agents", {
                method: "POST",
                headers: {"Content-Type": "application/json"},
                body: JSON.stringify(agent)
            });

            if (res.ok) {
                resetAgentForm();
                fetchAgents();
            } else {
                alert("Kunde inte spara agent.");
            }
        }

        async function deleteAgent(id) {
            if (!confirm("Är du säker på att du vill ta bort agenten " + id + "?")) return;
            
            const res = await fetch("/api/agents?id=" + id, {
                method: "DELETE"
            });

            if (res.ok) {
                fetchAgents();
            } else {
                alert("Kunde inte ta bort agent.");
            }
        }

        function editAgent(id, name, model) {
            document.getElementById("agent-id").value = id;
            document.getElementById("agent-id").disabled = true;
            document.getElementById("agent-name").value = name;
            document.getElementById("agent-model").value = model;
            document.getElementById("agent-form-title").innerText = "Redigera Agent";
        }

        function resetAgentForm() {
            document.getElementById("agent-id").value = "";
            document.getElementById("agent-id").disabled = false;
            document.getElementById("agent-name").value = "";
            document.getElementById("agent-model").value = "";
            document.getElementById("agent-form-title").innerText = "Lägg till Agent";
        }

        async function fetchConfig() {
            const res = await fetch("/api/config");
            const cfg = await res.json();
            document.getElementById("cfg-model").value = cfg.agents.defaults.model_name || cfg.agents.defaults.model;
            document.getElementById("cfg-telegram").checked = cfg.channels.telegram.enabled;
            document.getElementById("cfg-discord").checked = cfg.channels.discord.enabled;
        }

        function setupLogStream() {
            const logView = document.getElementById("log-view");
            const eventSource = new EventSource("/api/logs");

            eventSource.onmessage = function(event) {
                const div = document.createElement("div");
                div.textContent = event.data;
                logView.appendChild(div);
                
                // Keep only last 100 lines
                while (logView.childNodes.length > 100) {
                    logView.removeChild(logView.firstChild);
                }
                
                logView.scrollTop = logView.scrollHeight;
            };

            eventSource.onerror = function() {
                console.error("Log stream connection lost. Retrying in 5s...");
                eventSource.close();
                setTimeout(setupLogStream, 5000);
            };
        }

        async function sendChat() {
            const input = document.getElementById("chat-input");
            const msg = input.value;
            if (!msg) return;

            const history = document.getElementById("chat-history");
            history.innerHTML += "<div class='text-right'><span class='bg-zinc-700 px-3 py-2 rounded-lg text-sm inline-block max-w-[80%]'>" + msg + "</span></div>";
            input.value = "";
            
            // Add thinking indicator
            const thinkingId = "thinking-" + Date.now();
            history.innerHTML += "<div id='" + thinkingId + "' class='text-left'><span class='bg-sofia/10 border border-sofia/20 px-3 py-2 rounded-lg text-sm inline-block italic text-gray-400'>Sofia tänker... 💭</span></div>";
            history.scrollTop = history.scrollHeight;

            try {
                const res = await fetch("/api/chat", {
                    method: "POST",
                    headers: {"Content-Type": "application/json"},
                    body: JSON.stringify({message: msg})
                });
                const data = await res.json();
                
                // Remove thinking indicator
                const thinkingEl = document.getElementById(thinkingId);
                if (thinkingEl) thinkingEl.remove();

                history.innerHTML += "<div class='text-left'><span class='bg-sofia/20 border border-sofia/30 px-3 py-2 rounded-lg text-sm inline-block max-w-[80%]'>" + data.response + "</span></div>";
            } catch (err) {
                const thinkingEl = document.getElementById(thinkingId);
                if (thinkingEl) thinkingEl.remove();
                history.innerHTML += "<div class='text-left'><span class='bg-red-900/20 border border-red-900/30 px-3 py-2 rounded-lg text-sm inline-block text-red-400'>Fel: " + err.message + "</span></div>";
            }
            history.scrollTop = history.scrollHeight;
        }

        async function saveConfig() {
            const res = await fetch("/api/config");
            let cfg = await res.json();
            
            cfg.agents.defaults.model_name = document.getElementById("cfg-model").value;
            cfg.channels.telegram.enabled = document.getElementById("cfg-telegram").checked;
            cfg.channels.discord.enabled = document.getElementById("cfg-discord").checked;

            const saveRes = await fetch("/api/config", {
                method: "POST",
                headers: {"Content-Type": "application/json"},
                body: JSON.stringify(cfg)
            });
            
            if (saveRes.ok) {
                alert("Inställningar sparade! Starta om Sofia för att ändringarna ska träda i kraft helt.");
            }
        }

        fetchStatus();
        fetchConfig();
        setupLogStream();
        setInterval(fetchStatus, 5000);

        document.getElementById("chat-input").addEventListener("keypress", (e) => {
            if (e.key === "Enter") sendChat();
        });
    </script>
</body>
</html>
`
