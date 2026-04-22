package autonomy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/tools"
	"github.com/grasberg/sofia/pkg/utils"
)

const maxGoalAutoFixes = 2

// getGoalAutoFixCount reads the auto_fix_count property from a goal.
func getGoalAutoFixCount(gm *GoalManager, goalID int64) int {
	goal, err := gm.GetGoalByID(goalID)
	if err != nil || goal == nil {
		return 0
	}
	// The count is stored in the node's properties JSON. We need to read it
	// from the raw node because the Goal struct doesn't have this field.
	node, err := gm.memDB.GetNodeByID(goalID)
	if err != nil || node == nil {
		return 0
	}
	var props map[string]json.RawMessage
	if json.Unmarshal([]byte(node.Properties), &props) != nil {
		return 0
	}
	if v, ok := props["auto_fix_count"]; ok {
		var count int
		if json.Unmarshal(v, &count) == nil {
			return count
		}
	}
	return 0
}

// SetGoalAutoFixCount stores the auto_fix_count in the goal's properties.
func SetGoalAutoFixCount(gm *GoalManager, goalID int64, count int) {
	node, err := gm.memDB.GetNodeByID(goalID)
	if err != nil || node == nil {
		return
	}
	var props map[string]any
	if json.Unmarshal([]byte(node.Properties), &props) != nil {
		props = make(map[string]any)
	}
	props["auto_fix_count"] = count
	propsJSON, _ := json.Marshal(props)
	_, _ = gm.memDB.UpsertNode(node.AgentID, "Goal", node.Name, string(propsJSON))
}

// tryAutoResolveStepFailure attempts to self-heal a failed step before
// escalating to the user. Today it handles kind="tool" by looking up the
// missing binary in autoInstallMethods and running the platform-specific
// install command. Returns true if the step was successfully reset for
// retry; false if the caller should fall through to user notification.
//
// Safety envelope:
//   - Gated by AutonomyConfig.AutoInstallTools (default false).
//   - Only binaries present in autoInstallMethods are eligible — no arbitrary
//     install strings derived from LLM output.
//   - At most one install attempt per (goal, binary) pair, tracked in memory.
//   - Install command runs with a 2-minute timeout.
//
// When the install succeeds but ResetStepForRetry can't re-queue the step
// (e.g. plan moved on), returns false so the user is still notified.
func (s *Service) tryAutoResolveStepFailure(pm *tools.PlanManager, goalID int64, planID string, stepIdx int, stepDesc, result string) bool {
	if pm == nil || s.cfg == nil || !s.cfg.AutoInstallTools {
		return false
	}
	kind, detail := classifyStepError(result)
	if kind != "tool" || detail == "" {
		return false
	}
	return s.tryAutoInstallAndRetry(pm, goalID, planID, stepIdx, detail)
}

