package autonomy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
)

func (s *Service) evaluateRecentActivity(ctx context.Context) {
	// Rate limit proactive suggestions (#21).
	if !s.lastProactiveSuggestion.IsZero() && time.Since(s.lastProactiveSuggestion) < proactiveSuggestionMinInterval {
		logger.DebugCF("autonomy", "Proactive suggestion skipped, too recent", map[string]any{
			"last": s.lastProactiveSuggestion.Format(time.RFC3339),
		})
		return
	}

	// Budget check before LLM call (#20).
	if !s.checkBudget() {
		return
	}

	// Look at recent sessions to see if there is an open question or idle state where a proactive tip helps.
	sessions, err := s.memDB.ListSessions()
	if err != nil || len(sessions) == 0 {
		return
	}

	// For simplicity, just pick the top session (most recently active)
	topSession := sessions[0]
	// If it was updated less than 10 minutes ago, the user might still be actively chatting. Give them space.
	if time.Since(topSession.UpdatedAt) < 10*time.Minute {
		return
	}
	// If it's been idle for over 2 days, they might not care right now unless it's a critical goal.
	if time.Since(topSession.UpdatedAt) > 48*time.Hour {
		return
	}

	messages, err := s.memDB.GetMessages(topSession.Key)
	if err != nil || len(messages) == 0 {
		return
	}

	// Just grab the last 5 messages for context
	startIdx := len(messages) - 5
	if startIdx < 0 {
		startIdx = 0
	}
	recentMsgs := messages[startIdx:]

	var sb strings.Builder
	for _, m := range recentMsgs {
		fmt.Fprintf(&sb, "%s: %s\n", m.Role, m.Content)
	}

	prompt := fmt.Sprintf(
		`Analyze the recent conversation context and determine if a proactive suggestion or autonomous research would be highly beneficial to the user right now.

Recent Context:
%s

Instructions:
1. If the user was struggling with a concept, you might suggest doing background research.
2. If the user was building something, suggest the next logical component.
3. If there is no obvious high-value proactive action to take, reply ONLY with "NO_SUGGESTION".
4. If you DO have a proactive plan, outline it briefly.
`,
		sb.String(),
	)

	messagesToLLM := []providers.Message{
		{Role: "user", Content: prompt},
	}

	options := map[string]any{
		"max_tokens":  500,
		"temperature": 0.7,
	}

	resp, err := s.provider.Chat(ctx, messagesToLLM, nil, s.modelID, options)
	if err != nil || len(resp.Content) == 0 {
		return
	}

	// Track cost of this LLM call.
	if resp.Usage != nil {
		s.trackCost(resp.Usage.TotalTokens)
	}

	content := resp.Content
	if content == "NO_SUGGESTION" || strings.TrimSpace(content) == "" {
		return
	}

	// Update last suggestion timestamp for rate limiting.
	s.lastProactiveSuggestion = time.Now()

	logger.InfoCF("autonomy", "Proactive generation triggered", map[string]any{"response": content})

	// Inject this thought into the memory bus so the agent wakes up and messages the user
	s.bus.PublishInbound(bus.InboundMessage{
		Channel:  "cli", // Assuming we route to cli/default or similar system channel
		ChatID:   "proactive",
		SenderID: "autonomy_service",
		Content: fmt.Sprintf(
			"[PROACTIVE THOUGHT] Based on recent activity, I've had an idea:\n%s\n\nTake action on this using your tools, or simply send the suggestion to the user.",
			content,
		),
		SessionKey: topSession.Key, // inject directly into their ongoing session
	})

	// Broadcast to web UI so the notification inbox captures it
	s.broadcast(map[string]any{
		"type":     "proactive_suggestion",
		"content":  content,
		"agent_id": s.agentID,
	})

	// Send an OS desktop push notification so the user knows Sofia is thinking about them
	if s.push != nil {
		_ = s.push.Send(
			"Sofia: Proactive Suggestion",
			"I have an idea based on your recent activity. Check the web UI to review.",
		)
	}
}

// evaluateContextTriggers checks recent session messages against active context triggers.
// When a trigger's condition matches, its action is executed via the task runner.
func (s *Service) evaluateContextTriggers(ctx context.Context) {
	tm := NewTriggerManager(s.memDB)
	triggers, err := tm.ListActiveTriggers(s.agentID)
	if err != nil || len(triggers) == 0 {
		return
	}

	// Get recent messages from last active session
	lastCh := ""
	if s.lastChannelFn != nil {
		lastCh = s.lastChannelFn()
	}
	if lastCh == "" {
		return
	}

	sessionKey := fmt.Sprintf("%s:%s", s.agentID, lastCh)
	recentContent := s.getRecentSessionContent(sessionKey, 5)
	if recentContent == "" {
		return
	}

	for _, triggerAny := range triggers {
		t, ok := triggerAny.(*ContextTrigger)
		if !ok {
			continue
		}
		if matchesTriggerCondition(recentContent, t.Condition) {
			logger.InfoCF("autonomy", "Context trigger fired",
				map[string]any{"trigger": t.Name, "condition": t.Condition})

			if s.taskRunner != nil {
				taskCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
				_, err := s.taskRunner(taskCtx, s.agentID, sessionKey, t.Action, "", "")
				cancel()
				if err != nil {
					logger.WarnCF("autonomy", "Context trigger action failed",
						map[string]any{"trigger": t.Name, "error": err.Error()})
				}
			}
		}
	}
}

// getRecentSessionContent returns concatenated content of recent user messages.
func (s *Service) getRecentSessionContent(sessionKey string, count int) string {
	if s.memDB == nil {
		return ""
	}
	messages, err := s.memDB.GetMessages(sessionKey)
	if err != nil || len(messages) == 0 {
		return ""
	}
	// Take only the last N user messages
	var parts []string
	for i := len(messages) - 1; i >= 0 && len(parts) < count; i-- {
		if messages[i].Role == "user" {
			parts = append(parts, messages[i].Content)
		}
	}
	return strings.Join(parts, " ")
}

// matchesTriggerCondition checks if content matches the trigger's condition string.
// Uses case-insensitive substring matching.
func matchesTriggerCondition(content, condition string) bool {
	return strings.Contains(
		strings.ToLower(content),
		strings.ToLower(condition),
	)
}
