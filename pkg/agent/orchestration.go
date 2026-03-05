package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/grasberg/sofia/pkg/routing"
)

// OrchestrationResult represents the outcome of a multi-agent task.
type OrchestrationResult struct {
	AgentsUsed []string
	Summary    string
	IsComplete bool
}

// Orchestrate manages multiple sub-agents to solve a complex task.
// This is the beginning of autonomous multi-agent orchestration.
func (al *AgentLoop) Orchestrate(
	ctx context.Context,
	task string,
	channel, chatID string,
) (*OrchestrationResult, error) {
	// 1. Identify required expertise (simplified for now: use all agents as candidates)
	allAgents := al.registry.ListAgents()

	// 2. Select a team (e.g. top 2 agents by score)
	// 3. Coordinate execution (spawn tasks)
	// 4. Synthesize results

	results := make([]string, 0)
	for _, agent := range allAgents {
		if agent.ID == routing.DefaultAgentID {
			continue
		}

		// Run as sub-agent
		res, err := al.runSpawnedTaskAsAgent(ctx, agent.ID, "", task, channel, chatID)
		if err == nil {
			results = append(results, fmt.Sprintf("Agent %s: %s", agent.Name, res))
		}
	}

	return &OrchestrationResult{
		Summary:    strings.Join(results, "\n\n"),
		IsComplete: true,
	}, nil
}
