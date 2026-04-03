package eval

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvalScheduler_RunScheduledEval(t *testing.T) {
	store := newTestStore(t)

	// Create a temporary suite file.
	dir := t.TempDir()
	suitePath := filepath.Join(dir, "test-suite.json")
	suiteJSON := `{
		"name": "scheduler-test",
		"cases": [
			{"name": "t1", "input": "hello", "expected_output": "world"},
			{"name": "t2", "input": "foo", "expect_contains": ["bar"]}
		]
	}`
	require.NoError(t, os.WriteFile(suitePath, []byte(suiteJSON), 0o644))

	scheduler := NewEvalScheduler(store, dir, nil)

	sched := ScheduledEval{
		SuitePath:          suitePath,
		CronExpr:           "0 6 * * *",
		AgentID:            "test-agent",
		NotifyOnRegression: true,
	}

	report, err := scheduler.RunScheduledEval(context.Background(), sched)
	require.NoError(t, err)
	assert.Equal(t, 2, report.TotalTests)

	// Verify the run was persisted.
	history, err := store.GetRunHistory("scheduler-test", 10)
	require.NoError(t, err)
	assert.Len(t, history, 1)
	assert.Equal(t, "test-agent", history[0].AgentID)
}

func TestEvalScheduler_RegressionDetection(t *testing.T) {
	store := newTestStore(t)

	// Simulate a good previous run.
	goodReport := makeReport(5, 0, 0.95)
	_, err := store.SaveRun("regress-suite", "agent-1", "", goodReport)
	require.NoError(t, err)

	// Create a suite that will produce a lower score (empty outputs).
	dir := t.TempDir()
	suitePath := filepath.Join(dir, "regress.json")
	suiteJSON := `{
		"name": "regress-suite",
		"cases": [
			{"name": "t1", "input": "hello", "expected_output": "must-exist"},
			{"name": "t2", "input": "world", "expected_output": "must-exist"}
		]
	}`
	require.NoError(t, os.WriteFile(suitePath, []byte(suiteJSON), 0o644))

	scheduler := NewEvalScheduler(store, dir, nil)

	sched := ScheduledEval{
		SuitePath:          suitePath,
		CronExpr:           "0 6 * * *",
		AgentID:            "agent-1",
		NotifyOnRegression: true,
	}

	report, err := scheduler.RunScheduledEval(context.Background(), sched)
	require.NoError(t, err)
	// The new run should have lower scores since empty output won't match.
	assert.Less(t, report.AvgScore, 0.95)

	// Verify 2 runs are in the store now.
	history, err := store.GetRunHistory("regress-suite", 10)
	require.NoError(t, err)
	assert.Len(t, history, 2)
}

func TestEvalScheduler_InvalidSuite(t *testing.T) {
	store := newTestStore(t)
	scheduler := NewEvalScheduler(store, t.TempDir(), nil)

	sched := ScheduledEval{
		SuitePath: "/nonexistent/suite.json",
		AgentID:   "agent-1",
	}

	_, err := scheduler.RunScheduledEval(context.Background(), sched)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "load suite")
}
