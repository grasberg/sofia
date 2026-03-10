// Sofia - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Sofia contributors

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/playwright-community/playwright-go"
	"golang.org/x/sync/errgroup"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/channels"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/constants"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/routing"
	"github.com/grasberg/sofia/pkg/session"
	"github.com/grasberg/sofia/pkg/skills"
	"github.com/grasberg/sofia/pkg/state"
	"github.com/grasberg/sofia/pkg/tools"
	"github.com/grasberg/sofia/pkg/utils"
)

type AgentLoop struct {
	bus            *bus.MessageBus
	cfg            *config.Config
	registry       *AgentRegistry
	registryMu     sync.RWMutex
	state          *state.Manager
	memDB          *memory.MemoryDB
	running        atomic.Bool
	summarizing    sync.Map
	fallback       *providers.FallbackChain
	channelManager *channels.Manager
	activeAgentID  atomic.Value // string
	activeStatus   atomic.Value // string
	planManager    *tools.PlanManager
	scratchpad     *tools.SharedScratchpad
	a2aRouter      *A2ARouter

	// Rate limiting state
	rlMutex        sync.Mutex
	rpmCounts      map[string]int       // AgentID -> requests this minute
	rpmResetTime   map[string]time.Time // AgentID -> next reset time
	tokenCounts    map[string]int       // AgentID -> tokens this hour
	tokenResetTime map[string]time.Time // AgentID -> next reset time
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
}

const defaultResponse = "I've completed processing but have no response to give. Increase `max_tool_iterations` in config.json."

