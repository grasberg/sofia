// Sofia - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Sofia contributors

package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/playwright-community/playwright-go"

	"github.com/grasberg/sofia/pkg/audit"
	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/budget"
	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/channels"
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

type AgentLoop struct {
	bus             *bus.MessageBus
	cfg             *config.Config
	registry        *AgentRegistry
	registryMu      sync.RWMutex
	configMu        sync.Mutex // protects cfg.Agents.List mutations during dynamic agent creation
	state           *state.Manager
	memDB           *memory.MemoryDB
	running         atomic.Bool
	degradedMode    atomic.Bool // set when critical components fail to initialize
	summarizing     sync.Map
	fallback        *providers.FallbackChain
	channelManager  *channels.Manager
	activeAgentID   atomic.Value // string
	activeStatus    atomic.Value // string
	planManager     *tools.PlanManager
	scratchpad      *tools.SharedScratchpad
	checkpointMgr   *checkpoint.Manager
	a2aRouter       *A2ARouter
	semanticMatcher *tools.SemanticMatcher

	// Rate limiting state
	rlMutex        sync.Mutex
	rpmCounts      map[string]int       // AgentID -> requests this minute
	rpmResetTime   map[string]time.Time // AgentID -> next reset time
	tokenCounts    map[string]int       // AgentID -> tokens this hour
	tokenResetTime map[string]time.Time // AgentID -> next reset time

	autonomyMu       sync.Mutex // protects autonomyServices map
	autonomyServices map[string]*autonomy.Service
	pushService      *notifications.PushService
	dashboardHub     *dashboard.Hub
	toolTracker      *tools.ToolTracker
	budgetManager    *budget.BudgetManager
	auditLogger      *audit.AuditLogger
	evolutionEngine  *evolution.EvolutionEngine
	usageTracker     *UsageTracker
	verboseMode      sync.Map // sessionKey -> bool
	thinkingLevel    sync.Map // sessionKey -> ThinkingLevel
	elevatedMgr      *ElevatedManager
	torService       *tor.Service
	personaManager   *PersonaManager
	branchManager    *session.BranchManager
	approvalGate     *ApprovalGate

	agentModelMu sync.RWMutex // protects defaultAgent.Model writes/reads

	dispatchWg  sync.WaitGroup // tracks goroutines from dispatchPendingSteps
	subagentSem chan struct{}  // limits concurrent subagent tasks

	evolveRunning atomic.Bool // prevents duplicate /evolve run goroutines

	// Tool result deduplication cache
	toolResultCache    sync.Map // key: "toolName:argsHash" → *cacheEntry
	toolResultCacheTTL time.Duration

	tracer         *trace.Tracer             // structured execution tracing
	providerRanker *providers.ProviderRanker // adaptive provider ranking

	// processCancelMu protects processCancel
	processCancelMu sync.Mutex
	processCancel   context.CancelFunc // cancels the current in-flight LLM processing

	// killed is set by Reset() to immediately abort all processing.
	// Checked at the top of every processing entry point and at each
	// LLM iteration boundary.
	killed atomic.Bool

	// directCancelsMu protects directCancels — tracks cancel funcs for
	// in-flight ProcessDirect / ProcessDirectWithImages calls so Reset()
	// can cancel them.
	directCancelsMu sync.Mutex
	directCancels   map[string]context.CancelFunc

	playwrightCancel context.CancelFunc // cancels playwright install goroutine
}

// makeSubagentSem creates a buffered channel used as a semaphore to limit
// concurrent subagent tasks. A value <= 0 means unlimited.
func makeSubagentSem(max int) chan struct{} {
	if max <= 0 {
		return nil // no limit
	}
	return make(chan struct{}, max)
}

// processOptions configures how a message is processed
type processOptions struct {
	SessionKey      string      // Session identifier for history/context
	Channel         string      // Target channel for tool execution
	ChatID          string      // Target chat ID for tool execution
	UserMessage     string      // User message content (may include prefix)
	UserImages      []string    // Optional base64 data URLs for vision (e.g. "data:image/png;base64,...")
	DefaultResponse string      // Response when LLM returns empty
	EnableSummary   bool        // Whether to trigger summarization
	SendResponse    bool        // Whether to send response via bus
	NoHistory       bool        // If true, don't load session history (for heartbeat)
	ModelOverride   string      // If set, use this model alias instead of the agent's default
	ParentSpan      *trace.Span // Parent trace span for hierarchical tracing
	Ephemeral       bool        // If true, the exchange is not stored in session history
}

