package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grasberg/sofia/pkg/abtest"
	"github.com/grasberg/sofia/pkg/providers"
)

// ABTestTool exposes behavioral A/B testing to agents.
type ABTestTool struct {
	manager      *abtest.Manager
	provider     providers.LLMProvider
	defaultModel string
}

// NewABTestTool creates a new A/B testing tool.
func NewABTestTool(
	mgr *abtest.Manager,
	provider providers.LLMProvider,
	defaultModel string,
) *ABTestTool {
	return &ABTestTool{
		manager:      mgr,
		provider:     provider,
		defaultModel: defaultModel,
	}
}

func (t *ABTestTool) Name() string { return "ab_test" }

func (t *ABTestTool) Description() string {
	return "Run behavioral A/B tests to compare different approaches " +
		"(models, temperatures, prompts) and measure which works better. " +
		"Operations: create, run, score, analyze, list, conclude, delete."
}

func (t *ABTestTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type": "string",
				"enum": []string{
					"create", "run", "score",
					"analyze", "list", "conclude", "delete",
				},
				"description": "The operation to perform",
			},
			"experiment": map[string]any{
				"type":        "string",
				"description": "Experiment name",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Experiment description (for create)",
			},
			"variants": map[string]any{
				"type": "object",
				"description": "Map of variant name to config " +
					"(for create). Each config can have: model, " +
					"temperature, max_tokens, system_prompt, " +
					"prompt_prefix, prompt_suffix",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "Prompt to test (for run)",
			},
			"trial_id": map[string]any{
				"type":        "number",
				"description": "Trial ID to score (for score)",
			},
			"score": map[string]any{
				"type":        "number",
				"description": "Score from 0.0 to 1.0 (for score)",
			},
			"winner": map[string]any{
				"type":        "string",
				"description": "Winning variant name (for conclude)",
			},
		},
		"required": []string{"operation"},
	}
}

func (t *ABTestTool) Execute(
	ctx context.Context, args map[string]any,
) *ToolResult {
	op, _ := args["operation"].(string)

	switch op {
	case "create":
		return t.create(args)
	case "run":
		return t.run(ctx, args)
	case "score":
		return t.score(args)
	case "analyze":
		return t.analyze(args)
	case "list":
		return t.list()
	case "conclude":
		return t.conclude(args)
	case "delete":
		return t.deleteExp(args)
	default:
		return ErrorResult(fmt.Sprintf(
			"unknown operation %q: use create, run, score, "+
				"analyze, list, conclude, or delete", op,
		))
	}
}

func (t *ABTestTool) create(args map[string]any) *ToolResult {
	name, _ := args["experiment"].(string)
	if name == "" {
		return ErrorResult("experiment name is required")
	}
	desc, _ := args["description"].(string)

	variantsRaw, ok := args["variants"].(map[string]any)
	if !ok || len(variantsRaw) < 2 {
		return ErrorResult(
			"variants must be an object with at least 2 entries",
		)
	}

	variants := make(map[string]abtest.VariantConfig, len(variantsRaw))
	for vName, vCfgRaw := range variantsRaw {
		var cfg abtest.VariantConfig
		// Re-marshal/unmarshal to convert the map to struct.
		b, _ := json.Marshal(vCfgRaw)
		_ = json.Unmarshal(b, &cfg)
		variants[vName] = cfg
	}

	exp, err := t.manager.CreateExperiment(name, desc, variants)
	if err != nil {
		return ErrorResult(err.Error())
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Created experiment %q with %d variants:\n",
		exp.Name, len(exp.Variants))
	for _, v := range exp.Variants {
		cfgJSON, _ := json.Marshal(v.Config)
		fmt.Fprintf(&sb, "  - %s: %s\n", v.Name, string(cfgJSON))
	}
	return NewToolResult(sb.String())
}

func (t *ABTestTool) run(
	ctx context.Context, args map[string]any,
) *ToolResult {
	name, _ := args["experiment"].(string)
	if name == "" {
		return ErrorResult("experiment name is required")
	}
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return ErrorResult("prompt is required")
	}

	trials, err := t.manager.RunTrial(
		ctx, name, prompt, t.provider, t.defaultModel,
	)
	if err != nil {
		return ErrorResult(err.Error())
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Ran %d trials for experiment %q:\n\n",
		len(trials), name)
	for _, trial := range trials {
		fmt.Fprintf(&sb, "--- Variant: %s (trial #%d) ---\n",
			trial.VariantName, trial.ID)
		if trial.Error != "" {
			fmt.Fprintf(&sb, "ERROR: %s\n", trial.Error)
		} else {
			response := trial.Response
			if len(response) > 500 {
				response = response[:500] + "..."
			}
			fmt.Fprintf(&sb, "Response: %s\n", response)
			fmt.Fprintf(&sb, "Latency: %dms | Tokens: %d in, %d out\n",
				trial.LatencyMs, trial.TokensIn, trial.TokensOut)
		}
		sb.WriteString("\n")
	}
	sb.WriteString(
		"Use score operation with trial_id and score (0.0-1.0) " +
			"to rate each response.")
	return NewToolResult(sb.String())
}