func NewAgentLoop(cfg *config.Config, msgBus *bus.MessageBus, provider providers.LLMProvider) *AgentLoop {
	// Open the shared SQLite memory database.
	// Default path: ~/.sofia/memory.db (configurable via cfg.MemoryDB).
	memDBPath := cfg.MemoryDB
	if memDBPath == "" {
		home, _ := os.UserHomeDir()
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

	// Create state manager using default agent's workspace for channel recording
	defaultAgent := registry.GetDefaultAgent()
	var stateManager *state.Manager
	if defaultAgent != nil {
		stateManager = state.NewManager(defaultAgent.Workspace)
	}

	planMgr := tools.NewPlanManager()
	scratchpad := tools.NewSharedScratchpad()
	a2aRouter := NewA2ARouter()

	// Register all agents with the A2A router
	for _, id := range registry.ListAgentIDs() {
		a2aRouter.Register(id)
	}

	al := &AgentLoop{
		bus:            msgBus,
		cfg:            cfg,
		registry:       registry,
		state:          stateManager,
		memDB:          memDB,
		summarizing:    sync.Map{},
		fallback:       fallbackChain,
		planManager:    planMgr,
		scratchpad:     scratchpad,
		a2aRouter:      a2aRouter,
		rpmCounts:      make(map[string]int),
		rpmResetTime:   make(map[string]time.Time),
		tokenCounts:    make(map[string]int),
		tokenResetTime: make(map[string]time.Time),
	}

	// Ensure Playwright browser binaries are installed.
	// This is a no-op if they are already present.
	go func() {
		if err := playwright.Install(&playwright.RunOptions{Browsers: []string{"chromium"}}); err != nil {
			logger.WarnCF("agent", "Playwright browser install failed (web_browse may not work)",
				map[string]any{"error": err.Error()})
		} else {
			logger.InfoCF("agent", "Playwright chromium ready", nil)
		}
	}()

	// Register shared tools to all agents.
	registerSharedTools(cfg, msgBus, registry, provider, al.runSpawnedTaskAsAgent, planMgr, scratchpad, memDB, a2aRouter)

	al.activeAgentID.Store("")
	al.activeStatus.Store("Idle")
	return al
}

// registerSharedTools registers tools that are shared across all agents (web, message, spawn, plan, scratchpad, orchestrate, a2a).
func registerSharedTools(
	cfg *config.Config,
	msgBus *bus.MessageBus,
	registry *AgentRegistry,
	provider providers.LLMProvider,
	agentTaskRunner func(ctx context.Context, agentID, sessionKey, task, originChannel, originChatID string) (string, error),
	planMgr *tools.PlanManager,
	scratchpad *tools.SharedScratchpad,
	memDB *memory.MemoryDB,
	a2aRouter *A2ARouter,
) {
	for _, agentID := range registry.ListAgentIDs() {
		agent, ok := registry.GetAgent(agentID)
		if !ok {
			continue
		}

		// Web tools
		if searchTool := tools.NewWebSearchTool(tools.WebSearchToolOptions{
			BraveAPIKey:          cfg.Tools.Web.Brave.APIKey,
			BraveMaxResults:      cfg.Tools.Web.Brave.MaxResults,
			BraveEnabled:         cfg.Tools.Web.Brave.Enabled,
			TavilyAPIKey:         cfg.Tools.Web.Tavily.APIKey,
			TavilyBaseURL:        cfg.Tools.Web.Tavily.BaseURL,
			TavilyMaxResults:     cfg.Tools.Web.Tavily.MaxResults,
			TavilyEnabled:        cfg.Tools.Web.Tavily.Enabled,
			DuckDuckGoMaxResults: cfg.Tools.Web.DuckDuckGo.MaxResults,
			DuckDuckGoEnabled:    cfg.Tools.Web.DuckDuckGo.Enabled,
			PerplexityAPIKey:     cfg.Tools.Web.Perplexity.APIKey,
			PerplexityMaxResults: cfg.Tools.Web.Perplexity.MaxResults,
			PerplexityEnabled:    cfg.Tools.Web.Perplexity.Enabled,
			Proxy:                cfg.Tools.Web.Proxy,
		}); searchTool != nil {
			agent.Tools.Register(searchTool)
		}
		agent.Tools.Register(tools.NewWebFetchToolWithProxy(50000, cfg.Tools.Web.Proxy))
		agent.Tools.Register(tools.NewWebBrowseTool(tools.BrowseToolOptions{
			Headless:       cfg.Tools.Web.Browser.Headless,
			TimeoutSeconds: cfg.Tools.Web.Browser.TimeoutSeconds,
			BrowserType:    cfg.Tools.Web.Browser.BrowserType,
			ScreenshotDir:  cfg.Tools.Web.Browser.ScreenshotDir,
			Workspace:      agent.Workspace,
		}))

		if cfg.Tools.Google.Enabled {
			agent.Tools.Register(tools.NewGoogleCLITool(
				cfg.Tools.Google.BinaryPath,
				cfg.Tools.Google.TimeoutSeconds,
				cfg.Tools.Google.AllowedCommands,
			))
		}

		// Hardware tools (I2C, SPI) - Linux only, returns error on other platforms
		agent.Tools.Register(tools.NewI2CTool())
		agent.Tools.Register(tools.NewSPITool())

		// Computer use tool - macOS/Linux only; uses vision LLM for autonomous desktop control
		agent.Tools.Register(tools.NewComputerUseTool(tools.ComputerUseOptions{
			Workspace: agent.Workspace,
			Provider:  agent.Provider,
			ModelID:   agent.ModelID,
		}))

		// Message tool
		messageTool := tools.NewMessageTool()
		messageTool.SetSendCallback(func(channel, chatID, content string) error {
			msgBus.PublishOutbound(bus.OutboundMessage{
				Channel: channel,
				ChatID:  chatID,
				Content: content,
			})
			return nil
		})
		agent.Tools.Register(messageTool)

		// Skill discovery and installation tools
		registryMgr := skills.NewRegistryManagerFromConfig(skills.RegistryConfig{
			MaxConcurrentSearches: cfg.Tools.Skills.MaxConcurrentSearches,
			ClawHub:               skills.ClawHubConfig(cfg.Tools.Skills.Registries.ClawHub),
		})
		searchCache := skills.NewSearchCache(
			cfg.Tools.Skills.SearchCache.MaxSize,
			time.Duration(cfg.Tools.Skills.SearchCache.TTLSeconds)*time.Second,
		)
		agent.Tools.Register(tools.NewFindSkillsTool(registryMgr, searchCache))
		agent.Tools.Register(tools.NewInstallSkillTool(registryMgr, agent.Workspace))
		agent.Tools.Register(tools.NewCreateSkillTool(agent.Workspace))
		agent.Tools.Register(tools.NewUpdateSkillTool(agent.Workspace))

		// Spawn tool with allowlist checker.
		// Use agent.Provider (not the global provider) so that ad-hoc subagents spawned by
		// this agent inherit the agent's own model/API key rather than Sofia's global one.
		subagentManager := tools.NewSubagentManager(agent.Provider, agent.ModelID, agent.Workspace, msgBus)
		subagentManager.SetLLMOptions(agent.MaxTokens, agent.Temperature)
		subagentManager.SetAgentTaskRunner(agentTaskRunner)
		spawnTool := tools.NewSpawnTool(subagentManager)
		currentAgentID := agentID
		spawnTool.SetAllowlistChecker(func(targetAgentID string) bool {
			return registry.CanSpawnSubagent(currentAgentID, targetAgentID)
		})
		agent.Tools.Register(spawnTool)

		// Plan tool — structured plan-then-execute with template persistence
		agent.Tools.Register(tools.NewPlanTool(planMgr, memDB))

		// Scratchpad — agent-to-agent shared key-value store
		agent.Tools.Register(tools.NewScratchpadTool(scratchpad, "default"))

		// Subagent (synchronous) tool
		subagentTool := tools.NewSubagentTool(subagentManager)
		agent.Tools.Register(subagentTool)

		// Practice tool - self-improving training data generator
		if memDB != nil {
			practiceTool := tools.NewPracticeTool(memDB, subagentManager, agentID)
			agent.Tools.Register(practiceTool)
		}

		// Orchestrate tool — multi-agent task coordination
		orchCfg := tools.OrchestrateToolConfig{
			AgentScorer: func(candidateID, taskDescription string) float64 {
				candidate, ok := registry.GetAgent(candidateID)
				if !ok {
					return 0
				}
				return scoreCandidate(candidate, strings.ToLower(taskDescription))
			},
			ListAgentIDs: registry.ListAgentIDs,
			RunAgentTask: func(ctx context.Context, targetAgentID, task, channel, chatID string) (string, error) {
				return agentTaskRunner(ctx, targetAgentID, "", task, channel, chatID)
			},
			Scratchpad: scratchpad,
		}
		orchTool := tools.NewOrchestrateTool(orchCfg)
		agent.Tools.Register(orchTool)

		// Knowledge Graph — semantic memory with structured entities and relationships
		if memDB != nil {
			agent.Tools.Register(tools.NewKnowledgeGraphTool(memDB, agentID))
			agent.Tools.Register(tools.NewDistillKnowledgeTool(memDB, agentID))
		}

		// A2A — agent-to-agent communication protocol
		if a2aRouter != nil {
			a2aAdapter := tools.NewA2ARouterAdapter(
				func(from, to, msgType, subject, payload, replyTo string) (string, error) {
					msg := &A2AMessage{
						From:    from,
						To:      to,
						Type:    A2AMessageType(msgType),
						Subject: subject,
						Payload: payload,
						ReplyTo: replyTo,
					}
					if err := a2aRouter.Send(msg); err != nil {
						return "", err
					}
					return msg.ID, nil
				},
				a2aRouter.Broadcast,
				func(aid string, timeout time.Duration) *tools.A2AMessageForTool {
					msg := a2aRouter.Receive(aid, timeout)
					if msg == nil {
						return nil
					}
					return &tools.A2AMessageForTool{
						ID: msg.ID, From: msg.From, To: msg.To,
						Type: string(msg.Type), Subject: msg.Subject,
						Payload: msg.Payload, ReplyTo: msg.ReplyTo,
						Timestamp: msg.Timestamp,
					}
				},
				func(aid string) *tools.A2AMessageForTool {
					msg := a2aRouter.Poll(aid)
					if msg == nil {
						return nil
					}
					return &tools.A2AMessageForTool{
						ID: msg.ID, From: msg.From, To: msg.To,
						Type: string(msg.Type), Subject: msg.Subject,
						Payload: msg.Payload, ReplyTo: msg.ReplyTo,
						Timestamp: msg.Timestamp,
					}
				},
				a2aRouter.PendingCount,
			)
			agent.Tools.Register(tools.NewA2ATool(a2aAdapter, agentID))
		}

		// Self-Modification tool
		cwd, _ := os.Getwd()
		agent.Tools.Register(tools.NewSelfModifyTool(cwd))
	}
}

func (al *AgentLoop) Run(ctx context.Context) error {
	al.running.Store(true)

	for al.running.Load() {
		select {
		case <-ctx.Done():
			return nil
		default:
			msg, ok := al.bus.ConsumeInbound(ctx)
			if !ok {
				continue
			}

			response, err := al.processMessage(ctx, msg)
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

	registerSharedTools(al.cfg, al.bus, newRegistry, provider, al.runSpawnedTaskAsAgent, al.planManager, al.scratchpad, al.memDB, al.a2aRouter)

	al.registryMu.Lock()
	al.registry = newRegistry
	al.registryMu.Unlock()
	logger.InfoCF("agent", "Agents reloaded successfully", nil)
}

func (al *AgentLoop) runSpawnedTaskAsAgent(
	ctx context.Context,
	agentID, sessionKey, task, originChannel, originChatID string,
) (string, error) {
	target, ok := al.getRegistry().GetAgent(agentID)
	if !ok || target == nil {
		return "", fmt.Errorf("target agent %q not found", agentID)
	}

	if sessionKey == "" {
		sessionKey = "subagent:" + agentID
	}

	agentComp := fmt.Sprintf("agent:%s", agentID)
	agentName := target.Name
	if agentName == "" {
		agentName = agentID
	}
	taskPreview := utils.Truncate(task, 120)
	logger.InfoCF(agentComp, fmt.Sprintf("SUBAGENT: task started — %s", taskPreview),
		map[string]any{
			"agent_id":     agentID,
			"agent_name":   agentName,
			"model":        target.Model,
			"session_key":  sessionKey,
			"task_preview": taskPreview,
		})

	start := time.Now()
	result, err := al.runAgentLoop(ctx, target, processOptions{
		SessionKey:      sessionKey,
		Channel:         originChannel,
		ChatID:          originChatID,
		UserMessage:     task,
		DefaultResponse: defaultResponse,
		EnableSummary:   false,
		SendResponse:    false,
		NoHistory:       true,
	})
	dur := time.Since(start).Milliseconds()

	if err != nil {
		logger.WarnCF(agentComp, fmt.Sprintf("SUBAGENT: task failed after %dms", dur),
			map[string]any{
				"agent_id":    agentID,
				"agent_name":  agentName,
				"duration_ms": dur,
				"error":       err.Error(),
			})
		return result, err
	}

	logger.InfoCF(agentComp, fmt.Sprintf("SUBAGENT: task completed in %dms", dur),
		map[string]any{
			"agent_id":       agentID,
			"agent_name":     agentName,
			"duration_ms":    dur,
			"result_len":     len(result),
			"result_preview": utils.Truncate(result, 160),
		})
	return result, nil
}

// RecordLastChannel records the last active channel for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChannel(channel string) error {
	if al.state == nil {
		return nil
	}
	return al.state.SetLastChannel(channel)
}

// RecordLastChatID records the last active chat ID for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChatID(chatID string) error {
	if al.state == nil {
		return nil
	}
	return al.state.SetLastChatID(chatID)
}

func (al *AgentLoop) ProcessDirect(ctx context.Context, content, sessionKey string) (string, error) {
	return al.ProcessDirectWithChannel(ctx, content, sessionKey, "cli", "direct")
}

// ProcessDirectWithImages sends a message with optional image attachments directly
// to the default agent, bypassing channel routing. Images must be base64 data URLs.
func (al *AgentLoop) ProcessDirectWithImages(
	ctx context.Context,
	content, sessionKey string,
	images []string,
) (string, error) {
	agent := al.getRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent configured")
	}
	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      sessionKey,
		Channel:         "cli",
		ChatID:          "direct",
		UserMessage:     content,
		UserImages:      images,
		DefaultResponse: defaultResponse,
		EnableSummary:   true,
		SendResponse:    false,
	})
}

func (al *AgentLoop) ProcessDirectWithChannel(
	ctx context.Context,
	content, sessionKey, channel, chatID string,
) (string, error) {
	msg := bus.InboundMessage{
		Channel:    channel,
		SenderID:   "cron",
		ChatID:     chatID,
		Content:    content,
		SessionKey: sessionKey,
	}

	return al.processMessage(ctx, msg)
}

// ProcessHeartbeat processes a heartbeat request without session history.
// Each heartbeat is independent and doesn't accumulate context.
func (al *AgentLoop) ProcessHeartbeat(ctx context.Context, content, channel, chatID string) (string, error) {
	agent := al.getRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent configured")
	}
	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      "heartbeat",
		Channel:         channel,
		ChatID:          chatID,
		UserMessage:     content,
		DefaultResponse: defaultResponse,
		EnableSummary:   false,
		SendResponse:    false,
		NoHistory:       true, // Don't load session history for heartbeat
	})
}

