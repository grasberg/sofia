package memory

import (
	"database/sql"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Semantic node types
// ---------------------------------------------------------------------------

// SemanticNode represents an entity in the knowledge graph.
type SemanticNode struct {
	ID           int64
	AgentID      string
	Label        string
	Name         string
	Properties   string // JSON
	AccessCount  int
	LastAccessed *time.Time
	QualityScore float64 // 0.0-1.0, computed by memory quality scorer
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SemanticEdge represents a relationship between two nodes.
type SemanticEdge struct {
	ID         int64
	AgentID    string
	SourceID   int64
	TargetID   int64
	Relation   string
	Weight     float64
	Properties string // JSON
	CreatedAt  time.Time
	UpdatedAt  time.Time
	// Populated by join queries
	SourceName  string
	SourceLabel string
	TargetName  string
	TargetLabel string
}

// GraphResult is a combined node+edges result for graph queries.
type GraphResult struct {
	Node  SemanticNode
	Edges []SemanticEdge
}

// NodeStatSummary holds aggregated access stats for a node.
type NodeStatSummary struct {
	NodeID      int64
	Name        string
	Label       string
	AccessCount int
	QueryCount  int
	HitCount    int
}

func scanNode(row *sql.Row) (*SemanticNode, error) {
	var n SemanticNode
	var lastAccessed sql.NullString
	var created, updated string
	err := row.Scan(&n.ID, &n.AgentID, &n.Label, &n.Name, &n.Properties,
		&n.AccessCount, &lastAccessed, &n.QualityScore, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memory: scan node: %w", err)
	}
	n.CreatedAt, _ = time.Parse(time.RFC3339, created)
	n.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	if lastAccessed.Valid {
		t, _ := time.Parse(time.RFC3339, lastAccessed.String)
		n.LastAccessed = &t
	}
	return &n, nil
}
