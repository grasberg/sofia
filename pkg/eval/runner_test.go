package eval

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunTest_AllPass(t *testing.T) {
	runner := NewEvalRunner()
	tc := TestCase{
		Name:           "all-pass",
		Input:          "What is 2+2?",
		ExpectedOutput: "4",
		ExpectPattern:  `\d+`,
		ExpectContains: []string{"4"},
		NotContains:    []string{"error"},
	}

	result := runner.RunTest(tc, "The answer is 4.", 100*time.Millisecond)

	assert.True(t, result.Passed)
	assert.Equal(t, 1.0, result.Score)
	assert.Empty(t, result.Errors)
	assert.Equal(t, "all-pass", result.Name)
	assert.Equal(t, "What is 2+2?", result.Input)
	assert.Equal(t, "The answer is 4.", result.Output)
	assert.Equal(t, 100*time.Millisecond, result.Duration)
}

func TestRunTest_SubstringFail(t *testing.T) {
	runner := NewEvalRunner()

	t.Run("substring miss alone gives 0.5 score and still passes", func(t *testing.T) {
		tc := TestCase{
			Name:           "substring-borderline",
			Input:          "What is the capital of France?",
			ExpectedOutput: "Paris",
		}

		result := runner.RunTest(tc, "The capital is Berlin.", 50*time.Millisecond)

		assert.True(t, result.Passed)
		assert.Equal(t, 0.5, result.Score)
		require.Len(t, result.Errors, 1)
		assert.Contains(t, result.Errors[0], "Paris")
	})

	t.Run("substring miss plus contains miss fails", func(t *testing.T) {
		tc := TestCase{
			Name:           "substring-fail",
			Input:          "What is the capital of France?",
			ExpectedOutput: "Paris",
			ExpectContains: []string{"France"},
		}

		result := runner.RunTest(tc, "The capital is Berlin.", 50*time.Millisecond)

		assert.False(t, result.Passed)
		assert.Equal(t, 0.25, result.Score)
		require.Len(t, result.Errors, 2)
		assert.Contains(t, result.Errors[0], "Paris")
	})
}

func TestRunTest_PatternMatch(t *testing.T) {
	runner := NewEvalRunner()

	t.Run("pattern matches", func(t *testing.T) {
		tc := TestCase{
			Name:          "pattern-pass",
			Input:         "Give me a number",
			ExpectPattern: `\b\d{3}\b`,
		}

		result := runner.RunTest(tc, "The code is 123 ok", 10*time.Millisecond)

		assert.True(t, result.Passed)
		assert.Equal(t, 1.0, result.Score)
		assert.Empty(t, result.Errors)
	})

	t.Run("pattern does not match gives borderline pass", func(t *testing.T) {
		tc := TestCase{
			Name:          "pattern-borderline",
			Input:         "Give me a number",
			ExpectPattern: `\b\d{5}\b`,
		}

		result := runner.RunTest(tc, "The code is 123 ok", 10*time.Millisecond)

		assert.True(t, result.Passed)
		assert.Equal(t, 0.5, result.Score)
		require.Len(t, result.Errors, 1)
		assert.Contains(t, result.Errors[0], "did not match")
	})

	t.Run("pattern and substring both fail", func(t *testing.T) {
		tc := TestCase{
			Name:           "pattern-fail",
			Input:          "Give me a number",
			ExpectPattern:  `\b\d{5}\b`,
			ExpectedOutput: "99999",
		}

		result := runner.RunTest(tc, "The code is 123 ok", 10*time.Millisecond)

		assert.False(t, result.Passed)
		assert.Equal(t, 0.0, result.Score)
		assert.Len(t, result.Errors, 2)
	})

	t.Run("invalid regex", func(t *testing.T) {
		tc := TestCase{
			Name:           "bad-pattern",
			Input:          "test",
			ExpectPattern:  `[invalid`,
			ExpectedOutput: "required",
		}

		result := runner.RunTest(tc, "anything", 10*time.Millisecond)

		assert.False(t, result.Passed)
		assert.Equal(t, 0.0, result.Score)
		require.Len(t, result.Errors, 2)
		assert.Contains(t, result.Errors[0], "required")
		assert.Contains(t, result.Errors[1], "invalid regex")
	})
}

