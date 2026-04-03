package eval

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *EvalStore {
	t.Helper()

	store, err := OpenEvalStore(":memory:")
	require.NoError(t, err)

	t.Cleanup(func() { _ = store.Close() })

	return store
}

func makeReport(passed, failed int, avgScore float64) EvalReport {
	var results []TestResult

	for i := range passed {
		results = append(results, TestResult{
			Name:     "pass-" + string(rune('a'+i)),
			Passed:   true,
			Score:    1.0,
			Input:    "input",
			Output:   "output",
			Duration: 10 * time.Millisecond,
		})
	}

	for i := range failed {
		results = append(results, TestResult{
			Name:     "fail-" + string(rune('a'+i)),
			Passed:   false,
			Score:    0.2,
			Input:    "input",
			Output:   "bad output",
			Errors:   []string{"did not match"},
			Duration: 15 * time.Millisecond,
		})
	}

	return EvalReport{
		TotalTests: passed + failed,
		Passed:     passed,
		Failed:     failed,
		AvgScore:   avgScore,
		Duration:   100 * time.Millisecond,
		Results:    results,
		RunAt:      time.Now().UTC(),
	}
}

func TestEvalStore_SaveAndRetrieve(t *testing.T) {
	store := newTestStore(t)
	report := makeReport(3, 1, 0.8)

	runID, err := store.SaveRun("basic-suite", "agent-1", "gpt-4o", report)
	require.NoError(t, err)
	assert.Greater(t, runID, int64(0))

	history, err := store.GetRunHistory("basic-suite", 10)
	require.NoError(t, err)
	require.Len(t, history, 1)

	h := history[0]
	assert.Equal(t, runID, h.ID)
	assert.Equal(t, "basic-suite", h.SuiteName)
	assert.Equal(t, "agent-1", h.AgentID)
	assert.Equal(t, "gpt-4o", h.Model)
	assert.InDelta(t, 0.8, h.AvgScore, 0.001)
	assert.InDelta(t, 0.75, h.PassRate, 0.001)
	assert.Equal(t, 4, h.TotalTests)
	assert.Equal(t, 3, h.Passed)
	assert.Equal(t, 1, h.Failed)
	assert.Equal(t, int64(100), h.DurationMs)
}

func TestEvalStore_HistoryLimit(t *testing.T) {
	store := newTestStore(t)

	for range 5 {
		_, err := store.SaveRun("suite-a", "", "", makeReport(1, 0, 1.0))
		require.NoError(t, err)
	}

	// Limit to 3.
	history, err := store.GetRunHistory("suite-a", 3)
	require.NoError(t, err)
	assert.Len(t, history, 3)

	// No limit.
	all, err := store.GetRunHistory("suite-a", 0)
	require.NoError(t, err)
	assert.Len(t, all, 5)
}

func TestEvalStore_HistoryIsolation(t *testing.T) {
	store := newTestStore(t)

	_, err := store.SaveRun("suite-a", "", "", makeReport(1, 0, 1.0))
	require.NoError(t, err)

	_, err = store.SaveRun("suite-b", "", "", makeReport(2, 1, 0.7))
	require.NoError(t, err)

	histA, err := store.GetRunHistory("suite-a", 10)
	require.NoError(t, err)
	assert.Len(t, histA, 1)

	histB, err := store.GetRunHistory("suite-b", 10)
	require.NoError(t, err)
	assert.Len(t, histB, 1)
}

func TestEvalStore_TrendImproving(t *testing.T) {
	store := newTestStore(t)

	_, _ = store.SaveRun("s", "", "", makeReport(1, 2, 0.40))
	_, _ = store.SaveRun("s", "", "", makeReport(2, 1, 0.65))
	_, _ = store.SaveRun("s", "", "", makeReport(3, 0, 0.90))

	trend, err := store.GetTrend("s")
	require.NoError(t, err)
	assert.Equal(t, "improving", trend)
}

func TestEvalStore_TrendDeclining(t *testing.T) {
	store := newTestStore(t)

	_, _ = store.SaveRun("s", "", "", makeReport(3, 0, 0.95))
	_, _ = store.SaveRun("s", "", "", makeReport(2, 1, 0.70))
	_, _ = store.SaveRun("s", "", "", makeReport(1, 2, 0.40))

	trend, err := store.GetTrend("s")
	require.NoError(t, err)
	assert.Equal(t, "declining", trend)
}

func TestEvalStore_TrendStable(t *testing.T) {
	store := newTestStore(t)

	_, _ = store.SaveRun("s", "", "", makeReport(2, 1, 0.80))
	_, _ = store.SaveRun("s", "", "", makeReport(2, 1, 0.805))
	_, _ = store.SaveRun("s", "", "", makeReport(2, 1, 0.80))

	trend, err := store.GetTrend("s")
	require.NoError(t, err)
	assert.Equal(t, "stable", trend)
}

func TestEvalStore_TrendInsufficientData(t *testing.T) {
	store := newTestStore(t)

	// Zero runs.
	trend, err := store.GetTrend("empty")
	require.NoError(t, err)
	assert.Equal(t, "insufficient_data", trend)

	// One run.
	_, _ = store.SaveRun("one", "", "", makeReport(1, 0, 1.0))

	trend, err = store.GetTrend("one")
	require.NoError(t, err)
	assert.Equal(t, "insufficient_data", trend)
}

func TestEvalStore_SaveRunWithErrors(t *testing.T) {
	store := newTestStore(t)

	report := EvalReport{
		TotalTests: 1,
		Passed:     0,
		Failed:     1,
		AvgScore:   0.3,
		Duration:   50 * time.Millisecond,
		Results: []TestResult{
			{
				Name:     "error-test",
				Passed:   false,
				Score:    0.3,
				Input:    "test input",
				Output:   "bad output",
				Errors:   []string{"expected foo", "got bar"},
				Duration: 50 * time.Millisecond,
			},
		},
		RunAt: time.Now().UTC(),
	}

	runID, err := store.SaveRun("error-suite", "agent-1", "model-x", report)
	require.NoError(t, err)
	assert.Greater(t, runID, int64(0))
}

func TestEvalStore_EmptyReport(t *testing.T) {
	store := newTestStore(t)

	report := EvalReport{
		TotalTests: 0,
		RunAt:      time.Now().UTC(),
	}

	runID, err := store.SaveRun("empty-suite", "", "", report)
	require.NoError(t, err)
	assert.Greater(t, runID, int64(0))

	history, err := store.GetRunHistory("empty-suite", 10)
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, 0, history[0].TotalTests)
}
