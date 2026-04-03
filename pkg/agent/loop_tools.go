package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/abtest"
	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/checkpoint"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/reputation"
	"github.com/grasberg/sofia/pkg/skills"
	"github.com/grasberg/sofia/pkg/tools"
)

type sharedToolRegistrar struct {
	cfg             *config.Config
	msgBus          *bus.MessageBus
	registry        *AgentRegistry
	agentTaskRunner func(ctx context.Context, agentID, sessionKey, task, originChannel, originChatID string) (string, error)
	planMgr         *tools.PlanManager
	scratchpad      *tools.SharedScratchpad
	checkpointMgr   *checkpoint.Manager
	memDB           *memory.MemoryDB
	a2aRouter       *A2ARouter
	toolTracker     *tools.ToolTracker
}

func registerSharedTools(
	cfg *config.Config,
	msgBus *bus.MessageBus,
	registry *AgentRegistry,
	agentTaskRunner func(ctx context.Context, agentID, sessionKey, task, originChannel, originChatID string) (string, error),
	planMgr *tools.PlanManager,
	scratchpad *tools.SharedScratchpad,
	checkpointMgr *checkpoint.Manager,
	memDB *memory.MemoryDB,
	a2aRouter *A2ARouter,
	toolTracker *tools.ToolTracker,
) {
	registrar := sharedToolRegistrar{
		cfg:             cfg,
		msgBus:          msgBus,
		registry:        registry,
		agentTaskRunner: agentTaskRunner,
		planMgr:         planMgr,
		scratchpad:      scratchpad,
		checkpointMgr:   checkpointMgr,
		memDB:           memDB,
		a2aRouter:       a2aRouter,
		toolTracker:     toolTracker,
	}

	for _, agentID := range registry.ListAgentIDs() {
		agent, ok := registry.GetAgent(agentID)
		if !ok {
			continue
		}

		registrar.registerForAgent(agentID, agent)
	}
}

func (r sharedToolRegistrar) registerForAgent(agentID string, agent *AgentInstance) {
	agent.Tools.SetTracker(r.toolTracker)

	r.registerPerformanceTools(agent)
	r.registerWebAndSystemTools(agent)
	r.registerMessageTool(agent)
	r.registerSkillTools(agent)

	subagentManager := r.newSubagentManager(agentID, agent)
	r.registerCoordinationTools(agentID, agent, subagentManager)
	r.registerMemoryTools(agentID, agent)
	r.registerA2ATool(agentID, agent)
	r.registerSelfModifyTool(agent)
}

func (r sharedToolRegistrar) registerPerformanceTools(agent *AgentInstance) {
	agent.Tools.Register(tools.NewGetToolStatsTool(r.toolTracker))
	agent.Tools.Register(tools.NewCreatePipelineTool(agent.Tools))
}

