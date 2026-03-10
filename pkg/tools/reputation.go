package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/grasberg/sofia/pkg/reputation"
)

// ReputationTool lets agents query and manage agent reputation data.
type ReputationTool struct {
	manager *reputation.Manager
}

// NewReputationTool creates a new reputation tool.
func NewReputationTool(mgr *reputation.Manager) *ReputationTool {
	return &ReputationTool{manager: mgr}
}

func (t *ReputationTool) Name() string { return "agent_reputation" }

func (t *ReputationTool) Description() string {
	return "Track and query agent performance history. " +
		"View which agents perform best at which task " +
		"categories. Operations: stats, categories, " +
		"history, rank, record, score."
}

func (t *ReputationTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type": "string",
				"enum": []string{
					"stats", "categories", "history",
					"rank", "record", "score",
				},
				"description": "The operation to perform",
			},
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Agent ID to query or record for",
			},
			"category": map[string]any{
				"type": "string",
				"description": "Task category " +
					"(for rank or record)",
			},
			"task": map[string]any{
				"type":        "string",
				"description": "Task description (for record)",
			},
			"success": map[string]any{
				"type":        "boolean",
				"description": "Whether the task succeeded (for record)",
			},
			"outcome_id": map[string]any{
				"type":        "number",
				"description": "Outcome ID to score",
			},
			"score_value": map[string]any{
				"type":        "number",
				"description": "Quality score 0.0-1.0 (for score)",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "Max results for history (default 20)",
			},
		},
		"required": []string{"operation"},
	}
}

func (t *ReputationTool) Execute(
	_ context.Context, args map[string]any,
) *ToolResult {
	op, _ := args["operation"].(string) //nolint:errcheck

	switch op {
	case "stats":
		return t.stats(args)
	case "categories":
		return t.categories(args)
	case "history":
		return t.history(args)
	case "rank":
		return t.rank(args)
	case "record":
		return t.record(args)
	case "score":
		return t.scoreOp(args)
	default:
		return ErrorResult(fmt.Sprintf(
			"unknown operation %q: use stats, categories, "+
				"history, rank, record, or score", op,
		))
	}
}

func (t *ReputationTool) stats(args map[string]any) *ToolResult {
	agentID, _ := args["agent_id"].(string) //nolint:errcheck

	if agentID != "" {
		s, err := t.manager.GetAgentStats(agentID)
		if err != nil {
			return ErrorResult(err.Error())
		}
		return NewToolResult(formatAgentStats(*s))
	}

	// All agents.
	all, err := t.manager.GetAllAgentStats()
	if err != nil {
		return ErrorResult(err.Error())
	}
	if len(all) == 0 {
		return NewToolResult("No reputation data recorded yet.")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d agent(s) with reputation data:\n\n",
		len(all))
	for _, s := range all {
		sb.WriteString(formatAgentStats(s))
		sb.WriteString("\n")
	}
	return NewToolResult(sb.String())
}

