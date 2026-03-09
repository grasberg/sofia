package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/utils"
)

// ReflectionEngine runs post-task self-evaluation and stores structured reflections.
type ReflectionEngine struct {
	db      *memory.MemoryDB
	agentID string
}

// NewReflectionEngine creates a new ReflectionEngine.
func NewReflectionEngine(db *memory.MemoryDB, agentID string) *ReflectionEngine {
	return &ReflectionEngine{
		db:      db,
		agentID: agentID,
	}
}

// reflectionPrompt is the system prompt for post-task self-evaluation.
const reflectionPrompt = `You are performing a self-evaluation of your recent task performance.
Analyze the conversation and provide a structured JSON assessment.

Respond ONLY with valid JSON in this exact format:
{
  "task_summary": "Brief 1-line summary of what was asked",
  "what_worked": "What went well in handling this task",
  "what_failed": "What went wrong or could have been better (empty string if nothing)",
  "lessons": "Specific, actionable lesson for future similar tasks",
  "score": 0.8
}

Score guidelines (0.0-1.0):
- 1.0: Perfect execution, no errors, efficient approach
- 0.7-0.9: Good execution with minor issues
- 0.4-0.6: Mediocre — significant issues or inefficiency
- 0.0-0.3: Poor — major errors, wrong approach, or task failure

Be honest and specific. Focus on actionable lessons, not generic advice.`

// reflectionResult is the JSON structure returned by the self-evaluation LLM call.
type reflectionResult struct {
	TaskSummary string  `json:"task_summary"`
	WhatWorked  string  `json:"what_worked"`
	WhatFailed  string  `json:"what_failed"`
	Lessons     string  `json:"lessons"`
	Score       float64 `json:"score"`
}

// Reflect runs a post-task self-evaluation and stores the result.
func (re *ReflectionEngine) Reflect(
	ctx context.Context,
	agent *AgentInstance,
	sessionKey string,
	finalResponse string,
	toolCount, errorCount int,
	durationMs int64,
) error {
	if re.db == nil || agent == nil || agent.Provider == nil {
		return nil
	}

	// Build a conversation summary from session history for the LLM to evaluate
	history := agent.Sessions.GetHistory(sessionKey)
	conversationSummary := buildConversationSummary(history, finalResponse)

	evalPrompt := fmt.Sprintf(
		"Evaluate this conversation:\n\n%s\n\nMetrics: %d tool calls, %d errors, %dms duration.",
		conversationSummary, toolCount, errorCount, durationMs,
	)

	evalCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	response, err := agent.Provider.Chat(
		evalCtx,
		[]providers.Message{
			{Role: "system", Content: reflectionPrompt},
			{Role: "user", Content: evalPrompt},
		},
		nil,
		agent.ModelID,
		map[string]any{
			"max_tokens":       512,
			"temperature":      0.3,
			"prompt_cache_key": agent.ID + ":reflection",
		},
	)
	if err != nil {
		logger.WarnCF("reflection", "Self-evaluation LLM call failed",
			map[string]any{"agent_id": re.agentID, "error": err.Error()})
		return re.saveFallbackReflection(sessionKey, toolCount, errorCount, durationMs)
	}

	var result reflectionResult
	if err := json.Unmarshal([]byte(response.Content), &result); err != nil {
		// Try to extract JSON from response (in case LLM wrapped it in markdown)
		if extracted := extractJSON(response.Content); extracted != "" {
			if err2 := json.Unmarshal([]byte(extracted), &result); err2 != nil {
				logger.WarnCF("reflection", "Failed to parse reflection JSON",
					map[string]any{"error": err2.Error(), "content": utils.Truncate(response.Content, 200)})
				return re.saveFallbackReflection(sessionKey, toolCount, errorCount, durationMs)
			}
		} else {
			logger.WarnCF("reflection", "Failed to parse reflection JSON",
				map[string]any{"error": err.Error(), "content": utils.Truncate(response.Content, 200)})
			return re.saveFallbackReflection(sessionKey, toolCount, errorCount, durationMs)
		}
	}

	// Clamp score to valid range
	if result.Score < 0 {
		result.Score = 0
	}
	if result.Score > 1 {
		result.Score = 1
	}

	record := memory.ReflectionRecord{
		AgentID:     re.agentID,
		SessionKey:  sessionKey,
		TaskSummary: result.TaskSummary,
		WhatWorked:  result.WhatWorked,
		WhatFailed:  result.WhatFailed,
		Lessons:     result.Lessons,
		Score:       result.Score,
		ToolCount:   toolCount,
		ErrorCount:  errorCount,
		DurationMs:  durationMs,
	}

	if err := re.db.SaveReflection(record); err != nil {
		return fmt.Errorf("reflection: save: %w", err)
	}

	// Also log to daily memory for persistence
	memStore := NewMemoryStore(re.db, re.agentID)
	entry := fmt.Sprintf("- Reflection (score=%.1f): %s", result.Score, result.Lessons)
	_ = memStore.AppendToday(entry)

	logger.InfoCF("reflection", fmt.Sprintf("Self-evaluation complete (score=%.2f)", result.Score),
		map[string]any{
			"agent_id": re.agentID,
			"score":    result.Score,
			"lessons":  utils.Truncate(result.Lessons, 100),
			"tools":    toolCount,
			"errors":   errorCount,
			"duration": durationMs,
		})

	return nil
}

