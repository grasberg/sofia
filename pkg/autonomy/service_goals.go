package autonomy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/tools"
	"github.com/grasberg/sofia/pkg/utils"
)

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
	missingNetworkPatterns = []string{
		"no such host",
		"could not resolve host",
		"name or service not known",
		"dns lookup failed",
		"connection refused",
		"network is unreachable",
		"network unreachable",
		"no route to host",
		"connection reset by peer",
		"tls handshake timeout",
		"i/o timeout",
		"dial tcp",
		"connect: connection timed out",
	}
	diskExhaustedPatterns = []string{
		"no space left on device",
		"disk quota exceeded",
		"enospc",
		"out of disk space",
		"write error: no space",
	}
	rateLimitPatterns = []string{
		"rate limit",
		"rate-limited",
		"ratelimited",
		"too many requests",
		"429",
		"quota exceeded",
		"usage limit exceeded",
		"retry-after",
		"throttled",
	}
	// Filesystem/OS-level permission issues — distinct from API credentials.
	// The credential check already catches the ambiguous "permission denied"
	// substring, so these patterns target unambiguous OS signals.
	osPermissionPatterns = []string{
		"eacces",
		"eperm",
		"read-only file system",
		"operation not permitted",
		"requires sudo",
		"must be root",
		"must be run as root",
	}
	missingConfigPatterns = []string{
		"environment variable not set",
		"env var not set",
		"env variable not set",
		"required environment variable",
		"missing required config",
		"required setting",
		"config not found",
		"configuration file not found",
		"configuration key not found",
		"required configuration",
	}
)

// autoInstallMethods maps binary names to per-platform install commands.
// The map is intentionally conservative: only binaries where a single
// well-known command produces a working install. Only macOS (brew) is
// supported initially because it doesn't require elevated privileges for
// installs. Linux/apt requires root and is deferred until a safe story
// for non-interactive sudo lands.
var autoInstallMethods = map[string]map[string]string{
	"jq":        {"darwin": "brew install jq"},
	"rg":        {"darwin": "brew install ripgrep"},
	"ripgrep":   {"darwin": "brew install ripgrep"},
	"fd":        {"darwin": "brew install fd"},
	"bat":       {"darwin": "brew install bat"},
	"gh":        {"darwin": "brew install gh"},
	"tree":      {"darwin": "brew install tree"},
	"wget":      {"darwin": "brew install wget"},
	"yq":        {"darwin": "brew install yq"},
	"terraform": {"darwin": "brew install hashicorp/tap/terraform"},
	"kubectl":   {"darwin": "brew install kubectl"},
	"helm":      {"darwin": "brew install helm"},
	"node":      {"darwin": "brew install node"},
	"npm":       {"darwin": "brew install node"},
	"python3":   {"darwin": "brew install python"},
	"pip3":      {"darwin": "brew install python"},
	"cargo":     {"darwin": "brew install rust"},
	"rustc":     {"darwin": "brew install rust"},
	"deno":      {"darwin": "brew install deno"},
	"bun":       {"darwin": "brew install bun"},
	"ffmpeg":    {"darwin": "brew install ffmpeg"},
	"imagemagick": {"darwin": "brew install imagemagick"},
	"pandoc":    {"darwin": "brew install pandoc"},
	"sqlite3":   {"darwin": "brew install sqlite"},
	"postgres":  {"darwin": "brew install postgresql"},
	"psql":      {"darwin": "brew install postgresql"},
	"redis-cli": {"darwin": "brew install redis"},
	"aws":       {"darwin": "brew install awscli"},
	"gcloud":    {"darwin": "brew install --cask google-cloud-sdk"},
}

