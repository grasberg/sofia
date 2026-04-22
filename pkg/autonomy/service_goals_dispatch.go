package autonomy

import (
	"context"
	"fmt"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/tools"
)

// dispatchReadySteps finds steps whose dependencies are satisfied, claims them,
// and spawns subagents with verification and retry logic.
// Concurrency is bounded by goal.AgentCount (default 3 when auto/0).
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

	if plan.Status == tools.PlanStatusCompleted {
		s.finalizeGoal(ctx, gm, goal, plan)
		return
	}
	if plan.Status == tools.PlanStatusFailed {
		fixCount := getGoalAutoFixCount(gm, goal.ID)
		maxFixes := s.maxAutoFixAttempts()
		if fixCount < maxFixes {
			logger.InfoCF("autonomy", "Plan failed, attempting auto-fix", map[string]any{
				"goal_id":     goal.ID,
				"plan_id":     plan.ID,
				"fix_attempt": fixCount + 1,
				"max_fixes":   maxFixes,
			})
			s.broadcast(map[string]any{
				"type":        "goal_auto_fix",
				"agent_id":    s.agentID,
				"goal_id":     goal.ID,
				"goal_name":   goal.Name,
				"fix_attempt": fixCount + 1,
			})
			// Deep-copy steps so the goroutine doesn't race with the plan manager.
			stepsCopy := make([]tools.PlanStep, len(plan.Steps))
			copy(stepsCopy, plan.Steps)
			planCopy := *plan
			planCopy.Steps = stepsCopy
			go s.recoverableAutoFix(ctx, gm, goal, &planCopy, fixCount)
			return
		}

		logger.WarnCF("autonomy", "Plan permanently failed after auto-fix attempts, marking goal as failed", map[string]any{
			"goal_id":      goal.ID,
			"plan_id":      plan.ID,
			"fix_attempts": fixCount,
		})
		if _, err := gm.UpdateGoalStatus(goal.ID, GoalStatusFailed); err != nil {
			logger.ErrorCF("autonomy", "Failed to mark goal as failed", map[string]any{
				"goal_id": goal.ID,
				"error":   err.Error(),
			})
		}
		s.broadcast(map[string]any{
			"type":      "goal_failed",
			"agent_id":  s.agentID,
			"goal_id":   goal.ID,
			"goal_name": goal.Name,
			"plan_id":   plan.ID,
		})
		return
	}

	readyIndices := pm.ReadySteps(plan.ID)
	if len(readyIndices) == 0 {
		return
	}

	// Cap concurrency to the goal's agent_count setting.
	maxParallel := goal.AgentCount
	if maxParallel <= 0 {
		maxParallel = s.defaultGoalConcurrency()
	}
	if len(readyIndices) > maxParallel {
		readyIndices = readyIndices[:maxParallel]
	}

	maxRetries := s.maxStepRetries()

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

		step := plan.Steps[stepIdx]
		goalDir := s.ensureGoalFolder(goal.ID, goal.Name)

		// Build retry-aware prompt: if this is a retry, include prior failure context.
		taskPrompt := buildVerifyingTaskPrompt(goal.Name, step, goalDir)
		if step.RetryCount > 0 && step.Result != "" {
			taskPrompt = fmt.Sprintf(
				"PREVIOUS ATTEMPT FAILED (attempt %d). Learn from the error below and try a DIFFERENT approach.\n\n"+
					"Previous error:\n%s\n\n%s",
				step.RetryCount, truncate(step.Result, 1000), taskPrompt,
			)
		}

		capturedGoalID := goal.ID
		capturedGoalName := goal.Name
		capturedStepIdx := stepIdx
		capturedPlanID := plan.ID
		capturedAgentID := s.agentID
		capturedRetryCount := step.RetryCount
		hasVerifyCommand := step.VerifyCommand != ""
		stepStartTime := time.Now()

		s.broadcast(map[string]any{
			"type":       "goal_step_start",
			"agent_id":   s.agentID,
			"goal_id":    goal.ID,
			"goal_name":  goal.Name,
			"step_index": stepIdx,
			"step":       step.Description,
			"retry":      step.RetryCount,
		})

		callback := func(cbCtx context.Context, result *tools.ToolResult) {
			toolSuccess := result != nil && !result.IsError
			resultText := ""
			if result != nil {
				resultText = result.ForLLM
			}

			// Determine verification outcome.
			verifyText := ""
			verifyPassed := true // default pass if no verify command
			if hasVerifyCommand && toolSuccess {
				verifyText, verifyPassed = extractVerifyResult(resultText)
			}

			stepSuccess := toolSuccess && verifyPassed

			if !stepSuccess && capturedRetryCount < maxRetries {
				pm.FailAndRetryStep(capturedPlanID, capturedStepIdx, truncate(resultText, 2000), verifyText)

				logger.InfoCF("autonomy", "Step verification failed, retrying", map[string]any{
					"goal_id":     capturedGoalID,
					"step_index":  capturedStepIdx,
					"retry_count": capturedRetryCount + 1,
					"max_retries": maxRetries,
				})

				s.broadcast(map[string]any{
					"type":        "goal_step_retry",
					"agent_id":    capturedAgentID,
					"goal_id":     capturedGoalID,
					"goal_name":   capturedGoalName,
					"step_index":  capturedStepIdx,
					"retry_count": capturedRetryCount + 1,
				})

				// Exponential backoff before re-dispatch: 10s, 30s, 60s, ...
				baseBackoff := time.Duration(s.stepBackoffBaseSec()) * time.Second
				maxBackoff := time.Duration(s.stepBackoffMaxSec()) * time.Second
				backoff := baseBackoff * time.Duration(1<<uint(capturedRetryCount))
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				go func() {
					select {
					case <-cbCtx.Done():
						return
					case <-time.After(backoff):
					}
					updatedGoal, err := gm.GetGoalByID(capturedGoalID)
					if err == nil && updatedGoal != nil {
						s.dispatchReadySteps(cbCtx, gm, updatedGoal)
					}
				}()
				return
			}

			pm.CompleteStepWithVerify(capturedPlanID, capturedStepIdx, stepSuccess, truncate(resultText, 2000), verifyText)

			// If the step permanently failed, first try to self-heal (e.g.
			// install a missing whitelisted tool). Only if that fails do we
			// notify the user.
			if !stepSuccess {
				if s.tryAutoResolveStepFailure(pm, capturedGoalID, capturedPlanID, capturedStepIdx, step.Description, resultText) {
					// Step re-queued; dispatcher will pick it up on the next tick.
				} else {
					s.notifyUserActionNeeded(capturedGoalID, capturedGoalName, capturedStepIdx, step.Description, resultText)
				}
			}

			if s.memDB != nil {
				_ = s.memDB.InsertGoalLog(
					capturedGoalID,
					capturedAgentID,
					step.Description,
					truncate(resultText, 2000),
					stepSuccess,
					time.Since(stepStartTime).Milliseconds(),
				)
			}

			s.broadcast(map[string]any{
				"type":       "goal_step_end",
				"agent_id":   capturedAgentID,
				"goal_id":    capturedGoalID,
				"goal_name":  capturedGoalName,
				"step_index": capturedStepIdx,
				"step_desc":  step.Description,
				"result":     truncate(resultText, 200),
				"success":    stepSuccess,
				"verified":   hasVerifyCommand,
			})

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
