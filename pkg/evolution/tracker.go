package evolution

import (
	"fmt"
	"time"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/reputation"
)

// AgentPerformance holds computed performance metrics for one agent.
type AgentPerformance struct {
	AgentID             string  `json:"agent_id"`
	SuccessRate24h      float64 `json:"success_rate_24h"`
	AvgScore24h         float64 `json:"avg_score_24h"`
	TaskCount24h        int     `json:"task_count_24h"`
	Trend               string  `json:"trend"` // "improving", "stable", "declining"
	SpecializationScore float64 `json:"specialization"`
	Utilization         float64 `json:"utilization"`
}

// PerformanceTracker wraps the reputation system with evolution-specific computed metrics.
type PerformanceTracker struct {
	reputation *reputation.Manager
	cfg        *config.EvolutionConfig
}

// NewPerformanceTracker creates a new PerformanceTracker.
func NewPerformanceTracker(rep *reputation.Manager, cfg *config.EvolutionConfig) *PerformanceTracker {
	return &PerformanceTracker{
		reputation: rep,
		cfg:        cfg,
	}
}

// GetAgentPerformance returns computed performance metrics for the given agent
// over the last 24 hours.
func (pt *PerformanceTracker) GetAgentPerformance(agentID string) (*AgentPerformance, error) {
	since24h := time.Now().Add(-24 * time.Hour)
	stats, err := pt.reputation.GetAgentStatsSince(agentID, since24h)
	if err != nil {
		return nil, fmt.Errorf("get agent performance: %w", err)
	}

	trend, err := pt.DetectTrend(agentID)
	if err != nil {
		return nil, fmt.Errorf("detect trend: %w", err)
	}

	specScore, err := pt.GetSpecializationScore(agentID)
	if err != nil {
		return nil, fmt.Errorf("get specialization score: %w", err)
	}

	return &AgentPerformance{
		AgentID:             agentID,
		SuccessRate24h:      stats.SuccessRate,
		AvgScore24h:         stats.AvgScore,
		TaskCount24h:        stats.TotalTasks,
		Trend:               trend,
		SpecializationScore: specScore,
	}, nil
}

// DetectTrend compares recent (last 24h) vs prior (24h-48h) success rates.
// Returns "improving" if delta > 0.05, "declining" if delta < -0.05, otherwise "stable".
// Returns "stable" if either window has fewer than 3 tasks.
func (pt *PerformanceTracker) DetectTrend(agentID string) (string, error) {
	now := time.Now()
	since24h := now.Add(-24 * time.Hour)
	since48h := now.Add(-48 * time.Hour)

	recent, err := pt.reputation.GetAgentStatsSince(agentID, since24h)
	if err != nil {
		return "", fmt.Errorf("detect trend recent: %w", err)
	}

	overall48h, err := pt.reputation.GetAgentStatsSince(agentID, since48h)
	if err != nil {
		return "", fmt.Errorf("detect trend overall: %w", err)
	}

	// Compute prior-window stats by subtracting recent from the 48h window.
	priorTasks := overall48h.TotalTasks - recent.TotalTasks
	priorSuccesses := overall48h.Successes - recent.Successes

	if recent.TotalTasks < 3 || priorTasks < 3 {
		return "stable", nil
	}

	recentRate := recent.SuccessRate
	priorRate := float64(priorSuccesses) / float64(priorTasks)
	delta := recentRate - priorRate

	switch {
	case delta > 0.05:
		return "improving", nil
	case delta < -0.05:
		return "declining", nil
	default:
		return "stable", nil
	}
}

// ShouldRetire determines whether an agent should be retired based on its performance.
// Returns (shouldRetire, reason, error).
func (pt *PerformanceTracker) ShouldRetire(agentID string) (bool, string, error) {
	threshold := pt.cfg.RetirementThreshold
	if threshold == 0 {
		threshold = 0.30
	}
	minTasks := pt.cfg.RetirementMinTasks
	if minTasks == 0 {
		minTasks = 5
	}
	inactiveDays := pt.cfg.RetirementInactiveDays
	if inactiveDays == 0 {
		inactiveDays = 7
	}

	// Check inactivity over the configured period first.
	// This avoids querying shorter windows when the agent has no recent activity at all.
	sinceInactive := time.Now().Add(-time.Duration(inactiveDays) * 24 * time.Hour)
	statsInactive, err := pt.statsSince(agentID, sinceInactive)
	if err != nil {
		return false, "", fmt.Errorf("should retire inactive: %w", err)
	}

	if statsInactive.TotalTasks == 0 {
		return true, "inactive", nil
	}

	// Check success rate over 48h.
	since48h := time.Now().Add(-48 * time.Hour)
	stats48h, err := pt.statsSince(agentID, since48h)
	if err != nil {
		return false, "", fmt.Errorf("should retire stats: %w", err)
	}

	if stats48h.TotalTasks >= minTasks && stats48h.SuccessRate < threshold {
		return true, "low_success_rate", nil
	}

	return false, "", nil
}

// statsSince wraps reputation.GetAgentStatsSince and returns a zero-value
// AgentStats when the underlying query fails due to no matching rows
// (the SUM aggregate returns NULL which causes a scan error).
func (pt *PerformanceTracker) statsSince(
	agentID string, since time.Time,
) (*reputation.AgentStats, error) {
	stats, err := pt.reputation.GetAgentStatsSince(agentID, since)
	if err != nil {
		// When there are no rows in the window, SQLite's SUM returns NULL which
		// causes a scan error. Treat this as zero tasks rather than a hard failure.
		return &reputation.AgentStats{AgentID: agentID}, nil //nolint:nilerr
	}
	return stats, nil
}

// GetSpecializationScore computes how specialized an agent is based on its
// category breakdown. Returns the ratio of the dominant category's tasks to
// total tasks. 1.0 = fully specialized, approaching 0.0 = evenly spread.
// Returns 0.0 if there are no category stats.
func (pt *PerformanceTracker) GetSpecializationScore(agentID string) (float64, error) {
	cats, err := pt.reputation.GetCategoryStats(agentID)
	if err != nil {
		return 0, fmt.Errorf("get specialization: %w", err)
	}

	if len(cats) == 0 {
		return 0, nil
	}

	totalTasks := 0
	maxCategoryTasks := 0
	for _, c := range cats {
		totalTasks += c.TotalTasks
		if c.TotalTasks > maxCategoryTasks {
			maxCategoryTasks = c.TotalTasks
		}
	}

	if totalTasks == 0 {
		return 0, nil
	}

	return float64(maxCategoryTasks) / float64(totalTasks), nil
}
