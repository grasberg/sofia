package memory

import (
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Semantic edges CRUD
// ---------------------------------------------------------------------------

// UpsertEdge inserts or updates an edge between two nodes.
func (m *MemoryDB) UpsertEdge(
	agentID string,
	sourceID, targetID int64,
	relation string,
	weight float64,
	properties string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if properties == "" {
		properties = "{}"
	}
	if weight <= 0 {
		weight = 1.0
	}
	if weight > 1.0 {
		weight = 1.0
	}
	now := time.Now().UTC()
	_, err := m.db.Exec(
		`INSERT INTO semantic_edges (agent_id, source_id, target_id, relation, weight, properties, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(agent_id, source_id, target_id, relation) DO UPDATE SET
		   weight = excluded.weight,
		   properties = excluded.properties,
		   updated_at = excluded.updated_at`,
		agentID,
		sourceID,
		targetID,
		relation,
		weight,
		properties,
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("memory: upsert edge: %w", err)
	}
	return nil
}

// GetEdges returns all edges for a node (as source or target), with joined names.
func (m *MemoryDB) GetEdges(agentID string, nodeID int64) ([]SemanticEdge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.getEdgesLocked(agentID, nodeID)
}

// GetEdgesForNodes fetches all edges for multiple nodes in a single query (batch version).
func (m *MemoryDB) GetEdgesForNodes(agentID string, nodeIDs []int64) ([]SemanticEdge, error) {
	if len(nodeIDs) == 0 {
		return []SemanticEdge{}, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Build placeholders for IN clause
	placeholders := strings.Repeat("?,", len(nodeIDs))
	placeholders = placeholders[:len(placeholders)-1]

	query := fmt.Sprintf(
		`SELECT e.id, e.agent_id, e.source_id, e.target_id, e.relation, e.weight, e.properties,
		        e.created_at, e.updated_at,
		        s.name, s.label, t.name, t.label
		 FROM semantic_edges e
		 JOIN semantic_nodes s ON s.id = e.source_id
		 JOIN semantic_nodes t ON t.id = e.target_id
		 WHERE e.agent_id = ? AND (e.source_id IN (%s) OR e.target_id IN (%s))
		 ORDER BY e.weight DESC`,
		placeholders, placeholders,
	)

	// Build args: agentID + nodeIDs for source + nodeIDs for target
	args := make([]any, 0, 1+len(nodeIDs)*2)
	args = append(args, agentID)
	for _, id := range nodeIDs {
		args = append(args, id)
	}
	for _, id := range nodeIDs {
		args = append(args, id)
	}

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: get edges for nodes: %w", err)
	}
	defer rows.Close()

	var edges []SemanticEdge
	for rows.Next() {
		var e SemanticEdge
		var created, updated string
		if err = rows.Scan(&e.ID, &e.AgentID, &e.SourceID, &e.TargetID, &e.Relation,
			&e.Weight, &e.Properties, &created, &updated,
			&e.SourceName, &e.SourceLabel, &e.TargetName, &e.TargetLabel); err != nil {
			return nil, fmt.Errorf("memory: scan edges: %w", err)
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, created)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// getEdgesLocked is the lock-free version of GetEdges; caller must hold mu.
func (m *MemoryDB) getEdgesLocked(agentID string, nodeID int64) ([]SemanticEdge, error) {
	rows, err := m.db.Query(
		`SELECT e.id, e.agent_id, e.source_id, e.target_id, e.relation, e.weight, e.properties,
		        e.created_at, e.updated_at,
		        s.name, s.label, t.name, t.label
		 FROM semantic_edges e
		 JOIN semantic_nodes s ON s.id = e.source_id
		 JOIN semantic_nodes t ON t.id = e.target_id
		 WHERE e.agent_id = ? AND (e.source_id = ? OR e.target_id = ?)
		 ORDER BY e.weight DESC`,
		agentID, nodeID, nodeID,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get edges: %w", err)
	}
	defer rows.Close()

	var edges []SemanticEdge
	for rows.Next() {
		var e SemanticEdge
		var created, updated string
		if err = rows.Scan(&e.ID, &e.AgentID, &e.SourceID, &e.TargetID, &e.Relation,
			&e.Weight, &e.Properties, &created, &updated,
			&e.SourceName, &e.SourceLabel, &e.TargetName, &e.TargetLabel); err != nil {
			return nil, fmt.Errorf("memory: scan edge: %w", err)
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, created)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// getEdgesForNodesLocked is the lock-free batch version; caller must hold mu.
func (m *MemoryDB) getEdgesForNodesLocked(agentID string, nodeIDs []int64) ([]SemanticEdge, error) {
	if len(nodeIDs) == 0 {
		return []SemanticEdge{}, nil
	}

	// Build placeholders for IN clause
	placeholders := strings.Repeat("?,", len(nodeIDs))
	placeholders = placeholders[:len(placeholders)-1]

	query := fmt.Sprintf(
		`SELECT e.id, e.agent_id, e.source_id, e.target_id, e.relation, e.weight, e.properties,
		        e.created_at, e.updated_at,
		        s.name, s.label, t.name, t.label
		 FROM semantic_edges e
		 JOIN semantic_nodes s ON s.id = e.source_id
		 JOIN semantic_nodes t ON t.id = e.target_id
		 WHERE e.agent_id = ? AND (e.source_id IN (%s) OR e.target_id IN (%s))
		 ORDER BY e.weight DESC`,
		placeholders, placeholders,
	)

	// Build args: agentID + nodeIDs for source + nodeIDs for target
	args := make([]any, 0, 1+len(nodeIDs)*2)
	args = append(args, agentID)
	for _, id := range nodeIDs {
		args = append(args, id)
	}
	for _, id := range nodeIDs {
		args = append(args, id)
	}

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: get edges for nodes: %w", err)
	}
	defer rows.Close()

	var edges []SemanticEdge
	for rows.Next() {
		var e SemanticEdge
		var created, updated string
		if err = rows.Scan(&e.ID, &e.AgentID, &e.SourceID, &e.TargetID, &e.Relation,
			&e.Weight, &e.Properties, &created, &updated,
			&e.SourceName, &e.SourceLabel, &e.TargetName, &e.TargetLabel); err != nil {
			return nil, fmt.Errorf("memory: scan edges: %w", err)
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, created)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// ListEdges returns all edges for an agent, with source/target names populated.
func (m *MemoryDB) ListEdges(agentID string, limit int) ([]SemanticEdge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 500
	}
	rows, err := m.db.Query(
		`SELECT e.id, e.agent_id, e.source_id, e.target_id, e.relation, e.weight, e.properties,
		        e.created_at, e.updated_at,
		        s.name, s.label, t.name, t.label
		 FROM semantic_edges e
		 JOIN semantic_nodes s ON s.id = e.source_id
		 JOIN semantic_nodes t ON t.id = e.target_id
		 WHERE e.agent_id = ?
		 ORDER BY e.weight DESC
		 LIMIT ?`,
		agentID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: list edges: %w", err)
	}
	defer rows.Close()

	var edges []SemanticEdge
	for rows.Next() {
		var e SemanticEdge
		var created, updated string
		if err = rows.Scan(&e.ID, &e.AgentID, &e.SourceID, &e.TargetID, &e.Relation,
			&e.Weight, &e.Properties, &created, &updated,
			&e.SourceName, &e.SourceLabel, &e.TargetName, &e.TargetLabel); err != nil {
			return nil, fmt.Errorf("memory: scan edge: %w", err)
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, created)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// ReinforceEdge increases an edge's weight by delta (capped at 1.0).
// Errors are intentionally ignored — this is a best-effort update.
func (m *MemoryDB) ReinforceEdge(edgeID int64, delta float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.reinforceEdgeLocked(edgeID, delta)
}

// reinforceEdgeLocked is the lock-free version of ReinforceEdge; caller must hold mu.
func (m *MemoryDB) reinforceEdgeLocked(edgeID int64, delta float64) {
	_, _ = m.db.Exec( //nolint:errcheck // best-effort
		`UPDATE semantic_edges SET weight = MIN(1.0, weight + ?), updated_at = ? WHERE id = ?`,
		delta, time.Now().UTC(), edgeID,
	)
}

// DeleteEdge deletes a single edge by ID.
func (m *MemoryDB) DeleteEdge(edgeID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`DELETE FROM semantic_edges WHERE id = ?`, edgeID)
	return err
}
