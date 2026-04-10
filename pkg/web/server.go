package web

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/audit"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/cron"
	"github.com/grasberg/sofia/pkg/eval"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/skills"
)

//go:embed templates/layout.html
var layoutHTML []byte

//go:embed templates/chat.html
var chatHTML []byte

//go:embed templates/agents.html
var agentsHTML []byte

//go:embed templates/monitor.html
var monitorHTML []byte

//go:embed templates/settings/models.html
var settingsModelsHTML []byte

//go:embed templates/settings/channels.html
var settingsChannelsHTML []byte

//go:embed templates/settings/tools.html
var settingsToolsHTML []byte

//go:embed templates/settings/integrations.html
var settingsIntegrationsHTML []byte

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

//go:embed templates/settings/evolution.html
var settingsEvolutionHTML []byte

//go:embed templates/settings/autonomy.html
var settingsAutonomyHTML []byte

//go:embed templates/settings/intelligence.html
var settingsIntelligenceHTML []byte

//go:embed templates/settings/budget.html
var settingsBudgetHTML []byte

//go:embed templates/settings/tts.html
var settingsTTSHTML []byte

//go:embed templates/settings/webhooks.html
var settingsWebhooksHTML []byte

//go:embed templates/settings/triggers.html
var settingsTriggersHTML []byte

//go:embed templates/settings/remote.html
var settingsRemoteHTML []byte

//go:embed templates/settings/cron.html
var settingsCronHTML []byte

//go:embed templates/settings/personas.html
var settingsPersonasHTML []byte

//go:embed templates/settings_ai.html
var settingsAIHTML []byte

//go:embed templates/settings_platform.html
var settingsPlatformHTML []byte

//go:embed templates/calendar.html
var calendarHTML []byte

//go:embed templates/memory.html
var memoryHTML []byte

//go:embed templates/goals.html
var goalsHTML []byte

//go:embed templates/activity.html
var activityHTML []byte

//go:embed templates/completed.html
var completedHTML []byte

//go:embed templates/history.html
var historyHTML []byte

//go:embed templates/eval.html
var evalHTML []byte

//go:embed templates/files.html
var filesHTML []byte

