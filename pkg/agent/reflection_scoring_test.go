package agent

import (
	"testing"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerformanceScore_Perfect(t *testing.T) {
	scorer := NewPerformanceScorer()
	// 0 errors, low tool count, has response
	score := scorer.Score(3, 0, true)
	assert.Greater(t, score, 0.7, "perfect execution should score high")
}

func TestPerformanceScore_HighErrors(t *testing.T) {
	scorer := NewPerformanceScorer()
	// Many errors relative to tool calls
	score := scorer.Score(5, 4, true)
	assert.Less(t, score, 0.7, "high error rate should score lower than perfect")
}

func TestPerformanceScore_EmptyResponse(t *testing.T) {
	scorer := NewPerformanceScorer()
	// No errors, but empty response
	scoreFull := scorer.Score(3, 0, true)
	scoreEmpty := scorer.Score(3, 0, false)
	assert.Greater(t, scoreFull, scoreEmpty, "empty response should reduce score")
}

func TestPerformanceScore_ZeroTools(t *testing.T) {
	scorer := NewPerformanceScorer()
	// No tools used (direct answer) — should still produce valid score
	score := scorer.Score(0, 0, true)
	assert.Greater(t, score, 0.5, "direct answer should score reasonably")
	assert.LessOrEqual(t, score, 1.0, "score should not exceed 1.0")
}

func TestPerformanceScore_ManyTools(t *testing.T) {
	scorer := NewPerformanceScorer()
	// Lots of tools but no errors
	score := scorer.Score(50, 0, true)
	// Higher tool count reduces efficiency component but error rate is perfect
	assert.Greater(t, score, 0.4, "many tools with no errors should still score ok")
}

func TestPerformanceScore_Clamped(t *testing.T) {
	scorer := NewPerformanceScorer()
	// Score should always be in [0, 1]
	score := scorer.Score(0, 0, true)
	assert.LessOrEqual(t, score, 1.0)
	assert.GreaterOrEqual(t, score, 0.0)

	score = scorer.Score(100, 100, false)
	assert.LessOrEqual(t, score, 1.0)
	assert.GreaterOrEqual(t, score, 0.0)
}

func TestGetPerformanceTrend_NilDB(t *testing.T) {
	result, err := GetPerformanceTrend(nil, "a1", 30)
	assert.NoError(t, err)
	assert.Equal(t, "stable", result.Direction)
}

func TestGetPerformanceTrend_NotEnoughData(t *testing.T) {
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Only 1 reflection — not enough for trend
	_ = db.SaveReflection(memory.ReflectionRecord{
		AgentID: "a1",
		Score:   0.8,
	})

	result, err := GetPerformanceTrend(db, "a1", 30)
	assert.NoError(t, err)
	assert.Equal(t, "stable", result.Direction, "not enough data should be stable")
}
