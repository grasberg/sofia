package autonomy

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/tools"
)

func TestBuildPlanGenerationPromptWithSpec(t *testing.T) {
	goal := &Goal{
		ID:          1,
		Name:        "Deploy website",
		Description: "Set up deployment",
		Priority:    "high",
		Spec: &GoalSpec{
			Requirements:    []string{"CI pipeline", "hosting"},
			SuccessCriteria: []string{"deploys on push"},
			Constraints:     []string{"budget under $50"},
		},
	}

	prompt := buildPlanGenerationPrompt(goal, "/tmp/goal-1-deploy", "")

	mustContain(t, prompt, "CI pipeline")
	mustContain(t, prompt, "deploys on push")
	mustContain(t, prompt, "budget under $50")
	mustContain(t, prompt, "/tmp/goal-1-deploy")
	mustContain(t, prompt, "acceptance_criteria")
	mustContain(t, prompt, "verify_command")
	mustContain(t, prompt, "vertical slices")
}

func TestBuildPlanGenerationPromptNoSpec(t *testing.T) {
	goal := &Goal{
		ID:          2,
		Name:        "Build API",
		Description: "Create a REST API for user management",
		Priority:    "medium",
		// No Spec — the new default path
	}

	prompt := buildPlanGenerationPrompt(goal, "/tmp/goal-2-build-api", "")

	mustContain(t, prompt, "Build API")
	mustContain(t, prompt, "REST API for user management")
	mustContain(t, prompt, "/tmp/goal-2-build-api")
	mustContain(t, prompt, "acceptance_criteria")
	mustContain(t, prompt, "verify_command")
	mustNotContain(t, prompt, "Specification:")
}

func TestBuildVerifyingTaskPrompt(t *testing.T) {
	step := tools.PlanStep{
		Index:              0,
		Description:        "Create the config file",
		AcceptanceCriteria: "config.json exists with valid JSON",
		VerifyCommand:      "Read config.json and validate it parses as JSON",
	}

	prompt := buildVerifyingTaskPrompt("Deploy website", step, "/tmp/goal-1")

	mustContain(t, prompt, "Deploy website")
	mustContain(t, prompt, "Create the config file")
	mustContain(t, prompt, "config.json exists with valid JSON")
	mustContain(t, prompt, "Read config.json and validate it parses as JSON")
	mustContain(t, prompt, "---VERIFICATION---")
	mustContain(t, prompt, "RESULT: PASS or FAIL")
}

func TestBuildVerifyingTaskPromptNoVerify(t *testing.T) {
	step := tools.PlanStep{
		Index:       0,
		Description: "Do something",
	}

	prompt := buildVerifyingTaskPrompt("Goal", step, "/tmp/goal")

	mustNotContain(t, prompt, "---VERIFICATION---")
	mustContain(t, prompt, "summarize what you actually accomplished")
}

func TestExtractVerifyResultPass(t *testing.T) {
	output := `I created the file successfully.

---VERIFICATION---
RESULT: PASS
EVIDENCE: config.json exists and contains valid JSON
---END VERIFICATION---`

	text, passed := extractVerifyResult(output)
	if !passed {
		t.Fatal("expected pass")
	}
	mustContain(t, text, "RESULT: PASS")
	mustContain(t, text, "config.json exists")
}

func TestExtractVerifyResultFail(t *testing.T) {
	output := `I tried but it didn't work.

---VERIFICATION---
RESULT: FAIL
EVIDENCE: file not found
---END VERIFICATION---`

	text, passed := extractVerifyResult(output)
	if passed {
		t.Fatal("expected fail")
	}
	mustContain(t, text, "RESULT: FAIL")
}

func TestExtractVerifyResultMissing(t *testing.T) {
	output := "I did the thing and it worked great."

	_, passed := extractVerifyResult(output)
	if passed {
		t.Fatal("expected fail when no verification section")
	}
}

