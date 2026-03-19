package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/routing"
)

// delegationThreshold is the minimum score for automatic delegation.
const delegationThreshold = 0.35

// delegationCandidate holds an agent and its delegation score.
type delegationCandidate struct {
	Agent *AgentInstance
	Score float64
}

// swedishNames is a pool of Swedish girls' names used for auto-created subagents.
var swedishNames = []string{
	"Astrid", "Ebba", "Ella", "Elsa", "Freja", "Greta", "Hilda",
	"Ines", "Iris", "Klara", "Lova", "Maja", "Nora", "Saga",
	"Signe", "Sigrid", "Stella", "Svea", "Tyra", "Wilma",
	"Alma", "Alva", "Edith", "Elin", "Elvira", "Emmy", "Hilma",
	"Lykke", "Märta", "Ronja", "Siri", "Tilde", "Tuva", "Vera",
}

// usedNames tracks which names have already been assigned to avoid duplicates.
var usedNames = map[string]bool{}
var usedNamesMu sync.Mutex

func pickSwedishName() string {
	usedNamesMu.Lock()
	defer usedNamesMu.Unlock()

	// Try to find an unused name
	shuffled := make([]string, len(swedishNames))
	copy(shuffled, swedishNames)
	rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	for _, n := range shuffled {
		if !usedNames[n] {
			usedNames[n] = true
			return n
		}
	}
	// All names used — pick random with suffix
	base := swedishNames[rand.Intn(len(swedishNames))]
	name := fmt.Sprintf("%s-%d", base, rand.Intn(99)+1)
	usedNames[name] = true
	return name
}

