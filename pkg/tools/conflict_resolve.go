package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grasberg/sofia/pkg/conflict"
)

// ConflictResolveTool allows agents to detect and resolve conflicting outputs
// from parallel agents. It reads from the shared scratchpad to gather outputs
// and applies configurable resolution strategies.
type ConflictResolveTool struct {
	scratchpad *SharedScratchpad
}

// NewConflictResolveTool creates a new ConflictResolveTool.
func NewConflictResolveTool(scratchpad *SharedScratchpad) *ConflictResolveTool {
	return &ConflictResolveTool{scratchpad: scratchpad}
}

func (t *ConflictResolveTool) Name() string { return "conflict_resolve" }

func (t *ConflictResolveTool) Description() string {
	return "Detect and resolve conflicting outputs from parallel agents. " +
		"Operations: detect (find conflicts in outputs), resolve (apply a resolution strategy), " +
		"detect_scratchpad (detect conflicts in scratchpad group). " +
		"Strategies: majority_vote, priority, merge, shortest, longest, all."
}

func (t *ConflictResolveTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"detect", "resolve", "detect_scratchpad"},
				"description": "The operation to perform",
			},
			"outputs": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"agent_id": map[string]any{
							"type":        "string",
							"description": "Agent that produced this output",
						},
						"task_id": map[string]any{
							"type":        "string",
							"description": "Task identifier (optional)",
						},
						"content": map[string]any{
							"type":        "string",
							"description": "The agent's output content",
						},
						"priority": map[string]any{
							"type":        "integer",
							"description": "Priority level (higher = more authoritative)",
						},
					},
					"required": []string{"agent_id", "content"},
				},
				"description": "Agent outputs to analyze (for detect/resolve operations)",
			},
			"strategy": map[string]any{
				"type":        "string",
				"enum":        []string{"majority_vote", "priority", "merge", "shortest", "longest", "all"},
				"description": "Resolution strategy (for resolve operation, default: majority_vote)",
			},
			"group": map[string]any{
				"type":        "string",
				"description": "Scratchpad group to analyze (for detect_scratchpad operation)",
			},
		},
		"required": []string{"operation"},
	}
}

func (t *ConflictResolveTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	op, _ := args["operation"].(string)

	switch op {
	case "detect":
		return t.detect(args)
	case "resolve":
		return t.resolve(args)
	case "detect_scratchpad":
		return t.detectScratchpad(args)
	default:
		return ErrorResult(fmt.Sprintf("unknown conflict_resolve operation: %q", op))
	}
}

func (t *ConflictResolveTool) detect(args map[string]any) *ToolResult {
	outputs, err := parseOutputs(args)
	if err != nil {
		return ErrorResult(err.Error())
	}
	if len(outputs) < 2 {
		return SilentResult("Need at least 2 outputs to detect conflicts.")
	}

	result := conflict.Detect(outputs)
	return SilentResult(result.Format())
}

func (t *ConflictResolveTool) resolve(args map[string]any) *ToolResult {
	outputs, err := parseOutputs(args)
	if err != nil {
		return ErrorResult(err.Error())
	}
	if len(outputs) == 0 {
		return ErrorResult("outputs are required for resolve operation")
	}

	strategyStr, _ := args["strategy"].(string)
	strategy := conflict.StrategyMajorityVote
	switch strategyStr {
	case "priority":
		strategy = conflict.StrategyPriority
	case "merge":
		strategy = conflict.StrategyMerge
	case "shortest":
		strategy = conflict.StrategyShortest
	case "longest":
		strategy = conflict.StrategyLongest
	case "all":
		strategy = conflict.StrategyAll
	}

	// First detect, then resolve
	detection := conflict.Detect(outputs)
	resolution := conflict.Resolve(outputs, strategy)

	var sb strings.Builder
	sb.WriteString(detection.Format())
	sb.WriteString("\n")
	sb.WriteString(resolution.Format())

	// Include the winning/merged content for the LLM to use
	if resolution.Winner != nil {
		fmt.Fprintf(&sb, "\n[RESOLVED_CONTENT]\n%s", resolution.Winner.Content)
	} else if resolution.Merged != "" {
		fmt.Fprintf(&sb, "\n[RESOLVED_CONTENT]\n%s", resolution.Merged)
	}

	return SilentResult(sb.String())
}

func (t *ConflictResolveTool) detectScratchpad(args map[string]any) *ToolResult {
	group, _ := args["group"].(string)
	if group == "" {
		return ErrorResult("group is required for detect_scratchpad operation")
	}

	if t.scratchpad == nil {
		return ErrorResult("scratchpad not available")
	}

	keys := t.scratchpad.List(group)
	if len(keys) < 2 {
		return SilentResult(
			fmt.Sprintf("Scratchpad group %q has %d entries — no conflicts possible.", group, len(keys)),
		)
	}

	outputs := make([]conflict.Output, 0, len(keys))
	for _, key := range keys {
		val, ok := t.scratchpad.Read(group, key)
		if !ok || val == "" {
			continue
		}
		outputs = append(outputs, conflict.Output{
			AgentID: key, // task ID used as agent ID for scratchpad entries
			TaskID:  key,
			Content: val,
		})
	}

	if len(outputs) < 2 {
		return SilentResult("Not enough non-empty scratchpad entries to detect conflicts.")
	}

	result := conflict.Detect(outputs)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Scratchpad group %q conflict analysis:\n", group)
	sb.WriteString(result.Format())

	// If conflicts found, suggest resolution
	if result.HasConflicts {
		sb.WriteString("\nTo resolve, use: conflict_resolve with operation='resolve' and the outputs above, ")
		sb.WriteString("choosing a strategy: majority_vote, priority, merge, shortest, longest, or all.")
	}

	return SilentResult(sb.String())
}

func parseOutputs(args map[string]any) ([]conflict.Output, error) {
	rawOutputs, ok := args["outputs"]
	if !ok {
		return nil, fmt.Errorf("outputs parameter is required")
	}

	data, err := json.Marshal(rawOutputs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal outputs: %w", err)
	}

	var outputs []conflict.Output
	if err := json.Unmarshal(data, &outputs); err != nil {
		return nil, fmt.Errorf("failed to parse outputs: %w", err)
	}
	return outputs, nil
}
