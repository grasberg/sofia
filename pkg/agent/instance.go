package agent

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/mcp"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/routing"
	"github.com/grasberg/sofia/pkg/session"
	"github.com/grasberg/sofia/pkg/tools"
)

// AgentInstance represents a fully configured agent with its own workspace,
// session manager, context builder, and tool registry.
type AgentInstance struct {
	ID             string
	Name           string
	Template       string
	Model          string // User-facing alias (model_name from model_list)
	ModelID        string // Raw model ID without protocol prefix, passed to Chat()
	Fallbacks      []string
	Workspace      string
	MaxIterations  int
	MaxTokens      int
	Temperature    float64
	ContextWindow  int
	Provider       providers.LLMProvider
	Sessions       *session.SessionManager
	ContextBuilder *ContextBuilder
	Tools          *tools.ToolRegistry
	Subagents      *config.SubagentsConfig
	SkillsFilter   []string
	IsLocalModel   bool
	PurposePrompt  string
	Candidates     []providers.FallbackCandidate
}

// NewAgentInstance creates an agent instance from config.
func NewAgentInstance(
	agentCfg *config.AgentConfig,
	defaults *config.AgentDefaults,
	cfg *config.Config,
	provider providers.LLMProvider,
	memDB *memory.MemoryDB,
	mcpManager *mcp.GlobalManager,
) *AgentInstance {
	workspace := resolveAgentWorkspace(agentCfg, defaults)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		logger.WarnCF("agent",
			"Failed to create workspace directory",
			map[string]any{
				"workspace": workspace,
				"error":     err.Error(),
			})
	}

	model := resolveAgentModel(agentCfg, defaults)
	fallbacks := resolveAgentFallbacks(agentCfg, defaults)
	modelID := resolveAgentModelID(model, cfg)

	restrict := defaults.RestrictToWorkspace
	toolsRegistry := tools.NewToolRegistry()
	toolsRegistry.Register(tools.NewReadFileTool(workspace, restrict))
	toolsRegistry.Register(tools.NewWriteFileTool(workspace, restrict))
	toolsRegistry.Register(tools.NewListDirTool(workspace, restrict))
	toolsRegistry.Register(tools.NewExecToolWithConfig(workspace, restrict, cfg))
	toolsRegistry.Register(tools.NewEditFileTool(workspace, restrict))
	toolsRegistry.Register(tools.NewAppendFileTool(workspace, restrict))
	toolsRegistry.Register(tools.NewImageAnalyzeTool(workspace, restrict))
	toolsRegistry.Register(tools.NewScreenshotTool(workspace))
	toolsRegistry.Register(tools.NewDocGenTool(workspace))
	toolsRegistry.Register(tools.NewSearchHistoryTool(memDB))

	if mcpManager != nil {
		for _, srv := range mcpManager.GetServers() {
			for _, t := range srv.Tools {
				toolsRegistry.Register(tools.NewMCPToolAdapter(srv.Name, t, srv.Client))
			}
		}
	}

	agentID := routing.DefaultAgentID
	agentName := ""
	var subagents *config.SubagentsConfig
	var skillsFilter []string

	if agentCfg != nil {
		agentID = routing.NormalizeAgentID(agentCfg.ID)
		agentName = agentCfg.Name
		subagents = agentCfg.Subagents
		skillsFilter = agentCfg.Skills
	}

	contextBuilder := NewContextBuilder(workspace, cfg.UserName, memDB, agentID)
	contextBuilder.cacheTTL = 10 * time.Second // Skip file-system checks within TTL for performance
	contextBuilder.SetCodeEditor(defaults.CodeEditor)

	if agentCfg != nil {
		contextBuilder.SetPurposeTemplate(agentCfg.Template)
		if agentCfg.Template != "" {
			if t, err := LoadPurposeTemplate(agentCfg.Template); err == nil {
				contextBuilder.SetPurposeInstructions(t.Instructions)
				skillsMode := strings.TrimSpace(agentCfg.TemplateSkillsMode)
				if skillsMode == "overwrite" && len(t.Skills) > 0 {
					skillsFilter = append([]string(nil), t.Skills...)
				} else if len(skillsFilter) == 0 && len(t.Skills) > 0 {
					skillsFilter = append([]string(nil), t.Skills...)
				}
			}
		}
	}

	contextBuilder.SetSkillsFilter(skillsFilter)

	// Guardrail: Apply Prompt Injection system suffix if enabled
	if cfg.Guardrails.PromptInjection.Enabled && cfg.Guardrails.PromptInjection.SystemSuffix != "" {
		contextBuilder.SetSystemSuffix(cfg.Guardrails.PromptInjection.SystemSuffix)
	}

	sessionsManager := session.NewSessionManager(memDB, agentID)

	maxIter := defaults.MaxToolIterations
	if maxIter == 0 {
		maxIter = 20
	}

	maxTokens := defaults.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	// Per-model max_tokens overrides the agent default when set.
	if mc, err := cfg.GetModelConfig(model); err == nil && mc != nil && mc.MaxTokens > 0 {
		maxTokens = mc.MaxTokens
	}

	temperature := 0.7
	if defaults.Temperature != nil {
		temperature = *defaults.Temperature
	}

	// Resolve fallback candidates
	modelCfg := providers.ModelConfig{
		Primary:   model,
		Fallbacks: fallbacks,
	}
	candidates := providers.ResolveCandidates(modelCfg, defaults.Provider)

	// If this agent has a custom model that differs from the default, create a
	// per-agent provider from its model config. This allows different agents to
	// use different API keys or providers without sharing the global provider.
	agentProvider := provider
	if agentCfg != nil && agentCfg.Model != nil && strings.TrimSpace(agentCfg.Model.Primary) != "" {
		if mc, err := cfg.GetModelConfig(model); err == nil && mc != nil {
			if mc.Workspace == "" {
				mc.Workspace = cfg.WorkspacePath()
			}
			if p, _, err := providers.CreateProviderFromConfig(mc); err == nil && p != nil {
				agentProvider = p
			}
		}
	}

	isLocal := false
	if mc, err := cfg.GetModelConfig(model); err == nil && mc != nil {
		isLocal = strings.Contains(mc.APIBase, "localhost") || strings.Contains(mc.APIBase, "127.0.0.1")
	}

	return &AgentInstance{
		ID:   agentID,
		Name: agentName,
		Template: func() string {
			if agentCfg != nil {
				return agentCfg.Template
			}
			return ""
		}(),
		Model:          model,
		ModelID:        modelID,
		Fallbacks:      fallbacks,
		Workspace:      workspace,
		MaxIterations:  maxIter,
		MaxTokens:      maxTokens,
		Temperature:    temperature,
		ContextWindow:  maxTokens,
		Provider:       agentProvider,
		Sessions:       sessionsManager,
		ContextBuilder: contextBuilder,
		Tools:          toolsRegistry,
		Subagents:      subagents,
		SkillsFilter:   skillsFilter,
		IsLocalModel:   isLocal,
		PurposePrompt:  contextBuilder.purposeInstructions,
		Candidates:     candidates,
	}
}

