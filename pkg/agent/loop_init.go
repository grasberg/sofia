package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"

	"github.com/grasberg/sofia/pkg/audit"
	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/budget"
	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/checkpoint"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/dashboard"
	"github.com/grasberg/sofia/pkg/evolution"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/notifications"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/reputation"
	"github.com/grasberg/sofia/pkg/session"
	"github.com/grasberg/sofia/pkg/state"
	"github.com/grasberg/sofia/pkg/tools"
	"github.com/grasberg/sofia/pkg/tor"
	"github.com/grasberg/sofia/pkg/trace"
)

func NewAgentLoop(cfg *config.Config, msgBus *bus.MessageBus, provider providers.LLMProvider) *AgentLoop {
	// Open the shared SQLite memory database.
	// Default path: ~/.sofia/memory.db (configurable via cfg.MemoryDB).
	memDBPath := cfg.MemoryDB
	if memDBPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			logger.ErrorCF("agent",
				"Failed to get home directory",
				map[string]any{"error": err.Error()})
			home = "."
		}
		memDBPath = filepath.Join(home, ".sofia", "memory.db")
	}
	memDB, err := memory.Open(memDBPath)
	if err != nil {
		// Critical failure: log error and mark as degraded mode
		logger.ErrorCF("agent", "Failed to open memory database — running in DEGRADED MODE",
			map[string]any{
				"path":  memDBPath,
				"error": err.Error(),
			})
		logger.ErrorCF("agent", "⚠️  DEGRADED MODE: Memory, budgets, audit, tracing, and evolution will be disabled",
			map[string]any{"impact": "reduced functionality"})
		// memDB is nil, subsystems will check for nil and disable gracefully
	}

	// Seed the models table from the catalog and migrate any existing
	// model_list entries from config. After this, cfg.ModelList is populated
	// from the DB and config.json no longer needs to store model_list.
	if memDB != nil {
		if initErr := memDB.InitModels(cfg); initErr != nil {
			logger.WarnCF("agent", "Failed to initialise models DB — using in-memory list",
				map[string]any{"error": initErr.Error()})
		}
	}

	registry := NewAgentRegistry(cfg, provider, memDB)

	// Set up shared fallback chain
	cooldown := providers.NewCooldownTracker()
	fallbackChain := providers.NewFallbackChain(cooldown)

	// Tool filtering uses the keyword matcher (tools.KeywordMatchTools) which
	// works locally without any API calls. The semantic matcher that called
	// OpenAI's text-embedding-3-small has been removed to avoid external
	// dependencies and wasted API round-trips on non-OpenAI providers.
	var semanticMatcher *tools.SemanticMatcher

	// Create state manager using default agent's workspace for channel recording
	defaultAgent := registry.GetDefaultAgent()
	var stateManager *state.Manager
	if defaultAgent != nil {
		stateManager = state.NewManager(defaultAgent.Workspace)
	}

	planMgr := tools.NewPlanManager()
	// Persist plans to workspace so they survive restarts
	if defaultAgent != nil {
		planPath := filepath.Join(defaultAgent.Workspace, "plans.json")
		planMgr.SetPersistPath(planPath)
		if err := planMgr.Load(planPath); err != nil {
			logger.WarnCF("agent", "Failed to load saved plans", map[string]any{"error": err.Error()})
		}
	}
	scratchpad := tools.NewSharedScratchpad()
	checkpointMgr := checkpoint.NewManager(memDB)
	a2aRouter := NewA2ARouter()

	// Register all agents with the A2A router
	for _, id := range registry.ListAgentIDs() {
		a2aRouter.Register(id)
	}

	pushService := notifications.NewPushService("Sofia")
	if cfg.WebUI.Enabled && cfg.WebUI.Port > 0 {
		host := cfg.WebUI.Host
		if host == "" || host == "0.0.0.0" {
			host = "localhost"
		}
		pushService.SetOpenURL(fmt.Sprintf("http://%s:%d", host, cfg.WebUI.Port))
	}

	// Set up Tool Performance Tracker
	toolStatsPath := filepath.Join(filepath.Dir(memDBPath), "tool_stats.json")
	toolTracker := tools.NewToolTracker(toolStatsPath)

	// Set up Audit Logger for tool call tracing
	auditDBPath := filepath.Join(filepath.Dir(memDBPath), "audit.db")
	auditLog, auditErr := audit.NewAuditLogger(auditDBPath)
	if auditErr != nil {
		logger.WarnCF("agent", "Failed to open audit logger",
			map[string]any{"path": auditDBPath, "error": auditErr.Error()})
	}

	// Set up budget manager with SQLite persistence.
	var budgetMgr *budget.BudgetManager
	{
		var opts []func(*budget.BudgetManager)
		if memDB != nil {
			if bStore, bErr := budget.NewSQLiteStore(memDB.DB()); bErr != nil {
				logger.WarnCF("agent", "Failed to create budget store",
					map[string]any{"error": bErr.Error()})
			} else {
				opts = append(opts, budget.WithStore(bStore))
			}
		}
		budgetMgr = budget.NewBudgetManager(nil, opts...)
	}

	// Build persona definitions from config.
	personaMap := make(map[string]*Persona, len(cfg.Agents.Defaults.Personas))
	for name, pc := range cfg.Agents.Defaults.Personas {
		personaMap[name] = &Persona{
			Name:         name,
			SystemPrompt: pc.SystemPrompt,
			Model:        pc.Model,
			AllowedTools: pc.AllowedTools,
			Description:  pc.Description,
		}
	}

	al := &AgentLoop{
		bus:              msgBus,
		cfg:              cfg,
		registry:         registry,
		state:            stateManager,
		memDB:            memDB,
		summarizing:      sync.Map{},
		fallback:         fallbackChain,
		planManager:      planMgr,
		scratchpad:       scratchpad,
		semanticMatcher:  semanticMatcher,
		checkpointMgr:    checkpointMgr,
		a2aRouter:        a2aRouter,
		rpmCounts:        make(map[string]int),
		rpmResetTime:     make(map[string]time.Time),
		tokenCounts:      make(map[string]int),
		tokenResetTime:   make(map[string]time.Time),
		goalRestartTimes: make(map[int64]time.Time),
		autonomyServices: make(map[string]*autonomy.Service),
		pushService:      pushService,
		dashboardHub:     dashboard.NewHub(),
		toolTracker:      toolTracker,
		budgetManager:    budgetMgr,
		auditLogger:      auditLog,
		usageTracker:     NewUsageTracker(),
		elevatedMgr:      NewElevatedManager(),
		personaManager:   NewPersonaManager(personaMap),
		branchManager:    session.NewBranchManager(),
		approvalGate:     NewApprovalGate(cfg.Guardrails.Approval),
		tracer:           trace.NewTracer(memDB),
		providerRanker:   providers.NewProviderRanker(memDB),
		directCancels:    make(map[string]context.CancelFunc),
		subagentSem:      makeSubagentSem(cfg.Agents.Defaults.MaxConcurrentSubagents),
		torService:       tor.New(cfg.Tools.Web.Proxy),
	}

	// Set degraded mode flag if memory database failed to open
	if memDB == nil {
		al.degradedMode.Store(true)
	}

	// Initialize tool result deduplication cache (30 second TTL by default)
	al.toolResultCacheTTL = 30 * time.Second

	al.a2aRouter.SetMonitorCallback(func(msg *A2AMessage) {
		al.dashboardHub.Broadcast(map[string]any{
			"type":    "a2a_message",
			"from":    msg.From,
			"to":      msg.To,
			"subject": msg.Subject,
			"payload": msg.Payload,
			"msgType": msg.Type,
		})
	})

	// Ensure Playwright browser binaries are installed.
	// This is a no-op if they are already present. Timeout prevents
	// goroutine leak if download hangs.
	pwCtx, pwCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	al.playwrightCancel = pwCancel
	go func() {
		defer pwCancel()
		installDone := make(chan error, 1)
		go func() {
			installDone <- playwright.Install(
				&playwright.RunOptions{
					Browsers: []string{"chromium"},
				},
			)
		}()
		select {
		case err := <-installDone:
			if err != nil {
				logger.WarnCF("agent",
					"Playwright browser install failed "+
						"(web_browse may not work)",
					map[string]any{"error": err.Error()})
			} else {
				logger.InfoCF("agent",
					"Playwright chromium ready", nil)
			}
		case <-pwCtx.Done():
			logger.WarnCF("agent",
				"Playwright install canceled or timed out", nil)
		}
	}()

	al.startAutonomyServices(provider, pushService)

	// Register shared tools to all agents.
	registerSharedTools(
		cfg,
		msgBus,
		registry,
		al.runSpawnedTaskAsAgent,
		planMgr,
		scratchpad,
		checkpointMgr,
		memDB,
		a2aRouter,
		toolTracker,
		al.torService,
	)

	al.activeAgentID.Store("")
	al.activeStatus.Store("Idle")

	// Evolution: restore dynamic agents from store
	agentStore := evolution.NewAgentStore(memDB)
	if retiredIDs, err := agentStore.ListRetired(); err == nil {
		for _, id := range retiredIDs {
			_ = registry.RemoveAgent(id)
		}
	}
	if activeAgents, err := agentStore.ListActive(); err == nil {
		for _, aCfg := range activeAgents {
			inst := newAgentInstanceFromEvolution(aCfg, cfg, provider, memDB)
			if inst != nil {
				if err := registry.RegisterAgent(inst); err != nil {
					logger.WarnCF("agent", "Failed to restore evolution agent",
						map[string]any{"agent_id": aCfg.ID, "error": err.Error()})
				}
			}
		}
	}

	// Start evolution engine if enabled
	if cfg.Evolution.Enabled {
		repMgr := reputation.NewManager(memDB)
		changelogWriter := evolution.NewChangelogWriter(memDB)
		perfTracker := evolution.NewPerformanceTracker(repMgr, &cfg.Evolution)

		historyDir := filepath.Join(filepath.Dir(memDBPath), "evolution", "history")
		safeModifier := evolution.NewSafeModifier(
			historyDir, cfg.Evolution.ImmutableFiles, provider,
		)

		// Resolve the model for the evolution engine: config override > main agent > defaults.
		evoModel := cfg.Agents.Defaults.GetModelName()
		if mainAgent, ok := registry.GetAgent("main"); ok && mainAgent.Model != "" {
			evoModel = mainAgent.Model
		}
		if cfg.Evolution.Model != "" {
			evoModel = cfg.Evolution.Model
		}

		architect := evolution.NewAgentArchitect(
			provider, evoModel, registry, a2aRouter, agentStore, memDB,
			cfg.Agents.Defaults.Workspace,
		)

		var toolStats evolution.ToolStatsProvider
		if al.toolTracker != nil {
			toolStats = &toolStatsAdapter{tracker: al.toolTracker}
		}

		al.evolutionEngine = evolution.NewEvolutionEngine(
			provider, evoModel, memDB, registry, a2aRouter, toolStats,
			repMgr, agentStore, changelogWriter, perfTracker, architect,
			safeModifier, &cfg.Evolution, msgBus,
		)
	}

	return al
}

