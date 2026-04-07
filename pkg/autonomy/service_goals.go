package autonomy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
)

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

type goalRef struct {
	id       int64
	name     string
	priority string
}

type goalStepPlan struct {
	GoalID   int64  `json:"goal_id"`
	GoalName string `json:"goal_name"`
	Step     string `json:"step"`
}

type goalPlannerDecision struct {
	Plan           goalStepPlan
	NoAction       bool
	MarkComplete   bool
	CompleteGoalID int64
}

func buildGoalsSummary(goalsAny []any) (string, []goalRef) {
	var goalsSummary strings.Builder
	refs := make([]goalRef, 0, len(goalsAny))

	for _, gAny := range goalsAny {
		b, _ := json.Marshal(gAny)
		var g map[string]any
		if err := json.Unmarshal(b, &g); err != nil {
			continue
		}

		idValue, ok := g["id"].(float64)
		if !ok {
			continue
		}

		name, _ := g["name"].(string)
		desc, _ := g["description"].(string)
		priority, _ := g["priority"].(string)

		refs = append(refs, goalRef{id: int64(idValue), name: name, priority: priority})
		fmt.Fprintf(&goalsSummary, "- [ID:%d] %s (priority: %s)\n  %s\n", int64(idValue), name, priority, desc)
	}

	return goalsSummary.String(), refs
}

func (s *Service) buildGoalPlannerPrompt(goalsSummary string) string {
	return fmt.Sprintf(`You are an autonomous AI agent. You have the following active goals:

%s

Decide which goal to work on next (prioritize high-priority goals). Then determine a single, concrete, actionable next step that can be completed in one task.

Rules:
- Pick ONE goal and ONE step. Do not try to do everything at once.
- The step must be specific and achievable with available tools (read_file, write_file, exec, edit_file, list_dir, append_file).
- All file operations MUST use absolute paths under the workspace: %s
- If a goal needs research, the step could be "Research X and write findings to %s/research_X.md".
- If a goal needs code, the step could be "Create file at %s/filename with content Y".
- If a goal is already effectively complete, say GOAL_COMPLETE:<goal_id>.
- If none of the goals have a useful next step right now, reply ONLY with "NO_ACTION".

Respond in this exact JSON format (no markdown, no code fences):
{"goal_id": <number>, "goal_name": "<name>", "step": "<description of the concrete task to execute>"}

Or respond with NO_ACTION if nothing to do.`, goalsSummary, s.workspace, s.workspace, s.workspace)
}

func parseGoalPlannerResponse(content string) (goalPlannerDecision, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || trimmed == "NO_ACTION" {
		return goalPlannerDecision{NoAction: true}, nil
	}

	if strings.HasPrefix(trimmed, "GOAL_COMPLETE:") {
		idStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "GOAL_COMPLETE:"))
		var goalID int64
		if _, err := fmt.Sscanf(idStr, "%d", &goalID); err == nil {
			return goalPlannerDecision{MarkComplete: true, CompleteGoalID: goalID}, nil
		}
		return goalPlannerDecision{NoAction: true}, nil
	}

	cleaned := strings.TrimSpace(
		strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(trimmed, "```json"), "```"), "```"),
	)

	var plan goalStepPlan
	if err := json.Unmarshal([]byte(cleaned), &plan); err != nil {
		return goalPlannerDecision{}, err
	}
	if plan.Step == "" {
		return goalPlannerDecision{NoAction: true}, nil
	}

	return goalPlannerDecision{Plan: plan}, nil
}

func (s *Service) buildGoalTaskPrompt(plan goalStepPlan) string {
	return fmt.Sprintf(`You are working toward goal: "%s"

Your next step: %s

CRITICAL RULES:
- You MUST use tool calls (read_file, write_file, exec, list_dir, etc.) to do real work.
- All file operations MUST use absolute paths under the workspace: %s
- Do NOT just describe what you would do. Actually do it with tools.
- Do NOT roleplay or narrate. No stage directions. No fictional progress.
- Every response must contain at least one tool call unless the step is purely informational.
- When done, summarize what you actually accomplished (files created, commands run, results).`, plan.GoalName, plan.Step, s.workspace)
}

func (s *Service) executeGoalTask(ctx context.Context, plan goalStepPlan) (string, error) {
	taskPrompt := s.buildGoalTaskPrompt(plan)

	s.mu.Lock()
	runner := s.taskRunner
	s.mu.Unlock()

	if runner != nil {
		return runner(ctx, s.agentID, fmt.Sprintf("goal:%d", plan.GoalID), taskPrompt, "system", "autonomy")
	}

	taskMessages := []providers.Message{{Role: "user", Content: taskPrompt}}
	taskResp, err := s.provider.Chat(ctx, taskMessages, nil, s.modelID, map[string]any{
		"max_tokens":  2000,
		"temperature": 0.4,
	})
	if err != nil {
		return "", err
	}

	return taskResp.Content, nil
}

// executeOneGoalStep plans and executes a single goal step. Returns the outcome.
func (s *Service) executeOneGoalStep(ctx context.Context, gm *GoalManager, goalsAny []any, stepNum int) stepOutcome {
	goalsSummary, refs := buildGoalsSummary(goalsAny)

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
	prompt := s.buildGoalPlannerPrompt(goalsSummary)

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

	decision, err := parseGoalPlannerResponse(resp.Content)
	if err != nil {
		logger.WarnCF("autonomy", "Failed to parse goal planner response", map[string]any{
			"error":   err.Error(),
			"content": strings.TrimSpace(resp.Content),
		})
		return stepResultError
	}

	if decision.MarkComplete {
		if _, err := gm.UpdateGoalStatus(decision.CompleteGoalID, GoalStatusCompleted); err == nil {
			// Persist a summary result on the goal itself.
			if decision.Plan.Step != "" {
				_ = gm.UpdateGoalResult(decision.CompleteGoalID, decision.Plan.Step)
			}
			logger.InfoCF("autonomy", "Goal auto-completed", map[string]any{"goal_id": decision.CompleteGoalID})
			s.broadcast(map[string]any{
				"type":    "goal_completed",
				"goal_id": decision.CompleteGoalID,
			})
			s.notifyUser(fmt.Sprintf("🏁 Mål slutfört: *%d*", decision.CompleteGoalID))
		}
		return stepResultDone
	}

	if decision.NoAction {
		logger.DebugCF("autonomy", "Goal planner: no action needed", nil)
		return stepResultDone
	}

	plan := decision.Plan

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

	start := time.Now()
	result, taskErr := s.executeGoalTask(ctx, plan)

	dur := time.Since(start).Milliseconds()

	if taskErr != nil {
		logger.WarnCF("autonomy", "Goal step execution failed", map[string]any{
			"goal_id":     plan.GoalID,
			"goal_name":   plan.GoalName,
			"step":        plan.Step,
			"error":       taskErr.Error(),
			"duration_ms": dur,
		})

		// Persist failed step to goal log.
		if s.memDB != nil {
			_ = s.memDB.InsertGoalLog(plan.GoalID, s.agentID, plan.Step, taskErr.Error(), false, dur)
		}

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

	// Persist successful step to goal log.
	if s.memDB != nil {
		_ = s.memDB.InsertGoalLog(plan.GoalID, s.agentID, plan.Step, result, true, dur)
		// Update the goal's result field with the latest step output.
		_ = gm.UpdateGoalResult(plan.GoalID, truncate(result, 2000))
	}

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