// classifyStepError checks a failed step's output and returns a category of
// user action needed, or ("", "") for a generic failure the agent should
// retry on its own. Categories are checked from most-specific to least-
// specific so unambiguous OS/network signals win over broader auth matches.
func classifyStepError(result string) (kind, detail string) {
	lower := strings.ToLower(result)
	for _, p := range diskExhaustedPatterns {
		if strings.Contains(lower, p) {
			return "disk", ""
		}
	}
	for _, p := range missingNetworkPatterns {
		if strings.Contains(lower, p) {
			return "network", extractHostHint(result)
		}
	}
	for _, p := range rateLimitPatterns {
		if strings.Contains(lower, p) {
			return "rate_limit", extractCredentialHint(result)
		}
	}
	for _, p := range osPermissionPatterns {
		if strings.Contains(lower, p) {
			return "permission", extractPathHint(result)
		}
	}
	for _, p := range missingToolPatterns {
		if strings.Contains(lower, p) {
			return "tool", extractToolHint(result)
		}
	}
	for _, p := range missingConfigPatterns {
		if strings.Contains(lower, p) {
			return "config", extractConfigHint(result)
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
	// Hostname in messages like `dial tcp: lookup example.com: no such host`
	// or `Get "https://api.example.com/…": dial tcp 1.2.3.4:443: connect: …`.
	reLookupHost = regexp.MustCompile(`lookup\s+([A-Za-z0-9][A-Za-z0-9.\-]*\.[A-Za-z]{2,})`)
	reURLHost    = regexp.MustCompile(`https?://([A-Za-z0-9][A-Za-z0-9.\-]*\.[A-Za-z]{2,})`)
	// Absolute filesystem path after "permission denied:" or similar.
	rePathAfterColon = regexp.MustCompile(`(?:permission denied|operation not permitted|read-only file system)[^/]*?(/[^\s:'"]+)`)
	// Env var / config key names like "FOO_BAR is not set" or
	// "missing required config: FOO_BAR".
	reEnvVarName = regexp.MustCompile(`\b([A-Z][A-Z0-9_]{2,})\b`)
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
		"gmail":      "Gmail / Google",
		"google":     "Google",
		"openai":     "OpenAI",
		"anthropic":  "Anthropic",
		"openrouter": "OpenRouter",
		"github":     "GitHub",
		"smtp":       "Email (SMTP)",
		"imap":       "Email (IMAP)",
		"docker":     "Docker",
		"ollama.com": "Ollama Cloud",
	}
	for keyword, name := range services {
		if strings.Contains(lower, keyword) {
			return name
		}
	}
	return ""
}

// extractHostHint pulls a hostname out of network error text for display.
func extractHostHint(result string) string {
	if m := reLookupHost.FindStringSubmatch(result); len(m) > 1 {
		return m[1]
	}
	if m := reURLHost.FindStringSubmatch(result); len(m) > 1 {
		return m[1]
	}
	return ""
}

// extractPathHint pulls an absolute filesystem path from permission errors.
func extractPathHint(result string) string {
	if m := rePathAfterColon.FindStringSubmatch(strings.ToLower(result)); len(m) > 1 {
		return m[1]
	}
	return ""
}

// extractConfigHint tries to identify the missing config key (env var name).
func extractConfigHint(result string) string {
	if m := reEnvVarName.FindStringSubmatch(result); len(m) > 1 {
		return m[1]
	}
	return ""
}

// goalPriorityOrder maps priority strings to sort order.
var goalPriorityOrder = map[string]int{"high": 0, "medium": 1, "low": 2, "": 1}

// sortGoalsByPriority sorts goals so high-priority goals are processed first.
func sortGoalsByPriority(goals []*Goal) {
	sort.Slice(goals, func(i, j int) bool {
		pi := goalPriorityOrder[goals[i].Priority]
		pj := goalPriorityOrder[goals[j].Priority]
		return pi < pj
	})
}

// finalizeCompletedGoals scans active/in-progress goals and finalizes any
// whose plans are fully completed. This runs even when the autonomy goals
// flag is off, because goals created via the chat UI still need finalization.
func (s *Service) finalizeCompletedGoals(ctx context.Context) {
	s.mu.Lock()
	pm := s.planMgr
	s.mu.Unlock()
	if pm == nil {
		return
	}

	gm := NewGoalManager(s.memDB)
	allGoals, err := gm.ListAllGoals(s.agentID)
	if err != nil {
		return
	}

	for _, goal := range allGoals {
		if goal.Status != GoalStatusActive && goal.Status != GoalStatusInProgress {
			continue
		}
		plan := pm.GetPlanByGoalID(goal.ID)
		if plan != nil && plan.Status == tools.PlanStatusCompleted {
			logger.InfoCF("autonomy", "Finalizing completed goal", map[string]any{
				"goal_id":   goal.ID,
				"goal_name": goal.Name,
			})
			s.finalizeGoal(ctx, gm, goal, plan)
		}
	}
}

// pursueGoals is the phased pipeline entry point: plan → implement.
// Goals are processed in priority order (high → medium → low).
func (s *Service) pursueGoals(ctx context.Context) {
	gm := NewGoalManager(s.memDB)

	allGoals, err := gm.ListAllGoals(s.agentID)
	if err != nil {
		logger.WarnCF("autonomy", "Failed to list goals", map[string]any{"error": err.Error()})
		return
	}

	sortGoalsByPriority(allGoals)

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

// autoInstallCommandFor returns the platform-specific install command for a
// whitelisted binary, or ("", false) if the binary isn't in the map or the
// current platform isn't supported.
func autoInstallCommandFor(binary string) (string, bool) {
	methods, ok := autoInstallMethods[binary]
	if !ok {
		return "", false
	}
	cmd, ok := methods[runtime.GOOS]
	return cmd, ok
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

// maxStepRetries returns the configured max retries, defaulting to 2.
func (s *Service) maxStepRetries() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.MaxStepRetries > 0 {
		return s.cfg.MaxStepRetries
	}
	return 2
}

// maxAutoFixAttempts returns the configured max auto-fix attempts, defaulting to 2.
func (s *Service) maxAutoFixAttempts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.MaxAutoFixAttempts > 0 {
		return s.cfg.MaxAutoFixAttempts
	}
	return maxGoalAutoFixes
}

// NotifyGoalCreated triggers immediate plan generation for a newly created goal,
// bypassing the tick interval wait.
func (s *Service) NotifyGoalCreated(goalID int64) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.ErrorCF("autonomy", "Panic in NotifyGoalCreated", map[string]any{
					"goal_id": goalID,
					"panic":   fmt.Sprintf("%v", r),
				})
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		gm := NewGoalManager(s.memDB)
		goal, err := gm.GetGoalByID(goalID)
		if err != nil || goal == nil {
			logger.WarnCF("autonomy", "NotifyGoalCreated: goal not found", map[string]any{
				"goal_id": goalID, "error": fmt.Sprintf("%v", err),
			})
			return
		}
		if goal.Status != GoalStatusActive {
			return
		}
		s.generatePlanForGoal(ctx, gm, goal)
	}()
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

