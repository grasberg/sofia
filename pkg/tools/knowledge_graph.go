package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
)

// knowledgeGraphTool exposes the semantic memory as an LLM tool.
type knowledgeGraphTool struct {
	db      *memory.MemoryDB
	agentID string

	// Track writes for auto-consolidation
	mu            sync.Mutex
	writeCount    int
	consolidateAt int
}

// NewKnowledgeGraphTool creates a new knowledge graph tool.
func NewKnowledgeGraphTool(db *memory.MemoryDB, agentID string) Tool {
	return &knowledgeGraphTool{
		db:            db,
		agentID:       agentID,
		consolidateAt: 10,
	}
}

func (t *knowledgeGraphTool) Name() string { return "knowledge_graph" }
func (t *knowledgeGraphTool) Description() string {
	return `Manage structured knowledge graph for persistent semantic memory. Stores entities (people, projects, concepts, preferences) and their relationships. Operations: add_entity, add_relation, query, get_entity, delete_entity, consolidate, prune, stats.`
}

func (t *knowledgeGraphTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type": "string",
				"enum": []string{
					"add_entity",
					"add_relation",
					"query",
					"get_entity",
					"delete_entity",
					"consolidate",
					"prune",
					"stats",
				},
				"description": "The operation to perform on the knowledge graph",
			},
			"label": map[string]any{
				"type":        "string",
				"description": "Entity type/category (e.g., person, project, preference, concept, tool, language). Required for add_entity, get_entity, delete_entity.",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Entity name. Required for add_entity, get_entity, delete_entity.",
			},
			"properties": map[string]any{
				"type":        "object",
				"description": "Key-value metadata for the entity (optional, for add_entity)",
			},
			"source_label": map[string]any{
				"type":        "string",
				"description": "Source entity label (for add_relation)",
			},
			"source_name": map[string]any{
				"type":        "string",
				"description": "Source entity name (for add_relation)",
			},
			"relation": map[string]any{
				"type":        "string",
				"description": "Relationship type (e.g., knows, prefers, works_on, uses, created_by). Required for add_relation.",
			},
			"target_label": map[string]any{
				"type":        "string",
				"description": "Target entity label (for add_relation)",
			},
			"target_name": map[string]any{
				"type":        "string",
				"description": "Target entity name (for add_relation)",
			},
			"weight": map[string]any{
				"type":        "number",
				"description": "Edge strength 0.0-1.0 (optional, default 1.0, for add_relation)",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Search query for the knowledge graph (for query operation)",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results (optional, default 10)",
			},
			"dry_run": map[string]any{
				"type":        "boolean",
				"description": "Preview without deleting (for prune operation)",
			},
		},
		"required": []string{"operation"},
	}
}

func (t *knowledgeGraphTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.db == nil {
		return ErrorResult("Knowledge graph not available: database not initialized")
	}

	op, _ := args["operation"].(string)

	switch op {
	case "add_entity":
		return t.addEntity(args)
	case "add_relation":
		return t.addRelation(args)
	case "query":
		return t.query(args)
	case "get_entity":
		return t.getEntity(args)
	case "delete_entity":
		return t.deleteEntity(args)
	case "consolidate":
		return t.consolidate()
	case "prune":
		return t.prune(args)
	case "stats":
		return t.stats()
	default:
		return ErrorResult(fmt.Sprintf("Unknown knowledge graph operation: %s", op))
	}
}

func (t *knowledgeGraphTool) addEntity(args map[string]any) *ToolResult {
	label, _ := args["label"].(string)
	name, _ := args["name"].(string)
	if label == "" || name == "" {
		return ErrorResult("label and name are required for add_entity")
	}

	propsJSON := "{}"
	if props, ok := args["properties"]; ok && props != nil {
		b, err := json.Marshal(props)
		if err == nil {
			propsJSON = string(b)
		}
	}

	id, err := t.db.UpsertNode(t.agentID, label, name, propsJSON)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to add entity: %s", err))
	}

	_ = t.db.RecordStat(t.agentID, "add", &id, fmt.Sprintf("Added %s:%s", label, name))
	t.maybeConsolidate()

	return SilentResult(fmt.Sprintf("Entity [%s] %s stored (id=%d)", label, name, id))
}

