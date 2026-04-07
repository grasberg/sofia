package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

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
	return sm.GetContextWithBudget(0) // No budget, use default behavior
}

// GetContextWithBudget returns a formatted summary with token budget awareness.
// If maxTokens > 0, it limits the output to stay within the budget.
// Applies topic diversity constraints to prevent context dominance by single topics.
func (sm *SemanticMemory) GetContextWithBudget(maxTokens int) string {
	if sm.db == nil {
		return ""
	}

	// Fetch nodes with composite importance scoring
	nodes, err := sm.db.FindNodesByImportance(sm.agentID, 50) // Fetch more for diversity filtering
	if err != nil || len(nodes) == 0 {
		return ""
	}

	// Calculate importance scores and sort
	type scoredNode struct {
		node       memory.SemanticNode
		importance float64
	}

	scored := make([]scoredNode, len(nodes))
	for i, n := range nodes {
		scored[i] = scoredNode{
			node:       n,
			importance: calculateNodeImportance(n),
		}
	}

	// Sort by importance descending
	// (Note: FindNodesByImport should already return sorted, but let's ensure)

	var sb strings.Builder
	usedTokens := 0
	
	// Track topic diversity: max 3 nodes per label
	labelCounts := make(map[string]int)
	maxPerLabel := 3

	for _, sn := range scored {
		n := sn.node

		// Topic diversity constraint: skip if label already has max nodes
		if labelCounts[n.Label] >= maxPerLabel {
			continue
		}

		// Check budget
		if maxTokens > 0 {
			// Estimate tokens for this node
			estimatedNodeTokens := estimateNodeTokens(n)
			if usedTokens+estimatedNodeTokens > maxTokens && usedTokens > 0 {
				break // Budget exceeded
			}
			usedTokens += estimatedNodeTokens
		}

		var props map[string]any
		_ = json.Unmarshal([]byte(n.Properties), &props)

		fmt.Fprintf(&sb, "**[%s] %s**", n.Label, n.Name)
		if len(props) > 0 {
			propParts := make([]string, 0, len(props))
			for k, v := range props {
				propParts = append(propParts, fmt.Sprintf("%s: %v", k, v))
			}
			fmt.Fprintf(&sb, " (%s)", strings.Join(propParts, ", "))
		}
		sb.WriteString("\n")

		// Fetch edges for this node
		edges, _ := sm.db.GetEdges(sm.agentID, n.ID)
		for _, e := range edges {
			if e.SourceID == n.ID {
				fmt.Fprintf(&sb, "  → %s → [%s] %s\n", e.Relation, e.TargetLabel, e.TargetName)
			} else {
				fmt.Fprintf(&sb, "  ← %s ← [%s] %s\n", e.Relation, e.SourceLabel, e.SourceName)
			}
		}

		// Check budget again after adding edges
		if maxTokens > 0 && estimateTokensFromString(sb.String()) > maxTokens {
			break
		}
		
		// Track label usage for diversity
		labelCounts[n.Label]++
	}

	if sb.Len() == 0 {
		return ""
	}

	return "## Knowledge Graph\n\n" + sb.String()
}

// calculateNodeImportance computes a composite importance score for a node.
// Higher scores indicate more important/relevant nodes.
func calculateNodeImportance(n memory.SemanticNode) float64 {
	// Recency: exponential decay, half-life = 30 days
	recency := 0.5
	if n.LastAccessed != nil && !n.LastAccessed.IsZero() {
		age := time.Since(*n.LastAccessed)
		recency = 1.0 / (1.0 + float64(age.Hours())/(30*24)) // Decay factor
	}

	// Access: log scale to prevent runaway popular nodes
	accessScore := 0.0
	if n.AccessCount > 0 {
		accessScore = 1.0 - 1.0/(1.0+float64(n.AccessCount)/10.0)
	}

	// Quality: direct score (0.0-1.0)
	qualityScore := n.QualityScore

	// Connectedness: will be populated later if needed
	connectedness := 0.5 // Default middle value

	// Weighted composite
	return 0.3*accessScore + 0.3*recency + 0.25*qualityScore + 0.15*connectedness
}

// estimateNodeTokens estimates the token count for a semantic node.
func estimateNodeTokens(n memory.SemanticNode) int {
	tokens := utf8.RuneCountInString(n.Label) + utf8.RuneCountInString(n.Name)
	tokens += utf8.RuneCountInString(n.Properties)
	return tokens * 2 / 5
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

		fmt.Fprintf(&sb, "[%s] %s", r.Node.Label, r.Node.Name)
		if len(props) > 0 {
			propParts := make([]string, 0, len(props))
			for k, v := range props {
				propParts = append(propParts, fmt.Sprintf("%s=%v", k, v))
			}
			fmt.Fprintf(&sb, " {%s}", strings.Join(propParts, ", "))
		}
		fmt.Fprintf(&sb, " (accessed %d times)\n", r.Node.AccessCount)

		if len(r.Edges) > 0 {
			sb.WriteString("Relations:\n")
			for _, e := range r.Edges {
				if e.SourceID == r.Node.ID {
					fmt.Fprintf(&sb, "  → %s → [%s] %s (weight: %.2f)\n",
						e.Relation, e.TargetLabel, e.TargetName, e.Weight)
				} else {
					fmt.Fprintf(&sb, "  ← %s ← [%s] %s (weight: %.2f)\n",
						e.Relation, e.SourceLabel, e.SourceName, e.Weight)
				}
			}
		}
	}
	return sb.String()
}
