package evolution

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/reputation"
)

func newTrackerTestDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func defaultEvoCfg() *config.EvolutionConfig {
	return &config.EvolutionConfig{
		RetirementThreshold:    0.30,
		RetirementMinTasks:     5,
		RetirementInactiveDays: 7,
	}
}

// insertOutcome inserts a reputation outcome at a specific time offset from now.
func insertOutcome(
	t *testing.T, db *memory.MemoryDB,
	agentID, category string, success bool, score *float64, hoursAgo float64,
) {
	t.Helper()
	successInt := 0
	if success {
		successInt = 1
	}
	ts := time.Now().Add(-time.Duration(hoursAgo * float64(time.Hour)))
	_, err := db.Exec(
		`INSERT INTO agent_reputation
		 (agent_id, category, task, success, score, latency_ms, tokens_in, tokens_out, error, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		agentID, category, "test task", successInt, score, 100, 10, 20, "", ts,
	)
	require.NoError(t, err)
}

func TestPerformanceTracker_GetAgentPerformance(t *testing.T) {
	db := newTrackerTestDB(t)
	rep := reputation.NewManager(db)
	pt := NewPerformanceTracker(rep, defaultEvoCfg())

	// Insert 4 successes and 1 failure in the last 24h.
	score := 0.8
	for i := 0; i < 4; i++ {
		insertOutcome(t, db, "agent1", "coding", true, &score, float64(i+1))
	}
	insertOutcome(t, db, "agent1", "coding", false, nil, 2)

	// Insert old outcomes (outside 24h) that should not be counted.
	insertOutcome(t, db, "agent1", "coding", false, nil, 30)
	insertOutcome(t, db, "agent1", "coding", false, nil, 35)
	insertOutcome(t, db, "agent1", "coding", false, nil, 40)

	perf, err := pt.GetAgentPerformance("agent1")
	require.NoError(t, err)
	assert.Equal(t, "agent1", perf.AgentID)
	assert.Equal(t, 5, perf.TaskCount24h)
	assert.InDelta(t, 0.8, perf.SuccessRate24h, 0.01)
	assert.InDelta(t, 0.8, perf.AvgScore24h, 0.01)
	assert.NotEmpty(t, perf.Trend)
}

func TestPerformanceTracker_DetectTrend_Improving(t *testing.T) {
	db := newTrackerTestDB(t)
	rep := reputation.NewManager(db)
	pt := NewPerformanceTracker(rep, defaultEvoCfg())

	// Prior window (24h-48h ago): low success rate (1 out of 4).
	insertOutcome(t, db, "agent1", "coding", true, nil, 30)
	insertOutcome(t, db, "agent1", "coding", false, nil, 32)
	insertOutcome(t, db, "agent1", "coding", false, nil, 34)
	insertOutcome(t, db, "agent1", "coding", false, nil, 36)

	// Recent window (last 24h): high success rate (4 out of 4).
	insertOutcome(t, db, "agent1", "coding", true, nil, 1)
	insertOutcome(t, db, "agent1", "coding", true, nil, 2)
	insertOutcome(t, db, "agent1", "coding", true, nil, 3)
	insertOutcome(t, db, "agent1", "coding", true, nil, 4)

	trend, err := pt.DetectTrend("agent1")
	require.NoError(t, err)
	assert.Equal(t, "improving", trend)
}

func TestPerformanceTracker_DetectTrend_Declining(t *testing.T) {
	db := newTrackerTestDB(t)
	rep := reputation.NewManager(db)
	pt := NewPerformanceTracker(rep, defaultEvoCfg())

	// Prior window (24h-48h ago): high success rate (4 out of 4).
	insertOutcome(t, db, "agent1", "coding", true, nil, 30)
	insertOutcome(t, db, "agent1", "coding", true, nil, 32)
	insertOutcome(t, db, "agent1", "coding", true, nil, 34)
	insertOutcome(t, db, "agent1", "coding", true, nil, 36)

	// Recent window (last 24h): low success rate (1 out of 4).
	insertOutcome(t, db, "agent1", "coding", true, nil, 1)
	insertOutcome(t, db, "agent1", "coding", false, nil, 2)
	insertOutcome(t, db, "agent1", "coding", false, nil, 3)
	insertOutcome(t, db, "agent1", "coding", false, nil, 4)

	trend, err := pt.DetectTrend("agent1")
	require.NoError(t, err)
	assert.Equal(t, "declining", trend)
}

func TestPerformanceTracker_DetectTrend_Stable(t *testing.T) {
	db := newTrackerTestDB(t)
	rep := reputation.NewManager(db)
	pt := NewPerformanceTracker(rep, defaultEvoCfg())

	// Prior window (24h-48h ago): 3 out of 4 success.
	insertOutcome(t, db, "agent1", "coding", true, nil, 30)
	insertOutcome(t, db, "agent1", "coding", true, nil, 32)
	insertOutcome(t, db, "agent1", "coding", true, nil, 34)
	insertOutcome(t, db, "agent1", "coding", false, nil, 36)

	// Recent window (last 24h): 3 out of 4 success (same rate).
	insertOutcome(t, db, "agent1", "coding", true, nil, 1)
	insertOutcome(t, db, "agent1", "coding", true, nil, 2)
	insertOutcome(t, db, "agent1", "coding", true, nil, 3)
	insertOutcome(t, db, "agent1", "coding", false, nil, 4)

	trend, err := pt.DetectTrend("agent1")
	require.NoError(t, err)
	assert.Equal(t, "stable", trend)
}

func TestPerformanceTracker_DetectTrend_InsufficientData(t *testing.T) {
	db := newTrackerTestDB(t)
	rep := reputation.NewManager(db)
	pt := NewPerformanceTracker(rep, defaultEvoCfg())

	// Only 2 tasks in recent window (< 3 minimum).
	insertOutcome(t, db, "agent1", "coding", true, nil, 1)
	insertOutcome(t, db, "agent1", "coding", false, nil, 2)

	// 3 tasks in prior window.
	insertOutcome(t, db, "agent1", "coding", false, nil, 30)
	insertOutcome(t, db, "agent1", "coding", false, nil, 32)
	insertOutcome(t, db, "agent1", "coding", false, nil, 34)

	trend, err := pt.DetectTrend("agent1")
	require.NoError(t, err)
	assert.Equal(t, "stable", trend)
}

func TestPerformanceTracker_ShouldRetire_LowSuccess(t *testing.T) {
	db := newTrackerTestDB(t)
	rep := reputation.NewManager(db)
	pt := NewPerformanceTracker(rep, defaultEvoCfg())

	// Insert 6 tasks in last 48h, only 1 success (16.7% < 30% threshold).
	insertOutcome(t, db, "agent1", "coding", true, nil, 1)
	insertOutcome(t, db, "agent1", "coding", false, nil, 5)
	insertOutcome(t, db, "agent1", "coding", false, nil, 10)
	insertOutcome(t, db, "agent1", "coding", false, nil, 15)
	insertOutcome(t, db, "agent1", "coding", false, nil, 20)
	insertOutcome(t, db, "agent1", "coding", false, nil, 40)

	shouldRetire, reason, err := pt.ShouldRetire("agent1")
	require.NoError(t, err)
	assert.True(t, shouldRetire)
	assert.Equal(t, "low_success_rate", reason)
}

func TestPerformanceTracker_ShouldRetire_Inactive(t *testing.T) {
	db := newTrackerTestDB(t)
	rep := reputation.NewManager(db)
	pt := NewPerformanceTracker(rep, defaultEvoCfg())

	// Insert only old outcomes beyond the inactivity window (7 days).
	insertOutcome(t, db, "agent1", "coding", true, nil, 200) // ~8.3 days ago
	insertOutcome(t, db, "agent1", "coding", true, nil, 250) // ~10.4 days ago

	shouldRetire, reason, err := pt.ShouldRetire("agent1")
	require.NoError(t, err)
	assert.True(t, shouldRetire)
	assert.Equal(t, "inactive", reason)
}

func TestPerformanceTracker_ShouldRetire_Healthy(t *testing.T) {
	db := newTrackerTestDB(t)
	rep := reputation.NewManager(db)
	pt := NewPerformanceTracker(rep, defaultEvoCfg())

	// Insert 6 tasks in last 48h, 5 successes (83% > 30% threshold).
	insertOutcome(t, db, "agent1", "coding", true, nil, 1)
	insertOutcome(t, db, "agent1", "coding", true, nil, 5)
	insertOutcome(t, db, "agent1", "coding", true, nil, 10)
	insertOutcome(t, db, "agent1", "coding", true, nil, 15)
	insertOutcome(t, db, "agent1", "coding", true, nil, 20)
	insertOutcome(t, db, "agent1", "coding", false, nil, 40)

	shouldRetire, reason, err := pt.ShouldRetire("agent1")
	require.NoError(t, err)
	assert.False(t, shouldRetire)
	assert.Empty(t, reason)
}

func TestPerformanceTracker_GetSpecializationScore_FullySpecialized(t *testing.T) {
	db := newTrackerTestDB(t)
	rep := reputation.NewManager(db)
	pt := NewPerformanceTracker(rep, defaultEvoCfg())

	// All tasks in one category.
	for i := 0; i < 5; i++ {
		insertOutcome(t, db, "agent1", "coding", true, nil, float64(i+1))
	}

	score, err := pt.GetSpecializationScore("agent1")
	require.NoError(t, err)
	assert.InDelta(t, 1.0, score, 0.01)
}

func TestPerformanceTracker_GetSpecializationScore_Spread(t *testing.T) {
	db := newTrackerTestDB(t)
	rep := reputation.NewManager(db)
	pt := NewPerformanceTracker(rep, defaultEvoCfg())

	// Tasks spread across 4 categories equally.
	for _, cat := range []string{"coding", "writing", "research", "devops"} {
		insertOutcome(t, db, "agent1", cat, true, nil, 1)
	}

	score, err := pt.GetSpecializationScore("agent1")
	require.NoError(t, err)
	assert.InDelta(t, 0.25, score, 0.01)
}

func TestPerformanceTracker_GetSpecializationScore_NoData(t *testing.T) {
	db := newTrackerTestDB(t)
	rep := reputation.NewManager(db)
	pt := NewPerformanceTracker(rep, defaultEvoCfg())

	score, err := pt.GetSpecializationScore("nonexistent")
	require.NoError(t, err)
	assert.InDelta(t, 0.0, score, 0.01)
}
