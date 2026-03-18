package evolution

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	pt "github.com/grasberg/sofia/pkg/providers/protocoltypes"
	"github.com/grasberg/sofia/pkg/utils"
)

// slugPattern validates agent IDs: lowercase letters, digits, and hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// AgentArchitect designs and creates new agents using LLM-generated blueprints.
type AgentArchitect struct {
	provider  providers.LLMProvider
	registrar AgentRegistrar
	a2a       A2ARegistrar
	store     *AgentStore
	memDB     *memory.MemoryDB
	workspace string
}

// NewAgentArchitect creates a new AgentArchitect.
func NewAgentArchitect(
	provider providers.LLMProvider,
	registrar AgentRegistrar,
	a2a A2ARegistrar,
	store *AgentStore,
	memDB *memory.MemoryDB,
	workspace string,
) *AgentArchitect {
	return &AgentArchitect{
		provider:  provider,
		registrar: registrar,
		a2a:       a2a,
		store:     store,
		memDB:     memDB,
		workspace: workspace,
	}
}

// agentBlueprint is the JSON schema the LLM returns when designing an agent.
type agentBlueprint struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	PurposePrompt string   `json:"purpose_prompt"`
	Model         string   `json:"model"`
	SkillsFilter  []string `json:"skills_filter"`
	Temperature   float64  `json:"temperature"`
}

// DesignAgent asks the LLM to design a specialist agent for the given capability gap.
// It returns a fully populated EvolutionAgentConfig ready for CreateAgent.
func (a *AgentArchitect) DesignAgent(ctx context.Context, gapDescription string) (*EvolutionAgentConfig, error) {
	systemMsg := pt.Message{
		Role: "system",
		Content: "You are an AI system architect. You design specialist AI agents. " +
			"Always respond with valid JSON only, no markdown fences or extra text.",
	}
	userMsg := pt.Message{
		Role: "user",
		Content: fmt.Sprintf(
			"Design a specialist AI agent for: %s. "+
				"Return JSON: {\"id\": \"<lowercase-slug>\", \"name\": \"<display name>\", "+
				"\"purpose_prompt\": \"<system prompt for this agent>\", "+
				"\"model\": \"<model identifier>\", "+
				"\"skills_filter\": [\"<skill1>\", ...], "+
				"\"temperature\": <0.0-1.0>}",
			gapDescription,
		),
	}

	resp, err := a.provider.Chat(ctx, []pt.Message{systemMsg, userMsg}, nil, "", nil)
	if err != nil {
		return nil, fmt.Errorf("evolution/architect: LLM call failed: %w", err)
	}

	content := utils.CleanJSONFences(resp.Content)

	var bp agentBlueprint
	if err := json.Unmarshal([]byte(content), &bp); err != nil {
		return nil, fmt.Errorf("evolution/architect: failed to parse LLM response as JSON: %w", err)
	}

	// Validate required fields.
	if bp.ID == "" {
		return nil, fmt.Errorf("evolution/architect: designed agent has empty id")
	}
	if !slugPattern.MatchString(bp.ID) {
		return nil, fmt.Errorf("evolution/architect: designed agent id %q is not a valid slug", bp.ID)
	}
	if bp.Name == "" {
		return nil, fmt.Errorf("evolution/architect: designed agent has empty name")
	}

	cfg := &EvolutionAgentConfig{
		PurposePrompt: bp.PurposePrompt,
		ModelID:       bp.Model,
	}
	cfg.ID = bp.ID
	cfg.Name = bp.Name
	cfg.Skills = bp.SkillsFilter

	if bp.Temperature > 0 {
		temp := bp.Temperature
		cfg.Model = &configModelConfig{Primary: bp.Model}
		_ = temp // temperature stored in purpose_prompt context for now
	}

	logger.InfoCF("evolution", "Agent designed", map[string]any{
		"agent_id": bp.ID,
		"name":     bp.Name,
		"model":    bp.Model,
	})

	return cfg, nil
}

// configModelConfig is a local type alias to avoid importing config in the struct literal.
// EvolutionAgentConfig embeds config.AgentConfig which has Model *AgentModelConfig.
// We use the config package type through the embedded field.
type configModelConfig = config.AgentModelConfig

// CreateAgent persists the agent configuration and registers it for A2A routing.
// It does NOT instantiate the agent in the registry (that requires *AgentInstance
// from pkg/agent). The integration layer in loop.go handles full registration.
func (a *AgentArchitect) CreateAgent(ctx context.Context, cfg EvolutionAgentConfig) error {
	// 1. Persist to store.
	if err := a.store.Save(cfg.ID, cfg); err != nil {
		return fmt.Errorf("evolution/architect: save agent: %w", err)
	}

	// 2. Register with A2A router so other agents can delegate to it.
	a.a2a.Register(cfg.ID)

	// 3. Write a skill file so the agent's purpose is visible in the workspace.
	if err := a.writeSkillFile(cfg); err != nil {
		logger.WarnCF("evolution", "Failed to write skill file for new agent", map[string]any{
			"agent_id": cfg.ID,
			"error":    err.Error(),
		})
		// Non-fatal: the agent is still persisted and registered.
	}

	logger.InfoCF("evolution", "Agent created", map[string]any{
		"agent_id": cfg.ID,
		"name":     cfg.Name,
	})

	return nil
}

// writeSkillFile creates a SKILL.md for the agent in the workspace skills directory.
func (a *AgentArchitect) writeSkillFile(cfg EvolutionAgentConfig) error {
	skillDir := filepath.Join(a.workspace, "skills", cfg.ID)
	if err := os.MkdirAll(skillDir, 0o750); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}

	purpose := cfg.PurposePrompt
	if purpose == "" {
		purpose = fmt.Sprintf("Specialist agent: %s", cfg.Name)
	}

	content := fmt.Sprintf(`---
name: %s
description: %s
---

%s
`, cfg.Name, cfg.Name, purpose)

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}

	return nil
}
