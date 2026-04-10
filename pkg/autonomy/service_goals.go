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

// Patterns that indicate a missing tool or credential rather than a logic error.
// When matched in a failed step's output, the user is notified so they can
// install the tool or add the credential in Settings.
var (
	missingToolPatterns = []string{
		"command not found",
		"not found in PATH",
		"binary missing",
		"no such file or directory",
		"executable file not found",
		"program not found",
		"is not recognized",
	}
	missingCredentialPatterns = []string{
		"unauthorized",
		"authentication failed",
		"invalid api key",
		"invalid token",
		"401",
		"403",
		"forbidden",
		"access denied",
		"no credentials",
		"no api key",
		"permission denied",
		"credential",
		"re-authenticate",
		"oauth token",
	}
)

// classifyStepError checks a failed step's output for tool/credential issues.
// Returns ("tool", description) or ("credential", description) or ("", "") if
// it's a generic failure.
func classifyStepError(result string) (kind, detail string) {
	lower := strings.ToLower(result)
	for _, p := range missingToolPatterns {
		if strings.Contains(lower, p) {
			// Try to extract the tool name from common patterns.
			return "tool", extractToolHint(result)
		}
	}
	for _, p := range missingCredentialPatterns {
		if strings.Contains(lower, p) {
			return "credential", extractCredentialHint(result)
		}
	}
	return "", ""
}

// Pre-compiled regexes for extracting tool/binary names from error text.
var (
	reShCommandNotFound = regexp.MustCompile(`(?:sh|bash|zsh):\s*(\S+):\s*(?:command )?not found`)
	reExecNotFound      = regexp.MustCompile(`exec:\s*"?(\S+?)"?:\s*executable`)
)

// extractToolHint tries to pull the binary name from error text like
// "sh: gog: command not found" or "exec: pip: executable file not found".
func extractToolHint(result string) string {
	if m := reShCommandNotFound.FindStringSubmatch(result); len(m) > 1 {
		return m[1]
	}
	if m := reExecNotFound.FindStringSubmatch(result); len(m) > 1 {
		return m[1]
	}
	return ""
}

// extractCredentialHint tries to identify the service from auth error text.
func extractCredentialHint(result string) string {
	lower := strings.ToLower(result)
	services := map[string]string{
		"gmail":       "Gmail / Google",
		"google":      "Google",
		"openai":      "OpenAI",
		"anthropic":   "Anthropic",
		"openrouter":  "OpenRouter",
		"github":      "GitHub",
		"smtp":        "Email (SMTP)",
		"imap":        "Email (IMAP)",
		"docker":      "Docker",
		"ollama.com":  "Ollama Cloud",
	}
	for keyword, name := range services {
		if strings.Contains(lower, keyword) {
			return name
		}
	}
	return ""
}