func (t *knowledgeGraphTool) addRelation(args map[string]any) *ToolResult {
	sourceLabel, _ := args["source_label"].(string)
	sourceName, _ := args["source_name"].(string)
	relation, _ := args["relation"].(string)
	targetLabel, _ := args["target_label"].(string)
	targetName, _ := args["target_name"].(string)

	if sourceLabel == "" || sourceName == "" || relation == "" || targetLabel == "" || targetName == "" {
		return ErrorResult(
			"source_label, source_name, relation, target_label, target_name are all required for add_relation",
		)
	}

	weight := 1.0
	if w, ok := args["weight"].(float64); ok && w > 0 {
		weight = w
	}

	// Ensure both nodes exist
	sourceID, err := t.db.UpsertNode(t.agentID, sourceLabel, sourceName, "{}")
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to ensure source entity: %s", err))
	}
	targetID, err := t.db.UpsertNode(t.agentID, targetLabel, targetName, "{}")
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to ensure target entity: %s", err))
	}

	if err := t.db.UpsertEdge(t.agentID, sourceID, targetID, relation, weight, "{}"); err != nil {
		return ErrorResult(fmt.Sprintf("Failed to add relation: %s", err))
	}

	t.maybeConsolidate()

	return SilentResult(fmt.Sprintf("Relation stored: [%s] %s --%s--> [%s] %s (weight=%.2f)",
		sourceLabel, sourceName, relation, targetLabel, targetName, weight))
}

func (t *knowledgeGraphTool) query(args map[string]any) *ToolResult {
	q, _ := args["query"].(string)
	if q == "" {
		return ErrorResult("query is required for query operation")
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	results, err := t.db.QueryGraph(t.agentID, q, limit)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Query failed: %s", err))
	}

	if len(results) == 0 {
		return SilentResult(fmt.Sprintf("No knowledge found matching %q", q))
	}

	return SilentResult(formatGraphToolResults(results))
}

func (t *knowledgeGraphTool) getEntity(args map[string]any) *ToolResult {
	label, _ := args["label"].(string)
	name, _ := args["name"].(string)
	if label == "" || name == "" {
		return ErrorResult("label and name are required for get_entity")
	}

	node, err := t.db.GetNode(t.agentID, label, name)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to get entity: %s", err))
	}
	if node == nil {
		return SilentResult(fmt.Sprintf("Entity [%s] %s not found", label, name))
	}

	// Touch for self-evolution
	t.db.TouchNode(node.ID)
	_ = t.db.RecordStat(t.agentID, "query", &node.ID, fmt.Sprintf("Get %s:%s", label, name))

	edges, _ := t.db.GetEdges(t.agentID, node.ID)
	result := memory.GraphResult{Node: *node, Edges: edges}
	return SilentResult(formatGraphToolResults([]memory.GraphResult{result}))
}

func (t *knowledgeGraphTool) deleteEntity(args map[string]any) *ToolResult {
	label, _ := args["label"].(string)
	name, _ := args["name"].(string)
	if label == "" || name == "" {
		return ErrorResult("label and name are required for delete_entity")
	}

	node, err := t.db.GetNode(t.agentID, label, name)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to find entity: %s", err))
	}
	if node == nil {
		return SilentResult(fmt.Sprintf("Entity [%s] %s not found (nothing to delete)", label, name))
	}

	_ = t.db.RecordStat(t.agentID, "prune", &node.ID, fmt.Sprintf("Manual delete %s:%s", label, name))

	if err := t.db.DeleteNode(node.ID); err != nil {
		return ErrorResult(fmt.Sprintf("Failed to delete entity: %s", err))
	}

	return SilentResult(fmt.Sprintf("Deleted entity [%s] %s and all its relations", label, name))
}

func (t *knowledgeGraphTool) consolidate() *ToolResult {
	duplicates, err := t.db.FindDuplicateNodes(t.agentID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Consolidation failed: %s", err))
	}

	merged := 0
	var details []string

	for _, cluster := range duplicates {
		if len(cluster) < 2 {
			continue
		}
		primaryIdx := 0
		for i, n := range cluster {
			if n.AccessCount > cluster[primaryIdx].AccessCount {
				primaryIdx = i
			}
		}
		primary := cluster[primaryIdx]
		secondaryIDs := make([]int64, 0)
		for i, n := range cluster {
			if i != primaryIdx {
				secondaryIDs = append(secondaryIDs, n.ID)
			}
		}
		if err := t.db.MergeNodes(primary.ID, secondaryIDs); err == nil {
			merged += len(secondaryIDs)
			details = append(details, fmt.Sprintf("Merged %d nodes into %q", len(secondaryIDs), primary.Name))
			_ = t.db.RecordStat(t.agentID, "consolidation", &primary.ID,
				fmt.Sprintf("Merged %d duplicates", len(secondaryIDs)))
		}
	}

	// Resolve conflicting edges
	conflicts, _ := t.db.GetConflictingEdges(t.agentID)
	resolved := 0
	for _, edgeGroup := range conflicts {
		for i := 1; i < len(edgeGroup); i++ {
			if t.db.DeleteEdge(edgeGroup[i].ID) == nil {
				resolved++
			}
		}
	}

	totalNodes := t.db.CountNodes(t.agentID)

	return SilentResult(
		fmt.Sprintf(
			"Consolidation complete: merged %d duplicate nodes, resolved %d edge conflicts. Total nodes: %d.\n%s",
			merged,
			resolved,
			totalNodes,
			strings.Join(details, "\n"),
		),
	)
}