func (al *AgentLoop) processMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	preview := utils.Truncate(msg.Content, 120)
	logger.InfoCF("agent:main", fmt.Sprintf("SOFIA: message received — %s", preview),
		map[string]any{
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
			"sender_id":   msg.SenderID,
			"session_key": msg.SessionKey,
			"preview":     preview,
		})

	// Route system messages to processSystemMessage
	if msg.Channel == "system" {
		return al.processSystemMessage(ctx, msg)
	}

	// Guardrail: Input Validation
	if al.cfg.Guardrails.InputValidation.Enabled {
		if maxLen := al.cfg.Guardrails.InputValidation.MaxMessageLength; maxLen > 0 && len(msg.Content) > maxLen {
			errMsg := fmt.Sprintf("Error: message exceeds maximum allowed length of %d characters.", maxLen)
			logger.WarnCF("agent:main", "Guardrail blocked input: length exceeded", map[string]any{
				"length":     len(msg.Content),
				"max_length": maxLen,
			})
			logger.Audit("Input Validation Blocked", map[string]any{
				"reason":     "length exceeded",
				"length":     len(msg.Content),
				"max_length": maxLen,
				"sender":     msg.SenderID,
				"channel":    msg.Channel,
			})
			return errMsg, nil
		}

		for _, pattern := range al.cfg.Guardrails.InputValidation.DenyPatterns {
			if matched, _ := regexp.MatchString(pattern, msg.Content); matched {
				errMsg := "Error: message blocked by input security policy."
				logger.WarnCF("agent:main", "Guardrail blocked input: pattern match", map[string]any{
					"pattern": pattern,
				})
				logger.Audit("Input Validation Blocked", map[string]any{
					"reason":  "pattern match",
					"pattern": pattern,
					"sender":  msg.SenderID,
					"channel": msg.Channel,
				})
				return errMsg, nil
			}
		}
	}

	// Guardrail: Prompt Injection Defense (Heuristic check)
	if al.cfg.Guardrails.PromptInjection.Enabled {
		injectionPatterns := []string{
			`(?i)ignore (all )?previous instructions`,
			`(?i)ignore (all )?above`,
			`(?i)disregard (all )?previous`,
			`(?i)forget (all )?previous`,
			`(?i)system prompt`,
			`(?i)you are an assistant that`,
			`(?i)new instructions:`,
		}

		for _, pattern := range injectionPatterns {
			if matched, _ := regexp.MatchString(pattern, msg.Content); matched {
				if al.cfg.Guardrails.PromptInjection.Action == "block" {
					errMsg := "Error: input rejected due to potential prompt injection attempt."
					logger.WarnCF("agent:main", "Guardrail blocked input: prompt injection blocked", map[string]any{"pattern": pattern})
					logger.Audit("Prompt Injection Blocked", map[string]any{
						"pattern": pattern,
						"sender":  msg.SenderID,
						"channel": msg.Channel,
					})
					return errMsg, nil
				} else {
					// Warn action - just log it and maybe append a strong warning to the system prompt later
					logger.WarnCF("agent:main", "Guardrail detected potential prompt injection", map[string]any{"pattern": pattern})
					logger.Audit("Prompt Injection Detected (Warn)", map[string]any{
						"pattern": pattern,
						"sender":  msg.SenderID,
						"channel": msg.Channel,
					})
				}
			}
		}
	}

	// Check for commands
	if response, handled := al.handleCommand(ctx, msg); handled {
		return response, nil
	}

	// Route to determine agent and session key
	route := al.getRegistry().ResolveRoute(routing.RouteInput{
		Channel:    msg.Channel,
		AccountID:  msg.Metadata["account_id"],
		Peer:       extractPeer(msg),
		ParentPeer: extractParentPeer(msg),
		GuildID:    msg.Metadata["guild_id"],
		TeamID:     msg.Metadata["team_id"],
	})

	agent, ok := al.getRegistry().GetAgent(route.AgentID)
	if !ok {
		agent = al.getRegistry().GetDefaultAgent()
	}
	if agent == nil {
		return "", fmt.Errorf("no agent available for route %q", route.AgentID)
	}

	al.activeAgentID.Store(agent.ID)
	al.activeStatus.Store("Thinking...")

	// Use routed session key, but honor pre-set agent-scoped keys (for ProcessDirect/cron)
	sessionKey := route.SessionKey
	if msg.SessionKey != "" && strings.HasPrefix(msg.SessionKey, "agent:") {
		sessionKey = msg.SessionKey
	}

	agentComp := fmt.Sprintf("agent:%s", agent.ID)
	logger.InfoCF(agentComp, fmt.Sprintf("ROUTER: routed to agent %q via %s", agent.ID, route.MatchedBy),
		map[string]any{
			"agent_id":    agent.ID,
			"session_key": sessionKey,
			"matched_by":  route.MatchedBy,
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
		})

	// Delegation: first try keyword-based scoring, then fall back to semantic (LLM-based).
	subagent := al.delegateTo(msg.Content)
	if subagent == nil {
		// Semantic delegation fallback: use an LLM call to determine the best agent
		subagent = al.semanticDelegateTo(ctx, msg.Content)
	}
	if subagent != nil {
		score := scoreCandidate(subagent, strings.ToLower(msg.Content))
		logger.InfoCF(
			agentComp,
			fmt.Sprintf(
				"SOFIA: delegating to %q (score=%.2f, threshold=%.2f)",
				subagent.Name,
				score,
				delegationThreshold,
			),
			map[string]any{
				"from_agent": agent.ID,
				"to_agent":   subagent.ID,
				"agent_name": subagent.Name,
				"score":      fmt.Sprintf("%.2f", score),
				"threshold":  fmt.Sprintf("%.2f", delegationThreshold),
				"reason":     "skills+purpose+hint match",
				"preview":    utils.Truncate(msg.Content, 120),
			},
		)
		delegateStart := time.Now()
		subResult, err := al.runSpawnedTaskAsAgent(ctx, subagent.ID, "", msg.Content, msg.Channel, msg.ChatID)
		delegateDur := time.Since(delegateStart).Milliseconds()
		if err != nil {
			logger.WarnCF(
				agentComp,
				fmt.Sprintf("SOFIA: delegation to %q failed — falling back to Sofia", subagent.Name),
				map[string]any{
					"to_agent":    subagent.ID,
					"duration_ms": delegateDur,
					"error":       err.Error(),
				},
			)
		} else {
			logger.InfoCF(agentComp, fmt.Sprintf("SOFIA: sub-agent %q done, synthesizing result", subagent.Name),
				map[string]any{
					"to_agent":       subagent.ID,
					"duration_ms":    delegateDur,
					"result_len":     len(subResult),
					"result_preview": utils.Truncate(subResult, 160),
				})
			// Let Sofia synthesize and present the sub-agent's result to the user.
			synthesisMsg := fmt.Sprintf("[Subagent result from %s]\n\n%s", subagent.Name, subResult)
			return al.runAgentLoop(ctx, agent, processOptions{
				SessionKey:      sessionKey,
				Channel:         msg.Channel,
				ChatID:          msg.ChatID,
				UserMessage:     synthesisMsg,
				DefaultResponse: defaultResponse,
				EnableSummary:   true,
				SendResponse:    false,
			})
		}
	}

	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      sessionKey,
		Channel:         msg.Channel,
		ChatID:          msg.ChatID,
		UserMessage:     msg.Content,
		DefaultResponse: defaultResponse,
		EnableSummary:   true,
		SendResponse:    false,
	})
}

func (al *AgentLoop) processSystemMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	if msg.Channel != "system" {
		return "", fmt.Errorf("processSystemMessage called with non-system message channel: %s", msg.Channel)
	}

	logger.InfoCF("agent", "Processing system message",
		map[string]any{
			"sender_id": msg.SenderID,
			"chat_id":   msg.ChatID,
		})

	// Parse origin channel from chat_id (format: "channel:chat_id")
	var originChannel, originChatID string
	if idx := strings.Index(msg.ChatID, ":"); idx > 0 {
		originChannel = msg.ChatID[:idx]
		originChatID = msg.ChatID[idx+1:]
	} else {
		originChannel = "cli"
		originChatID = msg.ChatID
	}

	// Extract subagent result from message content
	// Format: "Task 'label' completed.\n\nResult:\n<actual content>"
	content := msg.Content
	if idx := strings.Index(content, "Result:\n"); idx >= 0 {
		content = content[idx+8:] // Extract just the result part
	}

	// Skip internal channels - only log, don't send to user
	if constants.IsInternalChannel(originChannel) {
		logger.InfoCF("agent", "Subagent completed (internal channel)",
			map[string]any{
				"sender_id":   msg.SenderID,
				"content_len": len(content),
				"channel":     originChannel,
			})
		return "", nil
	}

	// Use default agent for system messages
	agent := al.getRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent configured")
	}

	// Use the origin session for context
	sessionKey := routing.BuildAgentMainSessionKey(agent.ID)

	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      sessionKey,
		Channel:         originChannel,
		ChatID:          originChatID,
		UserMessage:     fmt.Sprintf("[Subagent result from %s]\n\n%s", msg.SenderID, content),
		DefaultResponse: "Background task completed.",
		EnableSummary:   false,
		SendResponse:    true,
	})
}

