package autonomy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/tools"
	"github.com/grasberg/sofia/pkg/utils"
)

// finalizeGoal gathers step results with verification evidence, evaluates success criteria, and completes the goal.
func (s *Service) finalizeGoal(ctx context.Context, gm *GoalManager, goal *Goal, plan *tools.Plan) {
	var sb strings.Builder
	var evidence []string

	for _, step := range plan.Steps {
		status := "completed"
		if step.Status == tools.PlanStatusFailed {
			status = "failed"
		}
		fmt.Fprintf(&sb, "Step %d (%s): %s\nResult: %s\n",
			step.Index, status, step.Description, truncate(step.Result, 500))
		if step.VerifyResult != "" {
			fmt.Fprintf(&sb, "Verification: %s\n", truncate(step.VerifyResult, 300))
			evidence = append(evidence, fmt.Sprintf("Step %d: %s", step.Index, truncate(step.VerifyResult, 200)))
		}
		sb.WriteString("\n")
	}

	if !s.checkBudget() {
		_ = gm.SetGoalResult(goal.ID, GoalResult{
			Summary:     "Goal completed (budget exceeded before summary generation)",
			Evidence:    evidence,
			CompletedAt: time.Now().UTC().Format(time.RFC3339),
		})
		s.completeGoal(gm, goal)
		return
	}

	// Build finalization prompt with spec success criteria.
	var specSection string
	if goal.Spec != nil && len(goal.Spec.SuccessCriteria) > 0 {
		specSection = fmt.Sprintf(`
Success criteria to evaluate:
%s

For each success criterion, determine if it was MET or UNMET based on the step results and verification evidence.
Include any unmet criteria in the "unmet_criteria" array.`,
			"- "+strings.Join(goal.Spec.SuccessCriteria, "\n- "))
	}

	prompt := fmt.Sprintf(`A goal has been completed. Summarize the outcome and evaluate success.

Goal: %s
Description: %s
%s

Step results:
%s

Respond in this exact JSON format (no markdown, no code fences):
{"summary": "...", "artifacts": ["file1.txt", ...], "next_steps": ["..."], "unmet_criteria": ["..."]}

The unmet_criteria array should be empty if all criteria are met.`, goal.Name, goal.Description, specSection, sb.String())

	messages := []providers.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := s.provider.Chat(ctx, messages, nil, s.modelID, map[string]any{
		"max_tokens":  600,
		"temperature": 0.3,
	})

	var goalResult GoalResult
	goalResult.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	goalResult.Evidence = evidence

	if err == nil && resp != nil && len(resp.Content) > 0 {
		if resp.Usage != nil {
			s.trackCost(resp.Usage.TotalTokens)
		}
		parsed, parseErr := parseGoalResultResponse(resp.Content)
		if parseErr == nil {
			goalResult.Summary = parsed.Summary
			goalResult.Artifacts = parsed.Artifacts
			goalResult.NextSteps = parsed.NextSteps
			goalResult.UnmetCriteria = parsed.UnmetCriteria
		} else {
			goalResult.Summary = truncate(resp.Content, 1000)
		}
	} else {
		// LLM summary failed — build a basic summary from step results.
		var completed, failed int
		for _, step := range plan.Steps {
			if step.Status == tools.PlanStatusCompleted {
				completed++
			} else if step.Status == tools.PlanStatusFailed {
				failed++
			}
		}
		if failed > 0 {
			goalResult.Summary = fmt.Sprintf("Completed %d of %d steps (%d failed).", completed, len(plan.Steps), failed)
		} else {
			goalResult.Summary = fmt.Sprintf("All %d steps completed successfully.", completed)
		}
	}

	_ = gm.SetGoalResult(goal.ID, goalResult)
	s.completeGoal(gm, goal)

	logger.InfoCF("autonomy", "Goal finalized", map[string]any{
		"goal_id":        goal.ID,
		"goal_name":      goal.Name,
		"summary":        truncate(goalResult.Summary, 200),
		"unmet_criteria": len(goalResult.UnmetCriteria),
	})

	s.broadcast(map[string]any{
		"type":           "goal_completed",
		"agent_id":       s.agentID,
		"goal_id":        goal.ID,
		"goal_name":      goal.Name,
		"summary":        goalResult.Summary,
		"unmet_criteria": goalResult.UnmetCriteria,
	})

	notification := fmt.Sprintf("Goal completed: %s\n\n%s", goal.Name, truncate(goalResult.Summary, 300))
	if len(goalResult.UnmetCriteria) > 0 {
		notification += fmt.Sprintf("\n\nUnmet criteria: %s", strings.Join(goalResult.UnmetCriteria, "; "))
	}
	s.notifyUser(notification)

	// Fire-and-forget goal-level reflection. Writes lessons + (on clean
	// success) a plan template, which feeds future calls to buildMemoryContext.
	go s.reflectOnGoal(goal, plan, goalResult)
}

