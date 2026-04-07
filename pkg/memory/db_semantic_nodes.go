package memory

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Semantic nodes CRUD
// ---------------------------------------------------------------------------

// UpsertNode inserts or updates a semantic node.
// Returns the node ID.
func (m *MemoryDB) UpsertNode(agentID, label, name, properties string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if properties == "" {
		properties = "{}"
	}
	now := time.Now().UTC()
	result, err := m.db.Exec(
		`INSERT INTO semantic_nodes (agent_id, label, name, properties, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(agent_id, label, name) DO UPDATE SET
		   properties = excluded.properties,
		   updated_at = excluded.updated_at`,
		agentID, label, name, properties, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("memory: upsert node: %w", err)
	}

	// If it was an update, get the existing ID
	id, insertErr := result.LastInsertId()
	if insertErr != nil || id == 0 {
		var nodeID int64
		err = m.db.QueryRow(
			`SELECT id FROM semantic_nodes WHERE agent_id = ? AND label = ? AND name = ?`,
			agentID, label, name,
		).Scan(&nodeID)
		if err != nil {
			return 0, fmt.Errorf("memory: get node id after upsert: %w", err)
		}
		return nodeID, nil
	}
	return id, nil
}

// GetNode returns a single semantic node by agent, label, and name.
// Returns nil if not found.
func (m *MemoryDB) GetNode(agentID, label, name string) (*SemanticNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	row := m.db.QueryRow(
		`SELECT id, agent_id, label, name, properties, access_count, last_accessed, quality_score, created_at, updated_at
		 FROM semantic_nodes WHERE agent_id = ? AND label = ? AND name = ?`,
		agentID,
		label,
		name,
	)
	return scanNode(row)
}

// GetNodeByID returns a node by its ID.
func (m *MemoryDB) GetNodeByID(nodeID int64) (*SemanticNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	row := m.db.QueryRow(
		`SELECT id, agent_id, label, name, properties, access_count, last_accessed, quality_score, created_at, updated_at
		 FROM semantic_nodes WHERE id = ?`,
		nodeID,
	)
	return scanNode(row)
}

// UpdateNodeQuality updates the quality score for a node.
func (m *MemoryDB) UpdateNodeQuality(nodeID int64, score float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(`UPDATE semantic_nodes SET quality_score = ? WHERE id = ?`, score, nodeID)
	return err
}

// FindNodes searches nodes by agent and optional filters.
// namePattern uses SQL LIKE syntax (% for wildcard).
func (m *MemoryDB) FindNodes(agentID, label, namePattern string, limit int) ([]SemanticNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.findNodesLocked(agentID, label, namePattern, limit)
}

// FindNodesByImportance returns nodes ordered by composite importance score.
// The score considers access count, recency, quality, and connectedness.
func (m *MemoryDB) FindNodesByImportance(agentID string, limit int) ([]SemanticNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	// Use a composite importance score formula:
	// score = 0.3*access_score + 0.3*recency + 0.25*quality + 0.15*connectedness
	// We approximate this with SQL ordering by:
	// (access_count * 0.3) + (quality_score * 0.25) + recency_factor
	query := `
		SELECT id, agent_id, label, name, properties, access_count, last_accessed, quality_score, created_at, updated_at
		FROM semantic_nodes
		WHERE agent_id = ?
		ORDER BY (
			(CAST(access_count AS FLOAT) / (1.0 + CAST(access_count AS FLOAT) / 10.0)) * 0.3 +
			quality_score * 0.25 +
			CASE
				WHEN last_accessed IS NOT NULL
				THEN 1.0 / (1.0 + (julianday('now') - julianday(last_accessed)) / 30.0)
				ELSE 0.5
			END * 0.3
		) DESC
		LIMIT ?
	`

	rows, err := m.db.Query(query, agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("memory: find nodes by importance: %w", err)
	}
	defer rows.Close()

	var nodes []SemanticNode
	for rows.Next() {
		var n SemanticNode
		var lastAccessed sql.NullString
		var created, updated string
		if err = rows.Scan(&n.ID, &n.AgentID, &n.Label, &n.Name, &n.Properties,
			&n.AccessCount, &lastAccessed, &n.QualityScore, &created, &updated); err != nil {
			return nil, fmt.Errorf("memory: scan node: %w", err)
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

// findNodesLocked is the lock-free version of FindNodes; caller must hold mu.
func (m *MemoryDB) findNodesLocked(agentID, label, namePattern string, limit int) ([]SemanticNode, error) {
	var args []any
	query := `SELECT id, agent_id, label, name, properties, access_count, last_accessed, quality_score, created_at, updated_at
		 FROM semantic_nodes WHERE agent_id = ?`
	args = append(args, agentID)

	if label != "" {
		query += ` AND label = ?`
		args = append(args, label)
	}
	if namePattern != "" {
		query += ` AND name LIKE ?`
		args = append(args, namePattern)
	}
	query += ` ORDER BY access_count DESC, updated_at DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: find nodes: %w", err)
	}
	defer rows.Close()

	var nodes []SemanticNode
	for rows.Next() {
		var n SemanticNode
		var lastAccessed sql.NullString
		var created, updated string
		if err = rows.Scan(&n.ID, &n.AgentID, &n.Label, &n.Name, &n.Properties,
			&n.AccessCount, &lastAccessed, &n.QualityScore, &created, &updated); err != nil {
			return nil, fmt.Errorf("memory: scan node row: %w", err)
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

// DeleteNode deletes a node and cascades to its edges.
func (m *MemoryDB) DeleteNode(nodeID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`DELETE FROM semantic_nodes WHERE id = ?`, nodeID)
	if err != nil {
		return fmt.Errorf("memory: delete node: %w", err)
	}
	return nil
}

// DeleteNodes batch-deletes nodes by IDs.
func (m *MemoryDB) DeleteNodes(nodeIDs []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(nodeIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(nodeIDs))
	args := make([]any, len(nodeIDs))
	for i, id := range nodeIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	//nolint:gosec // placeholders are safely generated "?"
	query := fmt.Sprintf(`DELETE FROM semantic_nodes WHERE id IN (%s)`, strings.Join(placeholders, ","))
	_, err := m.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("memory: delete nodes batch: %w", err)
	}
	return nil
}

// TouchNode increments access_count and updates last_accessed.
// Errors are intentionally ignored — this is a best-effort update.
func (m *MemoryDB) TouchNode(nodeID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.touchNodeLocked(nodeID)
}

// touchNodeLocked is the lock-free version of TouchNode; caller must hold mu.
func (m *MemoryDB) touchNodeLocked(nodeID int64) {
	_, _ = m.db.Exec( //nolint:errcheck // best-effort
		`UPDATE semantic_nodes SET access_count = access_count + 1, last_accessed = ? WHERE id = ?`,
		time.Now().UTC(), nodeID,
	)
}

// CountNodes returns the number of semantic nodes for an agent.
func (m *MemoryDB) CountNodes(agentID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int
	_ = m.db.QueryRow(`SELECT COUNT(*) FROM semantic_nodes WHERE agent_id = ?`, agentID).Scan(&count)
	return count
}
