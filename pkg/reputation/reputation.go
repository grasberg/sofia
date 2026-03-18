package reputation

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
)

// TaskOutcome records the result of a single agent task execution.
type TaskOutcome struct {
	ID        int64     `json:"id"`
	AgentID   string    `json:"agent_id"`
	Category  string    `json:"category"`
	Task      string    `json:"task"`
	Success   bool      `json:"success"`
	Score     *float64  `json:"score,omitempty"`
	LatencyMs int64     `json:"latency_ms"`
	TokensIn  int       `json:"tokens_in"`
	TokensOut int       `json:"tokens_out"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AgentStats holds aggregate reputation stats for one agent.
type AgentStats struct {
	AgentID      string  `json:"agent_id"`
	TotalTasks   int     `json:"total_tasks"`
	Successes    int     `json:"successes"`
	Failures     int     `json:"failures"`
	SuccessRate  float64 `json:"success_rate"`
	AvgScore     float64 `json:"avg_score"`
	ScoredCount  int     `json:"scored_count"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	AvgTokensOut float64 `json:"avg_tokens_out"`
}

// CategoryStats holds stats for one agent in one task category.
type CategoryStats struct {
	AgentID     string  `json:"agent_id"`
	Category    string  `json:"category"`
	TotalTasks  int     `json:"total_tasks"`
	SuccessRate float64 `json:"success_rate"`
	AvgScore    float64 `json:"avg_score"`
	ScoredCount int     `json:"scored_count"`
}

// Manager tracks agent reputation and task history.
type Manager struct {
	db *memory.MemoryDB
}

// NewManager creates a new reputation manager.
func NewManager(db *memory.MemoryDB) *Manager {
	return &Manager{db: db}
}

// RecordOutcome persists a task outcome for an agent.
func (m *Manager) RecordOutcome(o TaskOutcome) (int64, error) {
	category := o.Category
	if category == "" {
		category = classifyTask(o.Task)
	}

	successInt := 0
	if o.Success {
		successInt = 1
	}

	res, err := m.db.Exec(
		`INSERT INTO agent_reputation
		 (agent_id, category, task, success, score, latency_ms,
		  tokens_in, tokens_out, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.AgentID, category, truncate(o.Task, 500),
		successInt, o.Score, o.LatencyMs,
		o.TokensIn, o.TokensOut, o.Error,
	)
	if err != nil {
		return 0, fmt.Errorf("record outcome: %w", err)
	}
	id, _ := res.LastInsertId() //nolint:errcheck
	return id, nil
}

// ScoreOutcome assigns a quality score (0.0-1.0) to a past outcome.
func (m *Manager) ScoreOutcome(outcomeID int64, score float64) error {
	if score < 0 || score > 1 {
		return fmt.Errorf("score must be between 0.0 and 1.0")
	}
	_, err := m.db.Exec(
		`UPDATE agent_reputation SET score = ? WHERE id = ?`,
		score, outcomeID,
	)
	return err
}

// GetAgentStats returns aggregate stats for an agent.
func (m *Manager) GetAgentStats(agentID string) (*AgentStats, error) {
	row := m.db.QueryRow(`
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as successes,
			AVG(latency_ms) as avg_latency,
			AVG(tokens_out) as avg_tokens_out
		FROM agent_reputation
		WHERE agent_id = ?`, agentID)

	s := &AgentStats{AgentID: agentID}
	var avgLatency, avgTokens *float64
	if err := row.Scan(
		&s.TotalTasks, &s.Successes,
		&avgLatency, &avgTokens,
	); err != nil {
		return nil, err
	}

	s.Failures = s.TotalTasks - s.Successes
	if s.TotalTasks > 0 {
		s.SuccessRate = float64(s.Successes) / float64(s.TotalTasks)
	}
	if avgLatency != nil {
		s.AvgLatencyMs = *avgLatency
	}
	if avgTokens != nil {
		s.AvgTokensOut = *avgTokens
	}

	// Score stats (only scored outcomes).
	scoreRow := m.db.QueryRow(`
		SELECT COUNT(*), AVG(score)
		FROM agent_reputation
		WHERE agent_id = ? AND score IS NOT NULL`, agentID)

	var avgScore *float64
	if err := scoreRow.Scan(&s.ScoredCount, &avgScore); err == nil {
		if avgScore != nil {
			s.AvgScore = *avgScore
		}
	}

	return s, nil
}

// GetAgentStatsSince returns aggregate stats for an agent since the given time.
func (m *Manager) GetAgentStatsSince(agentID string, since time.Time) (*AgentStats, error) {
	row := m.db.QueryRow(`
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as successes,
			AVG(latency_ms) as avg_latency,
			AVG(tokens_out) as avg_tokens_out
		FROM agent_reputation
		WHERE agent_id = ? AND created_at >= ?`, agentID, since)

	s := &AgentStats{AgentID: agentID}
	var avgLatency, avgTokens *float64
	if err := row.Scan(
		&s.TotalTasks, &s.Successes,
		&avgLatency, &avgTokens,
	); err != nil {
		return nil, err
	}

	s.Failures = s.TotalTasks - s.Successes
	if s.TotalTasks > 0 {
		s.SuccessRate = float64(s.Successes) / float64(s.TotalTasks)
	}
	if avgLatency != nil {
		s.AvgLatencyMs = *avgLatency
	}
	if avgTokens != nil {
		s.AvgTokensOut = *avgTokens
	}

	// Score stats (only scored outcomes since the given time).
	scoreRow := m.db.QueryRow(`
		SELECT COUNT(*), AVG(score)
		FROM agent_reputation
		WHERE agent_id = ? AND score IS NOT NULL AND created_at >= ?`, agentID, since)

	var avgScore *float64
	if err := scoreRow.Scan(&s.ScoredCount, &avgScore); err == nil {
		if avgScore != nil {
			s.AvgScore = *avgScore
		}
	}

	return s, nil
}

// GetAllAgentStats returns stats for all agents that have outcomes.
func (m *Manager) GetAllAgentStats() ([]AgentStats, error) {
	rows, err := m.db.Query(`
		SELECT DISTINCT agent_id FROM agent_reputation
		ORDER BY agent_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	var agentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		agentIDs = append(agentIDs, id)
	}

	var stats []AgentStats
	for _, id := range agentIDs {
		s, err := m.GetAgentStats(id)
		if err != nil {
			continue
		}
		stats = append(stats, *s)
	}
	return stats, nil
}

