package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grasberg/sofia/pkg/memory"
)

// SemanticMemory provides a high-level API for the knowledge graph,
// analogous to MemoryStore for flat memory notes.
type SemanticMemory struct {
	db      *memory.MemoryDB
	agentID string
}

// NewSemanticMemory creates a new SemanticMemory backed by the given MemoryDB.
func NewSemanticMemory(db *memory.MemoryDB, agentID string) *SemanticMemory {
	return &SemanticMemory{
		db:      db,
		agentID: agentID,
	}
}

// AddFact adds or updates a knowledge entity.
func (sm *SemanticMemory) AddFact(label, name string, properties map[string]any) (int64, error) {
	if sm.db == nil {
		return 0, nil
	}
	propsJSON := "{}"
	if len(properties) > 0 {
		b, err := json.Marshal(properties)
		if err == nil {
			propsJSON = string(b)
		}
	}
	id, err := sm.db.UpsertNode(sm.agentID, label, name, propsJSON)
	if err != nil {
		return 0, err
	}
	// Record stat for self-evolution
	_ = sm.db.RecordStat(sm.agentID, "add", &id, fmt.Sprintf("Added %s:%s", label, name))
	return id, nil
}

// AddRelation creates a relationship between two entities.
// Entities are looked up (or created) by label+name.
func (sm *SemanticMemory) AddRelation(
	sourceLabel, sourceName, relation, targetLabel, targetName string,
	weight float64,
) error {
	if sm.db == nil {
		return nil
	}
	// Ensure both nodes exist
	sourceID, err := sm.db.UpsertNode(sm.agentID, sourceLabel, sourceName, "{}")
	if err != nil {
		return fmt.Errorf("semantic: ensure source node: %w", err)
	}
	targetID, err := sm.db.UpsertNode(sm.agentID, targetLabel, targetName, "{}")
	if err != nil {
		return fmt.Errorf("semantic: ensure target node: %w", err)
	}
	return sm.db.UpsertEdge(sm.agentID, sourceID, targetID, relation, weight, "{}")
}

// Query searches the knowledge graph and returns formatted text for the LLM.
func (sm *SemanticMemory) Query(query string, limit int) (string, error) {
	if sm.db == nil {
		return "", nil
	}
	results, err := sm.db.QueryGraph(sm.agentID, query, limit)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "No matching knowledge found.", nil
	}
	return formatGraphResults(results), nil
}

// GetEntity returns details for a specific entity.
func (sm *SemanticMemory) GetEntity(label, name string) (string, error) {
	if sm.db == nil {
		return "", nil
	}
	node, err := sm.db.GetNode(sm.agentID, label, name)
	if err != nil {
		return "", err
	}
	if node == nil {
		return fmt.Sprintf("Entity %s:%s not found.", label, name), nil
	}

	// Touch for self-evolution
	sm.db.TouchNode(node.ID)
	_ = sm.db.RecordStat(sm.agentID, "query", &node.ID, fmt.Sprintf("Get %s:%s", label, name))

	edges, _ := sm.db.GetEdges(sm.agentID, node.ID)
	result := memory.GraphResult{Node: *node, Edges: edges}
	return formatGraphResults([]memory.GraphResult{result}), nil
}

// DeleteEntity removes an entity and all its relations.
func (sm *SemanticMemory) DeleteEntity(label, name string) error {
	if sm.db == nil {
		return nil
	}
	node, err := sm.db.GetNode(sm.agentID, label, name)
	if err != nil {
		return err
	}
	if node == nil {
		return nil
	}
	_ = sm.db.RecordStat(sm.agentID, "prune", &node.ID, fmt.Sprintf("Deleted %s:%s", label, name))
	return sm.db.DeleteNode(node.ID)
}

// GetContext returns a formatted summary of the most important knowledge for the system prompt.
func (sm *SemanticMemory) GetContext(maxNodes int) string {
	if sm.db == nil {
		return ""
	}
	if maxNodes <= 0 {
		maxNodes = 10
	}
	nodes, err := sm.db.FindNodes(sm.agentID, "", "", maxNodes)
	if err != nil || len(nodes) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Knowledge Graph\n\n")

	for _, n := range nodes {
		var props map[string]any
		_ = json.Unmarshal([]byte(n.Properties), &props)

		sb.WriteString(fmt.Sprintf("**[%s] %s**", n.Label, n.Name))
		if len(props) > 0 {
			propParts := make([]string, 0, len(props))
			for k, v := range props {
				propParts = append(propParts, fmt.Sprintf("%s: %v", k, v))
			}
			sb.WriteString(fmt.Sprintf(" (%s)", strings.Join(propParts, ", ")))
		}
		sb.WriteString("\n")

		edges, _ := sm.db.GetEdges(sm.agentID, n.ID)
		for _, e := range edges {
			if e.SourceID == n.ID {
				sb.WriteString(fmt.Sprintf("  → %s → [%s] %s\n", e.Relation, e.TargetLabel, e.TargetName))
			} else {
				sb.WriteString(fmt.Sprintf("  ← %s ← [%s] %s\n", e.Relation, e.SourceLabel, e.SourceName))
			}
		}
	}
	return sb.String()
}

// formatGraphResults formats graph query results as readable text.
func formatGraphResults(results []memory.GraphResult) string {
	var sb strings.Builder
	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n---\n")
		}

		var props map[string]any
		_ = json.Unmarshal([]byte(r.Node.Properties), &props)

		sb.WriteString(fmt.Sprintf("[%s] %s", r.Node.Label, r.Node.Name))
		if len(props) > 0 {
			propParts := make([]string, 0, len(props))
			for k, v := range props {
				propParts = append(propParts, fmt.Sprintf("%s=%v", k, v))
			}
			sb.WriteString(fmt.Sprintf(" {%s}", strings.Join(propParts, ", ")))
		}
		sb.WriteString(fmt.Sprintf(" (accessed %d times)\n", r.Node.AccessCount))

		if len(r.Edges) > 0 {
			sb.WriteString("Relations:\n")
			for _, e := range r.Edges {
				if e.SourceID == r.Node.ID {
					sb.WriteString(fmt.Sprintf("  → %s → [%s] %s (weight: %.2f)\n",
						e.Relation, e.TargetLabel, e.TargetName, e.Weight))
				} else {
					sb.WriteString(fmt.Sprintf("  ← %s ← [%s] %s (weight: %.2f)\n",
						e.Relation, e.SourceLabel, e.SourceName, e.Weight))
				}
			}
		}
	}
	return sb.String()
}
