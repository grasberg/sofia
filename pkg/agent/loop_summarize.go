package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
)

func (al *AgentLoop) maybeSummarize(agent *AgentInstance, sessionKey, channel, chatID string) {
	newHistory := agent.Sessions.GetHistory(sessionKey)
	tokenEstimate := al.estimateTokens(newHistory)
	threshold := agent.ContextWindow * 75 / 100

	if len(newHistory) > 20 || tokenEstimate > threshold {
		summarizeKey := agent.ID + ":" + sessionKey
		if _, loading := al.summarizing.LoadOrStore(summarizeKey, true); !loading {
			go func() {
				defer al.summarizing.Delete(summarizeKey)
				logger.Debug("Memory threshold reached. Optimizing conversation history...")
				al.summarizeSession(agent, sessionKey)
			}()
		}
	}
}

// forceCompression reduces context using a protected-region approach:
// 1. Protect head (first 2 messages — system + initial user) and tail (last 30% of messages)
// 2. Truncate tool results in the compressible middle to placeholders
// 3. If still too large, drop the compressible middle entirely with a summary note
func (al *AgentLoop) forceCompression(agent *AgentInstance, sessionKey string) {
	history := agent.Sessions.GetHistory(sessionKey)
	if len(history) <= 6 {
		return
	}

	// Protected regions: head (first 2) and tail (last ~30%, min 4)
	headSize := 2
	if headSize > len(history) {
		headSize = 1
	}
	tailSize := len(history) * 30 / 100
	if tailSize < 4 {
		tailSize = 4
	}
	if headSize+tailSize >= len(history) {
		// Nothing to compress
		return
	}

	// Adjust tail start to a safe boundary
	tailStart := len(history) - tailSize
	tailStart = safeCutPoint(history[headSize:], tailStart-headSize) + headSize

	head := history[:headSize]
	middle := history[headSize:tailStart]
	tail := history[tailStart:]

	// Phase 1: Truncate tool results in the middle to placeholders
	truncatedCount := 0
	for i := range middle {
		if middle[i].Role == "tool" && len(middle[i].Content) > 200 {
			middle[i].Content = fmt.Sprintf("[Tool result truncated — originally %d chars]",
				utf8.RuneCountInString(middle[i].Content))
			truncatedCount++
		}
	}

	// Check if truncation was enough
	newHistory := make([]providers.Message, 0, len(head)+len(middle)+len(tail))
	newHistory = append(newHistory, head...)
	newHistory = append(newHistory, middle...)
	newHistory = append(newHistory, tail...)

	tokenEstimate := al.estimateTokens(newHistory)
	threshold := agent.ContextWindow * 90 / 100

	if tokenEstimate <= threshold {
		// Tool result truncation was sufficient
		agent.Sessions.SetHistory(sessionKey, newHistory)
		agent.Sessions.Save(sessionKey)
		logger.InfoCF("agent", "Context compression: truncated tool results", map[string]any{
			"session_key": sessionKey,
			"truncated":   truncatedCount,
			"new_count":   len(newHistory),
		})
		return
	}

	// Phase 2: Drop the compressible middle entirely
	droppedCount := len(middle)
	compressionNote := fmt.Sprintf(
		"\n\n[System Note: Context compression dropped %d messages from the middle of the conversation. "+
			"Recent context and initial context are preserved.]",
		droppedCount,
	)
	enhancedHead := make([]providers.Message, len(head))
	copy(enhancedHead, head)
	enhancedHead[0].Content = enhancedHead[0].Content + compressionNote

	newHistory = make([]providers.Message, 0, len(enhancedHead)+len(tail))
	newHistory = append(newHistory, enhancedHead...)
	newHistory = append(newHistory, tail...)

	agent.Sessions.SetHistory(sessionKey, newHistory)
	agent.Sessions.Save(sessionKey)

	logger.WarnCF("agent", "Forced compression: dropped middle region", map[string]any{
		"session_key":  sessionKey,
		"dropped_msgs": droppedCount,
		"new_count":    len(newHistory),
	})
}

// safeCutPoint adjusts a cut index forward so the kept messages don't start
// with an orphaned tool result or sit between an assistant tool-call and its
// results. It returns the adjusted index.
func safeCutPoint(msgs []providers.Message, idx int) int {
	if idx >= len(msgs) {
		return len(msgs)
	}
	// Walk forward past any tool-result messages — they belong to a preceding
	// assistant tool-call that would be dropped.
	for idx < len(msgs) && msgs[idx].Role == "tool" {
		idx++
	}
	// If we landed on an assistant message with tool calls, its tool results
	// follow it — skip the entire group so we don't split mid-exchange.
	if idx < len(msgs) && msgs[idx].Role == "assistant" && len(msgs[idx].ToolCalls) > 0 {
		idx++ // skip the assistant message
		for idx < len(msgs) && msgs[idx].Role == "tool" {
			idx++ // skip its tool results
		}
	}
	return idx
}