func (t *ABTestTool) score(args map[string]any) *ToolResult {
	trialID, ok := args["trial_id"].(float64)
	if !ok {
		return ErrorResult("trial_id is required")
	}
	score, ok := args["score"].(float64)
	if !ok {
		return ErrorResult("score is required (0.0-1.0)")
	}

	if err := t.manager.ScoreTrial(int64(trialID), score); err != nil {
		return ErrorResult(err.Error())
	}

	return NewToolResult(fmt.Sprintf(
		"Scored trial #%d with %.2f", int64(trialID), score,
	))
}

func (t *ABTestTool) analyze(args map[string]any) *ToolResult {
	name, _ := args["experiment"].(string)
	if name == "" {
		return ErrorResult("experiment name is required")
	}

	analysis, err := t.manager.Analyze(name)
	if err != nil {
		return ErrorResult(err.Error())
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Analysis for %q (%s, %d total trials):\n\n",
		analysis.ExperimentName, analysis.Status, analysis.TotalTrials)

	for _, s := range analysis.Stats {
		fmt.Fprintf(&sb, "Variant %q:\n", s.VariantName)
		fmt.Fprintf(&sb, "  Trials: %d (%d scored)\n",
			s.TrialCount, s.ScoredCount)
		if s.ScoredCount > 0 {
			fmt.Fprintf(&sb,
				"  Score: avg=%.2f min=%.2f max=%.2f stddev=%.2f\n",
				s.AvgScore, s.MinScore, s.MaxScore, s.StdDevScore)
		}
		fmt.Fprintf(&sb, "  Latency: %.0fms avg\n", s.AvgLatencyMs)
		fmt.Fprintf(&sb, "  Tokens: %.0f in, %.0f out avg\n",
			s.AvgTokensIn, s.AvgTokensOut)
		if s.ErrorCount > 0 {
			fmt.Fprintf(&sb, "  Errors: %d (%.0f%%)\n",
				s.ErrorCount, s.ErrorRate*100)
		}
		sb.WriteString("\n")
	}

	fmt.Fprintf(&sb, "Recommendation: %s", analysis.Recommendation)
	return NewToolResult(sb.String())
}

func (t *ABTestTool) list() *ToolResult {
	exps, err := t.manager.ListExperiments()
	if err != nil {
		return ErrorResult(err.Error())
	}
	if len(exps) == 0 {
		return NewToolResult("No experiments found.")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d experiment(s):\n", len(exps))
	for _, exp := range exps {
		line := fmt.Sprintf("  - %s [%s]", exp.Name, exp.Status)
		if exp.Winner != "" {
			line += fmt.Sprintf(" winner=%s", exp.Winner)
		}
		if exp.Description != "" {
			line += fmt.Sprintf(" — %s", exp.Description)
		}
		sb.WriteString(line + "\n")
	}
	return NewToolResult(sb.String())
}

func (t *ABTestTool) conclude(args map[string]any) *ToolResult {
	name, _ := args["experiment"].(string)
	if name == "" {
		return ErrorResult("experiment name is required")
	}
	winner, _ := args["winner"].(string)
	if winner == "" {
		return ErrorResult("winner variant name is required")
	}

	if err := t.manager.ConcludeExperiment(name, winner); err != nil {
		return ErrorResult(err.Error())
	}
	return NewToolResult(fmt.Sprintf(
		"Experiment %q concluded. Winner: %s", name, winner,
	))
}

func (t *ABTestTool) deleteExp(args map[string]any) *ToolResult {
	name, _ := args["experiment"].(string)
	if name == "" {
		return ErrorResult("experiment name is required")
	}

	if err := t.manager.DeleteExperiment(name); err != nil {
		return ErrorResult(err.Error())
	}
	return NewToolResult(fmt.Sprintf("Deleted experiment %q", name))
}
