package autonomy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/dashboard"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/notifications"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/tools"
)

// TaskRunner executes a task as a specific agent, returning the result.
type TaskRunner func(ctx context.Context, agentID, sessionKey, task, originChannel, originChatID string) (string, error)

// LastChannelFunc returns "channel:chatID" for the user's last active channel.
type LastChannelFunc func() string

// proactiveSuggestionMinInterval is the minimum time between proactive suggestions.
const proactiveSuggestionMinInterval = 30 * time.Minute

// defaultAutonomyMaxCostPerDay is the default daily budget for autonomy LLM calls.
const defaultAutonomyMaxCostPerDay = 1.0

// Service configures and runs periodic autonomy operations (Proactive Suggestions, Research, Goal Pursuit).
type Service struct {
	cfg           *config.AutonomyConfig
	memDB         *memory.MemoryDB
	bus           *bus.MessageBus
	provider      providers.LLMProvider
	subMgr        *tools.SubagentManager
	modelID       string
	agentID       string
	workspace     string
	lastChannelFn LastChannelFunc
	push          *notifications.PushService
	hub           *dashboard.Hub
	taskRunner    TaskRunner
	mu            sync.Mutex
	cancelFunc    context.CancelFunc

	// Budget tracking (#20)
	budgetSpent         float64
	budgetResetDate     time.Time
	lastProactiveSuggestion time.Time
}

// NewService instantiates the autonomy service for a specific agent.
func NewService(
	cfg *config.AutonomyConfig,
	memDB *memory.MemoryDB,
	msgBus *bus.MessageBus,
	provider providers.LLMProvider,
	subMgr *tools.SubagentManager,
	agentID string,
	modelID string,
	workspace string,
	push *notifications.PushService,
) *Service {
	return &Service{
		cfg:       cfg,
		memDB:     memDB,
		bus:       msgBus,
		provider:  provider,
		subMgr:    subMgr,
		agentID:   agentID,
		modelID:   modelID,
		workspace: workspace,
		push:      push,
	}
}

// SetDashboardHub sets the dashboard hub for broadcasting goal events.
func (s *Service) SetDashboardHub(hub *dashboard.Hub) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hub = hub
}

// SetTaskRunner sets the function used to execute tasks as agents.
func (s *Service) SetTaskRunner(runner TaskRunner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskRunner = runner
}

// SetLastChannelFunc sets the function to resolve the user's last active channel.
func (s *Service) SetLastChannelFunc(fn LastChannelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastChannelFn = fn
}

// Start spawns the background periodic ticker.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.cfg.Enabled {
		logger.InfoCF("autonomy", "Autonomy service is disabled in config", nil)
		return nil
	}
	if s.cancelFunc != nil {
		return fmt.Errorf("autonomy service already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancelFunc = cancel

	interval := s.cfg.IntervalMinutes
	if interval <= 0 {
		interval = 10 // Default to 10 minutes
	}
	if interval < 2 {
		interval = 2 // Minimum 2 minutes to prevent tight loops
	}

	go s.runLoop(ctx, time.Duration(interval)*time.Minute)
	logger.InfoCF("autonomy", "Autonomy service started", map[string]any{"interval_minutes": interval})
	return nil
}

// Stop shuts down the periodic tasks.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelFunc != nil {
		s.cancelFunc()
		s.cancelFunc = nil
		logger.InfoCF("autonomy", "Autonomy service stopped", nil)
	}
}

func (s *Service) runLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial delay so we don't jump the gun on boot when models are just warming up
	select {
	case <-ctx.Done():
		return
	case <-time.After(1 * time.Minute):
		s.performAutonomyTasks(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.performAutonomyTasks(ctx)
		}
	}
}

// checkBudget resets the daily budget if needed and returns true if budget is available.
func (s *Service) checkBudget() bool {
	today := time.Now().Truncate(24 * time.Hour)
	if s.budgetResetDate.IsZero() || !today.Equal(s.budgetResetDate) {
		s.budgetSpent = 0
		s.budgetResetDate = today
	}

	maxCost := s.cfg.MaxCostPerDay
	if maxCost <= 0 {
		maxCost = defaultAutonomyMaxCostPerDay
	}

	if s.budgetSpent >= maxCost {
		logger.InfoCF("autonomy", "Daily budget exceeded, skipping cycle", map[string]any{
			"spent": s.budgetSpent,
			"limit": maxCost,
		})
		return false
	}
	return true
}

