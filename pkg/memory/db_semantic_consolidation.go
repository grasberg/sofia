package memory

import (
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Consolidation: duplicate detection, conflict resolution, merging
// ---------------------------------------------------------------------------

// FindDuplicateNodes returns groups of nodes with the same label and similar names.
// Used by the consolidation process to merge duplicates.
func (m *MemoryDB) FindDuplicateNodes(agentID string) ([][]SemanticNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodes, err := m.findNodesLocked(agentID, "", "", 0)
	if err != nil {
		return nil, err
	}

	// Group by label
	byLabel := make(map[string][]SemanticNode)
	for _, n := range nodes {
		byLabel[n.Label] = append(byLabel[n.Label], n)
	}

	var duplicates [][]SemanticNode
	for _, group := range byLabel {
		if len(group) < 2 {
			continue
		}
		used := make(map[int]bool)
		for i := 0; i < len(group); i++ {
			if used[i] {
				continue
			}
			cluster := []SemanticNode{group[i]}
			for j := i + 1; j < len(group); j++ {
				if used[j] {
					continue
				}
				if isSimilarName(group[i].Name, group[j].Name) {
					cluster = append(cluster, group[j])
					used[j] = true
				}
			}
			if len(cluster) > 1 {
				duplicates = append(duplicates, cluster)
			}
		}
	}
	return duplicates, nil
}

// isSimilarName checks if two names are similar enough to be considered duplicates.
// Uses a simple case-insensitive comparison plus prefix/substring matching.
func isSimilarName(a, b string) bool {
	la := strings.ToLower(strings.TrimSpace(a))
	lb := strings.ToLower(strings.TrimSpace(b))
	if la == lb {
		return true
	}
	if strings.HasPrefix(la, lb) || strings.HasPrefix(lb, la) {
		return true
	}
	if len(la) <= 20 && len(lb) <= 20 && levenshteinDistance(la, lb) <= 2 {
		return true
	}
	return false
}

// levenshteinDistance computes the edit distance between two strings.
func levenshteinDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, min(curr[j-1]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// GetConflictingEdges returns edges between the same source+target with different relations.
func (m *MemoryDB) GetConflictingEdges(agentID string) ([][]SemanticEdge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(
		`SELECT e.id, e.agent_id, e.source_id, e.target_id, e.relation, e.weight, e.properties,
		        e.created_at, e.updated_at,
		        s.name, s.label, t.name, t.label
		 FROM semantic_edges e
		 JOIN semantic_nodes s ON s.id = e.source_id
		 JOIN semantic_nodes t ON t.id = e.target_id
		 WHERE e.agent_id = ?
		 ORDER BY e.source_id, e.target_id, e.weight DESC`,
		agentID,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get conflicting edges: %w", err)
	}
	defer rows.Close()

	type edgeKey struct {
		source, target int64
	}
	grouped := make(map[edgeKey][]SemanticEdge)
	var order []edgeKey

	for rows.Next() {
		var e SemanticEdge
		var created, updated string
		if err = rows.Scan(&e.ID, &e.AgentID, &e.SourceID, &e.TargetID, &e.Relation,
			&e.Weight, &e.Properties, &created, &updated,
			&e.SourceName, &e.SourceLabel, &e.TargetName, &e.TargetLabel); err != nil {
			return nil, fmt.Errorf("memory: scan conflict edge: %w", err)
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, created)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		key := edgeKey{e.SourceID, e.TargetID}
		if _, exists := grouped[key]; !exists {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], e)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	var conflicts [][]SemanticEdge
	for _, key := range order {
		if edges := grouped[key]; len(edges) > 1 {
			conflicts = append(conflicts, edges)
		}
	}
	return conflicts, nil
}

// MergeNodes merges secondary nodes into the primary node.
// All edges pointing to/from secondary nodes are redirected to the primary.
// Secondary nodes are then deleted.
func (m *MemoryDB) MergeNodes(primaryID int64, secondaryIDs []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(secondaryIDs) == 0 {
		return nil
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("memory: begin merge tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, secID := range secondaryIDs {
		// Redirect source edges: secID -> X becomes primaryID -> X
		_, _ = tx.Exec(
			`UPDATE OR IGNORE semantic_edges SET source_id = ?, updated_at = ? WHERE source_id = ?`,
			primaryID, time.Now().UTC(), secID,
		)
		// Redirect target edges: X -> secID becomes X -> primaryID
		_, _ = tx.Exec(
			`UPDATE OR IGNORE semantic_edges SET target_id = ?, updated_at = ? WHERE target_id = ?`,
			primaryID, time.Now().UTC(), secID,
		)
		// Delete remaining edges (duplicates that couldn't be redirected)
		_, _ = tx.Exec(`DELETE FROM semantic_edges WHERE source_id = ? OR target_id = ?`, secID, secID)
		// Delete the secondary node
		_, _ = tx.Exec(`DELETE FROM semantic_nodes WHERE id = ?`, secID)
	}

	return tx.Commit()
}