const defaultResponse = "I've completed processing but have no response to give. Increase `max_tool_iterations` in config.json."

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

func (al *AgentLoop) Run(ctx context.Context) error {
	al.running.Store(true)

	if al.evolutionEngine != nil {
		if err := al.evolutionEngine.Start(ctx); err != nil {
			logger.WarnCF("agent", "Failed to start evolution engine",
				map[string]any{"error": err.Error()})
		}
	}

	// Start the plan task dispatcher — auto-assigns pending plan steps to subagents
	go al.runPlanDispatcher(ctx)

	for al.running.Load() {
		select {
		case <-ctx.Done():
			return nil
		default:
			msg, ok := al.bus.ConsumeInbound(ctx)
			if !ok {
				continue
			}

			procCtx, procCancel := context.WithCancel(ctx)
			al.processCancelMu.Lock()
			al.processCancel = procCancel
			al.processCancelMu.Unlock()

			response, err := al.processMessage(procCtx, msg)

			al.processCancelMu.Lock()
			al.processCancel = nil
			al.processCancelMu.Unlock()
			procCancel() // ensure cleanup
			if err != nil {
				response = fmt.Sprintf("Error processing message: %v", err)
			}

			if response != "" {
				// Check if the message tool already sent a response during this round.
				// If so, skip publishing to avoid duplicate messages to the user.
				// Use default agent's tools to check (message tool is shared).
				alreadySent := false
				defaultAgent := al.getRegistry().GetDefaultAgent()
				if defaultAgent != nil {
					if tool, ok := defaultAgent.Tools.Get("message"); ok {
						if mt, ok := tool.(*tools.MessageTool); ok {
							alreadySent = mt.HasSentInRound()
						}
					}
				}

				if !alreadySent {
					al.bus.PublishOutbound(bus.OutboundMessage{
						Channel: msg.Channel,
						ChatID:  msg.ChatID,
						Content: response,
					})
				}
			}
		}
	}

	return nil
}

func (al *AgentLoop) Stop() {
	al.running.Store(false)
	if al.playwrightCancel != nil {
		al.playwrightCancel()
	}
	if al.evolutionEngine != nil {
		al.evolutionEngine.Stop()
	}
	al.stopAutonomyServices()
	if al.tracer != nil {
		al.tracer.Close()
	}

	// Wait for dispatched goroutines with a timeout
	done := make(chan struct{})
	go func() {
		al.dispatchWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		logger.WarnCF("agent", "Timed out waiting for dispatched goroutines to finish", nil)
	}
}

// Reset cancels any in-flight processing, clears all sessions, and resets all goals.
// The agent loop continues running and is ready for new messages.
func (al *AgentLoop) Reset() map[string]any {
	result := map[string]any{}

	// ── KILLSWITCH: set killed flag so every processing path aborts immediately ──
	al.killed.Store(true)
	logger.InfoCF("agent", "Reset: KILLSWITCH activated — aborting all work", nil)

	// 1. Cancel in-flight bus-driven processing
	al.processCancelMu.Lock()
	if al.processCancel != nil {
		al.processCancel()
		al.processCancel = nil
	}
	al.processCancelMu.Unlock()

	// 2. Cancel all in-flight ProcessDirect calls (Web UI, cron, heartbeat)
	al.directCancelsMu.Lock()
	directCancelled := len(al.directCancels)
	for key, cancel := range al.directCancels {
		cancel()
		delete(al.directCancels, key)
	}
	al.directCancelsMu.Unlock()
	result["direct_calls_canceled"] = directCancelled

	// 3. Stop all autonomy services (background goal pursuit, proactive suggestions)
	al.stopAutonomyServices()
	result["autonomy_stopped"] = true

	// 4. Stop the evolution engine
	if al.evolutionEngine != nil {
		al.evolutionEngine.Stop()
	}
	result["evolution_stopped"] = true

	// 5. Wait for dispatched plan goroutines to finish (with timeout)
	done := make(chan struct{})
	go func() {
		al.dispatchWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		logger.WarnCF("agent", "Reset: timed out waiting for plan dispatchers", nil)
	}

	// 6. Drain queued inbound messages so they don't fire after reset
	drained := 0
	for {
		select {
		case <-al.bus.InboundChan():
			drained++
		default:
			goto busDrained
		}
	}
busDrained:
	result["messages_drained"] = drained

	// 7. Clear all sessions for all agents
	sessionsCleared := 0
	for _, agentID := range al.getRegistry().ListAgentIDs() {
		if agent, ok := al.getRegistry().GetAgent(agentID); ok && agent.Sessions != nil {
			for _, meta := range agent.Sessions.ListSessions() {
				if err := agent.Sessions.DeleteSession(meta.Key); err == nil {
					sessionsCleared++
				}
			}
		}
	}
	result["sessions_cleared"] = sessionsCleared

	// 8. Reset all active goals
	goalsReset := 0
	if al.memDB != nil {
		gm := autonomy.NewGoalManager(al.memDB)
		for _, agentID := range al.getRegistry().ListAgentIDs() {
			goals, err := gm.ListAllGoals(agentID)
			if err != nil {
				continue
			}
			for _, g := range goals {
				if g.Status == autonomy.GoalStatusActive || g.Status == autonomy.GoalStatusPaused {
					if _, err := gm.UpdateGoalStatus(g.ID, autonomy.GoalStatusCompleted); err == nil {
						goalsReset++
					}
				}
			}
		}
	}
	result["goals_reset"] = goalsReset

	// 9. Clear active plan
	if al.planManager != nil {
		al.planManager.ClearPlan()
	}
	result["plan_cleared"] = true

	// 10. Reset status
	al.activeStatus.Store("Idle")
	al.activeAgentID.Store("")

	// ── Lift the killswitch so the system can accept new work ──
	al.killed.Store(false)

	logger.InfoCF("agent", "Reset: KILLSWITCH complete — system ready", result)
	return result
}

