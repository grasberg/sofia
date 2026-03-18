package reputation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/memory"
)

func newTestDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck
	return db
}

func TestRecordOutcome(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	id, err := mgr.RecordOutcome(TaskOutcome{
		AgentID:   "coder",
		Task:      "Fix the login bug",
		Success:   true,
		LatencyMs: 1500,
		TokensIn:  100,
		TokensOut: 200,
	})
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))
}

func TestRecordOutcomeAutoCategory(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	_, err := mgr.RecordOutcome(TaskOutcome{
		AgentID: "coder",
		Task:    "Implement a new function to parse JSON",
		Success: true,
	})
	require.NoError(t, err)

	cats, err := mgr.GetCategoryStats("coder")
	require.NoError(t, err)
	require.Len(t, cats, 1)
	assert.Equal(t, "coding", cats[0].Category)
}

func TestRecordOutcomeExplicitCategory(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	_, err := mgr.RecordOutcome(TaskOutcome{
		AgentID:  "writer",
		Category: "marketing",
		Task:     "Write copy",
		Success:  true,
	})
	require.NoError(t, err)

	cats, err := mgr.GetCategoryStats("writer")
	require.NoError(t, err)
	require.Len(t, cats, 1)
	assert.Equal(t, "marketing", cats[0].Category)
}

func TestScoreOutcome(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	id, err := mgr.RecordOutcome(TaskOutcome{
		AgentID: "coder",
		Task:    "Write tests",
		Success: true,
	})
	require.NoError(t, err)

	err = mgr.ScoreOutcome(id, 0.9)
	require.NoError(t, err)
}

func TestScoreOutcomeOutOfRange(t *testing.T) {
	mgr := NewManager(newTestDB(t))
	assert.Error(t, mgr.ScoreOutcome(1, -0.1))
	assert.Error(t, mgr.ScoreOutcome(1, 1.1))
}

func TestGetAgentStats(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	for i := 0; i < 5; i++ {
		success := i < 4
		id, _ := mgr.RecordOutcome(TaskOutcome{
			AgentID:   "coder",
			Task:      "task",
			Success:   success,
			LatencyMs: int64(100 * (i + 1)),
			TokensOut: 50,
		})
		if i < 3 {
			_ = mgr.ScoreOutcome(id, 0.8)
		}
	}

	stats, err := mgr.GetAgentStats("coder")
	require.NoError(t, err)
	assert.Equal(t, 5, stats.TotalTasks)
	assert.Equal(t, 4, stats.Successes)
	assert.Equal(t, 1, stats.Failures)
	assert.InDelta(t, 0.8, stats.SuccessRate, 0.01)
	assert.Equal(t, 3, stats.ScoredCount)
	assert.InDelta(t, 0.8, stats.AvgScore, 0.01)
	assert.Greater(t, stats.AvgLatencyMs, 0.0)
}

func TestGetAllAgentStats(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	for _, agent := range []string{"coder", "writer"} {
		_, _ = mgr.RecordOutcome(TaskOutcome{
			AgentID: agent,
			Task:    "task",
			Success: true,
		})
	}

	stats, err := mgr.GetAllAgentStats()
	require.NoError(t, err)
	assert.Len(t, stats, 2)
}

func TestGetCategoryStats(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	_, _ = mgr.RecordOutcome(TaskOutcome{
		AgentID:  "coder",
		Category: "coding",
		Task:     "fix bug",
		Success:  true,
	})
	_, _ = mgr.RecordOutcome(TaskOutcome{
		AgentID:  "coder",
		Category: "coding",
		Task:     "add feature",
		Success:  true,
	})
	_, _ = mgr.RecordOutcome(TaskOutcome{
		AgentID:  "coder",
		Category: "devops",
		Task:     "deploy",
		Success:  false,
	})

	cats, err := mgr.GetCategoryStats("coder")
	require.NoError(t, err)
	assert.Len(t, cats, 2)

	// Coding should be first (more tasks).
	assert.Equal(t, "coding", cats[0].Category)
	assert.Equal(t, 2, cats[0].TotalTasks)
	assert.InDelta(t, 1.0, cats[0].SuccessRate, 0.01)
}

func TestGetRecentOutcomes(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	for i := 0; i < 5; i++ {
		_, _ = mgr.RecordOutcome(TaskOutcome{
			AgentID: "coder",
			Task:    "task",
			Success: true,
		})
	}

	outcomes, err := mgr.GetRecentOutcomes("coder", 3)
	require.NoError(t, err)
	assert.Len(t, outcomes, 3)
}

func TestReputationScoreNoData(t *testing.T) {
	mgr := NewManager(newTestDB(t))
	score := mgr.ReputationScore("nonexistent", "coding")
	assert.InDelta(t, 0.5, score, 0.01)
}

