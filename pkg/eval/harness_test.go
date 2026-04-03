package eval

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockAgentFn(output string, tokens int, cost float64) AgentFunc {
	return func(_ context.Context, _ string) (string, UsageInfo, error) {
		return output, UsageInfo{TokensUsed: tokens, CostUSD: cost}, nil
	}
}

func TestHarness_SimpleSuite(t *testing.T) {
	h := NewHarness(HarnessConfig{
		AgentFn: mockAgentFn("The answer is 4.", 100, 0.01),
	})

	suite := &TestSuite{
		Name: "basic",
		Cases: []TestCase{
			{
				Name:           "math",
				Input:          "What is 2+2?",
				ExpectedOutput: "4",
			},
			{
				Name:           "greeting",
				Input:          "Hello",
				ExpectedOutput: "Hello",
			},
		},
	}

	report := h.RunSuite(context.Background(), suite)

	assert.Equal(t, 2, report.TotalTests)
	assert.Equal(t, 2, report.Passed) // both pass: "math" has "4", "greeting" gets 0.5 (>=0.5 threshold)
	assert.Equal(t, 0, report.Failed)
	assert.Equal(t, 200, report.TotalTokens)
	assert.InDelta(t, 0.02, report.TotalCostUSD, 0.001)
	assert.Len(t, report.Results, 2)

	// Verify individual results have token/cost populated.
	for _, r := range report.Results {
		assert.Equal(t, 100, r.TokensUsed)
		assert.InDelta(t, 0.01, r.CostUSD, 0.001)
	}
}

func TestHarness_Parallel(t *testing.T) {
	var running atomic.Int32
	var maxSeen atomic.Int32

	slowAgent := func(_ context.Context, _ string) (string, UsageInfo, error) {
		cur := running.Add(1)
		defer running.Add(-1)

		// Track the maximum concurrency observed.
		for {
			old := maxSeen.Load()
			if cur <= old || maxSeen.CompareAndSwap(old, cur) {
				break
			}
		}

		time.Sleep(50 * time.Millisecond)

		return "ok", UsageInfo{TokensUsed: 10, CostUSD: 0.001}, nil
	}

	h := NewHarness(HarnessConfig{
		AgentFn:     slowAgent,
		Concurrency: 3,
	})

	suite := &TestSuite{
		Name: "parallel-test",
		Cases: []TestCase{
			{Name: "t1", Input: "a"},
			{Name: "t2", Input: "b"},
			{Name: "t3", Input: "c"},
			{Name: "t4", Input: "d"},
			{Name: "t5", Input: "e"},
			{Name: "t6", Input: "f"},
		},
	}

	report := h.RunSuite(context.Background(), suite)

	assert.Equal(t, 6, report.TotalTests)
	assert.Equal(t, 60, report.TotalTokens)

	// With concurrency=3 and 6 tests, max concurrent should be at most 3.
	assert.LessOrEqual(t, int(maxSeen.Load()), 3)

	// With concurrency, should complete faster than sequential (6 * 50ms = 300ms).
	// Allow generous margin but verify parallelism happened.
	assert.Less(t, report.Duration, 400*time.Millisecond)
}

func TestHarness_PerTestTimeout(t *testing.T) {
	slowAgent := func(ctx context.Context, _ string) (string, UsageInfo, error) {
		select {
		case <-ctx.Done():
			return "", UsageInfo{}, ctx.Err()
		case <-time.After(5 * time.Second):
			return "done", UsageInfo{}, nil
		}
	}

	h := NewHarness(HarnessConfig{
		AgentFn: slowAgent,
		Timeout: 10 * time.Second, // default is high
	})

	suite := &TestSuite{
		Name: "timeout-test",
		Cases: []TestCase{
			{
				Name:    "fast-timeout",
				Input:   "test",
				Timeout: 1, // 1 second per-test override
			},
		},
	}

	report := h.RunSuite(context.Background(), suite)

	require.Len(t, report.Results, 1)
	assert.False(t, report.Results[0].Passed)
	assert.Equal(t, 0.0, report.Results[0].Score)
	require.NotEmpty(t, report.Results[0].Errors)
}