func (s *Service) defaultGoalConcurrency() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.DefaultGoalConcurrency > 0 {
		if s.cfg.DefaultGoalConcurrency > 10 {
			return 10
		}
		return s.cfg.DefaultGoalConcurrency
	}
	return 3
}

func (s *Service) stepBackoffBaseSec() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.StepBackoffBaseSec > 0 {
		return s.cfg.StepBackoffBaseSec
	}
	return 10
}

func (s *Service) stepBackoffMaxSec() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.StepBackoffMaxSec > 0 {
		return s.cfg.StepBackoffMaxSec
	}
	return 120
}

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

// completeGoal marks a goal as completed with phase update.
func (s *Service) completeGoal(gm *GoalManager, goal *Goal) {
	if _, err := gm.UpdateGoalStatus(goal.ID, GoalStatusCompleted); err != nil {
		logger.WarnCF("autonomy", "Failed to mark goal completed",
			map[string]any{"goal_id": goal.ID, "error": err.Error()})
	}
	_ = gm.UpdateGoalPhase(goal.ID, GoalPhaseCompleted)
}

// goalFolderPath returns the absolute path for a goal's working directory.
func (s *Service) goalFolderPath(goalID int64, goalName string) string {
	return filepath.Join(s.workspace, "goals", tools.GoalFolderName(goalID, goalName))
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