// goalReflectionPrompt is the system prompt for post-goal self-evaluation.
// Differs from chat-level reflectionPrompt: it evaluates a whole plan
// trajectory rather than a single conversation.
const goalReflectionPrompt = `You are performing a post-goal self-evaluation of an autonomous agent run.
Analyze the goal, its plan, the step outcomes, and the final result.

Respond ONLY with valid JSON in this exact format:
{
  "task_summary": "1-line summary of the goal",
  "what_worked": "What went well across the plan",
  "what_failed": "What went wrong or was inefficient (empty string if nothing)",
  "lessons": "One specific, actionable lesson usable when planning similar future goals. Keep to 1-2 sentences.",
  "score": 0.8
}

Score HIGHER for:
- All success criteria met with verification evidence
- Steps delivered vertical slices that actually worked end-to-end
- Few or no step retries / auto-fix rounds
- Plan structure that avoided rework

Score LOWER for:
- Unmet success criteria
- Steps that passed verification but didn't advance the goal (false-positive PASS)
- Many retries or auto-fix rounds
- Steps that had to be re-described mid-run
- Overly-layered plan (all-schemas-then-all-code) instead of vertical slices

Be honest, specific, and actionable. Generic lessons are worthless; prefer concrete rules like
"For deploy goals, always run the build+test before touching remote infrastructure" over
"Plan carefully".`

// goalReflectionResult is the parsed LLM response for goal-level reflection.
type goalReflectionResult struct {
	TaskSummary string  `json:"task_summary"`
	WhatWorked  string  `json:"what_worked"`
	WhatFailed  string  `json:"what_failed"`
	Lessons     string  `json:"lessons"`
	Score       float64 `json:"score"`
}

// buildGoalReflectionPrompt renders the user-message payload evaluated by the
// reflection LLM. It contains the goal spec, each step's outcome and
// verification snippet, and the finalized goal result with any unmet criteria.
func buildGoalReflectionPrompt(goal *Goal, plan *tools.Plan, result GoalResult) string {
	var stepSummary strings.Builder
	var completed, failed int
	for _, step := range plan.Steps {
		status := string(step.Status)
		if step.Status == tools.PlanStatusCompleted {
			completed++
		} else if step.Status == tools.PlanStatusFailed {
			failed++
		}
		fmt.Fprintf(&stepSummary, "Step %d [%s] (retries=%d): %s\n",
			step.Index, status, step.RetryCount, truncate(step.Description, 200))
		if step.VerifyResult != "" {
			fmt.Fprintf(&stepSummary, "  verification: %s\n", truncate(step.VerifyResult, 200))
		}
		if step.Status == tools.PlanStatusFailed && step.Result != "" {
			fmt.Fprintf(&stepSummary, "  failure: %s\n", truncate(step.Result, 200))
		}
	}

	var specSection string
	if goal.Spec != nil {
		specSection = fmt.Sprintf(`
Goal specification:
- Requirements: %s
- Success Criteria: %s
- Constraints: %s
`,
			strings.Join(goal.Spec.Requirements, "; "),
			strings.Join(goal.Spec.SuccessCriteria, "; "),
			strings.Join(goal.Spec.Constraints, "; "))
	}

	var unmetSection string
	if len(result.UnmetCriteria) > 0 {
		unmetSection = "\nUnmet success criteria:\n- " + strings.Join(result.UnmetCriteria, "\n- ") + "\n"
	}

	return fmt.Sprintf(`Evaluate this completed goal.

Goal name: %s
Description: %s
Priority: %s
%s
Plan outcome: %d step(s) completed, %d failed.

Step trajectory:
%s
Final summary: %s
%s`,
		goal.Name, goal.Description, goal.Priority, specSection,
		completed, failed, stepSummary.String(),
		truncate(result.Summary, 500), unmetSection)
}