// runAgentLoop is the core message processing logic.
func (al *AgentLoop) runAgentLoop(ctx context.Context, agent *AgentInstance, opts processOptions) (string, error) {
	agentComp := fmt.Sprintf("agent:%s", agent.ID)

	// Guard: if no provider is configured, return a friendly message
	if agent.Provider == nil {
		noModelMsg := "No model is configured. Please open the Web UI → Models page and add a model to get started."
		logger.WarnCF(agentComp, noModelMsg, nil)
		if opts.SendResponse && opts.Channel != "" && opts.ChatID != "" {
			al.bus.PublishOutbound(bus.OutboundMessage{
				Channel: opts.Channel,
				ChatID:  opts.ChatID,
				Content: noModelMsg,
			})
		}
		return noModelMsg, nil
	}

	// 0. Record last channel for heartbeat notifications (skip internal channels)
	if opts.Channel != "" && opts.ChatID != "" {
		// Don't record internal channels (cli, system, subagent)
		if !constants.IsInternalChannel(opts.Channel) {
			channelKey := fmt.Sprintf("%s:%s", opts.Channel, opts.ChatID)
			if err := al.RecordLastChannel(channelKey); err != nil {
				logger.WarnCF(agentComp, "Failed to record last channel", map[string]any{"error": err.Error()})
			}
		}
	}

	// 1. Update tool contexts
	al.updateToolContexts(agent, opts.Channel, opts.ChatID)

	// 1b. Signal thinking status to the channel (only if we intend to send a response)
	if opts.SendResponse && opts.Channel != "" && opts.ChatID != "" && !constants.IsInternalChannel(opts.Channel) {
		al.bus.PublishOutbound(bus.OutboundMessage{
			Channel: opts.Channel,
			ChatID:  opts.ChatID,
			Type:    "thinking",
		})
	}

	// 2. Build messages (skip history for heartbeat)
	var history []providers.Message
	var summary string
	if !opts.NoHistory {
		history = agent.Sessions.GetHistory(opts.SessionKey)
		summary = agent.Sessions.GetSummary(opts.SessionKey)
	}
	messages := agent.ContextBuilder.BuildMessages(
		history,
		summary,
		opts.UserMessage,
		nil,
		opts.Channel,
		opts.ChatID,
	)

	// Inject images into the last user message for vision-capable providers
	if len(opts.UserImages) > 0 {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "user" {
				messages[i].Images = opts.UserImages
				break
			}
		}
	}

	// 3. Save user message to session
	agent.Sessions.AddMessage(opts.SessionKey, "user", opts.UserMessage)

	// 4. Run LLM iteration loop
	isSynthesis := strings.HasPrefix(opts.UserMessage, "[Subagent result from")
	if isSynthesis {
		logger.InfoCF(
			agentComp,
			fmt.Sprintf("SOFIA: synthesis start — presenting sub-agent result via model %s", agent.Model),
			map[string]any{
				"model":       agent.Model,
				"session_key": opts.SessionKey,
				"input_len":   len(opts.UserMessage),
			},
		)
	} else {
		logger.InfoCF(agentComp, fmt.Sprintf("SOFIA: generating response — model %s", agent.Model),
			map[string]any{
				"model":       agent.Model,
				"session_key": opts.SessionKey,
			})
	}
	llmStart := time.Now()
	finalContent, iteration, errorCount, err := al.runLLMIteration(ctx, agent, messages, opts)
	llmDur := time.Since(llmStart).Milliseconds()
	if err != nil {
		return "", err
	}

	// If last tool had ForUser content and we already sent it, we might not need to send final response
	// This is controlled by the tool's Silent flag and ForUser content

	// 5. Handle empty response
	if finalContent == "" {
		finalContent = opts.DefaultResponse
	}

	// 6. Save final assistant message to session
	agent.Sessions.AddMessage(opts.SessionKey, "assistant", finalContent)
	agent.Sessions.Save(opts.SessionKey)

	// 7. Optional: summarization
	if opts.EnableSummary {
		al.maybeSummarize(agent, opts.SessionKey, opts.Channel, opts.ChatID)
	}

	// 7b. Learn from feedback (#8) — detect correction patterns in user message
	if al.cfg.Agents.Defaults.LearnFromFeedback {
		al.maybLearnFromFeedback(agent, opts.UserMessage)
	}

	// 7c. Post-task self-reflection
	if al.cfg.Agents.Defaults.PostTaskReflection && !opts.NoHistory {
		go al.maybeReflect(agent, opts.SessionKey, finalContent, iteration, errorCount, llmDur)
	}

	// Guardrail: Output Filtering
	if al.cfg.Guardrails.OutputFiltering.Enabled {
		for _, pattern := range al.cfg.Guardrails.OutputFiltering.RedactPatterns {
			re, err := regexp.Compile(pattern)
			if err == nil {
				if al.cfg.Guardrails.OutputFiltering.Action == "block" {
					if re.MatchString(finalContent) {
						finalContent = "[CONTENT BLOCKED BY OUTPUT FILTER]"
						logger.WarnCF(agentComp, "Guardrail blocked output: pattern match", map[string]any{"pattern": pattern})
						break
					}
				} else {
					// Default to redact
					if re.MatchString(finalContent) {
						finalContent = re.ReplaceAllString(finalContent, "[REDACTED]")
						logger.WarnCF(agentComp, "Guardrail redacted output: pattern match", map[string]any{"pattern": pattern})
					}
				}
			}
		}
	}

	// 8. Optional: send response via bus
	if opts.SendResponse {
		al.bus.PublishOutbound(bus.OutboundMessage{
			Channel: opts.Channel,
			ChatID:  opts.ChatID,
			Content: finalContent,
		})
	}

	responsePreview := utils.Truncate(finalContent, 120)
	logger.InfoCF(agentComp, fmt.Sprintf("SOFIA: response ready — %s", responsePreview),
		map[string]any{
			"agent_id":         agent.ID,
			"session_key":      opts.SessionKey,
			"iterations":       iteration,
			"duration_ms":      llmDur,
			"response_len":     len(finalContent),
			"response_preview": responsePreview,
		})

	al.activeAgentID.Store("")
	al.activeStatus.Store("Idle")

	return finalContent, nil
}