// trackCost adds an estimated cost based on token usage ($0.01 per 1K tokens as safe default).
func (s *Service) trackCost(totalTokens int) {
	s.budgetSpent += float64(totalTokens) / 1000.0 * 0.01
}

func (s *Service) performAutonomyTasks(ctx context.Context) {
	// Budget check before performing any work.
	if !s.checkBudget() {
		return
	}

	// 1. Goal pursuit — work toward active goals
	if s.cfg.Goals {
		s.pursueGoals(ctx)
	}

	// 2. Proactive Suggestions & Autonomous Research
	if s.cfg.Suggestions || s.cfg.Research {
		s.evaluateRecentActivity(ctx)
	}

	// 3. Context trigger evaluation — check recent messages against triggers
	if s.cfg.ContextTriggers {
		s.evaluateContextTriggers(ctx)
	}
}

// maxStepsPerCycle limits how many goal steps we execute per autonomy tick
// to prevent runaway execution while still making meaningful progress.
const maxStepsPerCycle = 5

// pursueGoals checks active goals and executes multiple steps in a loop
// until the LLM says NO_ACTION, a step fails, or maxStepsPerCycle is reached.
func (s *Service) pursueGoals(ctx context.Context) {
	gm := NewGoalManager(s.memDB)

	for step := 0; step < maxStepsPerCycle; step++ {
		// Check context cancellation between steps
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Re-fetch active goals each iteration (status may have changed)
		goalsAny, err := gm.ListActiveGoals(s.agentID)
		if err != nil {
			logger.WarnCF("autonomy", "Failed to list active goals", map[string]any{"error": err.Error()})
			return
		}
		if len(goalsAny) == 0 {
			if step == 0 {
				logger.DebugCF("autonomy", "No active goals to pursue", nil)
			} else {
				logger.InfoCF("autonomy", fmt.Sprintf("All goals completed after %d step(s)", step), nil)
			}
			return
		}

		if step == 0 {
			logger.InfoCF("autonomy", fmt.Sprintf("Pursuing %d active goal(s)", len(goalsAny)),
				map[string]any{"agent_id": s.agentID, "goal_count": len(goalsAny)})
		}

		result := s.executeOneGoalStep(ctx, gm, goalsAny, step)
		switch result {
		case stepResultDone:
			// LLM said NO_ACTION or GOAL_COMPLETE — re-evaluate on next iteration
			continue
		case stepResultSuccess:
			// Step succeeded — immediately plan and execute the next step
			logger.InfoCF("autonomy", fmt.Sprintf("Step %d/%d completed, continuing to next step",
				step+1, maxStepsPerCycle), nil)
			continue
		case stepResultFailed, stepResultError:
			// Step failed — stop this cycle, retry on next interval
			logger.InfoCF("autonomy", fmt.Sprintf("Stopping after %d step(s) due to failure", step+1), nil)
			return
		}
	}
	logger.InfoCF("autonomy", fmt.Sprintf("Completed maximum %d steps per cycle, pausing until next interval",
		maxStepsPerCycle), nil)
}

type stepOutcome int

const (
	stepResultSuccess stepOutcome = iota
	stepResultFailed
	stepResultDone  // NO_ACTION or GOAL_COMPLETE
	stepResultError // parse/LLM error
)