// pursueGoals is the phased pipeline entry point: specify → plan → implement.
func (s *Service) pursueGoals(ctx context.Context) {
	gm := NewGoalManager(s.memDB)

	allGoals, err := gm.ListAllGoals(s.agentID)
	if err != nil {
		logger.WarnCF("autonomy", "Failed to list goals", map[string]any{"error": err.Error()})
		return
	}

	for _, goal := range allGoals {
		if goal.Status != GoalStatusActive && goal.Status != GoalStatusInProgress {
			continue
		}
		select {
		case <-ctx.Done():
			return
		default:
		}

		phase := goal.Phase
		if phase == "" || phase == "specify" {
			phase = GoalPhasePlan
		}

		switch phase {
		case GoalPhasePlan:
			s.generatePlanForGoal(ctx, gm, goal)
		case GoalPhaseImplement:
			s.dispatchReadySteps(ctx, gm, goal)
		}
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
	Summary       string   `json:"summary"`
	Artifacts     []string `json:"artifacts"`
	NextSteps     []string `json:"next_steps"`
	UnmetCriteria []string `json:"unmet_criteria"`
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

// buildPlanGenerationPrompt creates the LLM prompt that asks for a complete plan with acceptance criteria and verification.
func buildPlanGenerationPrompt(goal *Goal) string {
	var specSection string
	if goal.Spec != nil {
		specSection = fmt.Sprintf(`
Specification:
- Requirements: %s
- Success Criteria: %s
- Constraints: %s

Your plan must address ALL requirements and enable verification of ALL success criteria.`,
			strings.Join(goal.Spec.Requirements, "; "),
			strings.Join(goal.Spec.SuccessCriteria, "; "),
			strings.Join(goal.Spec.Constraints, "; "))
	}

	return fmt.Sprintf(`You are an autonomous AI agent. Create a complete plan for the following goal:

Goal ID: %d
Goal Name: %s
Description: %s
Priority: %s
%s

Create a detailed plan with 3-10 steps. Each step must include:
- description: What to do (specific and actionable, delegatable to a subagent)
- acceptance_criteria: How to know the step is done correctly
- verify_command: A verification instruction the subagent should execute after completing the step to confirm it worked
- depends_on: Array of step indices (0-based) that must complete first

Prefer vertical slices — each step should deliver a complete, verifiable piece of work rather than a layer (e.g. "implement and test feature X" not "write all database schemas").

Respond in this exact JSON format (no markdown, no code fences):
{"goal_id": %d, "goal_name": "%s", "plan": {"steps": [{"description": "...", "acceptance_criteria": "...", "verify_command": "...", "depends_on": []}]}}`, goal.ID, goal.Name, goal.Description, goal.Priority, specSection, goal.ID, goal.Name)
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

	// If a plan already exists (e.g. created by the chat agent), advance the phase.
	if existing := pm.GetPlanByGoalID(goal.ID); existing != nil {
		_ = gm.UpdateGoalPhase(goal.ID, GoalPhaseImplement)
		if _, err := gm.UpdateGoalStatus(goal.ID, GoalStatusInProgress); err != nil {
			logger.WarnCF("autonomy", "Failed to transition goal to in_progress", map[string]any{
				"goal_id": goal.ID, "error": err.Error(),
			})
		}
		// If the plan is already done, finalize immediately.
		if existing.Status == tools.PlanStatusCompleted {
			s.finalizeGoal(ctx, gm, goal, existing)
		}
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

	// Transition goal to in_progress and phase to implement.
	if _, err := gm.UpdateGoalStatus(goal.ID, GoalStatusInProgress); err != nil {
		logger.WarnCF("autonomy", "Failed to transition goal to in_progress", map[string]any{
			"goal_id": goal.ID,
			"error":   err.Error(),
		})
	}
	if err := gm.UpdateGoalPhase(goal.ID, GoalPhaseImplement); err != nil {
		logger.WarnCF("autonomy", "Failed to update goal phase to implement", map[string]any{
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

// buildVerifyingTaskPrompt creates the subagent task prompt with acceptance criteria and verification.
func buildVerifyingTaskPrompt(goalName string, step tools.PlanStep, goalDir string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, `You are working toward goal: "%s"

Your task: %s
`, goalName, step.Description)

	if step.AcceptanceCriteria != "" {
		fmt.Fprintf(&sb, `
Acceptance criteria: %s
`, step.AcceptanceCriteria)
	}

	fmt.Fprintf(&sb, `
Working directory for this goal: %s

Rules:
- Use tools to do real work (read_file, write_file, exec, edit_file, list_dir, append_file).
- All file operations MUST use absolute paths under the goal folder.
- Do NOT just describe what you would do. Actually do it.
`, goalDir)

	if step.VerifyCommand != "" {
		fmt.Fprintf(&sb, `
VERIFICATION (mandatory):
After completing your task, you MUST verify your work:
%s

End your response with a verification section in this exact format:
---VERIFICATION---
RESULT: PASS or FAIL
EVIDENCE: [what you observed]
---END VERIFICATION---
`, step.VerifyCommand)
	} else {
		sb.WriteString("\nWhen done, summarize what you actually accomplished.\n")
	}

	return sb.String()
}

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
		"goal_id":   goalID,
		"step_index": stepIdx,
		"detail":    detail,
	})
}

// maxStepRetries returns the configured max retries, defaulting to 2.
func (s *Service) maxStepRetries() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.MaxStepRetries > 0 {
		return s.cfg.MaxStepRetries
	}
	return 2
}

// extractVerifyResult extracts the verification section from subagent output.
// Returns the verification text and whether verification passed.
func extractVerifyResult(output string) (verifyText string, passed bool) {
	const startMarker = "---VERIFICATION---"
	const endMarker = "---END VERIFICATION---"

	startIdx := strings.LastIndex(output, startMarker)
	if startIdx == -1 {
		return "", false
	}

	section := output[startIdx+len(startMarker):]
	endIdx := strings.Index(section, endMarker)
	if endIdx != -1 {
		section = section[:endIdx]
	}

	section = strings.TrimSpace(section)
	passed = strings.Contains(strings.ToUpper(section), "RESULT: PASS")
	return section, passed
}

// dispatchReadySteps finds steps whose dependencies are satisfied, claims them,
// and spawns subagents with verification and retry logic.
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
		if fixCount < maxGoalAutoFixes {
			logger.InfoCF("autonomy", "Plan failed, attempting auto-fix", map[string]any{
				"goal_id":   goal.ID,
				"plan_id":   plan.ID,
				"fix_attempt": fixCount + 1,
				"max_fixes": maxGoalAutoFixes,
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
			go s.autoFixGoal(ctx, gm, goal, &planCopy, fixCount)
			return
		}

		logger.WarnCF("autonomy", "Plan permanently failed after auto-fix attempts, marking goal as failed", map[string]any{
			"goal_id":     goal.ID,
			"plan_id":     plan.ID,
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
		taskPrompt := buildVerifyingTaskPrompt(goal.Name, step, goalDir)

		capturedGoalID := goal.ID
		capturedGoalName := goal.Name
		capturedStepIdx := stepIdx
		capturedPlanID := plan.ID
		capturedAgentID := s.agentID
		capturedRetryCount := step.RetryCount
		hasVerifyCommand := step.VerifyCommand != ""

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
				// Atomically record failure and reset for retry — avoids a
				// race where the tick cycle sees a temporarily-failed plan.
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

				// Re-dispatch (the retried step will be picked up as ready).
				updatedGoal, err := gm.GetGoalByID(capturedGoalID)
				if err == nil && updatedGoal != nil {
					s.dispatchReadySteps(cbCtx, gm, updatedGoal)
				}
				return
			}

			pm.CompleteStepWithVerify(capturedPlanID, capturedStepIdx, stepSuccess, truncate(resultText, 2000), verifyText)

			// If the step permanently failed, check if it's a tool/credential
			// issue and notify the user so they can fix it.
			if !stepSuccess {
				s.notifyUserActionNeeded(capturedGoalID, capturedGoalName, capturedStepIdx, step.Description, resultText)
			}

			if s.memDB != nil {
				_ = s.memDB.InsertGoalLog(
					capturedGoalID,
					capturedAgentID,
					step.Description,
					truncate(resultText, 2000),
					stepSuccess,
					0,
				)
			}

			s.broadcast(map[string]any{
				"type":       "goal_step_end",
				"agent_id":   capturedAgentID,
				"goal_id":    capturedGoalID,
				"goal_name":  capturedGoalName,
				"step_index": capturedStepIdx,
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

// autoFixGoal asks the LLM to diagnose why steps failed and produces revised
// step descriptions. It then resets the failed steps with the new instructions
// and lets the normal tick re-dispatch them.
func (s *Service) autoFixGoal(ctx context.Context, gm *GoalManager, goal *Goal, plan *tools.Plan, prevFixCount int) {
	// Collect failed step details for the LLM.
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

	prompt := fmt.Sprintf(`A goal's plan has failed. Diagnose the problems and produce revised step descriptions that fix the issues.

Goal: %s
Description: %s

Failed steps:
%s
Previous fix attempts: %d

For EACH failed step, analyze WHY it failed and write a REVISED description that addresses the root cause.
The revised description should include specific fixes — different commands, corrected paths, alternative approaches, etc.
Do NOT just repeat the same instructions.

Respond in this exact JSON format (no markdown, no code fences):
{"revisions": [{"step_index": 0, "diagnosis": "why it failed", "revised_description": "new step instructions"}]}`,
		goal.Name, goal.Description, sb.String(), prevFixCount)

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
		goalResult.Summary = "Goal completed (summary generation failed)"
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
}

// completeGoal marks a goal as completed with phase update.
func (s *Service) completeGoal(gm *GoalManager, goal *Goal) {
	if _, err := gm.UpdateGoalStatus(goal.ID, GoalStatusCompleted); err != nil {
		logger.WarnCF("autonomy", "Failed to mark goal completed",
			map[string]any{"goal_id": goal.ID, "error": err.Error()})
	}
	_ = gm.UpdateGoalPhase(goal.ID, GoalPhaseCompleted)
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