// getRegistry returns the current agent registry with proper synchronization.
func (al *AgentLoop) getRegistry() *AgentRegistry {
	al.registryMu.RLock()
	defer al.registryMu.RUnlock()
	return al.registry
}

func (al *AgentLoop) RegisterTool(tool tools.Tool) {
	for _, agentID := range al.getRegistry().ListAgentIDs() {
		if agent, ok := al.getRegistry().GetAgent(agentID); ok {
			agent.Tools.Register(tool)
		}
	}
}

func (al *AgentLoop) SetChannelManager(cm *channels.Manager) {
	al.channelManager = cm
}

// ReloadAgents reloads the agent registry and shared tools from the current config.
func (al *AgentLoop) ReloadAgents() {
	logger.InfoCF("agent", "Reloading agents from config", nil)

	// Create a new provider from the updated config every time.
	// This ensures changes to the default model or provider keys take effect immediately
	// without requiring a full process restart.
	provider, _, err := providers.CreateProvider(al.cfg)
	if err != nil {
		logger.ErrorCF("agent", "Cannot reload agents: provider creation failed", map[string]any{"error": err.Error()})
		// Fallback to existing provider if creation fails, so we don't crash
		if defaultAgent := al.getRegistry().GetDefaultAgent(); defaultAgent != nil {
			provider = defaultAgent.Provider
		}
	} else if provider == nil {
		logger.WarnCF("agent", "Cannot reload agents: no model configured", nil)
		// Fallback to existing
		if defaultAgent := al.getRegistry().GetDefaultAgent(); defaultAgent != nil {
			provider = defaultAgent.Provider
		}
	} else {
		logger.InfoCF("agent", "Created provider from updated config",
			map[string]any{"model": al.cfg.Agents.Defaults.GetModelName()})
	}

	newRegistry := NewAgentRegistry(al.cfg, provider, al.memDB)

	// Re-register new agents with the A2A router
	for _, id := range newRegistry.ListAgentIDs() {
		al.a2aRouter.Register(id)
	}

	toolStatsPath := filepath.Join(filepath.Dir(al.memDB.Path()), "tool_stats.json")
	var newToolTracker *tools.ToolTracker
	if al.registry != nil {
		newToolTracker = tools.NewToolTracker(toolStatsPath)
	}

	registerSharedTools(
		al.cfg,
		al.bus,
		newRegistry,
		al.runSpawnedTaskAsAgent,
		al.planManager,
		al.scratchpad,
		al.checkpointMgr,
		al.memDB,
		al.a2aRouter,
		newToolTracker,
		al.torService,
	)

	al.registryMu.Lock()
	al.registry = newRegistry
	al.toolTracker = newToolTracker
	al.registryMu.Unlock()

	al.stopAutonomyServices()
	al.startAutonomyServices(provider, al.pushService)

	logger.InfoCF("agent", "Agents reloaded successfully", nil)
}

// GetEvolutionEngine returns the evolution engine instance (may be nil).
func (al *AgentLoop) GetEvolutionEngine() *evolution.EvolutionEngine {
	return al.evolutionEngine
}

// TorService returns the Tor anonymity service used by agent web tools.
func (al *AgentLoop) TorService() *tor.Service {
	return al.torService
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