func TestGoalPhaseConstants(t *testing.T) {
	phases := []string{GoalPhasePlan, GoalPhaseImplement, GoalPhaseCompleted}
	expected := []string{"plan", "implement", "completed"}

	for i, phase := range phases {
		if phase != expected[i] {
			t.Errorf("expected phase %q, got %q", expected[i], phase)
		}
	}
}

func TestClassifyStepError(t *testing.T) {
	tests := []struct {
		name       string
		result     string
		wantKind   string
		wantDetail string
	}{
		{
			name:     "command not found",
			result:   "sh: gog: command not found",
			wantKind: "tool",
		},
		{
			name:     "executable not found",
			result:   `exec: "pip": executable file not found in $PATH`,
			wantKind: "tool",
		},
		{
			name:     "no such file",
			result:   "bash: /usr/local/bin/terraform: No such file or directory",
			wantKind: "tool",
		},
		{
			name:     "binary missing",
			result:   "Error: binary missing from PATH",
			wantKind: "tool",
		},
		{
			name:       "unauthorized",
			result:     "HTTP 401 Unauthorized: invalid api key",
			wantKind:   "credential",
		},
		{
			name:       "gmail auth",
			result:     "Gmail: authentication failed, please re-authenticate",
			wantKind:   "credential",
		},
		{
			name:       "github forbidden",
			result:     "GitHub API: 403 Forbidden - bad credentials",
			wantKind:   "credential",
		},
		{
			name:       "access denied",
			result:     "Error: access denied to resource",
			wantKind:   "credential",
		},
		{
			name:     "network dns",
			result:   `dial tcp: lookup api.example.com: no such host`,
			wantKind: "network",
		},
		{
			name:     "network connection refused",
			result:   "connection refused: 127.0.0.1:8080",
			wantKind: "network",
		},
		{
			name:     "disk full",
			result:   "write failed: no space left on device",
			wantKind: "disk",
		},
		{
			name:     "rate limit 429",
			result:   "HTTP 429 Too Many Requests: rate limit exceeded",
			wantKind: "rate_limit",
		},
		{
			name:     "quota exceeded",
			result:   "Error: quota exceeded for this API",
			wantKind: "rate_limit",
		},
		{
			name:     "os permission read-only",
			result:   "cannot write: read-only file system",
			wantKind: "permission",
		},
		{
			name:     "os permission eacces",
			result:   "open /etc/hosts: EACCES",
			wantKind: "permission",
		},
		{
			name:     "config missing env var",
			result:   "error: environment variable not set: DATABASE_URL",
			wantKind: "config",
		},
		{
			name:     "config missing required",
			result:   "missing required config: webhook_secret",
			wantKind: "config",
		},
		{
			name:     "generic failure",
			result:   "segfault at address 0x00",
			wantKind: "",
		},
		{
			name:     "empty input",
			result:   "",
			wantKind: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, _ := classifyStepError(tt.result)
			if kind != tt.wantKind {
				t.Errorf("classifyStepError(%q) kind = %q, want %q", tt.result, kind, tt.wantKind)
			}
		})
	}
}

