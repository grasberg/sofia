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

// Service configures and runs periodic autonomy operations (Proactive Suggestions, Research, Goal Pursuit).
type Service struct {
	cfg        *config.AutonomyConfig
	memDB      *memory.MemoryDB
	bus        *bus.MessageBus
	provider   providers.LLMProvider
	subMgr     *tools.SubagentManager
	modelID    string
	agentID    string
	workspace  string
	push       *notifications.PushService
	hub        *dashboard.Hub
	taskRunner TaskRunner
	mu         sync.Mutex
	cancelFunc context.CancelFunc
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
	if interval < 5 {
		interval = 60 // Default to 60 if set too low to prevent spam
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

func (s *Service) performAutonomyTasks(ctx context.Context) {
	// 1. Goal pursuit — work toward active goals
	if s.cfg.Goals {
		s.pursueGoals(ctx)
	}

	// 2. Proactive Suggestions & Autonomous Research
	if s.cfg.Suggestions || s.cfg.Research {
		s.evaluateRecentActivity(ctx)
	}
}

// pursueGoals checks active goals, asks the LLM to plan the next concrete step,
// then executes it via the agent's tool loop.
func (s *Service) pursueGoals(ctx context.Context) {
	gm := NewGoalManager(s.memDB)
	goalsAny, err := gm.ListActiveGoals(s.agentID)
	if err != nil || len(goalsAny) == 0 {
		return
	}

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
		return
	}

	logger.InfoCF("autonomy", "Evaluating active goals for autonomous work", map[string]any{
		"agent_id":   s.agentID,
		"goal_count": len(refs),
	})

	s.broadcast(map[string]any{
		"type":       "goal_evaluation_start",
		"agent_id":   s.agentID,
		"goal_count": len(refs),
	})

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

	resp, err := s.provider.Chat(ctx, messages, nil, s.modelID, map[string]any{
		"max_tokens":  500,
		"temperature": 0.3,
	})
	if err != nil || len(resp.Content) == 0 {
		logger.WarnCF("autonomy", "Goal planner LLM call failed", map[string]any{"error": fmt.Sprintf("%v", err)})
		return
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
			}
		}
		return
	}

	if content == "NO_ACTION" || content == "" {
		logger.DebugCF("autonomy", "Goal planner: no action needed", nil)
		return
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
		return
	}

	if plan.Step == "" {
		return
	}

	logger.InfoCF("autonomy", "Goal step planned", map[string]any{
		"agent_id":  s.agentID,
		"goal_id":   plan.GoalID,
		"goal_name": plan.GoalName,
		"step":      plan.Step,
	})

	s.broadcast(map[string]any{
		"type":      "goal_step_start",
		"agent_id":  s.agentID,
		"goal_id":   plan.GoalID,
		"goal_name": plan.GoalName,
		"step":      plan.Step,
	})

	// Execute the step
	taskPrompt := fmt.Sprintf(`You are working toward goal: "%s"

Your next step: %s

Execute this step using your available tools. Be thorough and complete the step fully.
When done, summarize what you accomplished.`, plan.GoalName, plan.Step)

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
		return
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

	// Notify user if push is available
	if s.push != nil {
		_ = s.push.Send(
			fmt.Sprintf("Sofia: Goal Progress — %s", plan.GoalName),
			fmt.Sprintf("Completed step: %s", truncate(plan.Step, 100)),
		)
	}
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

	prompt := fmt.Sprintf(`Analyze the recent conversation context and determine if a proactive suggestion or autonomous research would be highly beneficial to the user right now.

Recent Context:
%s

Instructions:
1. If the user was struggling with a concept, you might suggest doing background research.
2. If the user was building something, suggest the next logical component.
3. If there is no obvious high-value proactive action to take, reply ONLY with "NO_SUGGESTION".
4. If you DO have a proactive plan, outline it briefly.
`, sb.String())

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

	content := resp.Content
	if content == "NO_SUGGESTION" || strings.TrimSpace(content) == "" {
		return
	}

	logger.InfoCF("autonomy", "Proactive generation triggered", map[string]any{"response": content})

	// Inject this thought into the memory bus so the agent wakes up and messages the user
	s.bus.PublishInbound(bus.InboundMessage{
		Channel:    "cli", // Assuming we route to cli/default or similar system channel
		ChatID:     "proactive",
		SenderID:   "autonomy_service",
		Content:    fmt.Sprintf("[PROACTIVE THOUGHT] Based on recent activity, I've had an idea:\n%s\n\nTake action on this using your tools, or simply send the suggestion to the user.", content),
		SessionKey: topSession.Key, // inject directly into their ongoing session
	})

	// Send an OS desktop push notification so the user knows Sofia is thinking about them
	if s.push != nil {
		_ = s.push.Send("Sofia: Proactive Suggestion", "I have an idea based on your recent activity. Check the terminal/UI.")
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