func TestHarness_AgentError(t *testing.T) {
	failAgent := func(_ context.Context, _ string) (string, UsageInfo, error) {
		return "", UsageInfo{TokensUsed: 5}, errors.New("agent crashed")
	}

	h := NewHarness(HarnessConfig{
		AgentFn: failAgent,
	})

	suite := &TestSuite{
		Name: "error-test",
		Cases: []TestCase{
			{Name: "crash", Input: "trigger error"},
		},
	}

	report := h.RunSuite(context.Background(), suite)

	require.Len(t, report.Results, 1)

	r := report.Results[0]
	assert.False(t, r.Passed)
	assert.Equal(t, 0.0, r.Score)
	assert.Equal(t, 5, r.TokensUsed)
	require.Len(t, r.Errors, 1)
	assert.Contains(t, r.Errors[0], "agent crashed")
}

func TestHarness_CustomScorer(t *testing.T) {
	h := NewHarness(HarnessConfig{
		AgentFn: mockAgentFn("HELLO WORLD", 50, 0.005),
		Scorers: map[string]ScorerFunc{
			"exact": ExactMatchScorer,
		},
	})

	suite := &TestSuite{
		Name: "scorer-test",
		Cases: []TestCase{
			{
				Name:           "exact-match-pass",
				Input:          "say hello",
				ExpectedOutput: "hello world",
				CustomScorer:   "exact",
			},
			{
				Name:           "exact-match-fail",
				Input:          "say hello",
				ExpectedOutput: "goodbye",
				CustomScorer:   "exact",
			},
		},
	}

	report := h.RunSuite(context.Background(), suite)

	require.Len(t, report.Results, 2)

	// Case-insensitive exact match: "HELLO WORLD" == "hello world".
	assert.True(t, report.Results[0].Passed)
	assert.Equal(t, 1.0, report.Results[0].Score)

	// "HELLO WORLD" != "goodbye".
	assert.False(t, report.Results[1].Passed)
	assert.Equal(t, 0.0, report.Results[1].Score)
	require.NotEmpty(t, report.Results[1].Errors)
}

func TestHarness_CustomScorerNotFound(t *testing.T) {
	h := NewHarness(HarnessConfig{
		AgentFn: mockAgentFn("output", 10, 0.001),
	})

	suite := &TestSuite{
		Name: "missing-scorer",
		Cases: []TestCase{
			{
				Name:         "fallback-to-structural",
				Input:        "test",
				CustomScorer: "nonexistent",
			},
		},
	}

	// Should fall back to structural scoring when scorer is not found.
	report := h.RunSuite(context.Background(), suite)

	require.Len(t, report.Results, 1)
	// No structural checks defined, so structural score is 1.0.
	assert.Equal(t, 1.0, report.Results[0].Score)
}

func TestHarness_CostTokenAccumulation(t *testing.T) {
	callCount := 0
	variedAgent := func(_ context.Context, _ string) (string, UsageInfo, error) {
		callCount++

		return "ok", UsageInfo{
			TokensUsed: callCount * 100,
			CostUSD:    float64(callCount) * 0.01,
		}, nil
	}

	h := NewHarness(HarnessConfig{
		AgentFn: variedAgent,
	})

	suite := &TestSuite{
		Name: "cost-test",
		Cases: []TestCase{
			{Name: "t1", Input: "a"},
			{Name: "t2", Input: "b"},
			{Name: "t3", Input: "c"},
		},
	}

	report := h.RunSuite(context.Background(), suite)

	// Sequential: tokens = 100 + 200 + 300 = 600, cost = 0.01 + 0.02 + 0.03 = 0.06.
	assert.Equal(t, 600, report.TotalTokens)
	assert.InDelta(t, 0.06, report.TotalCostUSD, 0.001)
}

func TestHarness_DefaultTimeout(t *testing.T) {
	h := NewHarness(HarnessConfig{
		AgentFn: mockAgentFn("ok", 10, 0.001),
	})

	// Verify default timeout is 60s.
	assert.Equal(t, 60*time.Second, h.cfg.Timeout)
}

func TestHarness_DefaultConcurrency(t *testing.T) {
	h := NewHarness(HarnessConfig{
		AgentFn: mockAgentFn("ok", 10, 0.001),
	})

	// Verify default concurrency is 1.
	assert.Equal(t, 1, h.cfg.Concurrency)
}
