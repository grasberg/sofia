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
	Status      string // "pending", "running", "completed", "failed", "skipped"
	Result      string
}

// topoSortSubtasks performs a topological sort on subtasks using Kahn's algorithm.
// Returns ordered waves of task IDs that can run concurrently within each wave.
// Returns an error if a cycle is detected or a dependency references a non-existent task.
func topoSortSubtasks(subtasks []OrchestrationSubtask) ([][]string, error) {
	taskByID := make(map[string]*OrchestrationSubtask, len(subtasks))
	inDegree := make(map[string]int, len(subtasks))
	dependents := make(map[string][]string, len(subtasks)) // parent -> children that depend on it

	for i := range subtasks {
		st := &subtasks[i]
		if _, dup := taskByID[st.ID]; dup {
			return nil, fmt.Errorf("duplicate subtask ID %q", st.ID)
		}
		taskByID[st.ID] = st
		inDegree[st.ID] = 0
	}

	// Build dependency graph
	for i := range subtasks {
		st := &subtasks[i]
		for _, dep := range st.DependsOn {
			if _, ok := taskByID[dep]; !ok {
				return nil, fmt.Errorf("subtask %q depends on unknown task %q", st.ID, dep)
			}
			inDegree[st.ID]++
			dependents[dep] = append(dependents[dep], st.ID)
		}
	}

	// Kahn's algorithm: collect waves of tasks with zero in-degree
	var waves [][]string
	processed := 0

	for processed < len(subtasks) {
		var wave []string
		for id, deg := range inDegree {
			if deg == 0 {
				wave = append(wave, id)
			}
		}
		if len(wave) == 0 {
			// Remaining tasks all have unmet dependencies: cycle detected
			var cycleIDs []string
			for id, deg := range inDegree {
				if deg > 0 {
					cycleIDs = append(cycleIDs, id)
				}
			}
			return nil, fmt.Errorf("dependency cycle detected among subtasks: %v", cycleIDs)
		}

		waves = append(waves, wave)
		for _, id := range wave {
			delete(inDegree, id)
			processed++
			for _, child := range dependents[id] {
				if _, ok := inDegree[child]; ok {
					inDegree[child]--
				}
			}
		}
	}

	return waves, nil
}

// OrchestrateSubtasks executes pre-built subtasks respecting dependency order.
// Tasks within the same wave run concurrently. If a task fails, all transitive
// dependents are marked as skipped with a cascade failure message.
func (al *AgentLoop) OrchestrateSubtasks(
	ctx context.Context,
	subtasks []OrchestrationSubtask,
	channel, chatID string,
) (*OrchestrationResult, error) {
	if len(subtasks) == 0 {
		return &OrchestrationResult{
			Summary:    "No subtasks to execute.",
			IsComplete: true,
		}, nil
	}

	waves, err := topoSortSubtasks(subtasks)
	if err != nil {
		return nil, fmt.Errorf("orchestration dependency error: %w", err)
	}

	// Index subtasks by ID for mutable access
	taskByID := make(map[string]*OrchestrationSubtask, len(subtasks))
	for i := range subtasks {
		subtasks[i].Status = "pending"
		taskByID[subtasks[i].ID] = &subtasks[i]
	}

	// Execute wave by wave
	for waveIdx, wave := range waves {
		logger.InfoCF("orchestration",
			fmt.Sprintf("Executing wave %d with %d task(s): %v", waveIdx, len(wave), wave), nil)

		type taskResult struct {
			id     string
			result string
			err    error
		}

		results := make([]taskResult, len(wave))
		var wg sync.WaitGroup

		for i, taskID := range wave {
			st := taskByID[taskID]

			// Skip tasks whose dependencies failed (cascade)
			if st.Status == "skipped" {
				results[i] = taskResult{id: taskID, err: fmt.Errorf("skipped: upstream dependency failed")}
				continue
			}

			// Build context with dependency results for the task description
			taskDesc := st.Description
			for _, dep := range st.DependsOn {
				depTask := taskByID[dep]
				if depTask.Status == "completed" && depTask.Result != "" {
					taskDesc += fmt.Sprintf(
						"\n\n[Context from prerequisite task %q]:\n%s", dep, depTask.Result)
				}
			}

			wg.Add(1)
			go func(idx int, id, agentID, desc string) {
				defer wg.Done()

				logger.InfoCF("orchestration",
					fmt.Sprintf("Dispatching subtask %q to agent %q (wave %d)", id, agentID, waveIdx),
					map[string]any{"task_id": id, "agent_id": agentID, "wave": waveIdx})

				res, runErr := al.runSpawnedTaskAsAgent(ctx, agentID, "", desc, channel, chatID)
				results[idx] = taskResult{id: id, result: res, err: runErr}
			}(i, taskID, st.AgentID, taskDesc)
		}

		wg.Wait()

		// Process results and cascade failures
		for _, r := range results {
			st := taskByID[r.id]
			if r.err != nil {
				st.Status = "failed"
				st.Result = r.err.Error()
				// Mark all transitive dependents as skipped
				cascadeSkip(taskByID, r.id, buildDependentsMap(subtasks))
			} else {
				st.Status = "completed"
				st.Result = r.result
			}
		}
	}

	// Synthesize results in subtask order
	var sb strings.Builder
	var usedAgents []string
	completedCount := 0

	for _, st := range subtasks {
		usedAgents = append(usedAgents, st.AgentID)
		switch st.Status {
		case "completed":
			sb.WriteString(fmt.Sprintf("Task %s (agent %s):\n%s\n\n", st.ID, st.AgentID, st.Result))
			completedCount++
		case "failed":
			sb.WriteString(fmt.Sprintf("Task %s (agent %s): FAILED — %s\n\n", st.ID, st.AgentID, st.Result))
		case "skipped":
			sb.WriteString(fmt.Sprintf("Task %s (agent %s): SKIPPED — upstream dependency failed\n\n",
				st.ID, st.AgentID))
		}
	}

	return &OrchestrationResult{
		AgentsUsed: usedAgents,
		Summary:    sb.String(),
		IsComplete: completedCount == len(subtasks),
	}, nil
}

// buildDependentsMap returns a map from task ID to the IDs of tasks that depend on it.
func buildDependentsMap(subtasks []OrchestrationSubtask) map[string][]string {
	deps := make(map[string][]string, len(subtasks))
	for _, st := range subtasks {
		for _, dep := range st.DependsOn {
			deps[dep] = append(deps[dep], st.ID)
		}
	}
	return deps
}

// cascadeSkip marks all transitive dependents of a failed task as skipped.
func cascadeSkip(taskByID map[string]*OrchestrationSubtask, failedID string, dependents map[string][]string) {
	for _, childID := range dependents[failedID] {
		child := taskByID[childID]
		if child.Status == "pending" {
			child.Status = "skipped"
			child.Result = fmt.Sprintf("upstream task %q failed", failedID)
			cascadeSkip(taskByID, childID, dependents)
		}
	}
}

// Orchestrate manages multiple sub-agents to solve a complex task.
// Phase 1: Identify best agents for the task
// Phase 2: Execute subtasks respecting DependsOn ordering (concurrent within each wave)
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

	// Build subtasks — one per scored agent, no dependencies in auto-mode
	subtasks := make([]OrchestrationSubtask, len(scored))
	for i, sa := range scored {
		subtasks[i] = OrchestrationSubtask{
			ID:          sa.agent.ID,
			Description: task,
			AgentID:     sa.agent.ID,
		}
	}

	return al.OrchestrateSubtasks(ctx, subtasks, channel, chatID)
}