func (r sharedToolRegistrar) registerWebAndSystemTools(agent *AgentInstance) {
	if searchTool := tools.NewWebSearchTool(tools.WebSearchToolOptions{
		BraveAPIKey:          r.cfg.Tools.Web.Brave.APIKey,
		BraveMaxResults:      r.cfg.Tools.Web.Brave.MaxResults,
		BraveEnabled:         r.cfg.Tools.Web.Brave.Enabled,
		TavilyAPIKey:         r.cfg.Tools.Web.Tavily.APIKey,
		TavilyBaseURL:        r.cfg.Tools.Web.Tavily.BaseURL,
		TavilyMaxResults:     r.cfg.Tools.Web.Tavily.MaxResults,
		TavilyEnabled:        r.cfg.Tools.Web.Tavily.Enabled,
		DuckDuckGoMaxResults: r.cfg.Tools.Web.DuckDuckGo.MaxResults,
		DuckDuckGoEnabled:    r.cfg.Tools.Web.DuckDuckGo.Enabled,
		PerplexityAPIKey:     r.cfg.Tools.Web.Perplexity.APIKey,
		PerplexityMaxResults: r.cfg.Tools.Web.Perplexity.MaxResults,
		PerplexityEnabled:    r.cfg.Tools.Web.Perplexity.Enabled,
		Proxy:                r.cfg.Tools.Web.Proxy,
	}); searchTool != nil {
		agent.Tools.Register(searchTool)
	}

	agent.Tools.Register(tools.NewWebFetchToolWithProxy(50000, r.cfg.Tools.Web.Proxy))
	agent.Tools.Register(tools.NewWebBrowseTool(tools.BrowseToolOptions{
		Headless:       r.cfg.Tools.Web.Browser.Headless,
		TimeoutSeconds: r.cfg.Tools.Web.Browser.TimeoutSeconds,
		BrowserType:    r.cfg.Tools.Web.Browser.BrowserType,
		ScreenshotDir:  r.cfg.Tools.Web.Browser.ScreenshotDir,
		Workspace:      agent.Workspace,
	}))

	if r.cfg.Tools.Google.Enabled {
		agent.Tools.Register(tools.NewGoogleCLITool(
			r.cfg.Tools.Google.BinaryPath,
			r.cfg.Tools.Google.TimeoutSeconds,
			r.cfg.Tools.Google.AllowedCommands,
		))
	}

	if r.cfg.Tools.GitHub.Enabled {
		agent.Tools.Register(tools.NewGitHubCLITool(
			r.cfg.Tools.GitHub.BinaryPath,
			r.cfg.Tools.GitHub.TimeoutSeconds,
			r.cfg.Tools.GitHub.AllowedCommands,
		))
	}

	if r.cfg.Tools.Vercel.Enabled {
		agent.Tools.Register(tools.NewVercelTool(
			r.cfg.Tools.Vercel.BinaryPath,
			r.cfg.Tools.Vercel.TimeoutSeconds,
			r.cfg.Tools.Vercel.AllowedCommands,
		))
	}

	if r.cfg.Tools.BraveSearch.Enabled && r.cfg.Tools.BraveSearch.APIKey != "" {
		agent.Tools.Register(tools.NewBraveSearchTool(r.cfg.Tools.BraveSearch.APIKey))
	}

	if r.cfg.Tools.Porkbun.Enabled && r.cfg.Tools.Porkbun.APIKey != "" && r.cfg.Tools.Porkbun.SecretAPIKey != "" {
		agent.Tools.Register(tools.NewPorkbunTool(r.cfg.Tools.Porkbun.APIKey, r.cfg.Tools.Porkbun.SecretAPIKey))
	}

	if r.cfg.Tools.Cpanel.Enabled && r.cfg.Tools.Cpanel.Host != "" && r.cfg.Tools.Cpanel.APIToken != "" {
		agent.Tools.Register(tools.NewCpanelTool(
			r.cfg.Tools.Cpanel.Host,
			r.cfg.Tools.Cpanel.Port,
			r.cfg.Tools.Cpanel.Username,
			r.cfg.Tools.Cpanel.APIToken,
		))
	}

	if r.cfg.Tools.Bitcoin.Enabled {
		agent.Tools.Register(tools.NewBitcoinTool(
			r.cfg.Tools.Bitcoin.Network,
			r.cfg.Tools.Bitcoin.WalletPath,
			r.cfg.Tools.Bitcoin.Passphrase,
		))
	}

	agent.Tools.Register(tools.NewI2CTool())
	agent.Tools.Register(tools.NewSPITool())
	agent.Tools.Register(tools.NewComputerUseTool(tools.ComputerUseOptions{
		Workspace: agent.Workspace,
		Provider:  agent.Provider,
		ModelID:   agent.ModelID,
	}))
}

func (r sharedToolRegistrar) registerMessageTool(agent *AgentInstance) {
	messageTool := tools.NewMessageTool()
	messageTool.SetSendCallback(func(channel, chatID, content string) error {
		r.msgBus.PublishOutbound(bus.OutboundMessage{
			Channel: channel,
			ChatID:  chatID,
			Content: content,
		})
		return nil
	})
	agent.Tools.Register(messageTool)
}

func (r sharedToolRegistrar) registerSkillTools(agent *AgentInstance) {
	registryMgr := skills.NewRegistryManagerFromConfig(skills.RegistryConfig{
		MaxConcurrentSearches: r.cfg.Tools.Skills.MaxConcurrentSearches,
		ClawHub:               skills.ClawHubConfig(r.cfg.Tools.Skills.Registries.ClawHub),
	})
	searchCache := skills.NewSearchCache(
		r.cfg.Tools.Skills.SearchCache.MaxSize,
		time.Duration(r.cfg.Tools.Skills.SearchCache.TTLSeconds)*time.Second,
	)

	agent.Tools.Register(tools.NewFindSkillsTool(registryMgr, searchCache))
	agent.Tools.Register(tools.NewInstallSkillTool(registryMgr, agent.Workspace))
	agent.Tools.Register(tools.NewCreateSkillTool(agent.Workspace))
	agent.Tools.Register(tools.NewUpdateSkillTool(agent.Workspace))
}

func (r sharedToolRegistrar) newSubagentManager(agentID string, agent *AgentInstance) *tools.SubagentManager {
	subagentManager := tools.NewSubagentManager(agent.Provider, agent.ModelID, agent.Workspace, r.msgBus)
	subagentManager.SetLLMOptions(agent.MaxTokens, agent.Temperature)
	subagentManager.SetAgentTaskRunner(r.agentTaskRunner)
	subagentManager.SetSkillsLoader(agent.ContextBuilder.GetSkillsLoader())

	if r.memDB != nil {
		subagentManager.SetGoalContext(func() string {
			return formatActiveGoals(r.memDB, agentID)
		})
	}

	return subagentManager
}

