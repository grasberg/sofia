package memory

import (
	"database/sql"
	"time"
)

// ---------------------------------------------------------------------------
// Graph queries
// ---------------------------------------------------------------------------

// QueryGraph searches across nodes and their edges using LIKE matching.
// Returns up to limit results with their connected edges.
func (m *MemoryDB) QueryGraph(agentID, query string, limit int) ([]GraphResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if limit <= 0 {
		limit = 10
	}
	pattern := "%" + query + "%"

	nodes, err := m.findNodesLocked(agentID, "", pattern, limit)
	if err != nil {
		return nil, err
	}

	// Also search by label
	labelNodes, err := m.db.Query(
		`SELECT id, agent_id, label, name, properties, access_count, last_accessed, quality_score, created_at, updated_at
		 FROM semantic_nodes WHERE agent_id = ? AND label LIKE ? ORDER BY access_count DESC LIMIT ?`,
		agentID,
		pattern,
		limit,
	)
	if err == nil {
		defer labelNodes.Close()
		seen := make(map[int64]bool)
		for _, n := range nodes {
			seen[n.ID] = true
		}
		for labelNodes.Next() {
			var n SemanticNode
			var lastAccessed sql.NullString
			var created, updated string
			if err = labelNodes.Scan(&n.ID, &n.AgentID, &n.Label, &n.Name, &n.Properties,
				&n.AccessCount, &lastAccessed, &n.QualityScore, &created, &updated); err == nil {
				n.CreatedAt, _ = time.Parse(time.RFC3339, created)
				n.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
				if lastAccessed.Valid {
					t, _ := time.Parse(time.RFC3339, lastAccessed.String)
					n.LastAccessed = &t
				}
				if !seen[n.ID] {
					nodes = append(nodes, n)
					seen[n.ID] = true
				}
			}
		}
	}

	// Cap results
	if len(nodes) > limit {
		nodes = nodes[:limit]
	}

	var results []GraphResult
	for _, n := range nodes {
		// Touch node for self-evolution tracking
		m.touchNodeLocked(n.ID)
	}

	// Batch fetch all edges in single query instead of N+1
	nodeIDs := make([]int64, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.ID
	}
	allEdges, err := m.getEdgesForNodesLocked(agentID, nodeIDs)
	if err != nil {
		allEdges = nil
	}

	// Build edge lookup map
	edgeMap := make(map[int64][]SemanticEdge)
	for _, e := range allEdges {
		edgeMap[e.SourceID] = append(edgeMap[e.SourceID], e)
		edgeMap[e.TargetID] = append(edgeMap[e.TargetID], e)
	}

	for _, n := range nodes {
		edges := edgeMap[n.ID]

		// Reinforce traversed edges
		for _, e := range edges {
			m.reinforceEdgeLocked(e.ID, 0.01)
		}

		results = append(results, GraphResult{Node: n, Edges: edges})
	}
	return results, nil
}
