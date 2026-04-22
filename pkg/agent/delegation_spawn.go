package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/routing"
)

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

	// Protect config mutation with mutex
	al.configMu.Lock()
	al.cfg.Agents.List = append(al.cfg.Agents.List, *agentCfg)
	al.configMu.Unlock()

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

	// Protect config mutation with mutex
	al.configMu.Lock()
	al.cfg.Agents.List = append(al.cfg.Agents.List, *agentCfg)
	al.configMu.Unlock()

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