func TestExtractToolHint(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
	}{
		{"sh command not found", "sh: gog: command not found", "gog"},
		{"bash command not found", "bash: terraform: command not found", "terraform"},
		{"exec not found", `exec: "pip": executable file not found`, "pip"},
		{"no match", "something went wrong", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractToolHint(tt.input)
			if got != tt.want {
				t.Errorf("extractToolHint(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractHostHint(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lookup form", "dial tcp: lookup api.example.com: no such host", "api.example.com"},
		{"url form", `Get "https://api.openai.com/v1/models": dial tcp: connection refused`, "api.openai.com"},
		{"no match", "connection refused", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractHostHint(tt.input)
			if got != tt.want {
				t.Errorf("extractHostHint(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractPathHint(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"permission with path", "open /etc/hosts: permission denied", ""},
		{"denied then path", "permission denied: /var/log/app.log", "/var/log/app.log"},
		{"read-only then path", "read-only file system: /usr/local/bin/foo", "/usr/local/bin/foo"},
		{"no path", "operation not permitted", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPathHint(tt.input)
			if got != tt.want {
				t.Errorf("extractPathHint(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractConfigHint(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"env var", "environment variable not set: DATABASE_URL", "DATABASE_URL"},
		{"config key", "missing required config: WEBHOOK_SECRET", "WEBHOOK_SECRET"},
		{"no match", "something went wrong", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractConfigHint(tt.input)
			if got != tt.want {
				t.Errorf("extractConfigHint(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractCredentialHint(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"gmail", "Gmail OAuth token expired", "Gmail / Google"},
		{"openai", "OpenAI API key invalid", "OpenAI"},
		{"github", "GitHub: bad credentials", "GitHub"},
		{"docker", "Docker Hub: unauthorized", "Docker"},
		{"smtp", "SMTP connection refused: authentication failed", "Email (SMTP)"},
		{"no match", "random error text", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCredentialHint(tt.input)
			if got != tt.want {
				t.Errorf("extractCredentialHint(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractVerifyResult_MultipleMarkers(t *testing.T) {
	// When multiple verification sections exist, should use the last one.
	output := `First attempt:
---VERIFICATION---
RESULT: FAIL
EVIDENCE: missing file
---END VERIFICATION---

Retried and fixed:
---VERIFICATION---
RESULT: PASS
EVIDENCE: file created successfully
---END VERIFICATION---`

	text, passed := extractVerifyResult(output)
	if !passed {
		t.Fatal("expected pass (last verification section)")
	}
	mustContain(t, text, "file created successfully")
}

func TestExtractVerifyResult_NoEndMarker(t *testing.T) {
	output := `Done.
---VERIFICATION---
RESULT: PASS
EVIDENCE: all tests green`

	text, passed := extractVerifyResult(output)
	if !passed {
		t.Fatal("expected pass even without end marker")
	}
	mustContain(t, text, "all tests green")
}

func TestParseGoalPlanResponse_WithAcceptanceCriteria(t *testing.T) {
	input := `{"goal_id": 5, "goal_name": "Test", "plan": {"steps": [{"description": "Step 1", "acceptance_criteria": "File exists", "verify_command": "ls file.txt", "depends_on": []}]}}`
	resp, err := parseGoalPlanResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Plan.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(resp.Plan.Steps))
	}
	if resp.Plan.Steps[0].AcceptanceCriteria != "File exists" {
		t.Errorf("acceptance_criteria = %q, want %q", resp.Plan.Steps[0].AcceptanceCriteria, "File exists")
	}
	if resp.Plan.Steps[0].VerifyCommand != "ls file.txt" {
		t.Errorf("verify_command = %q, want %q", resp.Plan.Steps[0].VerifyCommand, "ls file.txt")
	}
}

func TestParseGoalResultResponse_WithUnmetCriteria(t *testing.T) {
	input := `{"summary": "Partial", "artifacts": [], "next_steps": ["Fix it"], "unmet_criteria": ["Tests must pass"]}`
	resp, err := parseGoalResultResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Summary != "Partial" {
		t.Errorf("summary = %q, want %q", resp.Summary, "Partial")
	}
	if len(resp.UnmetCriteria) != 1 || resp.UnmetCriteria[0] != "Tests must pass" {
		t.Errorf("unmet_criteria = %v, want [Tests must pass]", resp.UnmetCriteria)
	}
}

func mustContain(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected string to contain %q, but it didn't.\nString (first 300 chars): %s", substr, truncate(s, 300))
	}
}

func mustNotContain(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected string NOT to contain %q, but it did", substr)
	}
}

func TestBuildMemoryContext_NilDB(t *testing.T) {
	out := buildMemoryContext(nil, "agent-1", "Deploy website")
	if out != "" {
		t.Errorf("expected empty output for nil memDB, got %q", out)
	}
}

func TestBuildMemoryContext_EmptyQuery(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	out := buildMemoryContext(db, "agent-1", "   ")
	if out != "" {
		t.Errorf("expected empty output for blank query, got %q", out)
	}
}

func TestBuildMemoryContext_NoMatches(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	out := buildMemoryContext(db, "agent-1", "Deploy website")
	if out != "" {
		t.Errorf("expected empty output when nothing is stored, got %q", out)
	}
}

func TestBuildMemoryContext_IncludesHighScoreReflections(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	agentID := "agent-1"
	// Two relevant high-score lessons, one low-score that should be filtered.
	lessons := []memory.ReflectionRecord{
		{AgentID: agentID, SessionKey: "s1", TaskSummary: "Deploy website",
			Lessons: "Always run the build before deploying", Score: 0.9},
		{AgentID: agentID, SessionKey: "s2", TaskSummary: "Deploy website",
			Lessons: "Check env vars before starting the server", Score: 0.7},
		{AgentID: agentID, SessionKey: "s3", TaskSummary: "Deploy website",
			Lessons: "Low quality lesson that should be filtered", Score: 0.3},
	}
	for _, r := range lessons {
		if err := db.SaveReflection(r); err != nil {
			t.Fatalf("SaveReflection: %v", err)
		}
	}

	out := buildMemoryContext(db, agentID, "Deploy website")
	mustContain(t, out, "Past lessons relevant to this goal")
	mustContain(t, out, "Always run the build before deploying")
	mustContain(t, out, "Check env vars before starting the server")
	mustNotContain(t, out, "Low quality lesson")
}

func TestBuildMemoryContext_IncludesPlanTemplates(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	err := db.SavePlanTemplate(
		"Deploy static website",
		"Deploy website",
		[]string{
			"Build the static site",
			"Upload artifacts to CDN",
			"Invalidate cache",
		},
		"deploy,web",
	)
	if err != nil {
		t.Fatalf("SavePlanTemplate: %v", err)
	}

	out := buildMemoryContext(db, "agent-1", "Deploy website")
	mustContain(t, out, "Matching plan templates")
	mustContain(t, out, "Deploy static website")
	mustContain(t, out, "Build the static site")
	mustContain(t, out, "Invalidate cache")
}

func TestBuildMemoryContext_TruncatesLongTemplates(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	steps := []string{}
	for i := 0; i < 20; i++ {
		steps = append(steps, "Step number "+string(rune('A'+i)))
	}
	err := db.SavePlanTemplate("Huge plan", "Deploy website", steps, "deploy")
	if err != nil {
		t.Fatalf("SavePlanTemplate: %v", err)
	}

	out := buildMemoryContext(db, "agent-1", "Deploy website")
	mustContain(t, out, "more step(s)")
}

func TestBuildPlanGenerationPrompt_WithMemoryContext(t *testing.T) {
	goal := &Goal{
		ID:          1,
		Name:        "Deploy website",
		Description: "Set up deployment",
		Priority:    "high",
	}
	memCtx := "\n## Past lessons relevant to this goal\n- (score=0.9) Always run the build before deploying\n"
	prompt := buildPlanGenerationPrompt(goal, "/tmp/goal-deploy", memCtx)

	mustContain(t, prompt, "Always run the build before deploying")
	mustContain(t, prompt, "Past lessons relevant to this goal")
}

func TestBuildPlanGenerationPrompt_EmptyMemoryContext(t *testing.T) {
	goal := &Goal{
		ID:          1,
		Name:        "Deploy website",
		Description: "Set up deployment",
		Priority:    "high",
	}
	prompt := buildPlanGenerationPrompt(goal, "/tmp/goal-deploy", "")

	mustNotContain(t, prompt, "Past lessons")
	mustNotContain(t, prompt, "Matching plan templates")
}

// ----------------------------------------------------------------------------
// Goal-level reflection tests
// ----------------------------------------------------------------------------

func TestBuildGoalReflectionPrompt_IncludesAllSections(t *testing.T) {
	goal := &Goal{
		ID: 7, Name: "Deploy website", Description: "Set up CI",
		Priority: "high",
		Spec: &GoalSpec{
			Requirements:    []string{"CI pipeline"},
			SuccessCriteria: []string{"deploys on push"},
			Constraints:     []string{"budget < $50"},
		},
	}
	plan := &tools.Plan{
		Steps: []tools.PlanStep{
			{Index: 0, Description: "Build the site", Status: tools.PlanStatusCompleted, VerifyResult: "exit 0", RetryCount: 0},
			{Index: 1, Description: "Deploy artifacts", Status: tools.PlanStatusFailed, Result: "timeout", RetryCount: 2},
		},
	}
	result := GoalResult{
		Summary:       "Completed 1 of 2 steps.",
		UnmetCriteria: []string{"deploys on push"},
	}

	p := buildGoalReflectionPrompt(goal, plan, result)

	mustContain(t, p, "Deploy website")
	mustContain(t, p, "CI pipeline")
	mustContain(t, p, "deploys on push")
	mustContain(t, p, "budget < $50")
	mustContain(t, p, "Step 0")
	mustContain(t, p, "Build the site")
	mustContain(t, p, "Step 1")
	mustContain(t, p, "Deploy artifacts")
	mustContain(t, p, "failure: timeout")
	mustContain(t, p, "Unmet success criteria")
	mustContain(t, p, "1 step(s) completed, 1 failed")
}

func TestBuildGoalReflectionPrompt_NoSpec(t *testing.T) {
	goal := &Goal{ID: 8, Name: "Small task", Description: "Do a thing", Priority: "low"}
	plan := &tools.Plan{
		Steps: []tools.PlanStep{
			{Index: 0, Description: "Do a thing", Status: tools.PlanStatusCompleted},
		},
	}
	p := buildGoalReflectionPrompt(goal, plan, GoalResult{Summary: "Done."})

	mustNotContain(t, p, "Goal specification")
	mustNotContain(t, p, "Unmet success criteria")
	mustContain(t, p, "1 step(s) completed, 0 failed")
}

func TestParseGoalReflectionJSON_HappyPath(t *testing.T) {
	raw := `{"task_summary":"Deployed","what_worked":"build+test","what_failed":"","lessons":"Always run build first","score":0.9}`
	r, err := parseGoalReflectionJSON(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.Score != 0.9 || r.Lessons != "Always run build first" {
		t.Errorf("unexpected parse result: %+v", r)
	}
}

func TestParseGoalReflectionJSON_WithCodeFences(t *testing.T) {
	raw := "```json\n{\"task_summary\":\"x\",\"lessons\":\"y\",\"score\":0.5}\n```"
	r, err := parseGoalReflectionJSON(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.Lessons != "y" {
		t.Errorf("expected 'y' lesson, got %q", r.Lessons)
	}
}

func TestParseGoalReflectionJSON_Invalid(t *testing.T) {
	if _, err := parseGoalReflectionJSON("not json"); err == nil {
		t.Error("expected error for non-JSON input")
	}
}

// reflectOnGoal integration tests use a real memDB and a MockProvider whose
// response is the reflection JSON we want to store.
func newReflectionTestService(t *testing.T, db *memory.MemoryDB, responseJSON string) *Service {
	t.Helper()
	return &Service{
		cfg:      &config.AutonomyConfig{Enabled: true},
		memDB:    db,
		provider: &MockProvider{ResponseContent: responseJSON},
		agentID:  "agent-1",
		modelID:  "mock-model",
	}
}

func TestReflectOnGoal_SavesReflection(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	goal := &Goal{ID: 42, Name: "Deploy website", Description: "Set up deploy", Priority: "high"}
	plan := &tools.Plan{
		Steps: []tools.PlanStep{
			{Index: 0, Description: "Build", Status: tools.PlanStatusCompleted},
			{Index: 1, Description: "Deploy", Status: tools.PlanStatusCompleted},
		},
	}
	resp := `{"task_summary":"Deployed","what_worked":"all steps passed","what_failed":"","lessons":"Run build+test before deploy","score":0.9}`

	s := newReflectionTestService(t, db, resp)
	s.reflectOnGoal(goal, plan, GoalResult{Summary: "ok"})

	records, err := db.GetRecentReflections("agent-1", 5)
	if err != nil {
		t.Fatalf("GetRecentReflections: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 reflection, got %d", len(records))
	}
	if records[0].Lessons != "Run build+test before deploy" {
		t.Errorf("unexpected lessons: %q", records[0].Lessons)
	}
	if records[0].SessionKey != "goal-42" {
		t.Errorf("expected session_key goal-42, got %q", records[0].SessionKey)
	}
	if records[0].Score != 0.9 {
		t.Errorf("expected score 0.9, got %v", records[0].Score)
	}
}

func TestReflectOnGoal_SavesTemplateOnCleanSuccess(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	goal := &Goal{ID: 43, Name: "Deploy clean", Description: "Deploy", Priority: "high"}
	plan := &tools.Plan{
		Steps: []tools.PlanStep{
			{Index: 0, Description: "Build the site", Status: tools.PlanStatusCompleted},
			{Index: 1, Description: "Push to CDN", Status: tools.PlanStatusCompleted},
		},
	}
	resp := `{"task_summary":"Done","what_worked":"clean run","what_failed":"","lessons":"This worked","score":0.9}`

	s := newReflectionTestService(t, db, resp)
	s.reflectOnGoal(goal, plan, GoalResult{Summary: "ok"})

	tpl, err := db.GetPlanTemplate("Deploy clean")
	if err != nil {
		t.Fatalf("expected plan template, got error: %v", err)
	}
	if len(tpl.Steps) != 2 || tpl.Steps[0] != "Build the site" || tpl.Steps[1] != "Push to CDN" {
		t.Errorf("unexpected template steps: %+v", tpl.Steps)
	}
}

func TestReflectOnGoal_NoTemplateOnFailedSteps(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	goal := &Goal{ID: 44, Name: "Deploy with failure", Description: "Deploy", Priority: "high"}
	plan := &tools.Plan{
		Steps: []tools.PlanStep{
			{Index: 0, Description: "Build", Status: tools.PlanStatusCompleted},
			{Index: 1, Description: "Push", Status: tools.PlanStatusFailed, Result: "boom"},
		},
	}
	resp := `{"task_summary":"Partial","what_worked":"build","what_failed":"push","lessons":"Check creds","score":0.5}`

	s := newReflectionTestService(t, db, resp)
	s.reflectOnGoal(goal, plan, GoalResult{Summary: "partial"})

	if _, err := db.GetPlanTemplate("Deploy with failure"); err == nil {
		t.Error("expected no plan template to be saved when a step failed")
	}
}

func TestReflectOnGoal_NoTemplateOnLowScore(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	goal := &Goal{ID: 45, Name: "Lowscore run", Description: "Deploy", Priority: "high"}
	plan := &tools.Plan{
		Steps: []tools.PlanStep{
			{Index: 0, Description: "Do it", Status: tools.PlanStatusCompleted},
		},
	}
	resp := `{"task_summary":"Meh","what_worked":"","what_failed":"inefficient","lessons":"Plan better","score":0.5}`

	s := newReflectionTestService(t, db, resp)
	s.reflectOnGoal(goal, plan, GoalResult{Summary: "done but inefficient"})

	if _, err := db.GetPlanTemplate("Lowscore run"); err == nil {
		t.Error("expected no plan template to be saved for low-score run")
	}
}

func TestReflectOnGoal_NilMemDBNoCrash(t *testing.T) {
	s := &Service{cfg: &config.AutonomyConfig{Enabled: true}}
	s.reflectOnGoal(
		&Goal{ID: 1, Name: "x"},
		&tools.Plan{Steps: []tools.PlanStep{{Status: tools.PlanStatusCompleted}}},
		GoalResult{},
	)
}

func TestReflectOnGoal_EmptyPlanNoCrash(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	s := newReflectionTestService(t, db, `{"score":1}`)
	s.reflectOnGoal(&Goal{ID: 1, Name: "x"}, &tools.Plan{}, GoalResult{})

	records, _ := db.SearchReflections("agent-1", "x", 5)
	if len(records) != 0 {
		t.Errorf("expected no reflection for empty plan, got %d", len(records))
	}
}

// ----------------------------------------------------------------------------
// Auto-install tests
// ----------------------------------------------------------------------------

// registerTestInstallMethod adds a sentinel binary to autoInstallMethods for
// the duration of the test. Uses t.Cleanup so nothing leaks between tests.
func registerTestInstallMethod(t *testing.T, binary, cmd string) {
	t.Helper()
	if autoInstallMethods[binary] != nil {
		t.Fatalf("test binary %q already in autoInstallMethods", binary)
	}
	autoInstallMethods[binary] = map[string]string{runtime.GOOS: cmd}
	t.Cleanup(func() { delete(autoInstallMethods, binary) })
}

// setupResolveService builds a Service configured for auto-install resolution
// with a recording installer stub.
func setupResolveService(autoEnable bool, installerOK bool) (*Service, *[]string) {
	var called []string
	s := &Service{
		cfg:                 &config.AutonomyConfig{Enabled: true, AutoInstallTools: autoEnable},
		agentID:             "agent-1",
		autoInstallAttempts: make(map[int64]map[string]bool),
	}
	s.toolInstaller = func(ctx context.Context, command string) (bool, string, error) {
		called = append(called, command)
		if installerOK {
			return true, "installed", nil
		}
		return false, "install failed", nil
	}
	return s, &called
}

func TestAutoInstallCommandFor_UnknownBinary(t *testing.T) {
	if _, ok := autoInstallCommandFor("nonexistent-binary-xyz-123"); ok {
		t.Error("expected no install cmd for unknown binary")
	}
}

func TestTryAutoResolveStepFailure_ConfigDisabled(t *testing.T) {
	registerTestInstallMethod(t, "__autotest_disabled__", "echo install-disabled")
	pm := tools.NewPlanManager()
	plan := pm.CreatePlanForGoal(100, "test goal", []tools.PlanStepDef{{Description: "do it"}})
	pm.CompleteStepWithVerify(plan.ID, 0, false, "sh: __autotest_disabled__: command not found", "")

	s, called := setupResolveService(false /* disabled */, true)
	ok := s.tryAutoResolveStepFailure(pm, 100, plan.ID, 0, "do it",
		"sh: __autotest_disabled__: command not found")
	if ok {
		t.Error("expected false when AutoInstallTools is disabled")
	}
	if len(*called) != 0 {
		t.Errorf("expected no install calls when disabled, got %d", len(*called))
	}
}

func TestTryAutoResolveStepFailure_UnknownBinary(t *testing.T) {
	pm := tools.NewPlanManager()
	plan := pm.CreatePlanForGoal(101, "test goal", []tools.PlanStepDef{{Description: "do it"}})
	pm.CompleteStepWithVerify(plan.ID, 0, false, "sh: blerg-not-whitelisted-123: command not found", "")

	s, called := setupResolveService(true, true)
	ok := s.tryAutoResolveStepFailure(pm, 101, plan.ID, 0, "do it",
		"sh: blerg-not-whitelisted-123: command not found")
	if ok {
		t.Error("expected false for binary not in whitelist")
	}
	if len(*called) != 0 {
		t.Errorf("expected no install calls for non-whitelisted binary, got %d", len(*called))
	}
}

func TestTryAutoResolveStepFailure_NonToolError(t *testing.T) {
	pm := tools.NewPlanManager()
	plan := pm.CreatePlanForGoal(102, "test goal", []tools.PlanStepDef{{Description: "do it"}})
	pm.CompleteStepWithVerify(plan.ID, 0, false, "HTTP 401 unauthorized", "")

	s, called := setupResolveService(true, true)
	ok := s.tryAutoResolveStepFailure(pm, 102, plan.ID, 0, "do it", "HTTP 401 unauthorized")
	if ok {
		t.Error("expected false for credential-kind error, not tool")
	}
	if len(*called) != 0 {
		t.Errorf("expected no install calls for credential error, got %d", len(*called))
	}
}

func TestTryAutoResolveStepFailure_SuccessResetsStep(t *testing.T) {
	registerTestInstallMethod(t, "__autotest_success__", "echo installed")
	pm := tools.NewPlanManager()
	plan := pm.CreatePlanForGoal(103, "test goal", []tools.PlanStepDef{{Description: "do it"}})
	pm.CompleteStepWithVerify(plan.ID, 0, false, "sh: __autotest_success__: command not found", "")

	s, called := setupResolveService(true, true)
	ok := s.tryAutoResolveStepFailure(pm, 103, plan.ID, 0, "do it",
		"sh: __autotest_success__: command not found")
	if !ok {
		t.Fatal("expected true when install succeeds and step resets")
	}
	if len(*called) != 1 || !strings.Contains((*called)[0], "echo installed") {
		t.Errorf("expected one install call with 'echo installed', got %v", *called)
	}

	// Verify the step is back to pending so the dispatcher can retry it.
	got := pm.GetPlanByGoalID(103)
	if got == nil {
		t.Fatal("plan missing after reset")
	}
	if got.Steps[0].Status != tools.PlanStatusPending {
		t.Errorf("expected step pending after reset, got %q", got.Steps[0].Status)
	}
	if got.Steps[0].RetryCount != 0 {
		t.Errorf("expected retry_count=0 after auto-install reset, got %d", got.Steps[0].RetryCount)
	}
}

func TestTryAutoResolveStepFailure_InstallFails(t *testing.T) {
	registerTestInstallMethod(t, "__autotest_fail__", "echo install")
	pm := tools.NewPlanManager()
	plan := pm.CreatePlanForGoal(104, "test goal", []tools.PlanStepDef{{Description: "do it"}})
	pm.CompleteStepWithVerify(plan.ID, 0, false, "sh: __autotest_fail__: command not found", "")

	s, called := setupResolveService(true, false /* install fails */)
	ok := s.tryAutoResolveStepFailure(pm, 104, plan.ID, 0, "do it",
		"sh: __autotest_fail__: command not found")
	if ok {
		t.Error("expected false when install command fails")
	}
	if len(*called) != 1 {
		t.Errorf("expected one install attempt even when it fails, got %d", len(*called))
	}

	// Step should remain failed since we didn't reset it.
	got := pm.GetPlanByGoalID(104)
	if got.Steps[0].Status != tools.PlanStatusFailed {
		t.Errorf("expected step to remain failed after install failure, got %q", got.Steps[0].Status)
	}
}

func TestTryAutoResolveStepFailure_AttemptedOnceGuard(t *testing.T) {
	registerTestInstallMethod(t, "__autotest_once__", "echo install")
	pm := tools.NewPlanManager()
	plan := pm.CreatePlanForGoal(105, "test goal", []tools.PlanStepDef{{Description: "do it"}})

	// Fail + resolve + fail again scenario.
	pm.CompleteStepWithVerify(plan.ID, 0, false, "sh: __autotest_once__: command not found", "")
	s, called := setupResolveService(true, true)
	first := s.tryAutoResolveStepFailure(pm, 105, plan.ID, 0, "do it",
		"sh: __autotest_once__: command not found")
	if !first {
		t.Fatal("first resolve should succeed")
	}

	// Simulate the step failing again with the same error. Auto-install should
	// refuse to try a second time for the same (goal, binary) pair.
	pm.CompleteStepWithVerify(plan.ID, 0, false, "sh: __autotest_once__: command not found", "")
	second := s.tryAutoResolveStepFailure(pm, 105, plan.ID, 0, "do it",
		"sh: __autotest_once__: command not found")
	if second {
		t.Error("second resolve for same binary should be refused by the attempted-once guard")
	}
	if len(*called) != 1 {
		t.Errorf("expected exactly one install call (attempted-once guard), got %d", len(*called))
	}
}