// executeOneGoalStep plans and executes a single goal step. Returns the outcome.
func (s *Service) executeOneGoalStep(ctx context.Context, gm *GoalManager, goalsAny []any, stepNum int) stepOutcome {
	// Build a goals summary for the planner
	var goalsSummary strings.Builder
	type goalRef struct {
		id       int64
		name     string
		priority string
	}
	var refs []goalRef

	for _, gAny := range goalsAny {
		b, _ := json.Marshal(gAny)
		var g map[string]any
		if err := json.Unmarshal(b, &g); err != nil {
			continue
		}
		id := int64(g["id"].(float64))
		name, _ := g["name"].(string)
		desc, _ := g["description"].(string)
		priority, _ := g["priority"].(string)
		refs = append(refs, goalRef{id: id, name: name, priority: priority})
		goalsSummary.WriteString(fmt.Sprintf("- [ID:%d] %s (priority: %s)\n  %s\n", id, name, priority, desc))
	}

	if len(refs) == 0 {
		return stepResultDone
	}

	if stepNum == 0 {
		s.broadcast(map[string]any{
			"type":       "goal_evaluation_start",
			"agent_id":   s.agentID,
			"goal_count": len(refs),
		})
	}

	// Ask the LLM which goal to work on and what the next concrete step is
	prompt := fmt.Sprintf(`You are an autonomous AI agent. You have the following active goals:

%s

Decide which goal to work on next (prioritize high-priority goals). Then determine a single, concrete, actionable next step that can be completed in one task.

Rules:
- Pick ONE goal and ONE step. Do not try to do everything at once.
- The step must be specific and achievable with available tools (read_file, write_file, exec, edit_file, list_dir, append_file).
- If a goal needs research, the step could be "Research X and write findings to workspace/research_X.md".
- If a goal needs code, the step could be "Create file X with content Y".
- If a goal is already effectively complete, say GOAL_COMPLETE:<goal_id>.
- If none of the goals have a useful next step right now, reply ONLY with "NO_ACTION".

Respond in this exact JSON format (no markdown, no code fences):
{"goal_id": <number>, "goal_name": "<name>", "step": "<description of the concrete task to execute>"}

Or respond with NO_ACTION if nothing to do.`, goalsSummary.String())

	messages := []providers.Message{
		{Role: "user", Content: prompt},
	}

	// Budget check before LLM call.
	if !s.checkBudget() {
		return stepResultError
	}

	resp, err := s.provider.Chat(ctx, messages, nil, s.modelID, map[string]any{
		"max_tokens":  500,
		"temperature": 0.3,
	})
	if err != nil || len(resp.Content) == 0 {
		logger.WarnCF("autonomy", "Goal planner LLM call failed", map[string]any{"error": fmt.Sprintf("%v", err)})
		return stepResultError
	}

	// Track cost of this LLM call.
	if resp.Usage != nil {
		s.trackCost(resp.Usage.TotalTokens)
	}

	content := strings.TrimSpace(resp.Content)

	// Check for GOAL_COMPLETE
	if strings.HasPrefix(content, "GOAL_COMPLETE:") {
		idStr := strings.TrimPrefix(content, "GOAL_COMPLETE:")
		idStr = strings.TrimSpace(idStr)
		var goalID int64
		if _, err := fmt.Sscanf(idStr, "%d", &goalID); err == nil {
			if _, err := gm.UpdateGoalStatus(goalID, GoalStatusCompleted); err == nil {
				logger.InfoCF("autonomy", "Goal auto-completed", map[string]any{"goal_id": goalID})
				s.broadcast(map[string]any{
					"type":    "goal_completed",
					"goal_id": goalID,
				})
				s.notifyUser(fmt.Sprintf("🏁 Mål slutfört: *%s*", idStr))
			}
		}
		return stepResultDone
	}

	if content == "NO_ACTION" || content == "" {
		logger.DebugCF("autonomy", "Goal planner: no action needed", nil)
		return stepResultDone
	}

	// Parse the JSON response
	var plan struct {
		GoalID   int64  `json:"goal_id"`
		GoalName string `json:"goal_name"`
		Step     string `json:"step"`
	}

	// Strip markdown code fences if present
	cleaned := content
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	if err := json.Unmarshal([]byte(cleaned), &plan); err != nil {
		logger.WarnCF("autonomy", "Failed to parse goal planner response", map[string]any{
			"error":   err.Error(),
			"content": content,
		})
		return stepResultError
	}

	if plan.Step == "" {
		return stepResultDone
	}

	logger.InfoCF("autonomy", "Goal step planned", map[string]any{
		"agent_id":  s.agentID,
		"goal_id":   plan.GoalID,
		"goal_name": plan.GoalName,
		"step":      plan.Step,
		"step_num":  stepNum + 1,
	})

	s.broadcast(map[string]any{
		"type":      "goal_step_start",
		"agent_id":  s.agentID,
		"goal_id":   plan.GoalID,
		"goal_name": plan.GoalName,
		"step":      plan.Step,
	})

	s.notifyUser(fmt.Sprintf("🎯 *%s* (steg %d)\nArbetar på: %s", plan.GoalName, stepNum+1, plan.Step))

	// Execute the step
	taskPrompt := fmt.Sprintf(`You are working toward goal: "%s"

Your next step: %s

CRITICAL RULES:
- You MUST use tool calls (read_file, write_file, exec, list_dir, etc.) to do real work.
- Do NOT just describe what you would do. Actually do it with tools.
- Do NOT roleplay or narrate. No stage directions. No fictional progress.
- Every response must contain at least one tool call unless the step is purely informational.
- When done, summarize what you actually accomplished (files created, commands run, results).`, plan.GoalName, plan.Step)

	s.mu.Lock()
	runner := s.taskRunner
	s.mu.Unlock()

	start := time.Now()
	var result string
	var taskErr error

	if runner != nil {
		// Execute via the agent's full tool loop
		result, taskErr = runner(ctx, s.agentID, fmt.Sprintf("goal:%d", plan.GoalID), taskPrompt, "system", "autonomy")
	} else {
		// Fallback: simple LLM call without tools
		taskMessages := []providers.Message{
			{Role: "user", Content: taskPrompt},
		}
		taskResp, err := s.provider.Chat(ctx, taskMessages, nil, s.modelID, map[string]any{
			"max_tokens":  2000,
			"temperature": 0.4,
		})
		if err != nil {
			taskErr = err
		} else {
			result = taskResp.Content
		}
	}

	dur := time.Since(start).Milliseconds()

	if taskErr != nil {
		logger.WarnCF("autonomy", "Goal step execution failed", map[string]any{
			"goal_id":     plan.GoalID,
			"goal_name":   plan.GoalName,
			"step":        plan.Step,
			"error":       taskErr.Error(),
			"duration_ms": dur,
		})
		s.broadcast(map[string]any{
			"type":        "goal_step_end",
			"agent_id":    s.agentID,
			"goal_id":     plan.GoalID,
			"goal_name":   plan.GoalName,
			"success":     false,
			"error":       taskErr.Error(),
			"duration_ms": dur,
		})
		s.notifyUser(fmt.Sprintf("❌ *%s*\nMisslyckades: %s\n\nFel: %s",
			plan.GoalName, plan.Step, truncate(taskErr.Error(), 200)))
		return stepResultFailed
	}

	logger.InfoCF("autonomy", "Goal step completed", map[string]any{
		"goal_id":     plan.GoalID,
		"goal_name":   plan.GoalName,
		"step":        plan.Step,
		"duration_ms": dur,
		"result_len":  len(result),
	})

	s.broadcast(map[string]any{
		"type":        "goal_step_end",
		"agent_id":    s.agentID,
		"goal_id":     plan.GoalID,
		"goal_name":   plan.GoalName,
		"step":        plan.Step,
		"success":     true,
		"result":      truncate(result, 500),
		"duration_ms": dur,
	})

	// Notify user via their active channel
	s.notifyUser(fmt.Sprintf("✅ *%s* (steg %d)\nKlart: %s\n\nResultat: %s",
		plan.GoalName, stepNum+1, plan.Step, truncate(result, 300)))

	if s.push != nil {
		_ = s.push.Send(
			fmt.Sprintf("Sofia: Goal Progress — %s", plan.GoalName),
			fmt.Sprintf("Completed step: %s", truncate(plan.Step, 100)),
		)
	}

	return stepResultSuccess
}

// notifyUser sends a message to the user's last active channel (e.g. Telegram).
func (s *Service) notifyUser(message string) {
	s.mu.Lock()
	fn := s.lastChannelFn
	msgBus := s.bus
	s.mu.Unlock()

	if fn == nil || msgBus == nil {
		return
	}
	lastChannel := fn()
	if lastChannel == "" {
		return
	}
	parts := strings.SplitN(lastChannel, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return
	}
	msgBus.PublishOutbound(bus.OutboundMessage{
		Channel: parts[0],
		ChatID:  parts[1],
		Content: message,
	})
}

func (s *Service) broadcast(data map[string]any) {
	s.mu.Lock()
	hub := s.hub
	s.mu.Unlock()
	if hub != nil {
		hub.Broadcast(data)
	}
}

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
		sb.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
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
