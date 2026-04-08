package autonomy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/tools"
	"github.com/grasberg/sofia/pkg/utils"
)

var goalSlugRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// pursueGoals is the new plan-first, parallel-dispatch pipeline entry point.
// It processes active goals (generating plans) and in_progress goals (dispatching ready steps).
func (s *Service) pursueGoals(ctx context.Context) {
	gm := NewGoalManager(s.memDB)

	// Fetch all goals once, split by status to avoid duplicate DB queries.
	allGoals, err := gm.ListAllGoals(s.agentID)
	if err != nil {
		logger.WarnCF("autonomy", "Failed to list goals", map[string]any{"error": err.Error()})
		return
	}

	// Phase 1: Generate plans for active goals that don't have one yet.
	for _, goal := range allGoals {
		if goal.Status != GoalStatusActive {
			continue
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		s.generatePlanForGoal(ctx, gm, goal)
	}

	// Phase 2: Dispatch ready steps for in_progress goals.
	for _, goal := range allGoals {
		if goal.Status != GoalStatusInProgress {
			continue
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		s.dispatchReadySteps(ctx, gm, goal)
	}
}

// goalPlanResponse is the parsed LLM plan response.
type goalPlanResponse struct {
	GoalID   int64  `json:"goal_id"`
	GoalName string `json:"goal_name"`
	Plan     struct {
		Steps []tools.PlanStepDef `json:"steps"`
	} `json:"plan"`
	Steps []tools.PlanStepDef `json:"steps"` // fallback: steps at top level
}

// parseGoalPlanResponse parses the LLM's plan JSON response.
func parseGoalPlanResponse(content string) (*goalPlanResponse, error) {
	cleaned := utils.CleanJSONFences(content)

	var resp goalPlanResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, err
	}

	if len(resp.Plan.Steps) == 0 && len(resp.Steps) > 0 {
		resp.Plan.Steps = resp.Steps
	}

	if len(resp.Plan.Steps) == 0 {
		return nil, fmt.Errorf("plan contains no steps")
	}

	return &resp, nil
}

// goalResultResponse is the parsed LLM finalization response.
type goalResultResponse struct {
	Summary   string   `json:"summary"`
	Artifacts []string `json:"artifacts"`
	NextSteps []string `json:"next_steps"`
}

// parseGoalResultResponse parses the LLM's goal finalization JSON.
func parseGoalResultResponse(content string) (*goalResultResponse, error) {
	cleaned := utils.CleanJSONFences(content)

	var resp goalResultResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// buildPlanGenerationPrompt creates the LLM prompt that asks for a complete plan.
func buildPlanGenerationPrompt(goal *Goal) string {
	return fmt.Sprintf(`You are an autonomous AI agent. You need to create a complete plan for the following goal:

Goal ID: %d
Goal Name: %s
Description: %s
Priority: %s

Create a detailed plan with 3-10 steps. Each step should be:
- Specific and actionable (can be delegated to a subagent)
- Independent where possible, but declare dependencies when a step requires output from a prior step
- Ordered logically (research before implementation, implementation before testing)

Respond in this exact JSON format (no markdown, no code fences):
{"goal_id": %d, "goal_name": "%s", "plan": {"steps": [{"description": "...", "depends_on": []}]}}

The "depends_on" field is an array of step indices (0-based) that must complete before this step can start.
An empty depends_on means the step can start immediately.`, goal.ID, goal.Name, goal.Description, goal.Priority, goal.ID, goal.Name)
}

// generatePlanForGoal calls the LLM to produce a plan, creates it via PlanManager,
// transitions the goal to in_progress, and broadcasts the event.
func (s *Service) generatePlanForGoal(ctx context.Context, gm *GoalManager, goal *Goal) {
	s.mu.Lock()
	pm := s.planMgr
	s.mu.Unlock()

	if pm == nil {
		logger.WarnCF("autonomy", "PlanManager not set, skipping plan generation", nil)
		return
	}

	// Skip if goal already has a plan.
	if existing := pm.GetPlanByGoalID(goal.ID); existing != nil {
		return
	}

	if !s.checkBudget() {
		return
	}

	prompt := buildPlanGenerationPrompt(goal)
	messages := []providers.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := s.provider.Chat(ctx, messages, nil, s.modelID, map[string]any{
		"max_tokens":  1000,
		"temperature": 0.3,
	})
	if err != nil || resp == nil || len(resp.Content) == 0 {
		logger.WarnCF("autonomy", "Plan generation LLM call failed", map[string]any{
			"goal_id": goal.ID,
			"error":   fmt.Sprintf("%v", err),
		})
		return
	}

	if resp.Usage != nil {
		s.trackCost(resp.Usage.TotalTokens)
	}

	planResp, err := parseGoalPlanResponse(resp.Content)
	if err != nil {
		logger.WarnCF("autonomy", "Failed to parse plan response", map[string]any{
			"goal_id": goal.ID,
			"error":   err.Error(),
			"content": truncate(resp.Content, 500),
		})
		return
	}

	plan := pm.CreatePlanForGoal(goal.ID, goal.Name, planResp.Plan.Steps)

	// Transition goal to in_progress.
	if _, err := gm.UpdateGoalStatus(goal.ID, GoalStatusInProgress); err != nil {
		logger.WarnCF("autonomy", "Failed to transition goal to in_progress", map[string]any{
			"goal_id": goal.ID,
			"error":   err.Error(),
		})
	}

	logger.InfoCF("autonomy", "Plan created for goal", map[string]any{
		"goal_id":    goal.ID,
		"goal_name":  goal.Name,
		"plan_id":    plan.ID,
		"step_count": len(plan.Steps),
	})

	s.broadcast(map[string]any{
		"type":       "goal_plan_created",
		"agent_id":   s.agentID,
		"goal_id":    goal.ID,
		"goal_name":  goal.Name,
		"plan_id":    plan.ID,
		"step_count": len(plan.Steps),
	})
}

// dispatchReadySteps finds steps whose dependencies are satisfied, claims them,
// and spawns subagents to execute each one in parallel.
func (s *Service) dispatchReadySteps(ctx context.Context, gm *GoalManager, goal *Goal) {
	s.mu.Lock()
	pm := s.planMgr
	sm := s.subMgr
	s.mu.Unlock()

	if pm == nil || sm == nil {
		return
	}

	plan := pm.GetPlanByGoalID(goal.ID)
	if plan == nil {
		return
	}

	// Check if plan is already completed or failed.
	if plan.Status == tools.PlanStatusCompleted {
		s.finalizeGoal(ctx, gm, goal, plan)
		return
	}
	if plan.Status == tools.PlanStatusFailed {
		logger.WarnCF("autonomy", "Plan failed, not dispatching", map[string]any{
			"goal_id": goal.ID,
			"plan_id": plan.ID,
		})
		return
	}

	readyIndices := pm.ReadySteps(plan.ID)
	if len(readyIndices) == 0 {
		return
	}

	for _, stepIdx := range readyIndices {
		select {
		case <-ctx.Done():
			return
		default:
		}

		label := fmt.Sprintf("goal-%d-step-%d", goal.ID, stepIdx)
		if !pm.ClaimStep(plan.ID, stepIdx, label) {
			continue
		}

		stepDesc := plan.Steps[stepIdx].Description
		goalDir := s.ensureGoalFolder(goal.ID, goal.Name)

		taskPrompt := fmt.Sprintf(`You are working toward goal: "%s"

Your task: %s

Working directory for this goal: %s

Rules:
- Use tools to do real work (read_file, write_file, exec, edit_file, list_dir, append_file).
- All file operations MUST use absolute paths under the goal folder.
- Do NOT just describe what you would do. Actually do it.
- When done, summarize what you actually accomplished.`, goal.Name, stepDesc, goalDir)

		// Capture values for the closure to avoid race conditions.
		capturedGoalID := goal.ID
		capturedGoalName := goal.Name
		capturedStepIdx := stepIdx
		capturedPlanID := plan.ID
		capturedAgentID := s.agentID

		s.broadcast(map[string]any{
			"type":       "goal_step_start",
			"agent_id":   s.agentID,
			"goal_id":    goal.ID,
			"goal_name":  goal.Name,
			"step_index": stepIdx,
			"step":       stepDesc,
		})

		callback := func(cbCtx context.Context, result *tools.ToolResult) {
			success := result != nil && !result.IsError
			resultText := ""
			if result != nil {
				resultText = result.ForLLM
			}

			pm.CompleteStep(capturedPlanID, capturedStepIdx, success, truncate(resultText, 2000))

			// Log to goal_log.
			if s.memDB != nil {
				_ = s.memDB.InsertGoalLog(
					capturedGoalID,
					capturedAgentID,
					stepDesc,
					truncate(resultText, 2000),
					success,
					0,
				)
			}

			s.broadcast(map[string]any{
				"type":       "goal_step_end",
				"agent_id":   capturedAgentID,
				"goal_id":    capturedGoalID,
				"goal_name":  capturedGoalName,
				"step_index": capturedStepIdx,
				"success":    success,
			})

			// Recursively dispatch next ready steps (cascade).
			updatedGoal, err := gm.GetGoalByID(capturedGoalID)
			if err != nil || updatedGoal == nil {
				return
			}

			updatedPlan := pm.GetPlanByGoalID(capturedGoalID)
			if updatedPlan != nil && updatedPlan.Status == tools.PlanStatusCompleted {
				s.finalizeGoal(cbCtx, gm, updatedGoal, updatedPlan)
				return
			}

			s.dispatchReadySteps(cbCtx, gm, updatedGoal)
		}

		if _, err := sm.Spawn(ctx, taskPrompt, label, "", nil, "system", "autonomy", callback); err != nil {
			logger.WarnCF("autonomy", "Failed to spawn subagent for step", map[string]any{
				"goal_id":    goal.ID,
				"step_index": stepIdx,
				"error":      err.Error(),
			})
		}
	}
}

// finalizeGoal gathers step results, asks the LLM for a summary, and completes the goal.
func (s *Service) finalizeGoal(ctx context.Context, gm *GoalManager, goal *Goal, plan *tools.Plan) {
	// Build a summary of step results.
	var sb strings.Builder
	for _, step := range plan.Steps {
		status := "completed"
		if step.Status == tools.PlanStatusFailed {
			status = "failed"
		}
		fmt.Fprintf(
			&sb,
			"Step %d (%s): %s\nResult: %s\n\n",
			step.Index,
			status,
			step.Description,
			truncate(step.Result, 500),
		)
	}

	if !s.checkBudget() {
		// If no budget, just mark completed without LLM summary.
		_ = gm.SetGoalResult(goal.ID, GoalResult{
			Summary:     "Goal completed (budget exceeded before summary generation)",
			CompletedAt: time.Now().UTC().Format(time.RFC3339),
		})
		if _, err := gm.UpdateGoalStatus(goal.ID, GoalStatusCompleted); err != nil {
			logger.WarnCF(
				"autonomy",
				"Failed to mark goal completed",
				map[string]any{"goal_id": goal.ID, "error": err.Error()},
			)
		}
		return
	}

	prompt := fmt.Sprintf(`A goal has been completed. Summarize the outcome.

Goal: %s
Description: %s

Step results:
%s

Respond in this exact JSON format (no markdown, no code fences):
{"summary": "...", "artifacts": ["file1.txt", ...], "next_steps": ["..."]}`, goal.Name, goal.Description, sb.String())

	messages := []providers.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := s.provider.Chat(ctx, messages, nil, s.modelID, map[string]any{
		"max_tokens":  500,
		"temperature": 0.3,
	})

	var goalResult GoalResult
	goalResult.CompletedAt = time.Now().UTC().Format(time.RFC3339)

	if err == nil && resp != nil && len(resp.Content) > 0 {
		if resp.Usage != nil {
			s.trackCost(resp.Usage.TotalTokens)
		}
		parsed, parseErr := parseGoalResultResponse(resp.Content)
		if parseErr == nil {
			goalResult.Summary = parsed.Summary
			goalResult.Artifacts = parsed.Artifacts
			goalResult.NextSteps = parsed.NextSteps
		} else {
			goalResult.Summary = truncate(resp.Content, 1000)
		}
	} else {
		goalResult.Summary = "Goal completed (summary generation failed)"
	}

	_ = gm.SetGoalResult(goal.ID, goalResult)

	if _, err := gm.UpdateGoalStatus(goal.ID, GoalStatusCompleted); err != nil {
		logger.WarnCF(
			"autonomy",
			"Failed to mark goal completed",
			map[string]any{"goal_id": goal.ID, "error": err.Error()},
		)
	}

	logger.InfoCF("autonomy", "Goal finalized", map[string]any{
		"goal_id":   goal.ID,
		"goal_name": goal.Name,
		"summary":   truncate(goalResult.Summary, 200),
	})

	s.broadcast(map[string]any{
		"type":      "goal_completed",
		"agent_id":  s.agentID,
		"goal_id":   goal.ID,
		"goal_name": goal.Name,
		"summary":   goalResult.Summary,
	})

	s.notifyUser(fmt.Sprintf("Goal completed: %s\n\n%s", goal.Name, truncate(goalResult.Summary, 300)))
}

// goalFolderName returns a filesystem-safe folder name for a goal.
func goalFolderName(goalID int64, goalName string) string {
	slug := strings.ToLower(strings.TrimSpace(goalSlugRe.ReplaceAllString(goalName, "-")))
	slug = strings.Trim(slug, "-")
	if len(slug) > 50 {
		slug = slug[:50]
	}
	if slug == "" {
		slug = "goal"
	}
	return fmt.Sprintf("goal-%d-%s", goalID, slug)
}

// goalFolderPath returns the absolute path for a goal's working directory.
func (s *Service) goalFolderPath(goalID int64, goalName string) string {
	return filepath.Join(s.workspace, "goals", goalFolderName(goalID, goalName))
}

// ensureGoalFolder creates the goal folder if it doesn't exist.
func (s *Service) ensureGoalFolder(goalID int64, goalName string) string {
	dir := s.goalFolderPath(goalID, goalName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.WarnCF("autonomy", "Failed to create goal folder", map[string]any{
			"path":  dir,
			"error": err.Error(),
		})
	}
	return dir
}
