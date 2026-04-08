package autonomy

import (
	"testing"

	"github.com/grasberg/sofia/pkg/tools"
)

func TestBuildSpecificationPrompt(t *testing.T) {
	goal := &Goal{
		ID:          1,
		Name:        "Deploy website",
		Description: "Set up a production deployment pipeline",
		Priority:    "high",
	}

	prompt := buildSpecificationPrompt(goal)

	mustContain(t, prompt, "Deploy website")
	mustContain(t, prompt, "Set up a production deployment pipeline")
	mustContain(t, prompt, "high")
	mustContain(t, prompt, "requirements")
	mustContain(t, prompt, "success_criteria")
}

func TestParseSpecResponseValid(t *testing.T) {
	input := `{"requirements": ["build CI pipeline", "configure hosting"], "success_criteria": ["site loads in <2s", "deploys on push"], "constraints": ["budget under $50/mo"]}`

	resp, err := parseSpecResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Requirements) != 2 {
		t.Errorf("expected 2 requirements, got %d", len(resp.Requirements))
	}
	if len(resp.SuccessCriteria) != 2 {
		t.Errorf("expected 2 success criteria, got %d", len(resp.SuccessCriteria))
	}
	if len(resp.Constraints) != 1 {
		t.Errorf("expected 1 constraint, got %d", len(resp.Constraints))
	}
}

func TestParseSpecResponseWithFences(t *testing.T) {
	input := "```json\n{\"requirements\": [\"r1\"], \"success_criteria\": [\"s1\"]}\n```"

	resp, err := parseSpecResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Requirements) != 1 {
		t.Errorf("expected 1 requirement, got %d", len(resp.Requirements))
	}
}

func TestParseSpecResponseNoSuccessCriteria(t *testing.T) {
	input := `{"requirements": ["r1"], "success_criteria": []}`

	_, err := parseSpecResponse(input)
	if err == nil {
		t.Fatal("expected error for empty success criteria")
	}
}

func TestParseSpecResponseMalformed(t *testing.T) {
	_, err := parseSpecResponse("this is not json at all")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

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

	prompt := buildPlanGenerationPrompt(goal)

	mustContain(t, prompt, "CI pipeline")
	mustContain(t, prompt, "deploys on push")
	mustContain(t, prompt, "budget under $50")
	mustContain(t, prompt, "acceptance_criteria")
	mustContain(t, prompt, "verify_command")
	mustContain(t, prompt, "vertical slices")
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
	phases := []string{GoalPhaseSpecify, GoalPhasePlan, GoalPhaseImplement, GoalPhaseCompleted}
	expected := []string{"specify", "plan", "implement", "completed"}

	for i, phase := range phases {
		if phase != expected[i] {
			t.Errorf("expected phase %q, got %q", expected[i], phase)
		}
	}
}

func mustContain(t *testing.T, s, substr string) {
	t.Helper()
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return
		}
	}
	t.Errorf("expected string to contain %q, but it didn't.\nString (first 300 chars): %s", substr, truncate(s, 300))
}

func mustNotContain(t *testing.T, s, substr string) {
	t.Helper()
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			t.Errorf("expected string NOT to contain %q, but it did", substr)
			return
		}
	}
}
