package agent

import (
	"strings"

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
