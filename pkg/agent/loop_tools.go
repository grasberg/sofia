package agent

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/abtest"
	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/checkpoint"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/notifications"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/reputation"
	"github.com/grasberg/sofia/pkg/skills"
	"github.com/grasberg/sofia/pkg/tools"
)

func registerSharedTools(
	cfg *config.Config,
	msgBus *bus.MessageBus,
	registry *AgentRegistry,
	provider providers.LLMProvider,
	agentTaskRunner func(ctx context.Context, agentID, sessionKey, task, originChannel, originChatID string) (string, error),
	planMgr *tools.PlanManager,
	scratchpad *tools.SharedScratchpad,
	checkpointMgr *checkpoint.Manager,
	memDB *memory.MemoryDB,
	a2aRouter *A2ARouter,
	pushService *notifications.PushService,
	toolTracker *tools.ToolTracker,
) {
	for _, agentID := range registry.ListAgentIDs() {
		agent, ok := registry.GetAgent(agentID)
		if !ok {
			continue
		}

		// Attach tool tracker
		agent.Tools.SetTracker(toolTracker)

		// Performance & Pipeline additions
		agent.Tools.Register(tools.NewGetToolStatsTool(toolTracker))
		agent.Tools.Register(tools.NewCreatePipelineTool(agent.Tools))

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

		if cfg.Tools.GitHub.Enabled {
			agent.Tools.Register(tools.NewGitHubCLITool(
				cfg.Tools.GitHub.BinaryPath,
				cfg.Tools.GitHub.TimeoutSeconds,
				cfg.Tools.GitHub.AllowedCommands,
			))
		}

		if cfg.Tools.BraveSearch.Enabled && cfg.Tools.BraveSearch.APIKey != "" {
			agent.Tools.Register(tools.NewBraveSearchTool(cfg.Tools.BraveSearch.APIKey))
		}

		if cfg.Tools.Porkbun.Enabled && cfg.Tools.Porkbun.APIKey != "" && cfg.Tools.Porkbun.SecretAPIKey != "" {
			agent.Tools.Register(tools.NewPorkbunTool(cfg.Tools.Porkbun.APIKey, cfg.Tools.Porkbun.SecretAPIKey))
		}

		if cfg.Tools.Cpanel.Enabled && cfg.Tools.Cpanel.Host != "" && cfg.Tools.Cpanel.APIToken != "" {
			agent.Tools.Register(tools.NewCpanelTool(
				cfg.Tools.Cpanel.Host,
				cfg.Tools.Cpanel.Port,
				cfg.Tools.Cpanel.Username,
				cfg.Tools.Cpanel.APIToken,
			))
		}

		if cfg.Tools.Bitcoin.Enabled {
			agent.Tools.Register(tools.NewBitcoinTool(
				cfg.Tools.Bitcoin.Network,
				cfg.Tools.Bitcoin.WalletPath,
				cfg.Tools.Bitcoin.Passphrase,
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
		subagentManager.SetSkillsLoader(agent.ContextBuilder.GetSkillsLoader())
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

		// Checkpoint — save/restore execution state mid-task
		agent.Tools.Register(tools.NewCheckpointTool(checkpointMgr, agentID))

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
				keywordScore := scoreCandidate(
					candidate, strings.ToLower(taskDescription),
				)
				// Blend with reputation if available.
				if memDB != nil {
					reputationMgr := reputation.NewManager(memDB)
					repScore := reputationMgr.ReputationScore(
						candidateID, taskDescription,
					)
					// 70% keyword match, 30% reputation history.
					return 0.7*keywordScore + 0.3*repScore
				}
				return keywordScore
			},
			ListAgentIDs: registry.ListAgentIDs,
			RunAgentTask: func(ctx context.Context, targetAgentID, task, channel, chatID string) (string, error) {
				return agentTaskRunner(ctx, targetAgentID, "", task, channel, chatID)
			},
			Scratchpad: scratchpad,
		}
		orchTool := tools.NewOrchestrateTool(orchCfg)
		agent.Tools.Register(orchTool)

		// Conflict Resolution — detect and resolve conflicting outputs from parallel agents
		agent.Tools.Register(tools.NewConflictResolveTool(scratchpad))

		// A/B Testing — behavioral experiments to compare different approaches
		if memDB != nil {
			abtestMgr := abtest.NewManager(memDB)
			agent.Tools.Register(tools.NewABTestTool(
				abtestMgr, agent.Provider, agent.ModelID,
			))
		}

		// Dynamic Tool Creation — generate new tools on-the-fly
		if memDB != nil {
			agent.Tools.Register(tools.NewDynamicToolCreator(
				memDB, agent.Tools, agent.Workspace,
			))
			tools.LoadDynamicTools(memDB, agent.Tools, agent.Workspace)
		}

		// Agent Reputation — track which agents perform best at which tasks
		if memDB != nil {
			reputationMgr := reputation.NewManager(memDB)
			agent.Tools.Register(tools.NewReputationTool(reputationMgr))
		}

		// Knowledge Graph — semantic memory with structured entities and relationships
		if memDB != nil {
			agent.Tools.Register(tools.NewKnowledgeGraphTool(memDB, agentID))
			agent.Tools.Register(tools.NewDistillKnowledgeTool(memDB, agentID))

			// Autonomy Tools
			agent.Tools.Register(tools.NewManageGoalsTool(tools.ManageGoalsOptions{
				GoalManager: autonomy.NewGoalManager(memDB),
				AgentID:     agentID,
			}))
			agent.Tools.Register(tools.NewManageTriggersTool(tools.ManageTriggersOptions{
				TriggerManager: autonomy.NewTriggerManager(memDB),
				AgentID:        agentID,
			}))
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
