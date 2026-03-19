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

	"github.com/grasberg/sofia/pkg/autonomy"
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
)

type AgentLoop struct {
	bus             *bus.MessageBus
	cfg             *config.Config
	registry        *AgentRegistry
	registryMu      sync.RWMutex
	state           *state.Manager
	memDB           *memory.MemoryDB
	running         atomic.Bool
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

	autonomyServices map[string]*autonomy.Service
	pushService      *notifications.PushService
	dashboardHub     *dashboard.Hub
	toolTracker      *tools.ToolTracker
	evolutionEngine  *evolution.EvolutionEngine
	usageTracker     *UsageTracker
	verboseMode      sync.Map // sessionKey -> bool
	thinkingLevel    sync.Map // sessionKey -> ThinkingLevel
	elevatedMgr      *ElevatedManager
	personaManager   *PersonaManager
	branchManager    *session.BranchManager
	approvalGate     *ApprovalGate

	// processCancelMu protects processCancel
	processCancelMu sync.Mutex
	processCancel   context.CancelFunc // cancels the current in-flight LLM processing
}

// processOptions configures how a message is processed
type processOptions struct {
	SessionKey      string   // Session identifier for history/context
	Channel         string   // Target channel for tool execution
	ChatID          string   // Target chat ID for tool execution
	UserMessage     string   // User message content (may include prefix)
	UserImages      []string // Optional base64 data URLs for vision (e.g. "data:image/png;base64,...")
	DefaultResponse string   // Response when LLM returns empty
	EnableSummary   bool     // Whether to trigger summarization
	SendResponse    bool     // Whether to send response via bus
	NoHistory       bool     // If true, don't load session history (for heartbeat)
	ModelOverride   string   // If set, use this model alias instead of the agent's default
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
		// Non-fatal: log and continue without persistence.
		logger.ErrorCF("agent", "Failed to open memory database",
			map[string]any{"path": memDBPath, "error": err.Error()})
	}

	registry := NewAgentRegistry(cfg, provider, memDB)

	// Set up shared fallback chain
	cooldown := providers.NewCooldownTracker()
	fallbackChain := providers.NewFallbackChain(cooldown)

	// Set up semantic tool matcher if provider supports embeddings
	var semanticMatcher *tools.SemanticMatcher
	if embProvider, ok := provider.(providers.EmbeddingProvider); ok {
		semanticMatcher = tools.NewSemanticMatcher(embProvider, "text-embedding-3-small") // Defaulting for OpenAI-compat
	}

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

	// Set up Tool Performance Tracker
	toolStatsPath := filepath.Join(filepath.Dir(memDBPath), "tool_stats.json")
	toolTracker := tools.NewToolTracker(toolStatsPath)

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
		usageTracker:     NewUsageTracker(),
		elevatedMgr:      NewElevatedManager(),
		personaManager:   NewPersonaManager(personaMap),
		branchManager:    session.NewBranchManager(),
		approvalGate:     NewApprovalGate(cfg.Guardrails.Approval),
	}

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
	go func() {
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
		case <-time.After(5 * time.Minute):
			logger.WarnCF("agent",
				"Playwright install timed out after 5m", nil)
		}
	}()

	al.startAutonomyServices(provider, pushService)

	// Register shared tools to all agents.
	registerSharedTools(cfg, msgBus, registry, provider, al.runSpawnedTaskAsAgent, planMgr, scratchpad, checkpointMgr, memDB, a2aRouter, pushService, toolTracker)

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

		architect := evolution.NewAgentArchitect(
			provider, registry, a2aRouter, agentStore, memDB,
			cfg.Agents.Defaults.Workspace,
		)

		var toolStats evolution.ToolStatsProvider
		if al.toolTracker != nil {
			toolStats = &toolStatsAdapter{tracker: al.toolTracker}
		}

		al.evolutionEngine = evolution.NewEvolutionEngine(
			provider, memDB, registry, a2aRouter, toolStats,
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
	if al.evolutionEngine != nil {
		al.evolutionEngine.Stop()
	}
	al.stopAutonomyServices()
}

// Reset cancels any in-flight processing, clears all sessions, and resets all goals.
// The agent loop continues running and is ready for new messages.
func (al *AgentLoop) Reset() map[string]any {
	result := map[string]any{}

	// 1. Cancel in-flight processing
	al.processCancelMu.Lock()
	if al.processCancel != nil {
		al.processCancel()
		al.processCancel = nil
	}
	al.processCancelMu.Unlock()
	result["processing_cancelled"] = true
	logger.InfoCF("agent", "Reset: cancelled in-flight processing", nil)

	// 2. Clear all sessions for all agents
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
	logger.InfoCF("agent", fmt.Sprintf("Reset: cleared %d sessions", sessionsCleared), nil)

	// 3. Reset all active goals
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
	logger.InfoCF("agent", fmt.Sprintf("Reset: reset %d goals", goalsReset), nil)

	// 4. Clear active plan
	if al.planManager != nil {
		al.planManager.ClearPlan()
	}
	result["plan_cleared"] = true

	// 5. Reset status
	al.activeStatus.Store("Idle")
	al.activeAgentID.Store("")

	logger.InfoCF("agent", "Reset: complete", result)
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
	var toolTracker *tools.ToolTracker
	if al.registry != nil {
		toolTracker = tools.NewToolTracker(toolStatsPath)
	}

	registerSharedTools(al.cfg, al.bus, newRegistry, provider, al.runSpawnedTaskAsAgent, al.planManager, al.scratchpad, al.checkpointMgr, al.memDB, al.a2aRouter, al.pushService, toolTracker)

	al.registryMu.Lock()
	al.registry = newRegistry
	al.registryMu.Unlock()

	al.stopAutonomyServices()
	al.startAutonomyServices(provider, al.pushService)

	logger.InfoCF("agent", "Agents reloaded successfully", nil)
}

// GetEvolutionEngine returns the evolution engine instance (may be nil).
func (al *AgentLoop) GetEvolutionEngine() *evolution.EvolutionEngine {
	return al.evolutionEngine
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
