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

// delegationThreshold is the minimum score required to automatically delegate
// a user message to a sub-agent. Hardcoded for v1; not user-configurable.
const delegationThreshold = 0.72

// scoreCandidate computes a delegation score in [0,1] for a given sub-agent
// against a user message. Three components are summed with fixed weights:
//
//   - Skills keyword overlap (weight 0.60) — each skill name that appears in
//     the lowercased message adds to the fraction of skills matched.
//   - Purpose instructions overlap (weight 0.25) — words from the agent's
//     purpose prompt that appear in the message.
//   - Name / template hint (weight 0.15) — whether the agent's name or
//     template slug appears verbatim in the message.
func scoreCandidate(agent *AgentInstance, msgLower string) float64 {
	const (
		wSkills  = 0.60
		wPurpose = 0.25
		wHint    = 0.15
	)

	score := 0.0

	// --- Skills component ---
	if len(agent.SkillsFilter) > 0 {
		matched := 0
		for _, skill := range agent.SkillsFilter {
			if strings.Contains(msgLower, strings.ToLower(skill)) {
				matched++
			}
		}
		score += wSkills * (float64(matched) / float64(len(agent.SkillsFilter)))
	}

	// --- Purpose instructions component ---
	if agent.PurposePrompt != "" {
		purposeLower := strings.ToLower(agent.PurposePrompt)
		// Use the significant words from the purpose prompt (length > 3)
		words := strings.Fields(purposeLower)
		significant := make([]string, 0, len(words))
		for _, w := range words {
			w = strings.Trim(w, ".,;:!?\"'()")
			if len(w) > 3 {
				significant = append(significant, w)
			}
		}
		if len(significant) > 0 {
			matched := 0
			for _, w := range significant {
				if strings.Contains(msgLower, w) {
					matched++
				}
			}
			score += wPurpose * (float64(matched) / float64(len(significant)))
		}
	}

	// --- Name / template hint component ---
	agentNameLower := strings.ToLower(strings.TrimSpace(agent.Name))
	templateLower := strings.ToLower(strings.TrimSpace(agent.Template))
	if agentNameLower != "" && strings.Contains(msgLower, agentNameLower) {
		score += wHint
	} else if templateLower != "" && strings.Contains(msgLower, templateLower) {
		score += wHint
	}

	return score
}

// delegateTo returns the best sub-agent to handle msg, or nil if no candidate
// exceeds delegationThreshold. It never returns the main/default agent itself.
func (al *AgentLoop) delegateTo(msg string) *AgentInstance {
	msgLower := strings.ToLower(msg)

	var bestAgent *AgentInstance
	bestScore := 0.0

	for _, id := range al.registry.ListAgentIDs() {
		if routing.NormalizeAgentID(id) == routing.DefaultAgentID {
			continue // skip main/Sofia
		}
		candidate, ok := al.registry.GetAgent(id)
		if !ok || candidate == nil {
			continue
		}
		s := scoreCandidate(candidate, msgLower)
		if s > bestScore {
			bestScore = s
			bestAgent = candidate
		}
	}

	if bestScore >= delegationThreshold {
		return bestAgent
	}
	return nil
}

// semanticDelegateTo uses an LLM call to determine the best agent for a message.
// This is a fallback when keyword-based scoring doesn't find a match above threshold.
// Returns nil if no suitable agent is found or LLM delegation is unavailable.
func (al *AgentLoop) semanticDelegateTo(ctx context.Context, msg string) *AgentInstance {
	defaultAgent := al.getRegistry().GetDefaultAgent()
	if defaultAgent == nil || defaultAgent.Provider == nil {
		return nil
	}

	// Build agent descriptions for the LLM
	agents := al.getRegistry().ListAgents()
	if len(agents) <= 1 {
		return nil // Only the main agent exists
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

	prompt := fmt.Sprintf(`Which agent best handles this user message? Return ONLY a JSON object with "agent_id" field, or {"agent_id": ""} if none fit.

Available agents:
%s

User message: %s`, strings.Join(agentDescs, "\n"), msg)

	// Use a short timeout for this delegation call
	delegateCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := defaultAgent.Provider.Chat(delegateCtx, []providers.Message{
		{Role: "system", Content: "You are a routing assistant. Reply with only valid JSON."},
		{Role: "user", Content: prompt},
	}, nil, defaultAgent.ModelID, map[string]any{
		"max_tokens":  100,
		"temperature": 0.0,
	})
	if err != nil {
		logger.DebugCF("delegation", "Semantic delegation LLM call failed",
			map[string]any{"error": err.Error()})
		return nil
	}

	// Parse response
	var result struct {
		AgentID string `json:"agent_id"`
	}

	content := strings.TrimSpace(resp.Content)
	// Strip markdown code fences if present
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		logger.DebugCF("delegation", "Failed to parse semantic delegation response",
			map[string]any{"content": content, "error": err.Error()})
		return nil
	}

	if result.AgentID == "" {
		return nil
	}

	agent, ok := al.getRegistry().GetAgent(result.AgentID)
	if !ok {
		return nil
	}

	logger.InfoCF("delegation", fmt.Sprintf("Semantic delegation selected agent %q", agent.Name),
		map[string]any{"agent_id": agent.ID, "message_preview": msg[:min(len(msg), 80)]})
	return agent
}