func (t *ReputationTool) categories(
	args map[string]any,
) *ToolResult {
	agentID, _ := args["agent_id"].(string) //nolint:errcheck
	if agentID == "" {
		return ErrorResult("agent_id is required")
	}

	cats, err := t.manager.GetCategoryStats(agentID)
	if err != nil {
		return ErrorResult(err.Error())
	}
	if len(cats) == 0 {
		return NewToolResult(fmt.Sprintf(
			"No category data for agent %q.", agentID,
		))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Category performance for %q:\n", agentID)
	for _, c := range cats {
		fmt.Fprintf(&sb,
			"  %s: %d tasks, %.0f%% success",
			c.Category, c.TotalTasks, c.SuccessRate*100)
		if c.ScoredCount > 0 {
			fmt.Fprintf(&sb, ", avg score %.2f", c.AvgScore)
		}
		sb.WriteString("\n")
	}
	return NewToolResult(sb.String())
}

func (t *ReputationTool) history(args map[string]any) *ToolResult {
	agentID, _ := args["agent_id"].(string) //nolint:errcheck
	if agentID == "" {
		return ErrorResult("agent_id is required")
	}

	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	outcomes, err := t.manager.GetRecentOutcomes(agentID, limit)
	if err != nil {
		return ErrorResult(err.Error())
	}
	if len(outcomes) == 0 {
		return NewToolResult(fmt.Sprintf(
			"No history for agent %q.", agentID,
		))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Recent %d outcome(s) for %q:\n",
		len(outcomes), agentID)
	for _, o := range outcomes {
		status := "OK"
		if !o.Success {
			status = "FAIL"
		}
		taskPreview := o.Task
		if len(taskPreview) > 80 {
			taskPreview = taskPreview[:80] + "..."
		}
		fmt.Fprintf(&sb, "  #%d [%s] %s — %s (%dms)",
			o.ID, status, o.Category, taskPreview, o.LatencyMs)
		if o.Score != nil {
			fmt.Fprintf(&sb, " score=%.2f", *o.Score)
		}
		sb.WriteString("\n")
	}
	return NewToolResult(sb.String())
}

func (t *ReputationTool) rank(args map[string]any) *ToolResult {
	category, _ := args["category"].(string) //nolint:errcheck
	if category == "" {
		return ErrorResult(
			"category is required (e.g., coding, writing, research)",
		)
	}

	all, err := t.manager.GetAllAgentStats()
	if err != nil {
		return ErrorResult(err.Error())
	}
	if len(all) == 0 {
		return NewToolResult("No reputation data available.")
	}

	var agentIDs []string
	for _, s := range all {
		agentIDs = append(agentIDs, s.AgentID)
	}

	type ranked struct {
		id    string
		score float64
	}
	var rankings []ranked
	for _, id := range agentIDs {
		score := t.manager.ReputationScore(id, category)
		rankings = append(rankings, ranked{id, score})
	}

	// Sort by score descending.
	for i := 0; i < len(rankings); i++ {
		for j := i + 1; j < len(rankings); j++ {
			if rankings[j].score > rankings[i].score {
				rankings[i], rankings[j] = rankings[j], rankings[i]
			}
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Agent ranking for %q:\n", category)
	for i, r := range rankings {
		fmt.Fprintf(&sb, "  %d. %s — reputation score: %.3f\n",
			i+1, r.id, r.score)
	}
	return NewToolResult(sb.String())
}

func (t *ReputationTool) record(args map[string]any) *ToolResult {
	agentID, _ := args["agent_id"].(string) //nolint:errcheck
	if agentID == "" {
		return ErrorResult("agent_id is required")
	}
	task, _ := args["task"].(string) //nolint:errcheck
	if task == "" {
		return ErrorResult("task is required")
	}
	success, _ := args["success"].(bool) //nolint:errcheck
	category, _ := args["category"].(string) //nolint:errcheck

	id, err := t.manager.RecordOutcome(reputation.TaskOutcome{
		AgentID:  agentID,
		Category: category,
		Task:     task,
		Success:  success,
	})
	if err != nil {
		return ErrorResult(err.Error())
	}

	status := "success"
	if !success {
		status = "failure"
	}
	return NewToolResult(fmt.Sprintf(
		"Recorded %s for agent %q (outcome #%d)",
		status, agentID, id,
	))
}

func (t *ReputationTool) scoreOp(args map[string]any) *ToolResult {
	outcomeID, ok := args["outcome_id"].(float64)
	if !ok {
		return ErrorResult("outcome_id is required")
	}
	scoreVal, ok := args["score_value"].(float64)
	if !ok {
		return ErrorResult("score_value is required (0.0-1.0)")
	}

	if err := t.manager.ScoreOutcome(
		int64(outcomeID), scoreVal,
	); err != nil {
		return ErrorResult(err.Error())
	}
	return NewToolResult(fmt.Sprintf(
		"Scored outcome #%d with %.2f",
		int64(outcomeID), scoreVal,
	))
}

func formatAgentStats(s reputation.AgentStats) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Agent %q:\n", s.AgentID)
	fmt.Fprintf(&sb, "  Tasks: %d (%d success, %d failed)\n",
		s.TotalTasks, s.Successes, s.Failures)
	fmt.Fprintf(&sb, "  Success rate: %.0f%%\n",
		s.SuccessRate*100)
	if s.ScoredCount > 0 {
		fmt.Fprintf(&sb, "  Avg score: %.2f (%d scored)\n",
			s.AvgScore, s.ScoredCount)
	}
	fmt.Fprintf(&sb, "  Avg latency: %.0fms\n", s.AvgLatencyMs)
	return sb.String()
}