// saveFallbackReflection stores a metrics-only reflection when LLM evaluation fails.
func (re *ReflectionEngine) saveFallbackReflection(sessionKey string, toolCount, errorCount int, durationMs int64) error {
	scorer := NewPerformanceScorer()
	score := scorer.Score(toolCount, errorCount, true)

	record := memory.ReflectionRecord{
		AgentID:    re.agentID,
		SessionKey: sessionKey,
		Score:      score,
		ToolCount:  toolCount,
		ErrorCount: errorCount,
		DurationMs: durationMs,
	}
	return re.db.SaveReflection(record)
}

// FormatLessonsContext returns formatted text of recent lessons for the system prompt.
func (re *ReflectionEngine) FormatLessonsContext(limit int) string {
	if re.db == nil {
		return ""
	}
	if limit <= 0 {
		limit = 5
	}
	reflections, err := re.db.GetRecentReflections(re.agentID, limit)
	if err != nil || len(reflections) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Past Lessons (Self-Reflection)\n\n")

	for _, r := range reflections {
		if r.Lessons == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("- (score=%.1f) %s", r.Score, r.Lessons))
		if r.WhatFailed != "" {
			sb.WriteString(fmt.Sprintf(" [Failed: %s]", utils.Truncate(r.WhatFailed, 80)))
		}
		sb.WriteString("\n")
	}

	result := sb.String()
	if result == "## Past Lessons (Self-Reflection)\n\n" {
		return "" // No actual lessons to show
	}
	return result
}

// GetRelevantLessons searches past reflections matching a query.
func (re *ReflectionEngine) GetRelevantLessons(query string, limit int) ([]memory.ReflectionRecord, error) {
	if re.db == nil {
		return nil, nil
	}
	return re.db.SearchReflections(re.agentID, query, limit)
}

// buildConversationSummary creates a condensed version of the conversation for evaluation.
func buildConversationSummary(history []providers.Message, finalResponse string) string {
	var sb strings.Builder
	maxMessages := 10

	// Take the last few user/assistant exchanges
	count := 0
	for i := len(history) - 1; i >= 0 && count < maxMessages; i-- {
		msg := history[i]
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		content := utils.Truncate(msg.Content, 300)
		sb.WriteString(fmt.Sprintf("[%s] %s\n", msg.Role, content))
		count++
	}

	if finalResponse != "" {
		sb.WriteString(fmt.Sprintf("\n[final_response] %s\n", utils.Truncate(finalResponse, 300)))
	}

	return sb.String()
}

// extractJSON tries to extract a JSON object from text that may be wrapped in markdown.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return ""
}
