package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// GetToolStatsTool allows the LLM to query performance metrics for all executed tools.
type GetToolStatsTool struct {
	tracker *ToolTracker
}

func NewGetToolStatsTool(tracker *ToolTracker) *GetToolStatsTool {
	return &GetToolStatsTool{tracker: tracker}
}

func (t *GetToolStatsTool) Name() string { return "get_tool_stats" }
func (t *GetToolStatsTool) Description() string {
	return "Retrieve performance metrics and success rates for tools to understand which tools are most effective or failure-prone."
}

func (t *GetToolStatsTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{}, // No parameters needed
	}
}

func (t *GetToolStatsTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.tracker == nil {
		return ErrorResult("tool performance tracking is not enabled")
	}

	stats := t.tracker.GetStats()
	if len(stats) == 0 {
		return SilentResult("No tool execution statistics available yet.")
	}

	// Sort by highest usage first
	var keys []string
	for k := range stats {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return stats[keys[i]].UsageCount > stats[keys[j]].UsageCount
	})

	var sb strings.Builder
	sb.WriteString("Tool Performance Statistics:\n")
	sb.WriteString("===========================\n")

	for _, name := range keys {
		stat := stats[name]
		sb.WriteString(fmt.Sprintf("\nTool: %s\n", stat.Name))
		sb.WriteString(fmt.Sprintf("  Uses:    %d\n", stat.UsageCount))
		sb.WriteString(fmt.Sprintf("  Success: %d (%.1f%%)\n", stat.SuccessCount, stat.SuccessRate*100))
		sb.WriteString(fmt.Sprintf("  Errors:  %d\n", stat.ErrorCount))
		sb.WriteString(fmt.Sprintf("  AvgTime: %s\n", stat.AverageTime.Truncate(time.Millisecond)))
	}

	return SilentResult(sb.String())
}