// runLLMIteration executes the LLM call loop with tool handling.
// Returns (finalContent, iteration, errorCount, error).
func (al *AgentLoop) runLLMIteration(
	ctx context.Context,
	agent *AgentInstance,
	messages []providers.Message,
	opts processOptions,
) (string, int, int, error) {
	agentComp := fmt.Sprintf("agent:%s", agent.ID)
	iteration := 0
	errorCount := 0
	var finalContent string

	reflectionInterval := al.cfg.Agents.Defaults.ReflectionInterval
	parallelTools := al.cfg.Agents.Defaults.ParallelToolCalls

	for iteration < agent.MaxIterations {
		iteration++

		// Feature: Self-reflection checkpoint (#2)
		// At every N iterations, inject a system reflection prompt
		if reflectionInterval > 0 && iteration > 1 && (iteration-1)%reflectionInterval == 0 {
			reflectionPrompt := "[REFLECTION CHECKPOINT] " +
				"Pause and assess your progress. " +
				"What have you accomplished so far? Are you making meaningful progress? " +
				"Should you change your approach or strategy? " +
				"If stuck, consider alternative methods."

			// Inject plan status if active
			if plan := al.planManager.GetActivePlan(); plan != nil {
				reflectionPrompt += "\n\nCurrent plan status:\n" + plan.FormatStatus()
			}

			messages = append(messages, providers.Message{
				Role:    "user",
				Content: reflectionPrompt,
			})

			logger.InfoCF(agentComp, fmt.Sprintf("REFLECT: injected reflection at iteration %d", iteration),
				map[string]any{"agent_id": agent.ID, "iteration": iteration})
		}

		// Feature: Inject active plan context (#1)
		// On every iteration, if there's an active plan, include its status
		if iteration == 1 {
			if plan := al.planManager.GetActivePlan(); plan != nil {
				planStatus := plan.FormatStatus()
				// Append to the last system or user message rather than adding a new one
				for i := len(messages) - 1; i >= 0; i-- {
					if messages[i].Role == "user" {
						messages[i].Content += "\n\n[Active Plan Status]\n" + planStatus
						break
					}
				}
			}
		}

		logger.DebugCF(agentComp, "LLM iteration",
			map[string]any{
				"agent_id":  agent.ID,
				"iteration": iteration,
				"max":       agent.MaxIterations,
			})

		// Build tool definitions
		providerToolDefs := agent.Tools.ToProviderDefs()

		// Log LLM request details
		logger.DebugCF(agentComp, "LLM request",
			map[string]any{
				"agent_id":          agent.ID,
				"iteration":         iteration,
				"model":             agent.Model,
				"messages_count":    len(messages),
				"tools_count":       len(providerToolDefs),
				"max_tokens":        agent.MaxTokens,
				"temperature":       agent.Temperature,
				"system_prompt_len": len(messages[0].Content),
			})

		// Log full messages (detailed)
		logger.DebugCF(agentComp, "Full LLM request",
			map[string]any{
				"iteration":     iteration,
				"messages_json": formatMessagesForLog(messages),
				"tools_json":    formatToolsForLog(providerToolDefs),
			})

		// Call LLM with fallback chain if candidates are configured.
		var response *providers.LLMResponse
		var err error

		// Guardrail: Rate Limiting
		if al.cfg.Guardrails.RateLimiting.Enabled {
			al.rlMutex.Lock()
			now := time.Now()

			// Reset RPM counter if minute passed
			if now.After(al.rpmResetTime[agent.ID]) {
				al.rpmCounts[agent.ID] = 0
				al.rpmResetTime[agent.ID] = now.Add(time.Minute)
			}
			// Reset Token counter if hour passed
			if now.After(al.tokenResetTime[agent.ID]) {
				al.tokenCounts[agent.ID] = 0
				al.tokenResetTime[agent.ID] = now.Add(time.Hour)
			}

			// Check limits
			if maxRPM := al.cfg.Guardrails.RateLimiting.MaxRPM; maxRPM > 0 && al.rpmCounts[agent.ID] >= maxRPM {
				al.rlMutex.Unlock()
				logger.WarnCF(agentComp, "Guardrail: Rate limit exceeded (RPM)", map[string]any{"rpm": al.rpmCounts[agent.ID], "max": maxRPM})
				logger.Audit("Rate Limit Exceeded", map[string]any{
					"agent_id": agent.ID,
					"type":     "RPM",
					"rpm":      al.rpmCounts[agent.ID],
					"max":      maxRPM,
				})
				return "Error: Agent rate limit exceeded (requests per minute). Please try again later.", iteration, errorCount, nil
			}

			estimatedTokens := al.estimateTokens(messages) // Approximate
			if maxTokens := al.cfg.Guardrails.RateLimiting.MaxTokensPerHour; maxTokens > 0 && al.tokenCounts[agent.ID]+estimatedTokens > maxTokens {
				al.rlMutex.Unlock()
				logger.WarnCF(agentComp, "Guardrail: Rate limit exceeded (Tokens)", map[string]any{"tokens": al.tokenCounts[agent.ID], "max": maxTokens})
				logger.Audit("Rate Limit Exceeded", map[string]any{
					"agent_id": agent.ID,
					"type":     "TokensPerHour",
					"tokens":   al.tokenCounts[agent.ID],
					"max":      maxTokens,
				})
				return "Error: Agent rate limit exceeded (tokens per hour). Please try again later.", iteration, errorCount, nil
			}

			// Increment
			al.rpmCounts[agent.ID]++
			al.tokenCounts[agent.ID] += estimatedTokens
			al.rlMutex.Unlock()
		}

		// --- AUTO-TUNING ---
		// Dynamically adjust temperature based on task type inferred from messages
		callTemp := agent.Temperature
		// Only auto-tune if leaving at default (0.7)
		if callTemp == 0.7 {
			lastMsg := ""
			for i := len(messages) - 1; i >= 0; i-- {
				if messages[i].Role == "user" {
					lastMsg = strings.ToLower(messages[i].Content)
					break
				}
			}

			if strings.Contains(lastMsg, "code") || strings.Contains(lastMsg, "debug") || strings.Contains(lastMsg, "fix") {
				callTemp = 0.2 // Lower temp for analytical/coding tasks
				logger.DebugCF(agentComp, "Auto-Tuning: lowered temperature for coding task", map[string]any{"temp": callTemp})
			} else if strings.Contains(lastMsg, "brainstorm") || strings.Contains(lastMsg, "write") || strings.Contains(lastMsg, "creative") || strings.Contains(lastMsg, "idea") {
				callTemp = 0.8 // Higher temp for creative tasks
				logger.DebugCF(agentComp, "Auto-Tuning: raised temperature for creative task", map[string]any{"temp": callTemp})
			}
		}
		// -------------------

		callLLM := func() (*providers.LLMResponse, error) {
			if len(agent.Candidates) > 1 && al.fallback != nil {
				fbResult, fbErr := al.fallback.Execute(ctx, agent.Candidates,
					func(ctx context.Context, provider, model string) (*providers.LLMResponse, error) {
						return agent.Provider.Chat(ctx, messages, providerToolDefs, model, map[string]any{
							"max_tokens":       agent.MaxTokens,
							"temperature":      callTemp,
							"prompt_cache_key": agent.ID,
						})
					},
				)
				if fbErr != nil {
					return nil, fbErr
				}
				if fbResult.Provider != "" && len(fbResult.Attempts) > 0 {
					logger.InfoCF(agentComp, fmt.Sprintf("Fallback: succeeded with %s/%s after %d attempts",
						fbResult.Provider, fbResult.Model, len(fbResult.Attempts)+1),
						map[string]any{"agent_id": agent.ID, "iteration": iteration})
				}
				return fbResult.Response, nil
			}
			return agent.Provider.Chat(ctx, messages, providerToolDefs, agent.ModelID, map[string]any{
				"max_tokens":       agent.MaxTokens,
				"temperature":      callTemp,
				"prompt_cache_key": agent.ID,
			})
		}

		// Retry loop for context/token errors
		maxRetries := 2
		for retry := 0; retry <= maxRetries; retry++ {
			response, err = callLLM()
			if err == nil {
				break
			}

			errMsg := strings.ToLower(err.Error())
			isContextError := strings.Contains(errMsg, "token") ||
				strings.Contains(errMsg, "context") ||
				strings.Contains(errMsg, "invalidparameter") ||
				strings.Contains(errMsg, "length")

			if isContextError && retry < maxRetries {
				logger.WarnCF(agentComp, "Context window error detected, attempting compression", map[string]any{
					"error": err.Error(),
					"retry": retry,
				})

				if retry == 0 && !constants.IsInternalChannel(opts.Channel) {
					al.bus.PublishOutbound(bus.OutboundMessage{
						Channel: opts.Channel,
						ChatID:  opts.ChatID,
						Content: "Context window exceeded. Compressing history and retrying...",
					})
				}

				al.forceCompression(agent, opts.SessionKey)
				newHistory := agent.Sessions.GetHistory(opts.SessionKey)
				newSummary := agent.Sessions.GetSummary(opts.SessionKey)
				messages = agent.ContextBuilder.BuildMessages(
					newHistory, newSummary, "",
					nil, opts.Channel, opts.ChatID,
				)
				continue
			}
			break
		}

		if err != nil {
			logger.ErrorCF(agentComp, "LLM call failed",
				map[string]any{
					"agent_id":  agent.ID,
					"iteration": iteration,
					"error":     err.Error(),
				})
			return "", iteration, errorCount, fmt.Errorf("LLM call failed after retries: %w", err)
		}

		// Check if no tool calls - we're done
		if len(response.ToolCalls) == 0 {
			finalContent = response.Content
			logger.InfoCF(
				agentComp,
				fmt.Sprintf("SOFIA: LLM returned direct answer — %s", utils.Truncate(finalContent, 120)),
				map[string]any{
					"agent_id":         agent.ID,
					"iteration":        iteration,
					"content_len":      len(finalContent),
					"response_preview": utils.Truncate(finalContent, 120),
				},
			)
			break // no tool calls — direct answer
		}

		normalizedToolCalls := make([]providers.ToolCall, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			normalizedToolCalls = append(normalizedToolCalls, providers.NormalizeToolCall(tc))
		}

		// Log tool calls summary
		toolNames := make([]string, 0, len(normalizedToolCalls))
		for _, tc := range normalizedToolCalls {
			toolNames = append(toolNames, tc.Name)
		}
		logger.InfoCF(
			agentComp,
			fmt.Sprintf("TOOL: LLM requested %d tool(s): %s", len(normalizedToolCalls), strings.Join(toolNames, ", ")),
			map[string]any{
				"agent_id":  agent.ID,
				"tools":     toolNames,
				"count":     len(normalizedToolCalls),
				"iteration": iteration,
			},
		)

		// Build assistant message with tool calls
		assistantMsg := providers.Message{
			Role:             "assistant",
			Content:          response.Content,
			ReasoningContent: response.ReasoningContent,
		}
		for _, tc := range normalizedToolCalls {
			argumentsJSON, _ := json.Marshal(tc.Arguments)
			// Copy ExtraContent to ensure thought_signature is persisted for Gemini 3
			extraContent := tc.ExtraContent
			thoughtSignature := ""
			if tc.Function != nil {
				thoughtSignature = tc.Function.ThoughtSignature
			}

			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, providers.ToolCall{
				ID:   tc.ID,
				Type: "function",
				Name: tc.Name,
				Function: &providers.FunctionCall{
					Name:             tc.Name,
					Arguments:        string(argumentsJSON),
					ThoughtSignature: thoughtSignature,
				},
				ExtraContent:     extraContent,
				ThoughtSignature: thoughtSignature,
			})
		}
		messages = append(messages, assistantMsg)

		// Save assistant message with tool calls to session
		agent.Sessions.AddFullMessage(opts.SessionKey, assistantMsg)

		// Execute tool calls — parallel or sequential based on config
		type toolCallResult struct {
			index     int
			tc        providers.ToolCall
			result    *tools.ToolResult
			durMs     int64
			resultMsg providers.Message
		}

		executeSingleTool := func(idx int, tc providers.ToolCall) toolCallResult {
			al.activeStatus.Store(fmt.Sprintf("Executing tool: %s", tc.Name))
			argsJSON, _ := json.Marshal(tc.Arguments)
			argsPreview := utils.Truncate(string(argsJSON), 200)
			logger.InfoCF(agentComp, fmt.Sprintf("TOOL: started %s", tc.Name),
				map[string]any{
					"agent_id":     agent.ID,
					"tool":         tc.Name,
					"iteration":    iteration,
					"args_preview": argsPreview,
				})

			// Emit opencode indicator
			if tc.Name == "exec" {
				if cmd, ok := tc.Arguments["command"].(string); ok &&
					strings.Contains(strings.ToLower(cmd), "opencode") {
					logger.InfoCF(agentComp, "OPENCODE: started",
						map[string]any{"agent_id": agent.ID, "iteration": iteration})
				}
			}

			asyncCallback := func(callbackCtx context.Context, result *tools.ToolResult) {
				if !result.Silent && result.ForUser != "" {
					logger.InfoCF(agentComp, fmt.Sprintf("TOOL: async completed %s", tc.Name),
						map[string]any{
							"tool":           tc.Name,
							"for_user_len":   len(result.ForUser),
							"result_preview": utils.Truncate(result.ForUser, 160),
						})
				}
			}

			toolStart := time.Now()
			toolResult := agent.Tools.ExecuteWithContext(
				ctx, tc.Name, tc.Arguments, opts.Channel, opts.ChatID, asyncCallback,
			)
			toolDur := time.Since(toolStart).Milliseconds()

			toolStatus := "ok"
			toolErrStr := ""
			if toolResult.Err != nil {
				toolStatus = "error"
				toolErrStr = toolResult.Err.Error()
			}
			logger.InfoCF(agentComp, fmt.Sprintf("TOOL: finished %s in %dms — %s", tc.Name, toolDur, toolStatus),
				map[string]any{
					"agent_id":       agent.ID,
					"tool":           tc.Name,
					"duration_ms":    toolDur,
					"status":         toolStatus,
					"for_user_len":   len(toolResult.ForUser),
					"for_llm_len":    len(toolResult.ForLLM),
					"result_preview": utils.Truncate(toolResult.ForLLM, 160),
					"error":          toolErrStr,
				})

			if tc.Name == "exec" {
				if cmd, ok := tc.Arguments["command"].(string); ok &&
					strings.Contains(strings.ToLower(cmd), "opencode") {
					logger.InfoCF(agentComp,
						fmt.Sprintf("OPENCODE: finished in %dms — %s", toolDur, toolStatus),
						map[string]any{"agent_id": agent.ID, "duration_ms": toolDur, "status": toolStatus})
				}
			}

			contentForLLM := toolResult.ForLLM
			if contentForLLM == "" && toolResult.Err != nil {
				contentForLLM = toolResult.Err.Error()
			}

			return toolCallResult{
				index:  idx,
				tc:     tc,
				result: toolResult,
				durMs:  toolDur,
				resultMsg: providers.Message{
					Role:       "tool",
					Content:    contentForLLM,
					ToolCallID: tc.ID,
					Images:     toolResult.Images,
				},
			}
		}

		var tcResults []toolCallResult

		if parallelTools && len(normalizedToolCalls) > 1 {
			// Parallel tool execution using errgroup
			results := make([]toolCallResult, len(normalizedToolCalls))
			g, _ := errgroup.WithContext(ctx)

			for i, tc := range normalizedToolCalls {
				g.Go(func() error {
					results[i] = executeSingleTool(i, tc)
					return nil
				})
			}
			_ = g.Wait() // errors are always nil; tool errors are captured in results
			tcResults = results
		} else {
			// Sequential tool execution (default)
			for i, tc := range normalizedToolCalls {
				tcResults = append(tcResults, executeSingleTool(i, tc))
			}
		}

		// Process results in order and count errors
		confirmationNeeded := false
		for _, tcr := range tcResults {
			if tcr.result != nil && tcr.result.Err != nil {
				errorCount++
			}
			// Handle confirmation-required results (#5)
			if tcr.result.ConfirmationRequired {
				confirmationNeeded = true
				logger.InfoCF(agentComp, "CONFIRM: tool requires confirmation",
					map[string]any{
						"tool":   tcr.tc.Name,
						"prompt": tcr.result.ConfirmationPrompt,
					})

				// Send confirmation request to user
				if opts.SendResponse && opts.Channel != "" {
					al.bus.PublishOutbound(bus.OutboundMessage{
						Channel: opts.Channel,
						ChatID:  opts.ChatID,
						Content: fmt.Sprintf("⚠️ Confirmation required: %s\n\nReply 'yes' to proceed or 'no' to cancel.",
							tcr.result.ConfirmationPrompt),
					})
				}

				// Add a tool result indicating confirmation is pending
				tcr.resultMsg.Content = fmt.Sprintf("[CONFIRMATION_PENDING: %s — awaiting user response]",
					tcr.result.ConfirmationPrompt)
			}

			// Send ForUser content to user immediately if not Silent
			if !tcr.result.Silent && tcr.result.ForUser != "" && opts.SendResponse {
				outContent := tcr.result.ForUser
				// Guardrail: Output Filtering on Tool Results
				if outContent != "" {
					outContent = al.applyOutputFilter(agentComp, tcr.tc.Name, outContent)
				}

				if outContent != "" {
					al.bus.PublishOutbound(bus.OutboundMessage{
						Channel: opts.Channel,
						ChatID:  opts.ChatID,
						Content: outContent,
					})
				}
			}

			messages = append(messages, tcr.resultMsg)
			agent.Sessions.AddFullMessage(opts.SessionKey, tcr.resultMsg)
		}

		// If any tool requires confirmation, stop the loop and let the user respond
		if confirmationNeeded {
			finalContent = "Waiting for user confirmation before proceeding."
			break
		}
	}

	if finalContent != "" {
		finalContent = al.applyOutputFilter(agentComp, "llm_response", finalContent)
	}

	return finalContent, iteration, errorCount, nil
}