func (t *knowledgeGraphTool) prune(args map[string]any) *ToolResult {
	dryRun, _ := args["dry_run"].(bool)

	maxAge := 90 * 24 * time.Hour
	minAccess := 2
	threshold := 0.1
	halfLife := 30.0

	// Get candidates
	candidates, err := t.db.GetStaleNodes(t.agentID, maxAge, minAccess)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Prune failed: %s", err))
	}

	now := time.Now().UTC()
	var toPrune []int64
	var details []string

	for _, node := range candidates {
		var daysSinceAccess float64
		if node.LastAccessed != nil {
			daysSinceAccess = now.Sub(*node.LastAccessed).Hours() / 24
		} else {
			daysSinceAccess = now.Sub(node.CreatedAt).Hours() / 24
		}
		recencyFactor := 1.0 / (1.0 + daysSinceAccess/halfLife)
		score := float64(node.AccessCount) * recencyFactor

		if score < threshold {
			toPrune = append(toPrune, node.ID)
			details = append(details, fmt.Sprintf("  [%s] %s (score=%.3f, accesses=%d, days=%.0f)",
				node.Label, node.Name, score, node.AccessCount, daysSinceAccess))
		}
	}

	if len(toPrune) == 0 {
		return SilentResult("No stale memories to prune. All knowledge is fresh and relevant.")
	}

	prefix := "Would prune"
	if !dryRun {
		prefix = "Pruned"
		for _, id := range toPrune {
			idCopy := id
			_ = t.db.RecordStat(t.agentID, "prune", &idCopy, "Strategic forgetting via tool")
		}
		if err := t.db.DeleteNodes(toPrune); err != nil {
			return ErrorResult(fmt.Sprintf("Prune delete failed: %s", err))
		}
	}

	return SilentResult(fmt.Sprintf("%s %d stale memories:\n%s", prefix, len(toPrune), strings.Join(details, "\n")))
}

func (t *knowledgeGraphTool) stats() *ToolResult {
	stats, err := t.db.GetNodeStats(t.agentID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Stats failed: %s", err))
	}

	totalNodes := t.db.CountNodes(t.agentID)

	if len(stats) == 0 {
		return SilentResult(fmt.Sprintf("Knowledge graph stats: %d total nodes, no access data yet.", totalNodes))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Knowledge graph: %d total nodes\n\nTop entities by access:\n", totalNodes))
	limit := 20
	if len(stats) < limit {
		limit = len(stats)
	}
	for i := 0; i < limit; i++ {
		s := stats[i]
		sb.WriteString(fmt.Sprintf("  [%s] %s — %d accesses, %d queries, %d hits\n",
			s.Label, s.Name, s.AccessCount, s.QueryCount, s.HitCount))
	}
	return SilentResult(sb.String())
}

func (t *knowledgeGraphTool) maybeConsolidate() {
	t.mu.Lock()
	t.writeCount++
	shouldConsolidate := t.writeCount >= t.consolidateAt
	if shouldConsolidate {
		t.writeCount = 0
	}
	t.mu.Unlock()

	if shouldConsolidate {
		go func() {
			duplicates, err := t.db.FindDuplicateNodes(t.agentID)
			if err != nil || len(duplicates) == 0 {
				return
			}
			for _, cluster := range duplicates {
				if len(cluster) < 2 {
					continue
				}
				primaryIdx := 0
				for i, n := range cluster {
					if n.AccessCount > cluster[primaryIdx].AccessCount {
						primaryIdx = i
					}
				}
				secondaryIDs := make([]int64, 0)
				for i, n := range cluster {
					if i != primaryIdx {
						secondaryIDs = append(secondaryIDs, n.ID)
					}
				}
				_ = t.db.MergeNodes(cluster[primaryIdx].ID, secondaryIDs)
			}
			logger.DebugCF("memory", "Auto-consolidation completed", nil)
		}()
	}
}

// formatGraphToolResults formats graph results for the LLM.
func formatGraphToolResults(results []memory.GraphResult) string {
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
					sb.WriteString(fmt.Sprintf("  → %s → [%s] %s (w=%.2f)\n",
						e.Relation, e.TargetLabel, e.TargetName, e.Weight))
				} else {
					sb.WriteString(fmt.Sprintf("  ← %s ← [%s] %s (w=%.2f)\n",
						e.Relation, e.SourceLabel, e.SourceName, e.Weight))
				}
			}
		}
	}
	return sb.String()
}
