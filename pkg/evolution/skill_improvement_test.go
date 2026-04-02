package evolution

import (
	"testing"
)

func TestSkillImprovementTypes(t *testing.T) {
	// Test that SkillImprovement can be created
	imp := SkillImprovement{
		SkillName:  "test-skill",
		Issue:      "Missing error handling",
		Suggestion: "Add proper error handling for edge cases",
		Priority:   5,
	}

	if imp.SkillName != "test-skill" {
		t.Errorf("Expected skill name 'test-skill', got %s", imp.SkillName)
	}
	if imp.Priority != 5 {
		t.Errorf("Expected priority 5, got %d", imp.Priority)
	}

	// Test Suggestion type
	sug := Suggestion{
		Issue:      "Test issue",
		Suggestion: "Test suggestion",
	}

	if sug.Issue != "Test issue" {
		t.Errorf("Expected issue 'Test issue', got %s", sug.Issue)
	}
}