// applyOutputFilter checks a string against OutputFiltering redact patterns,
// optionally blocking or redacting the content, and logging audits.
func (al *AgentLoop) applyOutputFilter(agentComp, source, content string) string {
	if !al.cfg.Guardrails.OutputFiltering.Enabled || len(al.cfg.Guardrails.OutputFiltering.RedactPatterns) == 0 {
		return content
	}

	filteredContent := content
	for _, pattern := range al.cfg.Guardrails.OutputFiltering.RedactPatterns {
		// Tip: patterns could be cached/compiled once at startup for performance
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}

		if re.MatchString(filteredContent) {
			if al.cfg.Guardrails.OutputFiltering.Action == "block" {
				logger.WarnCF(agentComp, "Guardrail blocked output", map[string]any{
					"source":  source,
					"pattern": pattern,
				})
				logger.Audit("Output Blocked", map[string]any{
					"source":  source,
					"pattern": pattern,
				})
				return "[OUTPUT BLOCKED BY FILTER]"
			}

			// Redact action
			filteredContent = re.ReplaceAllString(filteredContent, "[REDACTED]")
			logger.WarnCF(agentComp, "Guardrail redacted output", map[string]any{
				"source":  source,
				"pattern": pattern,
			})
			logger.Audit("Output Redacted", map[string]any{
				"source":  source,
				"pattern": pattern,
			})
		}
	}

	return filteredContent
}

