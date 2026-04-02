package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/utils"
)

// EvaluationLoop scores LLM responses and re-runs the iteration when the
// score is below a configurable threshold.  It keeps the best-scoring
// response across retries and returns that one.
type EvaluationLoop struct {
	db         *memory.MemoryDB
	agentID    string
	threshold  float64
	maxRetries int
}

// NewEvaluationLoop creates an EvaluationLoop from the config.
func NewEvaluationLoop(db *memory.MemoryDB, agentID string, cfg config.EvaluationLoopConfig) *EvaluationLoop {
	threshold := cfg.Threshold
	if threshold <= 0 {
		threshold = 0.7
	}
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &EvaluationLoop{
		db:         db,
		agentID:    agentID,
		threshold:  threshold,
		maxRetries: maxRetries,
	}
}

// evaluationAttempt records one scored attempt.
type evaluationAttempt struct {
	content string
	score   float64
}

// EvaluateAndRetry scores finalContent using the ReflectionEngine. If the
// score is below threshold it injects feedback into messages and re-runs
// the LLM iteration.  Returns the best-scoring response.
func (el *EvaluationLoop) EvaluateAndRetry(
	ctx context.Context,
	agent *AgentInstance,
	messages []providers.Message,
	opts processOptions,
	al *AgentLoop,
	initialContent string,
	iteration, errorCount int,
	durationMs int64,
) (string, error) {
	if el.db == nil || agent == nil || agent.Provider == nil {
		return initialContent, nil
	}

	best := evaluationAttempt{content: initialContent, score: -1}

	for retry := 0; retry <= el.maxRetries; retry++ {
		content := initialContent
		if retry > 0 {
			// Re-run LLM with feedback injected
			var err error
			content, _, _, err = al.runLLMIteration(ctx, agent, messages, opts)
			if err != nil {
				logger.WarnCF("eval_loop", "Re-run LLM iteration failed",
					map[string]any{"retry": retry, "error": err.Error()})
				break
			}
			if content == "" {
				break
			}
		}

		// Score the response
		score, feedback, err := el.score(ctx, agent, opts.SessionKey, content, iteration, errorCount, durationMs)
		if err != nil {
			logger.WarnCF("eval_loop", "Scoring failed, accepting current response",
				map[string]any{"retry": retry, "error": err.Error()})
			if best.score < 0 {
				best = evaluationAttempt{content: content, score: 0.5}
			}
			break
		}

		logger.InfoCF("eval_loop",
			fmt.Sprintf(
				"Evaluation score=%.2f (threshold=%.2f, retry=%d/%d)",
				score, el.threshold, retry, el.maxRetries,
			),
			map[string]any{"agent_id": el.agentID, "score": score, "retry": retry})

		if score > best.score {
			best = evaluationAttempt{content: content, score: score}
		}

		// Good enough — stop retrying
		if score >= el.threshold {
			break
		}

		// Not the last retry — inject feedback for next attempt
		if retry < el.maxRetries {
			feedbackMsg := fmt.Sprintf(
				"[EVALUATION FEEDBACK] Your response scored %.2f (threshold %.2f). Issues: %s. "+
					"Please improve your response addressing these issues.",
				score, el.threshold, feedback,
			)
			messages = append(messages,
				providers.Message{Role: "assistant", Content: content},
				providers.Message{Role: "user", Content: feedbackMsg},
			)
		}
	}

	return best.content, nil
}

// score runs a lightweight reflection to get a numeric score and textual feedback.
func (el *EvaluationLoop) score(
	ctx context.Context,
	agent *AgentInstance,
	sessionKey, content string,
	iteration, errorCount int,
	durationMs int64,
) (float64, string, error) {
	evalCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	history := agent.Sessions.GetHistory(sessionKey)
	summary := buildConversationSummary(history, content)

	evalPrompt := fmt.Sprintf(
		"Evaluate this conversation:\n\n%s\n\nMetrics: %d tool calls, %d errors, %dms duration.",
		summary, iteration, errorCount, durationMs,
	)

	response, err := agent.Provider.Chat(
		evalCtx,
		[]providers.Message{
			{Role: "system", Content: reflectionPrompt},
			{Role: "user", Content: evalPrompt},
		},
		nil,
		agent.ModelID,
		map[string]any{
			"max_tokens":  512,
			"temperature": 0.3,
		},
	)
	if err != nil {
		return 0, "", err
	}

	result, err := parseReflectionJSON(response.Content)
	if err != nil {
		return 0, "", err
	}

	feedback := result.WhatFailed
	if feedback == "" {
		feedback = utils.Truncate(result.Lessons, 200)
	}

	return result.Score, feedback, nil
}