func (al *AgentLoop) summarizeSession(agent *AgentInstance, sessionKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	history := agent.Sessions.GetHistory(sessionKey)
	summary := agent.Sessions.GetSummary(sessionKey)

	// Keep last 4 messages for continuity
	if len(history) <= 4 {
		return
	}

	toSummarize := history[:len(history)-4]

	// Oversized Message Guard
	maxMessageTokens := agent.ContextWindow / 2
	validMessages := make([]providers.Message, 0)
	omitted := false

	for _, m := range toSummarize {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		msgTokens := len(m.Content) / 2
		if msgTokens > maxMessageTokens {
			omitted = true
			continue
		}
		validMessages = append(validMessages, m)
	}

	if len(validMessages) == 0 {
		return
	}

	// Multi-Part Summarization
	var finalSummary string
	if len(validMessages) > 10 {
		mid := len(validMessages) / 2
		part1 := validMessages[:mid]
		part2 := validMessages[mid:]

		s1, _ := al.summarizeBatch(ctx, agent, part1, "")
		s2, _ := al.summarizeBatch(ctx, agent, part2, "")

		mergePrompt := fmt.Sprintf(
			"Merge these two conversation summaries into one cohesive summary:\n\n1: %s\n\n2: %s",
			s1,
			s2,
		)
		resp, err := agent.Provider.Chat(
			ctx,
			[]providers.Message{{Role: "user", Content: mergePrompt}},
			nil,
			agent.ModelID,
			map[string]any{
				"max_tokens":       1024,
				"temperature":      0.3,
				"prompt_cache_key": agent.ID,
			},
		)
		if err == nil {
			finalSummary = resp.Content
		} else {
			finalSummary = s1 + " " + s2
		}
	} else {
		finalSummary, _ = al.summarizeBatch(ctx, agent, validMessages, summary)
	}

	if omitted && finalSummary != "" {
		finalSummary += "\n[Note: Some oversized messages were omitted from this summary for efficiency.]"
	}

	if finalSummary != "" {
		agent.Sessions.SetSummary(sessionKey, finalSummary)

		// Determine how many messages to keep. We want at least 4, but the
		// cut point must be at a safe boundary so tool-result messages aren't
		// orphaned from their preceding assistant tool-call message.
		keepLast := safeKeepCount(history, 4)
		agent.Sessions.TruncateHistory(sessionKey, keepLast)
		agent.Sessions.Save(sessionKey)
	}
}

// safeKeepCount returns the number of trailing messages to keep such that the
// kept portion doesn't start with orphaned tool-result messages. It walks
// backward from the end to find a safe starting point >= minKeep.
func safeKeepCount(msgs []providers.Message, minKeep int) int {
	if len(msgs) <= minKeep {
		return len(msgs)
	}
	keep := minKeep
	startIdx := len(msgs) - keep

	// If the kept portion starts with tool messages, expand backward to include
	// the assistant message that produced them.
	for startIdx > 0 && msgs[startIdx].Role == "tool" {
		startIdx--
		keep++
	}
	return keep
}

// summarizeBatch summarizes a batch of messages.
func (al *AgentLoop) summarizeBatch(
	ctx context.Context,
	agent *AgentInstance,
	batch []providers.Message,
	existingSummary string,
) (string, error) {
	var sb strings.Builder
	sb.WriteString("Provide a concise summary of this conversation segment, preserving core context and key points.\n")
	if existingSummary != "" {
		sb.WriteString("Existing context: ")
		sb.WriteString(existingSummary)
		sb.WriteString("\n")
	}
	sb.WriteString("\nCONVERSATION:\n")
	for _, m := range batch {
		fmt.Fprintf(&sb, "%s: %s\n", m.Role, m.Content)
	}
	prompt := sb.String()

	response, err := agent.Provider.Chat(
		ctx,
		[]providers.Message{{Role: "user", Content: prompt}},
		nil,
		agent.ModelID,
		map[string]any{
			"max_tokens":       1024,
			"temperature":      0.3,
			"prompt_cache_key": agent.ID,
		},
	)
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// estimateTokens estimates the number of tokens in a message list.
// Uses a safe heuristic of 2.5 characters per token to account for CJK and other
// overheads better than the previous 3 chars/token.
func (al *AgentLoop) estimateTokens(messages []providers.Message) int {
	totalChars := 0
	for _, m := range messages {
		totalChars += utf8.RuneCountInString(m.Content)
	}
	// 2.5 chars per token = totalChars * 2 / 5
	return totalChars * 2 / 5
}
