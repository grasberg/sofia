package eval

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeComparisonReport(results []TestResult) EvalReport {
	runner := NewEvalRunner()

	return runner.GenerateReport(results, 100*time.Millisecond)
}

func TestCompare_IdenticalReports(t *testing.T) {
	results := []TestResult{
		{Name: "test-1", Passed: true, Score: 0.8, Duration: 50 * time.Millisecond},
		{Name: "test-2", Passed: true, Score: 0.9, Duration: 50 * time.Millisecond},
		{Name: "test-3", Passed: false, Score: 0.4, Duration: 50 * time.Millisecond},
	}

	baseline := makeComparisonReport(results)
	candidate := makeComparisonReport(results)

	cmp := Compare(baseline, candidate)

	assert.Equal(t, "tie", cmp.Winner)
	assert.InDelta(t, 0.0, cmp.ScoreDelta, 0.001)
	assert.InDelta(t, 0.0, cmp.PassRateDelta, 0.001)
	assert.Contains(t, cmp.Summary, "Tie")
	assert.Len(t, cmp.PerTest, 3)

	// All per-test deltas should be zero.
	for _, pt := range cmp.PerTest {
		assert.InDelta(t, 0.0, pt.Delta, 0.001)
	}
}

func TestCompare_ClearWinner(t *testing.T) {
	baseResults := []TestResult{
		{Name: "test-1", Passed: true, Score: 0.5, Duration: 50 * time.Millisecond},
		{Name: "test-2", Passed: false, Score: 0.3, Duration: 50 * time.Millisecond},
		{Name: "test-3", Passed: false, Score: 0.2, Duration: 50 * time.Millisecond},
	}

	candResults := []TestResult{
		{Name: "test-1", Passed: true, Score: 1.0, Duration: 50 * time.Millisecond},
		{Name: "test-2", Passed: true, Score: 0.9, Duration: 50 * time.Millisecond},
		{Name: "test-3", Passed: true, Score: 0.8, Duration: 50 * time.Millisecond},
	}

	baseline := makeComparisonReport(baseResults)
	candidate := makeComparisonReport(candResults)

	cmp := Compare(baseline, candidate)

	assert.Equal(t, "candidate", cmp.Winner)
	assert.Greater(t, cmp.ScoreDelta, 0.05)
	assert.Greater(t, cmp.PassRateDelta, 0.0)
	assert.Contains(t, cmp.Summary, "Candidate wins")

	// Confidence interval should indicate significance.
	assert.True(t, cmp.CI.Significant)
}

func TestCompare_BaselineWins(t *testing.T) {
	baseResults := []TestResult{
		{Name: "test-1", Passed: true, Score: 1.0, Duration: 50 * time.Millisecond},
		{Name: "test-2", Passed: true, Score: 0.9, Duration: 50 * time.Millisecond},
	}

	candResults := []TestResult{
		{Name: "test-1", Passed: false, Score: 0.3, Duration: 50 * time.Millisecond},
		{Name: "test-2", Passed: false, Score: 0.2, Duration: 50 * time.Millisecond},
	}

	baseline := makeComparisonReport(baseResults)
	candidate := makeComparisonReport(candResults)

	cmp := Compare(baseline, candidate)

	assert.Equal(t, "baseline", cmp.Winner)
	assert.Less(t, cmp.ScoreDelta, -0.05)
	assert.Contains(t, cmp.Summary, "Baseline wins")
}

