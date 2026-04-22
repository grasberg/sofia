package abtest

import (
	"context"
	"fmt"
	"time"

	"github.com/grasberg/sofia/pkg/providers"
)

// RunTrial executes a prompt against all variants in an experiment
// and records the results.
func (m *Manager) RunTrial(
	ctx context.Context,
	experimentName, prompt string,
	provider providers.LLMProvider,
	defaultModel string,
) ([]Trial, error) {
	exp, err := m.GetExperiment(experimentName)
	if err != nil {
		return nil, err
	}
	if exp.Status != "active" {
		return nil, fmt.Errorf(
			"experiment %q is %s, not active",
			experimentName, exp.Status,
		)
	}

	var trials []Trial
	for _, v := range exp.Variants {
		trial := m.runSingleTrial(
			ctx, exp.ID, v, prompt, provider, defaultModel,
		)
		trials = append(trials, trial)
	}

	return trials, nil
}

func (m *Manager) runSingleTrial(
	ctx context.Context,
	experimentID int64,
	v Variant,
	prompt string,
	provider providers.LLMProvider,
	defaultModel string,
) Trial {
	// Build messages with variant overrides.
	var messages []providers.Message
	if v.Config.SystemPrompt != "" {
		messages = append(messages, providers.Message{
			Role:    "system",
			Content: v.Config.SystemPrompt,
		})
	}

	userContent := prompt
	if v.Config.PromptPrefix != "" {
		userContent = v.Config.PromptPrefix + "\n" + userContent
	}
	if v.Config.PromptSuffix != "" {
		userContent = userContent + "\n" + v.Config.PromptSuffix
	}
	messages = append(messages, providers.Message{
		Role:    "user",
		Content: userContent,
	})

	// Select model.
	model := defaultModel
	if v.Config.Model != "" {
		model = v.Config.Model
	}

	// Build options.
	opts := map[string]any{}
	if v.Config.Temperature != nil {
		opts["temperature"] = *v.Config.Temperature
	}
	if v.Config.MaxTokens > 0 {
		opts["max_tokens"] = v.Config.MaxTokens
	}

	start := time.Now()
	resp, err := provider.Chat(ctx, messages, nil, model, opts)
	latencyMs := time.Since(start).Milliseconds()

	trial := Trial{
		ExperimentID: experimentID,
		VariantID:    v.ID,
		VariantName:  v.Name,
		Prompt:       prompt,
		LatencyMs:    latencyMs,
		CreatedAt:    time.Now(),
	}

	if err != nil {
		trial.Error = err.Error()
	} else {
		trial.Response = resp.Content
		if resp.Usage != nil {
			trial.TokensIn = resp.Usage.PromptTokens
			trial.TokensOut = resp.Usage.CompletionTokens
		}
	}

	// Persist the trial.
	res, dbErr := m.db.Exec(
		`INSERT INTO ab_trials
		 (experiment_id, variant_id, prompt, response, latency_ms,
		  tokens_in, tokens_out, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		experimentID, v.ID, prompt, trial.Response, trial.LatencyMs,
		trial.TokensIn, trial.TokensOut, trial.Error,
	)
	if dbErr == nil {
		trial.ID, _ = res.LastInsertId() //nolint:errcheck
	}

	return trial
}

// ScoreTrial assigns a score (0.0-1.0) to a trial.
func (m *Manager) ScoreTrial(trialID int64, score float64) error {
	if score < 0 || score > 1 {
		return fmt.Errorf("score must be between 0.0 and 1.0")
	}
	_, err := m.db.Exec(
		`UPDATE ab_trials SET score = ? WHERE id = ?`, score, trialID,
	)
	return err
}