// GetCategoryStats returns per-category stats for an agent.
func (m *Manager) GetCategoryStats(
	agentID string,
) ([]CategoryStats, error) {
	rows, err := m.db.Query(`
		SELECT
			category,
			COUNT(*) as total,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as successes,
			COUNT(score) as scored,
			AVG(CASE WHEN score IS NOT NULL THEN score END) as avg_score
		FROM agent_reputation
		WHERE agent_id = ?
		GROUP BY category
		ORDER BY total DESC`, agentID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	var result []CategoryStats
	for rows.Next() {
		cs := CategoryStats{AgentID: agentID}
		var successes int
		var avgScore *float64
		if err := rows.Scan(
			&cs.Category, &cs.TotalTasks, &successes,
			&cs.ScoredCount, &avgScore,
		); err != nil {
			continue
		}
		if cs.TotalTasks > 0 {
			cs.SuccessRate = float64(successes) /
				float64(cs.TotalTasks)
		}
		if avgScore != nil {
			cs.AvgScore = *avgScore
		}
		result = append(result, cs)
	}
	return result, rows.Err()
}

// GetRecentOutcomes returns the N most recent outcomes for an agent.
func (m *Manager) GetRecentOutcomes(
	agentID string, limit int,
) ([]TaskOutcome, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := m.db.Query(`
		SELECT id, agent_id, category, task, success, score,
		       latency_ms, tokens_in, tokens_out, error, created_at
		FROM agent_reputation
		WHERE agent_id = ?
		ORDER BY created_at DESC
		LIMIT ?`, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	var outcomes []TaskOutcome
	for rows.Next() {
		var o TaskOutcome
		var successInt int
		if err := rows.Scan(
			&o.ID, &o.AgentID, &o.Category, &o.Task,
			&successInt, &o.Score, &o.LatencyMs,
			&o.TokensIn, &o.TokensOut, &o.Error, &o.CreatedAt,
		); err != nil {
			continue
		}
		o.Success = successInt == 1
		outcomes = append(outcomes, o)
	}
	return outcomes, rows.Err()
}

// ReputationScore computes a reputation-based score for an agent on a
// given task category. Returns a value in [0, 1] suitable for blending
// with keyword-based scores. Returns 0.5 (neutral) when there is
// insufficient data.
func (m *Manager) ReputationScore(
	agentID, category string,
) float64 {
	// Try category-specific stats first.
	row := m.db.QueryRow(`
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as successes,
			AVG(CASE WHEN score IS NOT NULL THEN score END) as avg_score,
			COUNT(score) as scored
		FROM agent_reputation
		WHERE agent_id = ? AND category = ?`,
		agentID, category)

	var total, successes, scored int
	var avgScore *float64
	if err := row.Scan(
		&total, &successes, &avgScore, &scored,
	); err != nil || total == 0 {
		// Fall back to overall agent stats.
		return m.overallReputation(agentID)
	}

	return computeReputation(total, successes, scored, avgScore)
}

func (m *Manager) overallReputation(agentID string) float64 {
	row := m.db.QueryRow(`
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as successes,
			AVG(CASE WHEN score IS NOT NULL THEN score END) as avg_score,
			COUNT(score) as scored
		FROM agent_reputation
		WHERE agent_id = ?`, agentID)

	var total, successes, scored int
	var avgScore *float64
	if err := row.Scan(
		&total, &successes, &avgScore, &scored,
	); err != nil || total == 0 {
		return 0.5 // Neutral: no data.
	}

	return computeReputation(total, successes, scored, avgScore)
}

func computeReputation(
	total, successes, scored int, avgScore *float64,
) float64 {
	if total == 0 {
		return 0.5
	}

	successRate := float64(successes) / float64(total)

	// Blend success rate with quality score if available.
	quality := successRate
	if scored > 0 && avgScore != nil {
		quality = 0.5*successRate + 0.5*(*avgScore)
	}

	// Apply confidence factor: more data → more trust in the score.
	// Uses a sigmoid-like ramp: confidence approaches 1.0 as tasks
	// grow, but stays low with few tasks to avoid overreacting.
	confidence := 1.0 - math.Exp(-float64(total)/5.0)

	// Blend between neutral (0.5) and actual quality based on
	// confidence.
	return 0.5 + (quality-0.5)*confidence
}

// BestAgentForCategory returns the agent with the highest reputation
// score for the given category, from the provided candidate list.
func (m *Manager) BestAgentForCategory(
	agentIDs []string, category string,
) (string, float64) {
	bestID := ""
	bestScore := 0.0

	for _, id := range agentIDs {
		score := m.ReputationScore(id, category)
		if score > bestScore {
			bestScore = score
			bestID = id
		}
	}
	return bestID, bestScore
}

// classifyTask extracts a simple category from a task description.
func classifyTask(task string) string {
	lower := strings.ToLower(task)

	categories := []struct {
		name     string
		keywords []string
	}{
		{"coding", []string{
			"code", "implement", "function", "bug", "fix",
			"refactor", "test", "debug", "compile", "build",
		}},
		{"writing", []string{
			"write", "draft", "essay", "article", "blog",
			"email", "letter", "document", "report",
		}},
		{"research", []string{
			"research", "find", "search", "look up", "analyze",
			"investigate", "explore", "summarize",
		}},
		{"data", []string{
			"data", "csv", "json", "parse", "transform",
			"database", "query", "sql", "export",
		}},
		{"creative", []string{
			"creative", "story", "poem", "image", "design",
			"brainstorm", "idea", "generate",
		}},
		{"devops", []string{
			"deploy", "docker", "kubernetes", "ci/cd", "server",
			"infrastructure", "monitor", "log",
		}},
		{"math", []string{
			"calculate", "math", "equation", "formula",
			"statistics", "probability", "number",
		}},
	}

	bestCategory := "general"
	bestCount := 0
	for _, cat := range categories {
		count := 0
		for _, kw := range cat.keywords {
			if strings.Contains(lower, kw) {
				count++
			}
		}
		if count > bestCount {
			bestCount = count
			bestCategory = cat.name
		}
	}
	return bestCategory
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