// toolStatsAdapter wraps *tools.ToolTracker to satisfy evolution.ToolStatsProvider.
// The evolution interface expects map[string]any so it stays decoupled from tools.
type toolStatsAdapter struct {
	tracker *tools.ToolTracker
}

func (a *toolStatsAdapter) GetStats() map[string]any {
	raw := a.tracker.GetStats()
	result := make(map[string]any, len(raw))
	for name, stat := range raw {
		result[name] = stat.ErrorCount
	}
	return result
}

// newAgentInstanceFromEvolution creates a minimal AgentInstance from an
// EvolutionAgentConfig. This is used to restore dynamically created agents
// on startup without requiring the full NewAgentInstance setup path.
func newAgentInstanceFromEvolution(
	aCfg evolution.EvolutionAgentConfig,
	cfg *config.Config,
	provider providers.LLMProvider,
	memDB *memory.MemoryDB,
) *AgentInstance {
	agentCfg := config.AgentConfig{
		ID:     aCfg.ID,
		Name:   aCfg.Name,
		Skills: aCfg.Skills,
	}
	if aCfg.Model != nil {
		agentCfg.Model = aCfg.Model
	} else if aCfg.ModelID != "" {
		agentCfg.Model = &config.AgentModelConfig{Primary: aCfg.ModelID}
	}
	return NewAgentInstance(&agentCfg, &cfg.Agents.Defaults, cfg, provider, memDB, nil)
}
