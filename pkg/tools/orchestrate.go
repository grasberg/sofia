package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/grasberg/sofia/pkg/conflict"
)

// OrchestrationTask represents a subtask in an orchestration plan.
type OrchestrationTask struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	AgentID     string   `json:"agent_id,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Status      string   `json:"status"`
	Result      string   `json:"result,omitempty"`
}

// OrchestrateToolConfig holds the dependencies needed by OrchestrateTool.
type OrchestrateToolConfig struct {
	// AgentScorer scores how well an agent matches a task description.
	// Returns a score in [0,1].
	AgentScorer func(agentID, taskDescription string) float64
	// ListAgentIDs returns all available agent IDs.
	ListAgentIDs func() []string
	// RunAgentTask runs a task as a specific agent and returns the result.
	RunAgentTask func(ctx context.Context, agentID, task, channel, chatID string) (string, error)
	// Scratchpad for sharing data between subtasks.
	Scratchpad *SharedScratchpad
}

// OrchestrateTool provides multi-agent orchestration capabilities.
type OrchestrateTool struct {
	config        OrchestrateToolConfig
	originChannel string
	originChatID  string
}

// NewOrchestrateTool creates a new OrchestrateTool.
func NewOrchestrateTool(cfg OrchestrateToolConfig) *OrchestrateTool {
	return &OrchestrateTool{
		config:        cfg,
		originChannel: "cli",
		originChatID:  "direct",
	}
}

func (t *OrchestrateTool) Name() string { return "orchestrate" }
func (t *OrchestrateTool) Description() string {
	return "Orchestrate multiple agents to solve a complex task. Provide a list of subtasks with optional agent assignments and dependencies. Independent subtasks run concurrently."
}

func (t *OrchestrateTool) SetContext(channel, chatID string) {
	t.originChannel = channel
	t.originChatID = chatID
}

func (t *OrchestrateTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"goal": map[string]any{
				"type":        "string",
				"description": "Overall goal of the orchestration",
			},
			"tasks": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{
							"type":        "string",
							"description": "Unique task identifier",
						},
						"description": map[string]any{
							"type":        "string",
							"description": "What the agent should do",
						},
						"agent_id": map[string]any{
							"type":        "string",
							"description": "Target agent ID (auto-assigned if empty)",
						},
						"depends_on": map[string]any{
							"type":        "array",
							"items":       map[string]any{"type": "string"},
							"description": "IDs of tasks that must complete first",
						},
					},
					"required": []string{"id", "description"},
				},
				"description": "List of subtasks to orchestrate",
			},
		},
		"required": []string{"goal", "tasks"},
	}
}

func (t *OrchestrateTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	goal, _ := args["goal"].(string)
	rawTasks, ok := args["tasks"]
	if !ok {
		return ErrorResult("tasks is required")
	}

	// Parse tasks
	tasksData, err := json.Marshal(rawTasks)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to parse tasks: %v", err))
	}

	var tasks []OrchestrationTask
	if err := json.Unmarshal(tasksData, &tasks); err != nil {
		return ErrorResult(fmt.Sprintf("failed to parse tasks: %v", err))
	}

	if len(tasks) == 0 {
		return ErrorResult("at least one task is required")
	}

	// Auto-assign agents using scorer
	agentIDs := t.config.ListAgentIDs()
	for i := range tasks {
		if tasks[i].AgentID == "" && t.config.AgentScorer != nil {
			bestID := ""
			bestScore := 0.0
			for _, id := range agentIDs {
				score := t.config.AgentScorer(id, tasks[i].Description)
				if score > bestScore {
					bestScore = score
					bestID = id
				}
			}
			if bestID != "" {
				tasks[i].AgentID = bestID
			} else if len(agentIDs) > 0 {
				tasks[i].AgentID = agentIDs[0]
			}
		}
		tasks[i].Status = "pending"
	}

	// Build dependency graph
	taskMap := make(map[string]*OrchestrationTask)
	for i := range tasks {
		taskMap[tasks[i].ID] = &tasks[i]
	}

	// Execute in dependency order
	completed := make(map[string]bool)
	failed := make(map[string]bool)
	var mu sync.Mutex

	for len(completed)+len(failed) < len(tasks) {
		// Find tasks ready to run (all dependencies satisfied)
		var ready []*OrchestrationTask
		for i := range tasks {
			task := &tasks[i]
			if task.Status != "pending" {
				continue
			}

			// Check if any dependency has failed
			depFailed := false
			for _, dep := range task.DependsOn {
				if failed[dep] {
					depFailed = true
					break
				}
			}
			if depFailed {
				task.Status = "failed"
				task.Result = "Skipped: dependency failed"
				failed[task.ID] = true
				continue
			}

			allDepsSatisfied := true
			for _, dep := range task.DependsOn {
				if !completed[dep] {
					allDepsSatisfied = false
					break
				}
			}
			if allDepsSatisfied {
				ready = append(ready, task)
			}
		}

		if len(ready) == 0 {
			break // Deadlock or all done
		}

		// Run ready tasks concurrently
		var wg sync.WaitGroup
		toolCfg := t.config
		channel := t.originChannel
		chatID := t.originChatID
		for _, task := range ready {
			task.Status = "running"
			wg.Add(1)
			go func(ot *OrchestrationTask) {
				defer wg.Done()

				// Include scratchpad context if available
				taskPrompt := ot.Description
				if sp := toolCfg.Scratchpad; sp != nil {
					keys := sp.List(goal)
					if len(keys) > 0 {
						var ctxBuf strings.Builder
						ctxBuf.WriteString("\n\nShared context from other agents:\n")
						for _, k := range keys {
							if v, ok := sp.Read(goal, k); ok {
								fmt.Fprintf(&ctxBuf, "- %s: %s\n", k, v)
							}
						}
						taskPrompt += ctxBuf.String()
					}
				}

				result, err := toolCfg.RunAgentTask(ctx, ot.AgentID, taskPrompt, channel, chatID)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					ot.Status = "failed"
					ot.Result = fmt.Sprintf("Error: %v", err)
					failed[ot.ID] = true
				} else {
					ot.Status = "completed"
					ot.Result = result
					completed[ot.ID] = true

					// Store result in scratchpad
					if sp := toolCfg.Scratchpad; sp != nil {
						sp.Write(goal, ot.ID, result)
					}
				}
			}(task)
		}
		wg.Wait()

		// Check for failures that block progress
		if ctx.Err() != nil {
			break
		}
	}

	// Synthesize results
	var sb strings.Builder
	fmt.Fprintf(&sb, "Orchestration Results for: %s\n\n", goal)
	for _, task := range tasks {
		status := task.Status
		icon := "[x]"
		switch status {
		case "failed":
			icon = "[!]"
		case "pending":
			icon = "[ ]"
		}
		fmt.Fprintf(&sb, "%s Task %s (agent: %s): %s\n", icon, task.ID, task.AgentID, task.Description)
		if task.Result != "" {
			resultPreview := task.Result
			if len(resultPreview) > 500 {
				resultPreview = resultPreview[:500] + "..."
			}
			fmt.Fprintf(&sb, "    Result: %s\n", resultPreview)
		}
		sb.WriteString("\n")
	}

	allCompleted := len(completed) == len(tasks)
	if allCompleted {
		fmt.Fprintf(&sb, "All %d tasks completed successfully.", len(tasks))
	} else {
		fmt.Fprintf(&sb, "Completed %d/%d tasks.", len(completed), len(tasks))
		if len(failed) > 0 {
			fmt.Fprintf(&sb, " %d task(s) failed.", len(failed))
		}
	}

	// Conflict detection: analyze completed task outputs for disagreements
	var completedOutputs []conflict.Output
	for _, task := range tasks {
		if task.Status == "completed" && task.Result != "" {
			completedOutputs = append(completedOutputs, conflict.Output{
				AgentID: task.AgentID,
				TaskID:  task.ID,
				Content: task.Result,
			})
		}
	}
	if len(completedOutputs) >= 2 {
		detection := conflict.Detect(completedOutputs)
		if detection.HasConflicts {
			sb.WriteString("\n\n⚠ CONFLICT DETECTION:\n")
			sb.WriteString(detection.Format())
			sb.WriteString("\nUse the conflict_resolve tool to resolve these conflicts, ")
			sb.WriteString("or review the outputs manually to determine the correct result.")
		} else {
			fmt.Fprintf(
				&sb,
				"\n\nNo conflicts detected among task outputs (agreement: %.0f%%).",
				detection.Agreement*100,
			)
		}
	}

	return SilentResult(sb.String())
}
