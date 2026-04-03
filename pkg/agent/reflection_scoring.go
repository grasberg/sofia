package agent

import (
	"github.com/grasberg/sofia/pkg/memory"
)

// PerformanceScorer calculates a deterministic performance score from task metrics.
type PerformanceScorer struct {
	// Weight factors for each component (must sum to 1.0)
	ErrorWeight      float64
	EfficiencyWeight float64
	CompletionWeight float64
	// ExpectedMaxTools is the expected maximum tool calls for a well-executed task.
	ExpectedMaxTools float64
}

// NewPerformanceScorer creates a scorer with default weights.
func NewPerformanceScorer() *PerformanceScorer {
	return &PerformanceScorer{
		ErrorWeight:      0.40,
		EfficiencyWeight: 0.30,
		CompletionWeight: 0.30,
		ExpectedMaxTools: 15.0,
	}
}

// MultiScore returns per-dimension scores for structured trace scoring.
func (ps *PerformanceScorer) MultiScore(toolCount, errorCount int, hasResponse bool) map[string]float64 {
	denominator := float64(toolCount)
	if denominator < 1 {
		denominator = 1
	}
	errorRate := 1.0 - float64(errorCount)/denominator
	if errorRate < 0 {
		errorRate = 0
	}
	efficiency := 1.0 / (1.0 + float64(toolCount)/ps.ExpectedMaxTools)
	completion := 0.5
	if hasResponse {
		completion = 1.0
	}
	return map[string]float64{
		"task_completion": completion,
		"efficiency":      efficiency,
		"error_rate":      errorRate,
	}
}

// Score calculates a 0.0-1.0 performance score from task metrics.
func (ps *PerformanceScorer) Score(toolCount, errorCount int, hasResponse bool) float64 {
	// Error rate: fewer errors = better
	denominator := float64(toolCount)
	if denominator < 1 {
		denominator = 1
	}
	errorRate := 1.0 - float64(errorCount)/denominator
	if errorRate < 0 {
		errorRate = 0
	}

	// Efficiency: fewer tools for same result = better
	efficiency := 1.0 / (1.0 + float64(toolCount)/ps.ExpectedMaxTools)

	// Completion: did we produce a response?
	completion := 0.5
	if hasResponse {
		completion = 1.0
	}

	score := errorRate*ps.ErrorWeight +
		efficiency*ps.EfficiencyWeight +
		completion*ps.CompletionWeight

	// Clamp to [0, 1]
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

// TrendResult describes the performance trajectory.
type TrendResult struct {
	Direction string  // "improving", "stable", "declining"
	RecentAvg float64 // Average score of recent period
	OlderAvg  float64 // Average score of older period
	Total     int     // Total reflections analyzed
}

// GetPerformanceTrend compares recent vs older reflection scores to detect a trend.
func GetPerformanceTrend(db *memory.MemoryDB, agentID string, days int) (TrendResult, error) {
	if db == nil {
		return TrendResult{Direction: "stable"}, nil
	}
	if days <= 0 {
		days = 30
	}

	// Get recent stats (last N/2 days) vs older stats (N/2 to N days)
	halfDays := days / 2
	if halfDays < 1 {
		halfDays = 1
	}

	recentStats, err := db.GetReflectionStats(agentID, halfDays)
	if err != nil {
		return TrendResult{Direction: "stable"}, err
	}

	olderStats, err := db.GetReflectionStats(agentID, days)
	if err != nil {
		return TrendResult{Direction: "stable"}, err
	}

	result := TrendResult{
		Direction: "stable",
		RecentAvg: recentStats.AvgScore,
		OlderAvg:  olderStats.AvgScore,
		Total:     olderStats.TotalReflections,
	}

	// Need at least a few reflections in each period to determine trend
	if recentStats.TotalReflections < 2 || olderStats.TotalReflections < 3 {
		return result, nil
	}

	diff := recentStats.AvgScore - olderStats.AvgScore
	if diff > 0.1 {
		result.Direction = "improving"
	} else if diff < -0.1 {
		result.Direction = "declining"
	}

	return result, nil
}