type Server struct {
	cfg            *config.Config
	agentLoop      *agent.AgentLoop
	Version        string
	server         *http.Server
	mux            *http.ServeMux
	mu             sync.RWMutex
	limiter        *rateLimiter
	skillInstaller *skills.SkillInstaller
	auditLogger    *audit.AuditLogger
	cronService    *cron.CronService
	evalStore      *eval.EvalStore
	stopCtxCancel  context.CancelFunc
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
	// Create a cancellable context for background goroutines (e.g. rate limiter cleanup).
	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		cfg:            cfg,
		agentLoop:      agentLoop,
		Version:        version,
		limiter:        newRateLimiter(120, time.Minute, ctx),
		skillInstaller: skills.NewSkillInstaller(cfg.WorkspacePath()),
		stopCtxCancel:  cancel,
	}

	mux := http.NewServeMux()
	assetsDir := resolveAssetsDir()
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(assetsDir))))
	mux.HandleFunc("/", s.handleIndex)

	// servePartial returns an auth-protected handler that serves a static HTML partial.
	// Cache-Control: no-cache ensures the browser revalidates on every request,
	// preventing stale JavaScript after binary updates.
	servePartial := func(content []byte) http.HandlerFunc {
		return s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Write(content)
		})
	}

	// HTMX Partials (all auth-protected)
	mux.HandleFunc("/ui/chat", servePartial(chatHTML))
	mux.HandleFunc("/ui/agents", servePartial(agentsHTML))
	mux.HandleFunc("/ui/monitor", servePartial(monitorHTML))
	mux.HandleFunc("/ui/settings", s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
<!-- SETTINGS TAB (HTMX Shell) -->
<div id="tab-settings" class="flex flex-col flex-grow min-h-0">
	<div id="subtab-content" class="flex flex-col flex-grow min-h-0" hx-get="/ui/settings/models" hx-trigger="load">
		<!-- HTMX will inject models.html here by default -->
	</div>
</div>
		`))
	}))
	mux.HandleFunc("/ui/ai", servePartial(settingsAIHTML))
	mux.HandleFunc("/ui/platform", servePartial(settingsPlatformHTML))
	mux.HandleFunc("/ui/settings/models", servePartial(settingsModelsHTML))
	mux.HandleFunc("/ui/settings/channels", servePartial(settingsChannelsHTML))
	mux.HandleFunc("/ui/settings/tools", servePartial(settingsToolsHTML))
	mux.HandleFunc("/ui/settings/integrations", servePartial(settingsIntegrationsHTML))
	mux.HandleFunc("/ui/settings/skills", servePartial(settingsSkillsHTML))
	mux.HandleFunc("/ui/settings/heartbeat", servePartial(settingsHeartbeatHTML))
	mux.HandleFunc("/ui/settings/security", servePartial(settingsSecurityHTML))
	mux.HandleFunc("/ui/settings/prompts", servePartial(settingsPromptsHTML))
	mux.HandleFunc("/ui/settings/logs", servePartial(settingsLogsHTML))
	mux.HandleFunc("/ui/settings/evolution", servePartial(settingsEvolutionHTML))
	mux.HandleFunc("/ui/settings/autonomy", servePartial(settingsAutonomyHTML))
	mux.HandleFunc("/ui/settings/intelligence", servePartial(settingsIntelligenceHTML))
	mux.HandleFunc("/ui/settings/budget", servePartial(settingsBudgetHTML))
	mux.HandleFunc("/ui/settings/tts", servePartial(settingsTTSHTML))
	mux.HandleFunc("/ui/settings/webhooks", servePartial(settingsWebhooksHTML))
	mux.HandleFunc("/ui/settings/triggers", servePartial(settingsTriggersHTML))
	mux.HandleFunc("/ui/settings/remote", servePartial(settingsRemoteHTML))
	mux.HandleFunc("/ui/settings/cron", servePartial(settingsCronHTML))
	mux.HandleFunc("/ui/settings/personas", servePartial(settingsPersonasHTML))
	mux.HandleFunc("/ui/calendar", servePartial(calendarHTML))
	mux.HandleFunc("/ui/memory", servePartial(memoryHTML))
	mux.HandleFunc("/ui/goals", servePartial(goalsHTML))
	mux.HandleFunc("/ui/activity", servePartial(activityHTML))
	mux.HandleFunc("/ui/completed", servePartial(completedHTML))
	mux.HandleFunc("/ui/history", servePartial(historyHTML))
	mux.HandleFunc("/ui/eval", servePartial(evalHTML))
	mux.HandleFunc("/ui/files", servePartial(filesHTML))

	// API routes: rate limiting runs FIRST (outermost), then auth.
	// This ensures unauthenticated brute-force attempts are rate-limited.
	api := func(handler http.HandlerFunc) http.HandlerFunc {
		return s.rateLimitMiddleware(s.authMiddleware(handler))
	}

	mux.HandleFunc("/api/status", api(s.handleStatus))
	mux.HandleFunc("/api/config", api(s.handleConfig))
	mux.HandleFunc("GET /api/models", api(s.handleModels))
	mux.HandleFunc("/api/chat/stream", api(s.handleChatStream))
	mux.HandleFunc("/api/chat", api(s.handleChat))
	mux.HandleFunc("/api/logs", api(s.handleLogs))
	mux.HandleFunc("/api/skills/add", api(s.handleSkillAdd))
	mux.HandleFunc("GET /api/skills", api(s.handleSkillsList))
	mux.HandleFunc("POST /api/skills/toggle", api(s.handleSkillsToggle))
	mux.HandleFunc("/api/agents", api(s.handleAgents))
	mux.HandleFunc("/api/agent-templates", api(s.handleAgentTemplates))
	mux.HandleFunc("/api/agent-templates/", api(s.handleAgentTemplateByName))
	mux.HandleFunc("/api/workspace-docs", api(s.handleWorkspaceDocs))
	mux.HandleFunc("GET /api/workspace/files", api(s.handleWorkspaceFiles))
	mux.HandleFunc("GET /api/workspace/file", api(s.handleWorkspaceFile))
	mux.HandleFunc("/api/restart", api(s.handleRestart))
	mux.HandleFunc("/api/update", api(s.handleUpdate))
	mux.HandleFunc("/api/sessions", api(s.handleSessions))
	mux.HandleFunc("/api/sessions/", api(s.handleSessionDetail))
	mux.HandleFunc("/api/goals", api(s.handleGoals))
	mux.HandleFunc("POST /api/goals/restart", api(s.handleGoalRestart))
	mux.HandleFunc("GET /api/goals/completed", api(s.handleGoalsCompleted))
	mux.HandleFunc("GET /api/activity", api(s.handleActivity))
	mux.HandleFunc("/api/goals/", api(s.handleGoalLog))
	mux.HandleFunc("/api/reset", api(s.handleReset))
	mux.HandleFunc("GET /api/search", api(s.handleSearch))
	mux.HandleFunc("GET /api/presence", api(s.handlePresence))
	mux.HandleFunc("GET /api/audit", api(s.handleAudit))
	mux.HandleFunc("GET /api/approvals", api(s.handleApprovals))
	mux.HandleFunc("/api/approvals/", api(s.handleApprovalAction))
	mux.HandleFunc("/api/cron", api(s.handleCron))
	mux.HandleFunc("/api/cron/toggle", api(s.handleCronToggle))
	mux.HandleFunc("GET /api/memory/notes", api(s.handleMemoryNotes))
	mux.HandleFunc("GET /api/memory/graph", api(s.handleMemoryGraph))
	mux.HandleFunc("GET /api/memory/reflections", api(s.handleMemoryReflections))
	mux.HandleFunc("GET /api/plan", api(s.handlePlan))
	mux.HandleFunc("GET /api/plans", api(s.handlePlans))
	mux.HandleFunc("GET /api/backup", api(s.handleBackupExport))
	mux.HandleFunc("GET /api/tor/status", api(s.handleTorStatus))
	mux.HandleFunc("POST /api/tor/toggle", api(s.handleTorToggle))
	mux.HandleFunc("GET /api/evolution/status", api(s.handleEvolutionStatus))
	mux.HandleFunc("GET /api/evolution/changelog", api(s.handleEvolutionChangelog))
	mux.HandleFunc("GET /api/eval/runs", api(s.handleEvalRuns))
	mux.HandleFunc("GET /api/eval/runs/", api(s.handleEvalRunDetail))
	mux.HandleFunc("GET /api/eval/trend", api(s.handleEvalTrend))
	mux.HandleFunc(
		"/ws/dashboard",
		s.rateLimitMiddleware(s.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
			s.agentLoop.DashboardHub().RegisterClient(w, r, func() any {
				return s.agentLoop.GetStartupInfo()
			})
		})),
	)

	s.mux = mux
	s.server = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.WebUI.Host, cfg.WebUI.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
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
	if s.stopCtxCancel != nil {
		s.stopCtxCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}