func (r sharedToolRegistrar) registerCoordinationTools(
	agentID string,
	agent *AgentInstance,
	subagentManager *tools.SubagentManager,
) {
	spawnTool := tools.NewSpawnTool(subagentManager)
	spawnTool.SetAllowlistChecker(func(targetAgentID string) bool {
		return r.registry.CanSpawnSubagent(agentID, targetAgentID)
	})
	agent.Tools.Register(spawnTool)

	agent.Tools.Register(tools.NewPlanTool(r.planMgr, r.memDB))
	agent.Tools.Register(tools.NewScratchpadTool(r.scratchpad, "default"))
	agent.Tools.Register(tools.NewCheckpointTool(r.checkpointMgr, agentID))
	agent.Tools.Register(tools.NewSubagentTool(subagentManager))

	if r.memDB != nil {
		agent.Tools.Register(tools.NewPracticeTool(r.memDB, subagentManager, agentID))
	}

	agent.Tools.Register(tools.NewOrchestrateTool(r.newOrchestrateToolConfig()))
	agent.Tools.Register(tools.NewConflictResolveTool(r.scratchpad))
}

func (r sharedToolRegistrar) newOrchestrateToolConfig() tools.OrchestrateToolConfig {
	var reputationMgr *reputation.Manager
	if r.memDB != nil {
		reputationMgr = reputation.NewManager(r.memDB)
	}

	return tools.OrchestrateToolConfig{
		AgentScorer: func(candidateID, taskDescription string) float64 {
			candidate, ok := r.registry.GetAgent(candidateID)
			if !ok {
				return 0
			}

			keywordScore := scoreCandidate(candidate, strings.ToLower(taskDescription))
			if reputationMgr == nil {
				return keywordScore
			}

			repScore := reputationMgr.ReputationScore(candidateID, taskDescription)
			return 0.7*keywordScore + 0.3*repScore
		},
		ListAgentIDs: r.registry.ListAgentIDs,
		RunAgentTask: func(ctx context.Context, targetAgentID, task, channel, chatID string) (string, error) {
			return r.agentTaskRunner(ctx, targetAgentID, "", task, channel, chatID)
		},
		Scratchpad: r.scratchpad,
	}
}

func (r sharedToolRegistrar) registerMemoryTools(agentID string, agent *AgentInstance) {
	if r.memDB == nil {
		return
	}

	abtestMgr := abtest.NewManager(r.memDB)
	agent.Tools.Register(tools.NewABTestTool(abtestMgr, agent.Provider, agent.ModelID))
	agent.Tools.Register(tools.NewDynamicToolCreator(r.memDB, agent.Tools, agent.Workspace))
	tools.LoadDynamicTools(r.memDB, agent.Tools, agent.Workspace)

	reputationMgr := reputation.NewManager(r.memDB)
	agent.Tools.Register(tools.NewReputationTool(reputationMgr))
	agent.Tools.Register(tools.NewKnowledgeGraphTool(r.memDB, agentID))
	agent.Tools.Register(tools.NewDistillKnowledgeTool(r.memDB, agentID))
	agent.Tools.Register(tools.NewManageGoalsTool(tools.ManageGoalsOptions{
		GoalManager: autonomy.NewGoalManager(r.memDB),
		AgentID:     agentID,
	}))
	agent.Tools.Register(tools.NewManageTriggersTool(tools.ManageTriggersOptions{
		TriggerManager: autonomy.NewTriggerManager(r.memDB),
		AgentID:        agentID,
	}))
}

func (r sharedToolRegistrar) registerA2ATool(agentID string, agent *AgentInstance) {
	if r.a2aRouter == nil {
		return
	}

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
			if err := r.a2aRouter.Send(msg); err != nil {
				return "", err
			}
			return msg.ID, nil
		},
		r.a2aRouter.Broadcast,
		func(aid string, timeout time.Duration) *tools.A2AMessageForTool {
			msg := r.a2aRouter.Receive(aid, timeout)
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
			msg := r.a2aRouter.Poll(aid)
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
		r.a2aRouter.PendingCount,
	)

	agent.Tools.Register(tools.NewA2ATool(a2aAdapter, agentID))
}

func (r sharedToolRegistrar) registerSelfModifyTool(agent *AgentInstance) {
	cwd, _ := os.Getwd()
	agent.Tools.Register(tools.NewSelfModifyTool(cwd))
}

// formatActiveGoals returns a formatted string of active goals for an agent,
// used to inject goal context into subagent prompts.
func formatActiveGoals(db *memory.MemoryDB, agentID string) string {
	nodes, err := db.FindNodes(agentID, "Goal", "", 10)
	if err != nil || len(nodes) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, n := range nodes {
		props := n.Properties
		if !strings.Contains(props, `"active"`) {
			continue
		}
		sb.WriteString(fmt.Sprintf("- [%d] %s\n", n.ID, n.Name))
	}
	return sb.String()
}