func TestCompare_PerTestMatchingByName(t *testing.T) {
	baseResults := []TestResult{
		{Name: "alpha", Passed: true, Score: 0.8, Duration: 10 * time.Millisecond},
		{Name: "beta", Passed: true, Score: 0.6, Duration: 10 * time.Millisecond},
		{Name: "gamma", Passed: false, Score: 0.2, Duration: 10 * time.Millisecond},
	}

	candResults := []TestResult{
		{Name: "beta", Passed: true, Score: 0.9, Duration: 10 * time.Millisecond},
		{Name: "gamma", Passed: true, Score: 0.7, Duration: 10 * time.Millisecond},
		{Name: "delta", Passed: true, Score: 1.0, Duration: 10 * time.Millisecond},
	}

	baseline := makeComparisonReport(baseResults)
	candidate := makeComparisonReport(candResults)

	cmp := Compare(baseline, candidate)

	// Should have 4 per-test comparisons: beta, gamma, delta (from candidate), alpha (baseline-only).
	require.Len(t, cmp.PerTest, 4)

	byName := make(map[string]TestComparison, len(cmp.PerTest))
	for _, pt := range cmp.PerTest {
		byName[pt.TestName] = pt
	}

	// beta: matched, candidate is better.
	beta := byName["beta"]
	assert.InDelta(t, 0.6, beta.BaselineScore, 0.001)
	assert.InDelta(t, 0.9, beta.CandidateScore, 0.001)
	assert.InDelta(t, 0.3, beta.Delta, 0.001)

	// gamma: matched, candidate is better.
	gamma := byName["gamma"]
	assert.InDelta(t, 0.2, gamma.BaselineScore, 0.001)
	assert.InDelta(t, 0.7, gamma.CandidateScore, 0.001)
	assert.InDelta(t, 0.5, gamma.Delta, 0.001)

	// delta: candidate-only.
	delta := byName["delta"]
	assert.InDelta(t, 0.0, delta.BaselineScore, 0.001)
	assert.InDelta(t, 1.0, delta.CandidateScore, 0.001)
	assert.InDelta(t, 1.0, delta.Delta, 0.001)

	// alpha: baseline-only.
	alpha := byName["alpha"]
	assert.InDelta(t, 0.8, alpha.BaselineScore, 0.001)
	assert.InDelta(t, 0.0, alpha.CandidateScore, 0.001)
	assert.InDelta(t, -0.8, alpha.Delta, 0.001)
}

func TestCompare_TieWithinThreshold(t *testing.T) {
	baseResults := []TestResult{
		{Name: "test-1", Passed: true, Score: 0.80, Duration: 50 * time.Millisecond},
		{Name: "test-2", Passed: true, Score: 0.80, Duration: 50 * time.Millisecond},
	}

	// Scores differ by 0.02 on average, which is within the 0.05 tie threshold.
	candResults := []TestResult{
		{Name: "test-1", Passed: true, Score: 0.82, Duration: 50 * time.Millisecond},
		{Name: "test-2", Passed: true, Score: 0.82, Duration: 50 * time.Millisecond},
	}

	baseline := makeComparisonReport(baseResults)
	candidate := makeComparisonReport(candResults)

	cmp := Compare(baseline, candidate)

	assert.Equal(t, "tie", cmp.Winner)
	assert.InDelta(t, 0.02, cmp.ScoreDelta, 0.001)
}

func TestCompare_CostDelta(t *testing.T) {
	baseResults := []TestResult{
		{Name: "test-1", Passed: true, Score: 0.8, Duration: 50 * time.Millisecond, CostUSD: 0.01},
		{Name: "test-2", Passed: true, Score: 0.8, Duration: 50 * time.Millisecond, CostUSD: 0.02},
	}

	candResults := []TestResult{
		{Name: "test-1", Passed: true, Score: 0.8, Duration: 50 * time.Millisecond, CostUSD: 0.05},
		{Name: "test-2", Passed: true, Score: 0.8, Duration: 50 * time.Millisecond, CostUSD: 0.06},
	}

	baseline := makeComparisonReport(baseResults)
	candidate := makeComparisonReport(candResults)

	cmp := Compare(baseline, candidate)

	// Candidate cost 0.11, baseline cost 0.03, delta = 0.08.
	assert.InDelta(t, 0.08, cmp.CostDelta, 0.001)
}

func TestCompare_SpeedDelta(t *testing.T) {
	baseResults := []TestResult{
		{Name: "test-1", Passed: true, Score: 0.8, Duration: 50 * time.Millisecond},
	}

	candResults := []TestResult{
		{Name: "test-1", Passed: true, Score: 0.8, Duration: 50 * time.Millisecond},
	}

	baseline := makeComparisonReport(baseResults)
	baseline.Duration = 200 * time.Millisecond

	candidate := makeComparisonReport(candResults)
	candidate.Duration = 300 * time.Millisecond

	cmp := Compare(baseline, candidate)

	assert.InDelta(t, 100.0, cmp.SpeedDelta, 0.001)
}

func TestCompare_EmptyReports(t *testing.T) {
	baseline := makeComparisonReport(nil)
	candidate := makeComparisonReport(nil)

	cmp := Compare(baseline, candidate)

	assert.Equal(t, "tie", cmp.Winner)
	assert.InDelta(t, 0.0, cmp.ScoreDelta, 0.001)
	assert.Empty(t, cmp.PerTest)
}
