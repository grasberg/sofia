package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/routing"
)

// semanticDelegateToAll uses an LLM call to determine ALL suitable agents.
// Returns agent IDs that the LLM thinks should handle parts of the message.
func (al *AgentLoop) semanticDelegateToAll(ctx context.Context, msg string) []delegationCandidate {
	defaultAgent := al.getRegistry().GetDefaultAgent()
	if defaultAgent == nil || defaultAgent.Provider == nil {
		return nil
	}

	agents := al.getRegistry().ListAgents()
	if len(agents) <= 1 {
		return nil
	}

	var agentDescs []string
	for _, agent := range agents {
		if agent.ID == routing.DefaultAgentID {
			continue
		}
		desc := fmt.Sprintf("- id: %q, name: %q", agent.ID, agent.Name)
		if agent.PurposePrompt != "" {
			purpose := agent.PurposePrompt
			if len(purpose) > 100 {
				purpose = purpose[:100] + "..."
			}
			desc += fmt.Sprintf(", purpose: %q", purpose)
		}
		if len(agent.SkillsFilter) > 0 {
			desc += fmt.Sprintf(", skills: %v", agent.SkillsFilter)
		}
		agentDescs = append(agentDescs, desc)
	}

	prompt := fmt.Sprintf(
		`Which agents should handle this user message? Multiple agents can work in parallel on different aspects. Return ONLY a JSON object with "agent_ids" field (array of strings). Return {"agent_ids": []} if none fit.

Available agents:
%s

User message: %s`,
		strings.Join(agentDescs, "\n"),
		msg,
	)

	delegateCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := defaultAgent.Provider.Chat(delegateCtx, []providers.Message{
		{
			Role:    "system",
			Content: "You are a routing assistant. Reply with only valid JSON. Select ALL agents whose skills are relevant to any part of the message.",
		},
		{Role: "user", Content: prompt},
	}, nil, defaultAgent.ModelID, map[string]any{
		"max_tokens":  200,
		"temperature": 0.0,
	})
	if err != nil {
		logger.DebugCF("delegation", "Semantic delegation LLM call failed",
			map[string]any{"error": err.Error()})
		return nil
	}

	var result struct {
		AgentIDs []string `json:"agent_ids"`
	}

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		logger.DebugCF("delegation", "Failed to parse semantic delegation response",
			map[string]any{"content": content, "error": err.Error()})
		return nil
	}

	var candidates []delegationCandidate
	msgLower := strings.ToLower(msg)
	for _, id := range result.AgentIDs {
		if id == "" {
			continue
		}
		agent, ok := al.getRegistry().GetAgent(id)
		if !ok {
			continue
		}
		s := scoreCandidate(agent, msgLower)
		candidates = append(candidates, delegationCandidate{Agent: agent, Score: s})
	}

	if len(candidates) > 0 {
		logger.InfoCF("delegation",
			fmt.Sprintf("Semantic delegation selected %d agent(s)", len(candidates)),
			map[string]any{"agents": result.AgentIDs})
	}
	return candidates
}

// findMissingCapabilities checks the message for capability needs not covered by existing agents.
// Returns capabilities that should be auto-spawned.
func (al *AgentLoop) findMissingCapabilities(msg string) []AgentCapability {
	// Collect all skills covered by existing agents
	coveredSkills := map[string]bool{}
	for _, id := range al.registry.ListAgentIDs() {
		agent, ok := al.registry.GetAgent(id)
		if !ok {
			continue
		}
		for _, s := range agent.SkillsFilter {
			coveredSkills[strings.ToLower(s)] = true
		}
	}

	// Find capabilities matching the message that no agent covers
	matched := FindCapabilitiesForMessage(msg)
	var missing []AgentCapability
	for _, cap := range matched {
		// Check if any existing agent already covers this capability
		covered := false
		for _, skill := range cap.Skills {
			if coveredSkills[skill] {
				covered = true
				break
			}
		}
		if !covered {
			missing = append(missing, cap)
		}
	}
	return missing
}

// findMissingSkills checks available skills against the message and existing agents.
// Returns skill names that are relevant to the message but not covered by any agent.
func (al *AgentLoop) findMissingSkills(msg string) []string {
	defaultAgent := al.getRegistry().GetDefaultAgent()
	if defaultAgent == nil {
		return nil
	}

	coveredSkills := map[string]bool{}
	for _, id := range al.registry.ListAgentIDs() {
		agent, ok := al.registry.GetAgent(id)
		if !ok {
			continue
		}
		for _, s := range agent.SkillsFilter {
			coveredSkills[strings.ToLower(s)] = true
		}
	}

	loader := defaultAgent.ContextBuilder.GetSkillsLoader()
	if loader == nil {
		return nil
	}
	allSkills := loader.ListSkills()

	msgLower := strings.ToLower(msg)
	var missing []string
	for _, skill := range allSkills {
		nameLower := strings.ToLower(skill.Name)
		if strings.Contains(msgLower, nameLower) && !coveredSkills[nameLower] {
			missing = append(missing, skill.Name)
		}
	}
	return missing
}
