package autonomy

import (
	"testing"

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

	prompt := buildPlanGenerationPrompt(goal)

	mustContain(t, prompt, "CI pipeline")
	mustContain(t, prompt, "deploys on push")
	mustContain(t, prompt, "budget under $50")
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

	prompt := buildPlanGenerationPrompt(goal)

	mustContain(t, prompt, "Build API")
	mustContain(t, prompt, "REST API for user management")
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