// updateToolContexts updates the context for tools that need channel/chatID info.
func (al *AgentLoop) updateToolContexts(agent *AgentInstance, channel, chatID string) {
	contextualTools := []string{"message", "spawn", "subagent", "orchestrate"}
	for _, name := range contextualTools {
		if tool, ok := agent.Tools.Get(name); ok {
			if ct, ok := tool.(tools.ContextualTool); ok {
				ct.SetContext(channel, chatID)
			}
		}
	}
}

// maybeSummarize triggers summarization if the session history exceeds thresholds.
func (al *AgentLoop) maybeSummarize(agent *AgentInstance, sessionKey, channel, chatID string) {
	newHistory := agent.Sessions.GetHistory(sessionKey)
	tokenEstimate := al.estimateTokens(newHistory)
	threshold := agent.ContextWindow * 75 / 100

	if len(newHistory) > 20 || tokenEstimate > threshold {
		summarizeKey := agent.ID + ":" + sessionKey
		if _, loading := al.summarizing.LoadOrStore(summarizeKey, true); !loading {
			go func() {
				defer al.summarizing.Delete(summarizeKey)
				logger.Debug("Memory threshold reached. Optimizing conversation history...")
				al.summarizeSession(agent, sessionKey)
			}()
		}
	}
}

// forceCompression aggressively reduces context when the limit is hit.
// It drops the oldest 50% of messages (keeping system prompt and last user message).
func (al *AgentLoop) forceCompression(agent *AgentInstance, sessionKey string) {
	history := agent.Sessions.GetHistory(sessionKey)
	if len(history) <= 4 {
		return
	}

	// Keep system prompt (usually [0]) and the very last message (user's trigger)
	// We want to drop the oldest half of the *conversation*
	// Assuming [0] is system, [1:] is conversation
	conversation := history[1 : len(history)-1]
	if len(conversation) == 0 {
		return
	}

	// Helper to find the mid-point of the conversation
	mid := len(conversation) / 2

	// New history structure:
	// 1. System Prompt (with compression note appended)
	// 2. Second half of conversation
	// 3. Last message

	droppedCount := mid
	keptConversation := conversation[mid:]

	newHistory := make([]providers.Message, 0, 1+len(keptConversation)+1)

	// Append compression note to the original system prompt instead of adding a new system message
	// This avoids having two consecutive system messages which some APIs (like Zhipu) reject
	compressionNote := fmt.Sprintf(
		"\n\n[System Note: Emergency compression dropped %d oldest messages due to context limit]",
		droppedCount,
	)
	enhancedSystemPrompt := history[0]
	enhancedSystemPrompt.Content = enhancedSystemPrompt.Content + compressionNote
	newHistory = append(newHistory, enhancedSystemPrompt)

	newHistory = append(newHistory, keptConversation...)
	newHistory = append(newHistory, history[len(history)-1]) // Last message

	// Update session
	agent.Sessions.SetHistory(sessionKey, newHistory)
	agent.Sessions.Save(sessionKey)

	logger.WarnCF("agent", "Forced compression executed", map[string]any{
		"session_key":  sessionKey,
		"dropped_msgs": droppedCount,
		"new_count":    len(newHistory),
	})
}

// correctionPatterns detects user messages that contain corrections or preferences.
var correctionPatterns = []string{
	"no, ",
	"actually,",
	"actually ",
	"i meant",
	"i prefer",
	"always use",
	"never use",
	"don't use",
	"do not use",
	"please always",
	"please never",
	"from now on",
	"remember that",
	"keep in mind",
	"i want you to",
	"use this instead",
	"that's wrong",
	"that's not right",
	"incorrect",
	"that is wrong",
}

// maybLearnFromFeedback checks if the user message contains correction patterns
// and extracts preferences to store in long-term memory.
func (al *AgentLoop) maybLearnFromFeedback(agent *AgentInstance, userMsg string) {
	if userMsg == "" || al.memDB == nil {
		return
	}

	lower := strings.ToLower(userMsg)
	isCorrection := false
	for _, pattern := range correctionPatterns {
		if strings.Contains(lower, pattern) {
			isCorrection = true
			break
		}
	}

	if !isCorrection {
		return
	}

	// Extract and store the preference
	memStore := NewMemoryStore(al.memDB, agent.ID)
	existing := memStore.ReadLongTerm()

	// Truncate user message for storage
	preference := userMsg
	if len(preference) > 200 {
		preference = preference[:200] + "..."
	}

	entry := fmt.Sprintf("- User preference: %s", preference)

	// Avoid duplicates
	if strings.Contains(existing, preference[:min(len(preference), 50)]) {
		return
	}

	var newContent string
	if existing == "" {
		newContent = "## User Preferences (auto-learned)\n\n" + entry
	} else {
		newContent = existing + "\n" + entry
	}

	if err := memStore.WriteLongTerm(newContent); err != nil {
		logger.WarnCF("agent", "Failed to save learned preference",
			map[string]any{"error": err.Error()})
	} else {
		logger.InfoCF("agent", "Learned user preference from feedback",
			map[string]any{"preference": utils.Truncate(preference, 80)})
	}

	// Feedback-Driven Evolution: Also append directly to USER.md
	userMDPath := filepath.Join(agent.Workspace, "USER.md")
	f, err := os.OpenFile(userMDPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		f.WriteString(fmt.Sprintf("\n- Auto-learned preference from feedback: %s\n", preference))
		f.Close()
	}
}

// maybeReflect runs a post-task self-evaluation asynchronously.
// It uses the LLM to evaluate the conversation quality and stores structured reflections.
func (al *AgentLoop) maybeReflect(
	agent *AgentInstance,
	sessionKey, finalContent string,
	iteration, errorCount int,
	durationMs int64,
) {
	if al.memDB == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	engine := NewReflectionEngine(al.memDB, agent.ID)
	if err := engine.Reflect(ctx, agent, sessionKey, finalContent, iteration, errorCount, durationMs); err != nil {
		logger.WarnCF("reflection", "Post-task reflection failed",
			map[string]any{"agent_id": agent.ID, "error": err.Error()})
	}
}

// GetDefaultSessionManager returns the session manager for the default agent.
// This is used by the web server to expose session history endpoints.
func (al *AgentLoop) GetDefaultSessionManager() *session.SessionManager {
	agent := al.getRegistry().GetDefaultAgent()
	if agent == nil {
		return nil
	}
	return agent.Sessions
}

// GetStartupInfo returns information about loaded tools and skills for logging.
func (al *AgentLoop) GetStartupInfo() map[string]any {
	info := make(map[string]any)

	agent := al.getRegistry().GetDefaultAgent()
	if agent == nil {
		return info
	}

	// Tools info
	toolsList := agent.Tools.List()
	detailedTools := make([]map[string]string, 0, len(toolsList))
	for _, name := range toolsList {
		if t, ok := agent.Tools.Get(name); ok {
			detailedTools = append(detailedTools, map[string]string{
				"name":        t.Name(),
				"description": t.Description(),
			})
		}
	}

	info["tools"] = map[string]any{
		"count": len(toolsList),
		"names": toolsList,
		"list":  detailedTools,
	}

	// Skills info
	info["skills"] = agent.ContextBuilder.GetSkillsInfo()

	// Agents info — per-agent metadata for Agent Monitor
	allAgents := al.getRegistry().ListAgents()
	agentMeta := make([]map[string]any, 0, len(allAgents))
	for _, a := range allAgents {
		role := "subagent"
		if a.ID == routing.DefaultAgentID {
			role = "sofia"
		}
		agentMeta = append(agentMeta, map[string]any{
			"id":       a.ID,
			"name":     a.Name,
			"role":     role,
			"model":    a.Model,
			"model_id": a.ModelID,
		})
	}
	info["agents"] = map[string]any{
		"count": len(allAgents),
		"ids":   al.getRegistry().ListAgentIDs(),
		"list":  agentMeta,
		"active": map[string]any{
			"id":     al.activeAgentID.Load(),
			"status": al.activeStatus.Load(),
		},
	}

	return info
}

