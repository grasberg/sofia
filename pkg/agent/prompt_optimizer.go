package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/abtest"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/trace"
)

// PromptOptimizer implements lightweight automatic prompt optimization
// inspired by Agent Lightning's APO (Automatic Prompt Optimization).
// Instead of full beam search, it uses score-triggered LLM critique
// and Sofia's existing A/B test infrastructure.
type PromptOptimizer struct {
	tracer   *trace.Tracer
	memDB    *memory.MemoryDB
	abMgr    *abtest.Manager
	cfg      config.PromptOptimizationConfig
	provider providers.LLMProvider
}

// NewPromptOptimizer creates an optimizer with the given dependencies.
func NewPromptOptimizer(
	tracer *trace.Tracer,
	memDB *memory.MemoryDB,
	provider providers.LLMProvider,
	cfg config.PromptOptimizationConfig,
) *PromptOptimizer {
	po := &PromptOptimizer{
		tracer:   tracer,
		memDB:    memDB,
		abMgr:    abtest.NewManager(memDB),
		cfg:      cfg,
		provider: provider,
	}
	// Apply defaults
	if po.cfg.ScoreThreshold <= 0 {
		po.cfg.ScoreThreshold = 0.6
	}
	if po.cfg.MinTraces <= 0 {
		po.cfg.MinTraces = 20
	}
	if po.cfg.MaxVariants <= 0 {
		po.cfg.MaxVariants = 2
	}
	if po.cfg.TrialsPerVariant <= 0 {
		po.cfg.TrialsPerVariant = 10
	}
	return po
}

// PromptReview contains the evaluation result for an agent's prompt performance.
type PromptReview struct {
	AgentID   string
	AvgScore  float64
	LowTraces []trace.TraceSummary
	NeedsWork bool
	Critique  string
}

// Evaluate checks whether an agent's prompts need optimization based on recent trace scores.
func (po *PromptOptimizer) Evaluate(agentID string) (*PromptReview, error) {
	if po.tracer == nil {
		return &PromptReview{AgentID: agentID}, nil
	}

	since := time.Now().AddDate(0, 0, -7) // last 7 days
	summaries, err := po.tracer.QueryTraces(trace.TraceFilter{
		AgentID: agentID,
		Since:   since,
		Limit:   po.cfg.MinTraces * 2,
	})
	if err != nil {
		return nil, fmt.Errorf("query traces: %w", err)
	}

	if len(summaries) < po.cfg.MinTraces {
		return &PromptReview{AgentID: agentID, NeedsWork: false}, nil
	}

	// Compute average task_completion score
	var totalScore float64
	var scored int
	var lowTraces []trace.TraceSummary
	for _, s := range summaries {
		tc, ok := s.Scores["task_completion"]
		if !ok {
			continue
		}
		totalScore += tc
		scored++
		if tc < po.cfg.ScoreThreshold {
			lowTraces = append(lowTraces, s)
		}
	}

	if scored == 0 {
		return &PromptReview{AgentID: agentID, NeedsWork: false}, nil
	}

	avgScore := totalScore / float64(scored)
	review := &PromptReview{
		AgentID:   agentID,
		AvgScore:  avgScore,
		NeedsWork: avgScore < po.cfg.ScoreThreshold,
	}

	// Keep only the 3 worst-scoring traces for critique
	if len(lowTraces) > 3 {
		lowTraces = lowTraces[:3]
	}
	review.LowTraces = lowTraces

	return review, nil
}

