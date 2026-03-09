package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/routing"
)

// OrchestrationResult represents the outcome of a multi-agent task.
type OrchestrationResult struct {
	AgentsUsed []string
	Summary    string
	IsComplete bool
}

// OrchestrationSubtask represents a decomposed subtask for orchestration.
type OrchestrationSubtask struct {
	ID          string
	Description string
	AgentID     string
	DependsOn   []string
	Status      string
	Result      string
}

// Orchestrate manages multiple sub-agents to solve a complex task.
// Phase 1: Identify best agents for the task
// Phase 2: Execute subtasks concurrently (independent) or sequentially (dependent)
// Phase 3: Synthesize results
func (al *AgentLoop) Orchestrate(
	ctx context.Context,
	task string,
	channel, chatID string,
) (*OrchestrationResult, error) {
	allAgents := al.registry.ListAgents()

	// Filter to sub-agents only (skip main)
	var candidates []*AgentInstance
	for _, agent := range allAgents {
		if agent.ID == routing.DefaultAgentID {
			continue
		}
		candidates = append(candidates, agent)
	}

	if len(candidates) == 0 {
		return &OrchestrationResult{
			Summary:    "No sub-agents available for orchestration.",
			IsComplete: false,
		}, nil
	}

	// Score and rank candidates for this task
	msgLower := strings.ToLower(task)
	type scoredAgent struct {
		agent *AgentInstance
		score float64
	}

	var scored []scoredAgent
	for _, agent := range candidates {
		s := scoreCandidate(agent, msgLower)
		if s > 0.1 { // Only include agents with some relevance
			scored = append(scored, scoredAgent{agent: agent, score: s})
		}
	}

	// If no agents score above threshold, use all candidates
	if len(scored) == 0 {
		for _, agent := range candidates {
			scored = append(scored, scoredAgent{agent: agent, score: 0})
		}
	}

	// Execute tasks concurrently
	type agentResult struct {
		agentID string
		name    string
		result  string
		err     error
	}

	results := make([]agentResult, len(scored))
	var wg sync.WaitGroup

	for i, sa := range scored {
		wg.Add(1)
		go func() {
			defer wg.Done()

			logger.InfoCF("orchestration", fmt.Sprintf("Dispatching to agent %q (score=%.2f)", sa.agent.Name, sa.score),
				map[string]any{"agent_id": sa.agent.ID, "score": sa.score})

			res, err := al.runSpawnedTaskAsAgent(ctx, sa.agent.ID, "", task, channel, chatID)
			results[i] = agentResult{
				agentID: sa.agent.ID,
				name:    sa.agent.Name,
				result:  res,
				err:     err,
			}
		}()
	}
	wg.Wait()

	// Synthesize results
	var sb strings.Builder
	var usedAgents []string
	completedCount := 0

	for _, r := range results {
		if r.err != nil {
			sb.WriteString(fmt.Sprintf("Agent %s: Error — %v\n\n", r.name, r.err))
		} else {
			sb.WriteString(fmt.Sprintf("Agent %s:\n%s\n\n", r.name, r.result))
			completedCount++
		}
		usedAgents = append(usedAgents, r.agentID)
	}

	return &OrchestrationResult{
		AgentsUsed: usedAgents,
		Summary:    sb.String(),
		IsComplete: completedCount == len(results),
	}, nil
}