// formatMessagesForLog formats messages for logging
func formatMessagesForLog(messages []providers.Message) string {
	if len(messages) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteString("[\n")
	for i, msg := range messages {
		fmt.Fprintf(&sb, "  [%d] Role: %s\n", i, msg.Role)
		if len(msg.ToolCalls) > 0 {
			sb.WriteString("  ToolCalls:\n")
			for _, tc := range msg.ToolCalls {
				fmt.Fprintf(&sb, "    - ID: %s, Type: %s, Name: %s\n", tc.ID, tc.Type, tc.Name)
				if tc.Function != nil {
					fmt.Fprintf(&sb, "      Arguments: %s\n", utils.Truncate(tc.Function.Arguments, 200))
				}
			}
		}
		if msg.Content != "" {
			content := utils.Truncate(msg.Content, 200)
			fmt.Fprintf(&sb, "  Content: %s\n", content)
		}
		if msg.ToolCallID != "" {
			fmt.Fprintf(&sb, "  ToolCallID: %s\n", msg.ToolCallID)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("]")
	return sb.String()
}

// formatToolsForLog formats tool definitions for logging
func formatToolsForLog(toolDefs []providers.ToolDefinition) string {
	if len(toolDefs) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteString("[\n")
	for i, tool := range toolDefs {
		fmt.Fprintf(&sb, "  [%d] Type: %s, Name: %s\n", i, tool.Type, tool.Function.Name)
		fmt.Fprintf(&sb, "      Description: %s\n", tool.Function.Description)
		if len(tool.Function.Parameters) > 0 {
			fmt.Fprintf(&sb, "      Parameters: %s\n", utils.Truncate(fmt.Sprintf("%v", tool.Function.Parameters), 200))
		}
	}
	sb.WriteString("]")
	return sb.String()
}

// summarizeSession summarizes the conversation history for a session.
func (al *AgentLoop) summarizeSession(agent *AgentInstance, sessionKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	history := agent.Sessions.GetHistory(sessionKey)
	summary := agent.Sessions.GetSummary(sessionKey)

	// Keep last 4 messages for continuity
	if len(history) <= 4 {
		return
	}

	toSummarize := history[:len(history)-4]

	// Oversized Message Guard
	maxMessageTokens := agent.ContextWindow / 2
	validMessages := make([]providers.Message, 0)
	omitted := false

	for _, m := range toSummarize {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		msgTokens := len(m.Content) / 2
		if msgTokens > maxMessageTokens {
			omitted = true
			continue
		}
		validMessages = append(validMessages, m)
	}

	if len(validMessages) == 0 {
		return
	}

	// Multi-Part Summarization
	var finalSummary string
	if len(validMessages) > 10 {
		mid := len(validMessages) / 2
		part1 := validMessages[:mid]
		part2 := validMessages[mid:]

		s1, _ := al.summarizeBatch(ctx, agent, part1, "")
		s2, _ := al.summarizeBatch(ctx, agent, part2, "")

		mergePrompt := fmt.Sprintf(
			"Merge these two conversation summaries into one cohesive summary:\n\n1: %s\n\n2: %s",
			s1,
			s2,
		)
		resp, err := agent.Provider.Chat(
			ctx,
			[]providers.Message{{Role: "user", Content: mergePrompt}},
			nil,
			agent.ModelID,
			map[string]any{
				"max_tokens":       1024,
				"temperature":      0.3,
				"prompt_cache_key": agent.ID,
			},
		)
		if err == nil {
			finalSummary = resp.Content
		} else {
			finalSummary = s1 + " " + s2
		}
	} else {
		finalSummary, _ = al.summarizeBatch(ctx, agent, validMessages, summary)
	}

	if omitted && finalSummary != "" {
		finalSummary += "\n[Note: Some oversized messages were omitted from this summary for efficiency.]"
	}

	if finalSummary != "" {
		agent.Sessions.SetSummary(sessionKey, finalSummary)
		agent.Sessions.TruncateHistory(sessionKey, 4)
		agent.Sessions.Save(sessionKey)
	}
}

// summarizeBatch summarizes a batch of messages.
func (al *AgentLoop) summarizeBatch(
	ctx context.Context,
	agent *AgentInstance,
	batch []providers.Message,
	existingSummary string,
) (string, error) {
	var sb strings.Builder
	sb.WriteString("Provide a concise summary of this conversation segment, preserving core context and key points.\n")
	if existingSummary != "" {
		sb.WriteString("Existing context: ")
		sb.WriteString(existingSummary)
		sb.WriteString("\n")
	}
	sb.WriteString("\nCONVERSATION:\n")
	for _, m := range batch {
		fmt.Fprintf(&sb, "%s: %s\n", m.Role, m.Content)
	}
	prompt := sb.String()

	response, err := agent.Provider.Chat(
		ctx,
		[]providers.Message{{Role: "user", Content: prompt}},
		nil,
		agent.ModelID,
		map[string]any{
			"max_tokens":       1024,
			"temperature":      0.3,
			"prompt_cache_key": agent.ID,
		},
	)
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// estimateTokens estimates the number of tokens in a message list.
// Uses a safe heuristic of 2.5 characters per token to account for CJK and other
// overheads better than the previous 3 chars/token.
func (al *AgentLoop) estimateTokens(messages []providers.Message) int {
	totalChars := 0
	for _, m := range messages {
		totalChars += utf8.RuneCountInString(m.Content)
	}
	// 2.5 chars per token = totalChars * 2 / 5
	return totalChars * 2 / 5
}

func (al *AgentLoop) handleCommand(ctx context.Context, msg bus.InboundMessage) (string, bool) {
	content := strings.TrimSpace(msg.Content)
	if !strings.HasPrefix(content, "/") {
		return "", false
	}

	parts := strings.Fields(content)
	if len(parts) == 0 {
		return "", false
	}

	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "/show":
		if len(args) < 1 {
			return "Usage: /show [model|channel|agents]", true
		}
		switch args[0] {
		case "model":
			defaultAgent := al.getRegistry().GetDefaultAgent()
			if defaultAgent == nil {
				return "No default agent configured", true
			}
			return fmt.Sprintf("Current model: %s", defaultAgent.Model), true
		case "channel":
			return fmt.Sprintf("Current channel: %s", msg.Channel), true
		case "agents":
			agentIDs := al.getRegistry().ListAgentIDs()
			return fmt.Sprintf("Registered agents: %s", strings.Join(agentIDs, ", ")), true
		default:
			return fmt.Sprintf("Unknown show target: %s", args[0]), true
		}

	case "/list":
		if len(args) < 1 {
			return "Usage: /list [models|channels|agents]", true
		}
		switch args[0] {
		case "models":
			return "Available models: configured in config.json per agent", true
		case "channels":
			if al.channelManager == nil {
				return "Channel manager not initialized", true
			}
			channels := al.channelManager.GetEnabledChannels()
			if len(channels) == 0 {
				return "No channels enabled", true
			}
			return fmt.Sprintf("Enabled channels: %s", strings.Join(channels, ", ")), true
		case "agents":
			agentIDs := al.getRegistry().ListAgentIDs()
			return fmt.Sprintf("Registered agents: %s", strings.Join(agentIDs, ", ")), true
		default:
			return fmt.Sprintf("Unknown list target: %s", args[0]), true
		}

	case "/switch":
		if len(args) < 3 || args[1] != "to" {
			return "Usage: /switch [model|channel] to <name>", true
		}
		target := args[0]
		value := args[2]

		switch target {
		case "model":
			defaultAgent := al.getRegistry().GetDefaultAgent()
			if defaultAgent == nil {
				return "No default agent configured", true
			}
			// Validate the model name against model_list
			mc, err := al.cfg.GetModelConfig(value)
			if err != nil || mc == nil {
				return fmt.Sprintf(
					"Model %q not found in model_list. Use /list models to see available models.",
					value,
				), true
			}
			oldModel := defaultAgent.Model
			defaultAgent.Model = value
			_, defaultAgent.ModelID = providers.ExtractProtocol(mc.Model)
			return fmt.Sprintf("Switched model from %s to %s", oldModel, value), true
		case "channel":
			if al.channelManager == nil {
				return "Channel manager not initialized", true
			}
			if _, exists := al.channelManager.GetChannel(value); !exists && value != "cli" {
				return fmt.Sprintf("Channel '%s' not found or not enabled", value), true
			}
			return fmt.Sprintf("Switched target channel to %s", value), true
		default:
			return fmt.Sprintf("Unknown switch target: %s", target), true
		}
	}

	return "", false
}

// extractPeer extracts the routing peer from inbound message metadata.
func extractPeer(msg bus.InboundMessage) *routing.RoutePeer {
	peerKind := msg.Metadata["peer_kind"]
	if peerKind == "" {
		return nil
	}
	peerID := msg.Metadata["peer_id"]
	if peerID == "" {
		if peerKind == "direct" {
			peerID = msg.SenderID
		} else {
			peerID = msg.ChatID
		}
	}
	return &routing.RoutePeer{Kind: peerKind, ID: peerID}
}

// extractParentPeer extracts the parent peer (reply-to) from inbound message metadata.
func extractParentPeer(msg bus.InboundMessage) *routing.RoutePeer {
	parentKind := msg.Metadata["parent_peer_kind"]
	parentID := msg.Metadata["parent_peer_id"]
	if parentKind == "" || parentID == "" {
		return nil
	}
	return &routing.RoutePeer{Kind: parentKind, ID: parentID}
}