func TestReputationScoreWithData(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	// Record 10 successful coding tasks.
	for i := 0; i < 10; i++ {
		_, _ = mgr.RecordOutcome(TaskOutcome{
			AgentID:  "coder",
			Category: "coding",
			Task:     "code task",
			Success:  true,
		})
	}

	score := mgr.ReputationScore("coder", "coding")
	// With all successes and enough data, should be > 0.7.
	assert.Greater(t, score, 0.7)
}

func TestReputationScoreLowPerformance(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	// Record 10 failed tasks.
	for i := 0; i < 10; i++ {
		_, _ = mgr.RecordOutcome(TaskOutcome{
			AgentID:  "bad_agent",
			Category: "coding",
			Task:     "code task",
			Success:  false,
		})
	}

	score := mgr.ReputationScore("bad_agent", "coding")
	assert.Less(t, score, 0.3)
}

func TestReputationScoreFallbackToOverall(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	// Record outcomes only for "writing" category.
	for i := 0; i < 10; i++ {
		_, _ = mgr.RecordOutcome(TaskOutcome{
			AgentID:  "writer",
			Category: "writing",
			Task:     "write essay",
			Success:  true,
		})
	}

	// Query for "coding" — should fall back to overall stats.
	score := mgr.ReputationScore("writer", "coding")
	assert.Greater(t, score, 0.5)
}

func TestBestAgentForCategory(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	// Coder: great at coding.
	for i := 0; i < 10; i++ {
		_, _ = mgr.RecordOutcome(TaskOutcome{
			AgentID:  "coder",
			Category: "coding",
			Task:     "code",
			Success:  true,
		})
	}
	// Writer: bad at coding.
	for i := 0; i < 10; i++ {
		_, _ = mgr.RecordOutcome(TaskOutcome{
			AgentID:  "writer",
			Category: "coding",
			Task:     "code",
			Success:  false,
		})
	}

	best, score := mgr.BestAgentForCategory(
		[]string{"coder", "writer"}, "coding",
	)
	assert.Equal(t, "coder", best)
	assert.Greater(t, score, 0.5)
}

func TestClassifyTask(t *testing.T) {
	tests := []struct {
		task     string
		expected string
	}{
		{"Fix the bug in the login function", "coding"},
		{"Write a blog post about AI", "writing"},
		{"Research the latest papers on LLMs", "research"},
		{"Parse the CSV data file", "data"},
		{"Deploy the Docker container", "devops"},
		{"Calculate the probability", "math"},
		{"Create a poem about nature", "creative"},
		{"Do something unrelated", "general"},
	}

	for _, tc := range tests {
		t.Run(tc.task, func(t *testing.T) {
			assert.Equal(t, tc.expected, classifyTask(tc.task))
		})
	}
}

func TestComputeReputationNeutral(t *testing.T) {
	r := computeReputation(0, 0, 0, nil)
	assert.InDelta(t, 0.5, r, 0.01)
}

func TestComputeReputationHighConfidence(t *testing.T) {
	score := 0.9
	r := computeReputation(20, 18, 10, &score)
	assert.Greater(t, r, 0.7)
}

func TestComputeReputationLowConfidence(t *testing.T) {
	// With only 1 task, confidence is low → score stays near 0.5.
	r := computeReputation(1, 1, 0, nil)
	assert.InDelta(t, 0.5, r, 0.15)
}

func TestGetAgentStatsSince(t *testing.T) {
	db := newTestDB(t)
	mgr := NewManager(db)

	// Insert an "old" outcome by writing directly with an old timestamp.
	_, err := db.Exec(
		`INSERT INTO agent_reputation
		 (agent_id, category, task, success, score, latency_ms, tokens_in, tokens_out, error, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"coder", "coding", "old task", 1, nil, 100, 10, 20, "",
		time.Now().Add(-48*time.Hour),
	)
	require.NoError(t, err)

	// Insert a "recent" outcome via RecordOutcome (uses current time).
	_, err = mgr.RecordOutcome(TaskOutcome{
		AgentID:   "coder",
		Category:  "coding",
		Task:      "recent task",
		Success:   true,
		LatencyMs: 200,
		TokensIn:  50,
		TokensOut: 80,
	})
	require.NoError(t, err)

	// Query since 24 hours ago — should only see the recent outcome.
	since := time.Now().Add(-24 * time.Hour)
	stats, err := mgr.GetAgentStatsSince("coder", since)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.TotalTasks)
	assert.Equal(t, 1, stats.Successes)
	assert.Equal(t, 0, stats.Failures)

	// Query since 72 hours ago — should see both outcomes.
	sinceOld := time.Now().Add(-72 * time.Hour)
	statsAll, err := mgr.GetAgentStatsSince("coder", sinceOld)
	require.NoError(t, err)
	assert.Equal(t, 2, statsAll.TotalTasks)
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 10))
	assert.Equal(t, "hel", truncate("hello", 3))
}