// scoreCandidate computes a delegation score in [0,1] for a given sub-agent.
func scoreCandidate(agent *AgentInstance, msgLower string) float64 {
	const (
		wSkills  = 0.60
		wPurpose = 0.25
		wHint    = 0.15
	)

	score := 0.0

	if len(agent.SkillsFilter) > 0 {
		matched := 0
		for _, skill := range agent.SkillsFilter {
			if strings.Contains(msgLower, strings.ToLower(skill)) {
				matched++
			}
		}
		score += wSkills * (float64(matched) / float64(len(agent.SkillsFilter)))
	}

	if agent.PurposePrompt != "" {
		purposeLower := strings.ToLower(agent.PurposePrompt)
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

	agentNameLower := strings.ToLower(strings.TrimSpace(agent.Name))
	templateLower := strings.ToLower(strings.TrimSpace(agent.Template))
	if agentNameLower != "" && strings.Contains(msgLower, agentNameLower) {
		score += wHint
	} else if templateLower != "" && strings.Contains(msgLower, templateLower) {
		score += wHint
	}

	return score
}

// delegateToAll returns ALL sub-agents that score above delegationThreshold,
// sorted by score descending. Returns nil if none qualify.
func (al *AgentLoop) delegateToAll(msg string) []delegationCandidate {
	msgLower := strings.ToLower(msg)
	var candidates []delegationCandidate

	for _, id := range al.registry.ListAgentIDs() {
		if routing.NormalizeAgentID(id) == routing.DefaultAgentID {
			continue
		}
		agent, ok := al.registry.GetAgent(id)
		if !ok || agent == nil {
			continue
		}
		s := scoreCandidate(agent, msgLower)
		if s >= delegationThreshold {
			candidates = append(candidates, delegationCandidate{Agent: agent, Score: s})
		}
	}

	// Sort by score descending
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].Score > candidates[i].Score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	if len(candidates) == 0 {
		return nil
	}
	return candidates
}

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

	prompt := fmt.Sprintf(`Which agents should handle this user message? Multiple agents can work in parallel on different aspects. Return ONLY a JSON object with "agent_ids" field (array of strings). Return {"agent_ids": []} if none fit.

Available agents:
%s

User message: %s`, strings.Join(agentDescs, "\n"), msg)

	delegateCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := defaultAgent.Provider.Chat(delegateCtx, []providers.Message{
		{Role: "system", Content: "You are a routing assistant. Reply with only valid JSON. Select ALL agents whose skills are relevant to any part of the message."},
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

// spawnAgentForCapability creates a new agent from a built-in capability template
// with a random Swedish girls' name.
func (al *AgentLoop) spawnAgentForCapability(cap AgentCapability) (*AgentInstance, error) {
	name := pickSwedishName()
	id := strings.ToLower(name)

	if existing, ok := al.getRegistry().GetAgent(id); ok {
		return existing, nil
	}

	defaultAgent := al.getRegistry().GetDefaultAgent()
	if defaultAgent == nil {
		return nil, fmt.Errorf("no default agent to derive config from")
	}

	agentCfg := &config.AgentConfig{
		ID:     id,
		Name:   name,
		Skills: cap.Skills,
		Subagents: &config.SubagentsConfig{
			AllowAgents: []string{"*"},
		},
	}

	instance := NewAgentInstance(
		agentCfg,
		&al.cfg.Agents.Defaults,
		al.cfg,
		defaultAgent.Provider,
		al.memDB,
		nil,
	)

	// Apply capability instructions as purpose prompt
	instance.PurposePrompt = cap.Instructions
	instance.Template = cap.ID

	if err := al.getRegistry().RegisterAgent(instance); err != nil {
		return nil, fmt.Errorf("failed to register agent %q: %w", id, err)
	}

	al.cfg.Agents.List = append(al.cfg.Agents.List, *agentCfg)

	logger.InfoCF("delegation",
		fmt.Sprintf("Auto-created %s agent %q (%s) with skills %v",
			cap.Name, name, cap.ID, cap.Skills),
		map[string]any{"agent_id": id, "name": name, "capability": cap.ID, "skills": cap.Skills})

	return instance, nil
}

// spawnAgentForSkills creates a new agent at runtime with the given skills.
// Tries to match a built-in capability template first.
func (al *AgentLoop) spawnAgentForSkills(skills []string) (*AgentInstance, error) {
	// Try to match a built-in capability
	cap := FindCapabilityForSkills(skills)
	if cap != nil {
		return al.spawnAgentForCapability(*cap)
	}

	// Fallback: generic agent with skills
	name := pickSwedishName()
	id := strings.ToLower(name)

	if existing, ok := al.getRegistry().GetAgent(id); ok {
		return existing, nil
	}

	defaultAgent := al.getRegistry().GetDefaultAgent()
	if defaultAgent == nil {
		return nil, fmt.Errorf("no default agent to derive config from")
	}

	agentCfg := &config.AgentConfig{
		ID:     id,
		Name:   name,
		Skills: skills,
		Subagents: &config.SubagentsConfig{
			AllowAgents: []string{"*"},
		},
	}

	instance := NewAgentInstance(
		agentCfg,
		&al.cfg.Agents.Defaults,
		al.cfg,
		defaultAgent.Provider,
		al.memDB,
		nil,
	)

	if err := al.getRegistry().RegisterAgent(instance); err != nil {
		return nil, fmt.Errorf("failed to register agent %q: %w", id, err)
	}

	al.cfg.Agents.List = append(al.cfg.Agents.List, *agentCfg)

	logger.InfoCF("delegation",
		fmt.Sprintf("Auto-created agent %q with skills %v", name, skills),
		map[string]any{"agent_id": id, "name": name, "skills": skills})

	return instance, nil
}

// runMultiDelegation executes multiple subagents in parallel and returns
// a combined synthesis message with all results.
func (al *AgentLoop) runMultiDelegation(
	ctx context.Context,
	candidates []delegationCandidate,
	msg, channel, chatID string,
) (string, error) {
	type result struct {
		agentName string
		agentID   string
		content   string
		err       error
		dur       int64
	}

	results := make([]result, len(candidates))
	var wg sync.WaitGroup

	for i, c := range candidates {
		wg.Add(1)
		go func(idx int, cand delegationCandidate) {
			defer wg.Done()
			agentName := cand.Agent.Name
			if agentName == "" {
				agentName = cand.Agent.ID
			}

			al.dashboardHub.Broadcast(map[string]any{
				"type":          "agent_delegating",
				"from_agent_id": routing.DefaultAgentID,
				"to_agent_id":   cand.Agent.ID,
				"to_agent_name": agentName,
				"reason":        "multi_delegation",
			})

			start := time.Now()
			content, err := al.runSpawnedTaskAsAgent(ctx, cand.Agent.ID, "", msg, channel, chatID)
			dur := time.Since(start).Milliseconds()

			results[idx] = result{
				agentName: agentName,
				agentID:   cand.Agent.ID,
				content:   content,
				err:       err,
				dur:       dur,
			}
		}(i, c)
	}

	wg.Wait()

	// Build synthesis message from all results
	var parts []string
	successCount := 0
	for _, r := range results {
		if r.err != nil {
			logger.WarnCF("delegation",
				fmt.Sprintf("Agent %q failed during multi-delegation", r.agentName),
				map[string]any{"agent_id": r.agentID, "error": r.err.Error(), "duration_ms": r.dur})
			parts = append(parts,
				fmt.Sprintf("[Result from %s — FAILED: %s]", r.agentName, r.err.Error()))
		} else {
			successCount++
			logger.InfoCF("delegation",
				fmt.Sprintf("Agent %q completed in %dms", r.agentName, r.dur),
				map[string]any{"agent_id": r.agentID, "duration_ms": r.dur, "result_len": len(r.content)})
			parts = append(parts,
				fmt.Sprintf("[Result from %s]\n\n%s", r.agentName, r.content))
		}
	}

	if successCount == 0 {
		return "", fmt.Errorf("all %d delegated agents failed", len(candidates))
	}

	return strings.Join(parts, "\n\n---\n\n"), nil
}
