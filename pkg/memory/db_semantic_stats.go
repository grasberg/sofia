package memory

import (
	"database/sql"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Strategic forgetting: stale node detection
// ---------------------------------------------------------------------------

// GetStaleNodes returns nodes that haven't been accessed recently and have low usage.
func (m *MemoryDB) GetStaleNodes(agentID string, maxAge time.Duration, minAccessCount int) ([]SemanticNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cutoff := time.Now().UTC().Add(-maxAge)
	rows, err := m.db.Query(
		`SELECT id, agent_id, label, name, properties, access_count, last_accessed, quality_score, created_at, updated_at
		 FROM semantic_nodes
		 WHERE agent_id = ?
		   AND access_count < ?
		   AND (last_accessed IS NULL OR last_accessed < ?)
		 ORDER BY access_count ASC, last_accessed ASC`,
		agentID, minAccessCount, cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get stale nodes: %w", err)
	}
	defer rows.Close()

	var nodes []SemanticNode
	for rows.Next() {
		var n SemanticNode
		var lastAccessed sql.NullString
		var created, updated string
		if err = rows.Scan(&n.ID, &n.AgentID, &n.Label, &n.Name, &n.Properties,
			&n.AccessCount, &lastAccessed, &n.QualityScore, &created, &updated); err != nil {
			return nil, fmt.Errorf("memory: scan stale node: %w", err)
		}
		n.CreatedAt, _ = time.Parse(time.RFC3339, created)
		n.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		if lastAccessed.Valid {
			t, _ := time.Parse(time.RFC3339, lastAccessed.String)
			n.LastAccessed = &t
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// ---------------------------------------------------------------------------
// Memory stats (self-evolution tracking)
// ---------------------------------------------------------------------------

// RecordStat logs an event in memory_stats.
// Every 100th call, old stats (>30 days) are pruned automatically.
func (m *MemoryDB) RecordStat(agentID, eventType string, nodeID *int64, details string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`INSERT INTO memory_stats (agent_id, event_type, node_id, details, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		agentID, eventType, nodeID, details, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("memory: record stat: %w", err)
	}

	// Prune old stats every 100 calls.
	if m.statCount.Add(1)%100 == 0 {
		m.pruneStatsLocked()
	}

	return nil
}

// pruneStatsLocked deletes memory_stats entries older than 30 days.
// Caller must hold mu.
func (m *MemoryDB) pruneStatsLocked() {
	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	_, _ = m.db.Exec(`DELETE FROM memory_stats WHERE created_at < ?`, cutoff) //nolint:errcheck // best-effort
}

// GetNodeStats returns aggregated access statistics for an agent's nodes.
func (m *MemoryDB) GetNodeStats(agentID string) ([]NodeStatSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(
		`SELECT n.id, n.name, n.label, n.access_count,
		        COALESCE(SUM(CASE WHEN s.event_type = 'query' THEN 1 ELSE 0 END), 0) AS query_count,
		        COALESCE(SUM(CASE WHEN s.event_type = 'hit' THEN 1 ELSE 0 END), 0) AS hit_count
		 FROM semantic_nodes n
		 LEFT JOIN memory_stats s ON s.node_id = n.id AND s.agent_id = n.agent_id
		 WHERE n.agent_id = ?
		 GROUP BY n.id
		 ORDER BY n.access_count DESC`,
		agentID,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get node stats: %w", err)
	}
	defer rows.Close()

	var stats []NodeStatSummary
	for rows.Next() {
		var s NodeStatSummary
		if err = rows.Scan(&s.NodeID, &s.Name, &s.Label, &s.AccessCount, &s.QueryCount, &s.HitCount); err != nil {
			return nil, fmt.Errorf("memory: scan node stat: %w", err)
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}