// parseGoalReflectionJSON extracts the goalReflectionResult from an LLM response,
// tolerating ```json code fences.
func parseGoalReflectionJSON(content string) (goalReflectionResult, error) {
	cleaned := utils.CleanJSONFences(content)
	var r goalReflectionResult
	if err := json.Unmarshal([]byte(cleaned), &r); err != nil {
		return r, err
	}
	return r, nil
}

// reflectOnGoal runs a post-goal self-evaluation, saves a ReflectionRecord
// against the goal's session key, and — on clean success — persists the plan
// as a reusable PlanTemplate. Called as a goroutine from finalizeGoal so it
// never delays user-visible completion. Safe to call with a nil or empty plan.
func (s *Service) reflectOnGoal(goal *Goal, plan *tools.Plan, result GoalResult) {
	defer func() {
		if r := recover(); r != nil {
			logger.WarnCF("autonomy", "reflectOnGoal panic", map[string]any{"goal_id": goal.ID, "panic": fmt.Sprint(r)})
		}
	}()

	if s.memDB == nil || plan == nil || len(plan.Steps) == 0 {
		return
	}
	if !s.checkBudget() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prompt := buildGoalReflectionPrompt(goal, plan, result)
	resp, err := s.provider.Chat(ctx, []providers.Message{
		{Role: "system", Content: goalReflectionPrompt},
		{Role: "user", Content: prompt},
	}, nil, s.modelID, map[string]any{
		"max_tokens":       500,
		"temperature":      0.3,
		"prompt_cache_key": s.agentID + ":goal-reflection",
	})
	if err != nil || resp == nil || resp.Content == "" {
		if err != nil {
			logger.WarnCF("autonomy", "Goal reflection LLM call failed", map[string]any{
				"goal_id": goal.ID, "error": err.Error(),
			})
		}
		return
	}
	if resp.Usage != nil {
		s.trackCost(resp.Usage.TotalTokens)
	}

	parsed, perr := parseGoalReflectionJSON(resp.Content)
	if perr != nil {
		logger.WarnCF("autonomy", "Goal reflection parse failed", map[string]any{
			"goal_id": goal.ID, "error": perr.Error(),
			"content": truncate(resp.Content, 200),
		})
		return
	}

	var failed int
	var toolCount int
	for _, step := range plan.Steps {
		if step.Status == tools.PlanStatusFailed {
			failed++
		}
		toolCount += step.RetryCount + 1
	}

	record := memory.ReflectionRecord{
		AgentID:     s.agentID,
		SessionKey:  fmt.Sprintf("goal-%d", goal.ID),
		TaskSummary: parsed.TaskSummary,
		WhatWorked:  parsed.WhatWorked,
		WhatFailed:  parsed.WhatFailed,
		Lessons:     parsed.Lessons,
		Score:       parsed.Score,
		ToolCount:   toolCount,
		ErrorCount:  failed,
	}
	if err := s.memDB.SaveReflection(record); err != nil {
		logger.WarnCF("autonomy", "SaveReflection failed", map[string]any{
			"goal_id": goal.ID, "error": err.Error(),
		})
		return
	}

	logger.InfoCF("autonomy", "Goal reflection saved", map[string]any{
		"goal_id": goal.ID,
		"score":   parsed.Score,
		"lessons": truncate(parsed.Lessons, 120),
	})

	// Promote clean successes to reusable plan templates. Require no failed
	// steps, no unmet criteria, and a self-reported score of at least 0.7.
	if failed == 0 && len(result.UnmetCriteria) == 0 && parsed.Score >= 0.7 {
		stepDescs := make([]string, 0, len(plan.Steps))
		for _, step := range plan.Steps {
			stepDescs = append(stepDescs, step.Description)
		}
		tags := goal.Priority
		if err := s.memDB.SavePlanTemplate(goal.Name, goal.Description, stepDescs, tags); err != nil {
			logger.WarnCF("autonomy", "SavePlanTemplate failed", map[string]any{
				"goal_id": goal.ID, "error": err.Error(),
			})
		} else {
			logger.InfoCF("autonomy", "Plan template saved from successful goal", map[string]any{
				"goal_id": goal.ID,
				"name":    goal.Name,
				"steps":   len(stepDescs),
			})
		}
	}
}
