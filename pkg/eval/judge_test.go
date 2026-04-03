package eval

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHarness_JudgeScoring(t *testing.T) {
	mockJudge := func(_ context.Context, _, _, _ string) (float64, error) {
		return 0.8, nil
	}

	h := NewHarness(HarnessConfig{
		AgentFn: mockAgentFn("The answer is 4.", 100, 0.01),
		JudgeFn: mockJudge,
	})

	suite := &TestSuite{
		Name: "judge-test",
		Cases: []TestCase{
			{
				Name:           "judged-test",
				Input:          "What is 2+2?",
				ExpectedOutput: "4",
				JudgeCriteria:  "Response should contain the correct numerical answer",
			},
		},
	}

	report := h.RunSuite(context.Background(), suite)

	require.Len(t, report.Results, 1)

	r := report.Results[0]
	// Structural score is 1.0 (contains "4"), judge score is 0.8.
	// Blended: 0.5 * 1.0 + 0.5 * 0.8 = 0.9.
	assert.InDelta(t, 0.9, r.Score, 0.001)
	assert.True(t, r.Passed)
}

func TestHarness_JudgeWithLowStructural(t *testing.T) {
	mockJudge := func(_ context.Context, _, _, _ string) (float64, error) {
		return 1.0, nil
	}

	h := NewHarness(HarnessConfig{
		AgentFn: mockAgentFn("wrong answer", 50, 0.005),
		JudgeFn: mockJudge,
	})

	suite := &TestSuite{
		Name: "judge-blend",
		Cases: []TestCase{
			{
				Name:           "low-structural",
				Input:          "What is 2+2?",
				ExpectedOutput: "4",
				ExpectContains: []string{"four"},
				JudgeCriteria:  "Check answer",
			},
		},
	}

	report := h.RunSuite(context.Background(), suite)

	require.Len(t, report.Results, 1)

	r := report.Results[0]
	// Structural: 1.0 - 0.5 (no "4") - 0.25 (no "four") = 0.25.
	// Judge: 1.0.
	// Blended: 0.5 * 0.25 + 0.5 * 1.0 = 0.625.
	assert.InDelta(t, 0.625, r.Score, 0.001)
	assert.True(t, r.Passed) // 0.625 >= 0.5
}

func TestHarness_JudgeError(t *testing.T) {
	failJudge := func(_ context.Context, _, _, _ string) (float64, error) {
		return 0.0, errors.New("LLM unavailable")
	}

	h := NewHarness(HarnessConfig{
		AgentFn: mockAgentFn("The answer is 4.", 100, 0.01),
		JudgeFn: failJudge,
	})

	suite := &TestSuite{
		Name: "judge-error",
		Cases: []TestCase{
			{
				Name:           "judge-fails",
				Input:          "What is 2+2?",
				ExpectedOutput: "4",
				JudgeCriteria:  "Check answer",
			},
		},
	}

	report := h.RunSuite(context.Background(), suite)

	require.Len(t, report.Results, 1)

	r := report.Results[0]
	// On judge error, the structural score is used as-is.
	assert.Equal(t, 1.0, r.Score)
	assert.True(t, r.Passed)
}

func TestHarness_NoCriteriaSkipsJudge(t *testing.T) {
	called := false
	mockJudge := func(_ context.Context, _, _, _ string) (float64, error) {
		called = true

		return 0.0, nil
	}

	h := NewHarness(HarnessConfig{
		AgentFn: mockAgentFn("output", 10, 0.001),
		JudgeFn: mockJudge,
	})

	suite := &TestSuite{
		Name: "no-criteria",
		Cases: []TestCase{
			{
				Name:  "no-judge",
				Input: "test",
				// No JudgeCriteria set.
			},
		},
	}

	h.RunSuite(context.Background(), suite)

	assert.False(t, called, "judge should not be called when JudgeCriteria is empty")
}

func TestHarness_JudgeScoreClamping(t *testing.T) {
	t.Run("clamp high", func(t *testing.T) {
		mockJudge := func(_ context.Context, _, _, _ string) (float64, error) {
			return 1.5, nil // Above 1.0
		}

		h := NewHarness(HarnessConfig{
			AgentFn: mockAgentFn("ok", 10, 0.001),
			JudgeFn: mockJudge,
		})

		suite := &TestSuite{
			Name: "clamp-high",
			Cases: []TestCase{
				{Name: "t", Input: "x", JudgeCriteria: "check"},
			},
		}

		report := h.RunSuite(context.Background(), suite)
		// Structural: 1.0, Judge clamped to 1.0. Blended: 1.0.
		assert.InDelta(t, 1.0, report.Results[0].Score, 0.001)
	})

	t.Run("clamp low", func(t *testing.T) {
		mockJudge := func(_ context.Context, _, _, _ string) (float64, error) {
			return -0.5, nil // Below 0.0
		}

		h := NewHarness(HarnessConfig{
			AgentFn: mockAgentFn("ok", 10, 0.001),
			JudgeFn: mockJudge,
		})

		suite := &TestSuite{
			Name: "clamp-low",
			Cases: []TestCase{
				{Name: "t", Input: "x", JudgeCriteria: "check"},
			},
		}

		report := h.RunSuite(context.Background(), suite)
		// Structural: 1.0, Judge clamped to 0.0. Blended: 0.5 * 1.0 + 0.5 * 0.0 = 0.5.
		assert.InDelta(t, 0.5, report.Results[0].Score, 0.001)
	})
}

func TestDefaultJudgePrompt(t *testing.T) {
	assert.Contains(t, DefaultJudgePrompt, "{{INPUT}}")
	assert.Contains(t, DefaultJudgePrompt, "{{OUTPUT}}")
	assert.Contains(t, DefaultJudgePrompt, "{{CRITERIA}}")
	assert.Contains(t, DefaultJudgePrompt, "0.0 to 1.0")
}
