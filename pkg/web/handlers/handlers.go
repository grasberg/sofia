// Package handlers provides HTTP handler functions for the Sofia web UI.
// Handlers are organized by domain to improve maintainability and testability.
package handlers

import (
	"net/http"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/audit"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/cron"
	"github.com/grasberg/sofia/pkg/eval"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/skills"
)

// Server provides shared dependencies to all handlers.
type Server struct {
	Cfg            *config.Config
	AgentLoop      *agent.AgentLoop
	AuditLogger    *audit.AuditLogger
	CronService    *cron.CronService
	EvalStore      *eval.EvalStore
	SkillInstaller *skills.SkillInstaller
	MemDB          *memory.MemoryDB
	Version        string
}

// RegisterAll registers all HTTP handlers on the mux.
func RegisterAll(mux *http.ServeMux, srv *Server) {
	// Core endpoints
	mux.HandleFunc("/", srv.HandleIndex)
	mux.HandleFunc("/api/config", srv.serveConfig)
	mux.HandleFunc("/api/status", srv.serveStatus)
	mux.HandleFunc("/api/chat", srv.serveChat)
	mux.HandleFunc("/api/chat/stream", srv.serveChatStream)
	mux.HandleFunc("/api/logs", srv.serveLogs)
	mux.HandleFunc("/api/search", srv.serveSearch)

	// Agents
	mux.HandleFunc("/api/agents", srv.serveAgents)
	mux.HandleFunc("/api/agent/templates", srv.serveAgentTemplates)
	mux.HandleFunc("/api/agent/templates/", srv.serveAgentTemplateByName)

	// Sessions
	mux.HandleFunc("/api/sessions", srv.serveSessions)
	mux.HandleFunc("/api/session", srv.serveSessionDetail)

	// Goals
	mux.HandleFunc("/api/goals", srv.serveGoals)
	mux.HandleFunc("/api/goal-log", srv.serveGoalLog)

	// Workspace
	mux.HandleFunc("/api/workspace/files", srv.serveWorkspaceFiles)
	mux.HandleFunc("/api/workspace/file", srv.serveWorkspaceFile)
	mux.HandleFunc("/api/workspace/docs", srv.serveWorkspaceDocs)

	// Skills
	mux.HandleFunc("/api/skills", srv.serveSkillsList)
	mux.HandleFunc("/api/skills/add", srv.serveSkillAdd)
	mux.HandleFunc("/api/skills/toggle", srv.serveSkillsToggle)

	// Memory
	mux.HandleFunc("/api/memory/notes", srv.serveMemoryNotes)
	mux.HandleFunc("/api/memory/graph", srv.serveMemoryGraph)
	mux.HandleFunc("/api/memory/reflections", srv.serveMemoryReflections)

	// Evolution
	mux.HandleFunc("/api/evolution/status", srv.serveEvolutionStatus)
	mux.HandleFunc("/api/evolution/changelog", srv.serveEvolutionChangelog)

	// Cron
	mux.HandleFunc("/api/cron", srv.serveCron)
	mux.HandleFunc("/api/cron/toggle", srv.serveCronToggle)

	// Eval
	mux.HandleFunc("/api/eval/runs", srv.serveEvalRuns)
	mux.HandleFunc("/api/eval/run", srv.serveEvalRunDetail)
	mux.HandleFunc("/api/eval/trend", srv.serveEvalTrend)

	// Audit & Approvals
	mux.HandleFunc("/api/audit", srv.serveAudit)
	mux.HandleFunc("/api/approvals", srv.serveApprovals)
	mux.HandleFunc("/api/approval", srv.serveApprovalAction)

	// System
	mux.HandleFunc("/api/restart", srv.serveRestart)
	mux.HandleFunc("/api/update", srv.serveUpdate)
	mux.HandleFunc("/api/reset", srv.serveReset)
	mux.HandleFunc("/api/backup", srv.serveBackupExport)
	mux.HandleFunc("/api/presence", srv.servePresence)

	// Plans
	mux.HandleFunc("/api/plan", srv.servePlan)
}