func TestRunTest_ContainsCheck(t *testing.T) {
	runner := NewEvalRunner()
	tc := TestCase{
		Name:           "contains-check",
		Input:          "List fruits",
		ExpectContains: []string{"apple", "banana", "cherry"},
	}

	t.Run("all present", func(t *testing.T) {
		result := runner.RunTest(tc, "I like apple, banana, and cherry.", 10*time.Millisecond)

		assert.True(t, result.Passed)
		assert.Equal(t, 1.0, result.Score)
		assert.Empty(t, result.Errors)
	})

	t.Run("one missing", func(t *testing.T) {
		result := runner.RunTest(tc, "I like apple and banana.", 10*time.Millisecond)

		assert.True(t, result.Passed)
		assert.Equal(t, 0.75, result.Score)
		require.Len(t, result.Errors, 1)
		assert.Contains(t, result.Errors[0], "cherry")
	})

	t.Run("all missing", func(t *testing.T) {
		result := runner.RunTest(tc, "I like grapes.", 10*time.Millisecond)

		assert.False(t, result.Passed)
		assert.Equal(t, 0.25, result.Score)
		assert.Len(t, result.Errors, 3)
	})
}

func TestRunTest_NotContainsViolation(t *testing.T) {
	runner := NewEvalRunner()
	tc := TestCase{
		Name:        "not-contains",
		Input:       "Be polite",
		NotContains: []string{"stupid", "dumb", "idiot"},
	}

	t.Run("none found", func(t *testing.T) {
		result := runner.RunTest(tc, "You are wonderful.", 10*time.Millisecond)

		assert.True(t, result.Passed)
		assert.Equal(t, 1.0, result.Score)
		assert.Empty(t, result.Errors)
	})

	t.Run("one violation", func(t *testing.T) {
		result := runner.RunTest(tc, "That was a stupid mistake.", 10*time.Millisecond)

		assert.True(t, result.Passed)
		assert.Equal(t, 0.75, result.Score)
		require.Len(t, result.Errors, 1)
		assert.Contains(t, result.Errors[0], "stupid")
	})

	t.Run("multiple violations", func(t *testing.T) {
		result := runner.RunTest(tc, "stupid and dumb and idiot", 10*time.Millisecond)

		assert.False(t, result.Passed)
		assert.Equal(t, 0.25, result.Score)
		assert.Len(t, result.Errors, 3)
	})
}

func TestRunTest_ScoreClamp(t *testing.T) {
	runner := NewEvalRunner()

	t.Run("clamp to zero", func(t *testing.T) {
		tc := TestCase{
			Name:           "max-penalty",
			Input:          "test",
			ExpectedOutput: "AAAA",
			ExpectPattern:  `^ZZZZ$`,
			ExpectContains: []string{"x1", "x2", "x3", "x4"},
			NotContains:    []string{"bad"},
		}

		result := runner.RunTest(tc, "bad output", 10*time.Millisecond)

		assert.False(t, result.Passed)
		assert.Equal(t, 0.0, result.Score)
	})
}

func TestGenerateReport(t *testing.T) {
	runner := NewEvalRunner()
	results := []TestResult{
		{Name: "test-1", Passed: true, Score: 1.0, Duration: 100 * time.Millisecond},
		{Name: "test-2", Passed: true, Score: 0.75, Duration: 200 * time.Millisecond},
		{Name: "test-3", Passed: false, Score: 0.25, Duration: 150 * time.Millisecond},
	}

	report := runner.GenerateReport(results, 500*time.Millisecond)

	assert.Equal(t, 3, report.TotalTests)
	assert.Equal(t, 2, report.Passed)
	assert.Equal(t, 1, report.Failed)
	assert.Equal(t, 2.0, report.TotalScore)
	assert.InDelta(t, 0.6667, report.AvgScore, 0.001)
	assert.Equal(t, 500*time.Millisecond, report.Duration)
	assert.Len(t, report.Results, 3)
	assert.False(t, report.RunAt.IsZero())
}

func TestGenerateReport_Empty(t *testing.T) {
	runner := NewEvalRunner()

	report := runner.GenerateReport(nil, 0)

	assert.Equal(t, 0, report.TotalTests)
	assert.Equal(t, 0, report.Passed)
	assert.Equal(t, 0, report.Failed)
	assert.Equal(t, 0.0, report.TotalScore)
	assert.Equal(t, 0.0, report.AvgScore)
}