// resolveAgentModelID resolves the raw model ID (without protocol prefix) for a given alias.
// It looks up the alias in cfg.ModelList; if found, it extracts the model ID from the
// Model field (e.g. "openai/gpt-4o" -> "gpt-4o"). Falls back to the alias itself if not found.
func resolveAgentModelID(alias string, cfg *config.Config) string {
	if alias == "" {
		return ""
	}
	mc, err := cfg.GetModelConfig(alias)
	if err != nil || mc == nil {
		// Not found in model_list — alias might already be a raw model ID
		_, id := providers.ExtractProtocol(alias)
		return id
	}
	_, id := providers.ExtractProtocol(mc.Model)
	return id
}

// resolveAgentWorkspace determines the workspace directory for an agent.
func resolveAgentWorkspace(agentCfg *config.AgentConfig, defaults *config.AgentDefaults) string {
	if agentCfg != nil && strings.TrimSpace(agentCfg.Workspace) != "" {
		return expandHome(strings.TrimSpace(agentCfg.Workspace))
	}
	if agentCfg == nil || agentCfg.Default || agentCfg.ID == "" || routing.NormalizeAgentID(agentCfg.ID) == "main" {
		return expandHome(defaults.Workspace)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	id := routing.NormalizeAgentID(agentCfg.ID)
	return filepath.Join(home, ".sofia", "workspace-"+id)
}

// resolveAgentModel resolves the primary model for an agent.
func resolveAgentModel(agentCfg *config.AgentConfig, defaults *config.AgentDefaults) string {
	if agentCfg != nil && agentCfg.Model != nil && strings.TrimSpace(agentCfg.Model.Primary) != "" {
		return strings.TrimSpace(agentCfg.Model.Primary)
	}
	return defaults.GetModelName()
}

// resolveAgentFallbacks resolves the fallback models for an agent.
func resolveAgentFallbacks(agentCfg *config.AgentConfig, defaults *config.AgentDefaults) []string {
	if agentCfg != nil && agentCfg.Model != nil && agentCfg.Model.Fallbacks != nil {
		return agentCfg.Model.Fallbacks
	}
	return defaults.ModelFallbacks
}

func expandHome(path string) string {
	if path == "" {
		return path
	}
	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		if len(path) > 1 && path[1] == '/' {
			return home + path[1:]
		}
		return home
	}
	return path
}