// GenerateVariants uses LLM critique to propose improved prompt variants.
// Returns A/B test variant configs ready for experimentation.
func (po *PromptOptimizer) GenerateVariants(
	ctx context.Context,
	review *PromptReview,
	currentPrompt string,
) (map[string]abtest.VariantConfig, error) {
	if po.provider == nil {
		return nil, fmt.Errorf("no LLM provider available for prompt optimization")
	}

	// Build critique request from low-scoring traces
	var traceDescriptions strings.Builder
	for i, t := range review.LowTraces {
		fmt.Fprintf(&traceDescriptions, "\nTrace %d (score: %.2f):\n", i+1, t.Scores["task_completion"])
		fmt.Fprintf(&traceDescriptions, "  Agent: %s\n", t.AgentID)
		fmt.Fprintf(&traceDescriptions, "  Name: %s\n", t.Name)
		fmt.Fprintf(&traceDescriptions, "  Duration: %dms\n", t.DurationMs)
		fmt.Fprintf(&traceDescriptions, "  Scores: %v\n", t.Scores)
	}

	critiquePrompt := fmt.Sprintf(`You are an expert at optimizing AI system prompts.

The following agent has been underperforming (avg score: %.2f, threshold: %.2f).

Current system prompt:
---
%s
---

Low-scoring interaction traces:
%s

Analyze the system prompt and the low-scoring traces. Identify specific weaknesses in the prompt that may be causing poor performance.

Then generate exactly %d improved variant(s) of the system prompt.

For each variant:
1. Explain what you changed and why (1-2 sentences)
2. Provide the full improved system prompt

Format your response as:
CRITIQUE: <your analysis>

VARIANT 1:
CHANGES: <what changed>
PROMPT: <full improved prompt>

VARIANT 2:
CHANGES: <what changed>
PROMPT: <full improved prompt>`,
		review.AvgScore, po.cfg.ScoreThreshold,
		currentPrompt,
		traceDescriptions.String(),
		po.cfg.MaxVariants,
	)

	messages := []providers.Message{
		{Role: "user", Content: critiquePrompt},
	}

	resp, err := po.provider.Chat(ctx, messages, nil, "", map[string]any{
		"max_tokens":  4096,
		"temperature": 0.7,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM critique call failed: %w", err)
	}

	// Parse variants from the response
	variants := po.parseVariants(resp.Content, currentPrompt)
	if len(variants) == 0 {
		return nil, fmt.Errorf("no valid variants generated from critique")
	}

	return variants, nil
}

// CreateExperiment sets up an A/B test experiment for the generated variants.
func (po *PromptOptimizer) CreateExperiment(
	agentID string,
	variants map[string]abtest.VariantConfig,
) (*abtest.Experiment, error) {
	name := fmt.Sprintf("prompt-opt-%s-%d", agentID, time.Now().Unix())
	desc := fmt.Sprintf("Automatic prompt optimization for agent %s", agentID)
	return po.abMgr.CreateExperiment(name, desc, variants)
}

// CheckAndApply analyzes an active experiment and applies the winner if conclusive.
func (po *PromptOptimizer) CheckAndApply(experimentName string) (string, error) {
	analysis, err := po.abMgr.Analyze(experimentName)
	if err != nil {
		return "", err
	}

	if analysis.Recommendation == "" || analysis.Recommendation == "insufficient_data" {
		return "", nil
	}

	// Conclude the experiment
	if err := po.abMgr.ConcludeExperiment(experimentName, analysis.Recommendation); err != nil {
		return "", fmt.Errorf("conclude experiment: %w", err)
	}

	logger.InfoCF("prompt-optimizer",
		fmt.Sprintf("Experiment %s concluded — winner: %s", experimentName, analysis.Recommendation),
		map[string]any{"recommendation": analysis.Recommendation})

	return analysis.Recommendation, nil
}

// parseVariants extracts prompt variants from the LLM critique response.
func (po *PromptOptimizer) parseVariants(response, baseline string) map[string]abtest.VariantConfig {
	variants := make(map[string]abtest.VariantConfig)

	// Always include the baseline as "control"
	variants["control"] = abtest.VariantConfig{SystemPrompt: baseline}

	// Parse VARIANT sections from the response
	parts := strings.Split(response, "VARIANT ")
	for i, part := range parts {
		if i == 0 {
			continue // skip the critique preamble
		}

		// Find the PROMPT: section
		promptIdx := strings.Index(part, "PROMPT:")
		if promptIdx < 0 {
			promptIdx = strings.Index(part, "PROMPT :")
		}
		if promptIdx < 0 {
			continue
		}

		prompt := strings.TrimSpace(part[promptIdx+7:])
		// Trim trailing VARIANT marker or end-of-content
		if nextVariant := strings.Index(prompt, "\nVARIANT "); nextVariant > 0 {
			prompt = prompt[:nextVariant]
		}
		prompt = strings.TrimSpace(prompt)

		if len(prompt) < 20 {
			continue // too short to be a real prompt
		}

		name := fmt.Sprintf("variant_%d", i)
		variants[name] = abtest.VariantConfig{SystemPrompt: prompt}

		if len(variants) > po.cfg.MaxVariants+1 { // +1 for control
			break
		}
	}

	// Need at least control + 1 variant
	if len(variants) < 2 {
		return nil
	}
	return variants
}