// tryAutoInstallAndRetry runs the install command for `binary` and, on
// success, resets the step to pending so the dispatcher picks it up again.
func (s *Service) tryAutoInstallAndRetry(pm *tools.PlanManager, goalID int64, planID string, stepIdx int, binary string) bool {
	cmd, ok := autoInstallCommandFor(binary)
	if !ok {
		return false
	}

	s.mu.Lock()
	if s.autoInstallAttempts == nil {
		s.autoInstallAttempts = make(map[int64]map[string]bool)
	}
	attempts := s.autoInstallAttempts[goalID]
	if attempts == nil {
		attempts = make(map[string]bool)
		s.autoInstallAttempts[goalID] = attempts
	}
	if attempts[binary] {
		s.mu.Unlock()
		logger.InfoCF("autonomy", "Auto-install already attempted for this goal, skipping", map[string]any{
			"goal_id": goalID, "binary": binary,
		})
		return false
	}
	attempts[binary] = true
	installer := s.toolInstaller
	s.mu.Unlock()

	if installer == nil {
		installer = execInstaller
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	logger.InfoCF("autonomy", "Attempting auto-install of missing tool", map[string]any{
		"goal_id": goalID, "binary": binary, "command": cmd,
	})
	ok, out, err := installer(ctx, cmd)
	if !ok {
		logger.WarnCF("autonomy", "Auto-install failed", map[string]any{
			"goal_id": goalID, "binary": binary, "error": fmt.Sprint(err),
			"output": truncate(out, 300),
		})
		return false
	}

	if !pm.ResetStepForRetry(planID, stepIdx) {
		logger.WarnCF("autonomy", "Auto-install succeeded but step could not be reset", map[string]any{
			"goal_id": goalID, "plan_id": planID, "step_index": stepIdx, "binary": binary,
		})
		return false
	}

	logger.InfoCF("autonomy", "Auto-install succeeded; step re-queued for retry", map[string]any{
		"goal_id": goalID, "plan_id": planID, "step_index": stepIdx, "binary": binary,
	})
	s.broadcast(map[string]any{
		"type":       "goal_auto_resolved",
		"agent_id":   s.agentID,
		"goal_id":    goalID,
		"step_index": stepIdx,
		"resolution": "installed:" + binary,
	})
	return true
}

// notifyUserActionNeeded checks whether a failed step's output indicates a
// missing tool or credential and, if so, sends actionable notifications to the
// user through all available channels (dashboard bell, push, chat channel).
func (s *Service) notifyUserActionNeeded(goalID int64, goalName string, stepIdx int, stepDesc, result string) {
	kind, detail := classifyStepError(result)
	if kind == "" {
		return
	}

	var title, body string
	switch kind {
	case "tool":
		if detail != "" {
			title = "Missing tool: " + detail
			body = fmt.Sprintf("Goal \"%s\" (step %d) failed because the command \"%s\" was not found.\n"+
				"Please install it and make sure it is in PATH, then restart the goal.",
				goalName, stepIdx, detail)
		} else {
			title = "Missing tool"
			body = fmt.Sprintf("Goal \"%s\" (step %d: %s) failed because a required tool is not installed.\n"+
				"Check the error details in the goal log and install the missing tool.",
				goalName, stepIdx, truncate(stepDesc, 80))
		}
	case "credential":
		if detail != "" {
			title = "Missing credentials: " + detail
			body = fmt.Sprintf("Goal \"%s\" (step %d) failed due to an authentication error with %s.\n"+
				"Please add or update the credentials in Settings, then restart the goal.",
				goalName, stepIdx, detail)
		} else {
			title = "Authentication error"
			body = fmt.Sprintf("Goal \"%s\" (step %d: %s) failed due to missing or invalid credentials.\n"+
				"Check the error details in the goal log and update your credentials in Settings.",
				goalName, stepIdx, truncate(stepDesc, 80))
		}
	case "network":
		if detail != "" {
			title = "Network error: cannot reach " + detail
			body = fmt.Sprintf("Goal \"%s\" (step %d) failed because %s is unreachable.\n"+
				"Please check your internet connection, VPN, DNS, or firewall, then restart the goal.",
				goalName, stepIdx, detail)
		} else {
			title = "Network error"
			body = fmt.Sprintf("Goal \"%s\" (step %d: %s) failed due to a network problem.\n"+
				"Please check your internet connection, VPN, or firewall and restart the goal.",
				goalName, stepIdx, truncate(stepDesc, 80))
		}
	case "disk":
		title = "Disk full"
		body = fmt.Sprintf("Goal \"%s\" (step %d: %s) failed because the disk is out of space.\n"+
			"Please free up disk space (or expand the volume / quota), then restart the goal.",
			goalName, stepIdx, truncate(stepDesc, 80))
	case "rate_limit":
		if detail != "" {
			title = "Rate limit hit: " + detail
			body = fmt.Sprintf("Goal \"%s\" (step %d) was rate-limited by %s.\n"+
				"Please wait for the limit to reset or upgrade the plan, then restart the goal.",
				goalName, stepIdx, detail)
		} else {
			title = "Rate limit hit"
			body = fmt.Sprintf("Goal \"%s\" (step %d: %s) was rate-limited by an external API.\n"+
				"Please wait for the limit to reset or upgrade the plan, then restart the goal.",
				goalName, stepIdx, truncate(stepDesc, 80))
		}
	case "permission":
		if detail != "" {
			title = "Permission denied: " + detail
			body = fmt.Sprintf("Goal \"%s\" (step %d) failed because access to %s is not permitted.\n"+
				"Please adjust file/folder permissions (chmod/chown) or run Sofia as a user with access, then restart the goal.",
				goalName, stepIdx, detail)
		} else {
			title = "Permission denied"
			body = fmt.Sprintf("Goal \"%s\" (step %d: %s) failed because the operation is not permitted by the OS.\n"+
				"Please adjust file/folder permissions or run Sofia as a user with the required access, then restart the goal.",
				goalName, stepIdx, truncate(stepDesc, 80))
		}
	case "config":
		if detail != "" {
			title = "Missing configuration: " + detail
			body = fmt.Sprintf("Goal \"%s\" (step %d) failed because the required configuration value \"%s\" is not set.\n"+
				"Please set it in Settings (or the environment) and restart the goal.",
				goalName, stepIdx, detail)
		} else {
			title = "Missing configuration"
			body = fmt.Sprintf("Goal \"%s\" (step %d: %s) failed because a required configuration value is not set.\n"+
				"Please add the missing setting and restart the goal.",
				goalName, stepIdx, truncate(stepDesc, 80))
		}
	}

	// 1. Dashboard notification bell
	s.broadcast(map[string]any{
		"type":      "user_action_needed",
		"title":     title,
		"content":   body,
		"goal_id":   goalID,
		"goal_name": goalName,
		"category":  kind,
	})

	// 2. Desktop push notification
	s.mu.Lock()
	push := s.push
	s.mu.Unlock()
	if push != nil {
		push.Alert("Sofia: "+title, body)
	}

	// 3. User's last active channel (Telegram/Discord/Email)
	s.notifyUser("Action needed: " + title + "\n\n" + body)

	logger.InfoCF("autonomy", "Notified user of missing "+kind, map[string]any{
		"goal_id":    goalID,
		"step_index": stepIdx,
		"detail":     detail,
	})
}

func (s *Service) recoverableAutoFix(ctx context.Context, gm *GoalManager, goal *Goal, plan *tools.Plan, fixCount int) {
	defer func() {
		if r := recover(); r != nil {
			logger.ErrorCF("autonomy", "Panic in autoFixGoal", map[string]any{
				"goal_id": goal.ID,
				"panic":   fmt.Sprintf("%v", r),
			})
		}
	}()
	s.autoFixGoal(ctx, gm, goal, plan, fixCount)
}

// autoFixGoal asks the LLM to diagnose why steps failed and produces revised
// step descriptions. It then resets the failed steps with the new instructions
// and lets the normal tick re-dispatch them.
func (s *Service) autoFixGoal(ctx context.Context, gm *GoalManager, goal *Goal, plan *tools.Plan, prevFixCount int) {
	var sb strings.Builder
	for _, step := range plan.Steps {
		if step.Status != tools.PlanStatusFailed {
			continue
		}
		fmt.Fprintf(&sb, "Step %d: %s\n", step.Index, step.Description)
		fmt.Fprintf(&sb, "  Error/Result: %s\n", truncate(step.Result, 600))
		if step.VerifyResult != "" {
			fmt.Fprintf(&sb, "  Verification: %s\n", truncate(step.VerifyResult, 300))
		}
		sb.WriteString("\n")
	}

	// Include goal folder contents for workspace context.
	var workspaceContext string
	goalDir := s.goalFolderPath(goal.ID, goal.Name)
	if entries, err := os.ReadDir(goalDir); err == nil && len(entries) > 0 {
		var files []string
		for _, e := range entries {
			files = append(files, e.Name())
		}
		workspaceContext = "\nExisting files in goal folder:\n- " + strings.Join(files, "\n- ") + "\n"
	}

	prompt := fmt.Sprintf(`A goal's plan has failed. Diagnose the problems and produce revised step descriptions that fix the issues.

Goal: %s
Description: %s
%s
Failed steps:
%s
Previous fix attempts: %d

For EACH failed step, analyze WHY it failed and write a REVISED description that addresses the root cause.
The revised description should include specific fixes — different commands, corrected paths, alternative approaches, etc.
Do NOT just repeat the same instructions.

Respond in this exact JSON format (no markdown, no code fences):
{"revisions": [{"step_index": 0, "diagnosis": "why it failed", "revised_description": "new step instructions"}]}`,
		goal.Name, goal.Description, workspaceContext, sb.String(), prevFixCount)

	resp, err := s.provider.Chat(ctx, []providers.Message{
		{Role: "user", Content: prompt},
	}, nil, s.modelID, map[string]any{
		"max_tokens":  1024,
		"temperature": 0.4,
	})

	if err != nil {
		logger.WarnCF("autonomy", "Auto-fix LLM call failed, marking goal as failed", map[string]any{
			"goal_id": goal.ID,
			"error":   err.Error(),
		})
		SetGoalAutoFixCount(gm, goal.ID, maxGoalAutoFixes) // exhaust attempts
		if _, err := gm.UpdateGoalStatus(goal.ID, GoalStatusFailed); err != nil {
			logger.ErrorCF("autonomy", "Failed to mark goal as failed", map[string]any{
				"goal_id": goal.ID, "error": err.Error(),
			})
		}
		return
	}

	// Parse the revisions.
	type revision struct {
		StepIndex   int    `json:"step_index"`
		Diagnosis   string `json:"diagnosis"`
		Description string `json:"revised_description"`
	}
	type fixResponse struct {
		Revisions []revision `json:"revisions"`
	}

	cleaned := utils.CleanJSONFences(resp.Content)
	var fix fixResponse
	if err := json.Unmarshal([]byte(cleaned), &fix); err != nil || len(fix.Revisions) == 0 {
		logger.WarnCF("autonomy", "Auto-fix: could not parse LLM revisions, falling back to plain reset", map[string]any{
			"goal_id": goal.ID,
			"error":   fmt.Sprintf("parse: %v, revisions: %d", err, len(fix.Revisions)),
		})
		// Fall back to a plain reset (same descriptions, but retry count cleared).
		s.mu.Lock()
		pm := s.planMgr
		s.mu.Unlock()
		pm.ResetPlan(plan.ID)
	} else {
		// Apply revisions to the failed steps.
		revMap := make(map[int]string, len(fix.Revisions))
		for _, r := range fix.Revisions {
			if r.Description != "" {
				revMap[r.StepIndex] = r.Description
				logger.InfoCF("autonomy", "Auto-fix: revised step", map[string]any{
					"goal_id":    goal.ID,
					"step_index": r.StepIndex,
					"diagnosis":  truncate(r.Diagnosis, 200),
				})
			}
		}
		s.mu.Lock()
		pm := s.planMgr
		s.mu.Unlock()
		pm.ReviseFailedSteps(plan.ID, revMap)
	}

	// Increment fix count and keep the goal active so the tick re-dispatches.
	SetGoalAutoFixCount(gm, goal.ID, prevFixCount+1)

	// Log the auto-fix for observability.
	if s.memDB != nil {
		_ = s.memDB.InsertGoalLog(goal.ID, s.agentID,
			fmt.Sprintf("Auto-fix attempt %d: diagnosed and revised failed steps", prevFixCount+1),
			truncate(resp.Content, 1000), true, 0)
	}

	s.broadcast(map[string]any{
		"type":        "goal_auto_fix_applied",
		"agent_id":    s.agentID,
		"goal_id":     goal.ID,
		"goal_name":   goal.Name,
		"fix_attempt": prevFixCount + 1,
	})

	logger.InfoCF("autonomy", "Auto-fix applied, goal will be re-dispatched", map[string]any{
		"goal_id":     goal.ID,
		"fix_attempt": prevFixCount + 1,
	})
}
