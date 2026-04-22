package autonomy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/tools"
	"github.com/grasberg/sofia/pkg/utils"
)

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
// memoryContext is an optional pre-formatted string (from buildMemoryContext)
// with relevant past lessons and plan templates — pass "" to skip.
func buildPlanGenerationPrompt(goal *Goal, goalDir, memoryContext string) string {
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

	// Include workspace context for better plans.
	var workspaceContext string
	if entries, err := os.ReadDir(goalDir); err == nil && len(entries) > 0 {
		var files []string
		for _, e := range entries {
			files = append(files, e.Name())
		}
		workspaceContext = fmt.Sprintf("\nExisting files in goal folder:\n- %s\n", strings.Join(files, "\n- "))
	}
	// Also check for go.mod / package.json in parent workspace.
	for _, probe := range []string{"go.mod", "package.json", "Cargo.toml", "requirements.txt"} {
		if content, err := os.ReadFile(filepath.Join(goalDir, "..", "..", probe)); err == nil && len(content) > 0 {
			workspaceContext += fmt.Sprintf("\n%s:\n%s\n", probe, truncate(string(content), 500))
			break
		}
	}

	return fmt.Sprintf(`You are an autonomous AI agent. Create a complete plan for the following goal:

Goal ID: %d
Goal Name: %s
Description: %s
Priority: %s
Goal Folder: %s
%s%s%s

Create a detailed plan with 3-10 steps. Each step must include:
- description: What to do (specific and actionable, delegatable to a subagent). MUST include the goal folder path and instruct the subagent to save all files there.
- acceptance_criteria: How to know the step is done correctly
- verify_command: A verification instruction the subagent should execute after completing the step to confirm it worked
- depends_on: Array of step indices (0-based) that must complete first

All file operations in every step MUST use absolute paths under the goal folder: %s

Prefer vertical slices — each step should deliver a complete, verifiable piece of work rather than a layer (e.g. "implement and test feature X" not "write all database schemas").

Respond in this exact JSON format (no markdown, no code fences):
{"goal_id": %d, "goal_name": "%s", "plan": {"steps": [{"description": "...", "acceptance_criteria": "...", "verify_command": "...", "depends_on": []}]}}`, goal.ID, goal.Name, goal.Description, goal.Priority, goalDir, specSection, workspaceContext, memoryContext, goalDir, goal.ID, goal.Name)
}

// buildMemoryContext formats recent high-scoring reflections and top-matching
// plan templates into an advisory section for the plan-generation prompt.
// Returns "" when the memory store is nil, the query is empty, or no
// relevant entries exist. The output is capped at ~2K chars to prevent
// prompt bloat on goals whose name matches many past entries.
func buildMemoryContext(memDB *memory.MemoryDB, agentID, query string) string {
	const (
		maxReflections   = 5
		minReflectScore  = 0.6
		maxTemplates     = 3
		maxTemplateSteps = 6
		maxTotalChars    = 2000
	)

	query = strings.TrimSpace(query)
	if memDB == nil || query == "" {
		return ""
	}

	var sb strings.Builder

	if refs, err := memDB.SearchReflections(agentID, query, maxReflections); err == nil {
		var lessons []string
		for _, r := range refs {
			if r.Score < minReflectScore {
				continue
			}
			lesson := strings.TrimSpace(r.Lessons)
			if lesson == "" {
				continue
			}
			lessons = append(lessons, fmt.Sprintf("- (score=%.1f) %s", r.Score, truncate(lesson, 300)))
		}
		if len(lessons) > 0 {
			sb.WriteString("\n## Past lessons relevant to this goal\n")
			sb.WriteString(strings.Join(lessons, "\n"))
			sb.WriteString("\n")
		}
	}

	if templates, err := memDB.FindPlanTemplates(query, maxTemplates); err == nil && len(templates) > 0 {
		sb.WriteString("\n## Matching plan templates (scaffolds from past successful goals)\n")
		for _, t := range templates {
			fmt.Fprintf(&sb, "- %q (used %d time(s), success %.0f%%):\n",
				t.Name, t.UseCount, t.SuccessRate*100)
			for i, step := range t.Steps {
				if i >= maxTemplateSteps {
					fmt.Fprintf(&sb, "    … %d more step(s)\n", len(t.Steps)-maxTemplateSteps)
					break
				}
				fmt.Fprintf(&sb, "    %d. %s\n", i+1, truncate(step, 200))
			}
		}
	}

	if sb.Len() == 0 {
		return ""
	}

	out := sb.String()
	if len(out) > maxTotalChars {
		out = out[:maxTotalChars] + "\n… (truncated)\n"
	}
	return "\n" + out + "\nUse the above as guidance — adapt, don't copy blindly. If a lesson conflicts with this goal's requirements, follow the requirements.\n"
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

	goalDir := s.ensureGoalFolder(goal.ID, goal.Name)
	memoryContext := buildMemoryContext(s.memDB, s.agentID, goal.Name+" "+goal.Description)
	prompt := buildPlanGenerationPrompt(goal, goalDir, memoryContext)
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
