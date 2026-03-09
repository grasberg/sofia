package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/grasberg/sofia/pkg/memory"
)

// DistillKnowledgeTool compresses learned experiences into reusable knowledge nodes.
type DistillKnowledgeTool struct {
	db      *memory.MemoryDB
	agentID string
}

// NewDistillKnowledgeTool creates a new DistillKnowledgeTool.
func NewDistillKnowledgeTool(db *memory.MemoryDB, agentID string) *DistillKnowledgeTool {
	return &DistillKnowledgeTool{
		db:      db,
		agentID: agentID,
	}
}

func (t *DistillKnowledgeTool) Name() string { return "distill_knowledge" }

func (t *DistillKnowledgeTool) Description() string {
	return "Compress and summarize learned experiences or patterns into a persistent, reusable knowledge graph entity (Insight or Pattern). Use this after solving a complex problem to store the high-level takeaways."
}

func (t *DistillKnowledgeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"topic": map[string]any{
				"type":        "string",
				"description": "Short, specific name for the distilled knowledge (e.g., 'Go Memory Leaks', 'React Performance').",
			},
			"type": map[string]any{
				"type":        "string",
				"enum":        []string{"Insight", "Pattern"},
				"description": "Entity label for the distilled knowledge. Insight for facts/learnings, Pattern for reusable solutions.",
			},
			"learnings": map[string]any{
				"type":        "string",
				"description": "Markdown summary of the core insights, gotchas, or steps to remember.",
			},
			"source_node_ids": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "integer"},
				"description": "Optional list of existing knowledge graph node IDs that informed this distillation.",
			},
		},
		"required": []string{"topic", "type", "learnings"},
	}
}

func (t *DistillKnowledgeTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.db == nil {
		return ErrorResult("Knowledge graph not available: database not initialized")
	}

	topic, _ := args["topic"].(string)
	entityType, _ := args["type"].(string)
	learnings, _ := args["learnings"].(string)

	if topic == "" || entityType == "" || learnings == "" {
		return ErrorResult("topic, type, and learnings are required")
	}

	// Clean learnings for JSON properties
	cleanLearnings := strings.ReplaceAll(learnings, "\n", "\\n")
	cleanLearnings = strings.ReplaceAll(cleanLearnings, "\"", "\\\"")
	propsJSON := fmt.Sprintf(`{"summary": "%s"}`, cleanLearnings)

	insightID, err := t.db.UpsertNode(t.agentID, entityType, topic, propsJSON)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to store distilled knowledge: %s", err))
	}

	_ = t.db.RecordStat(t.agentID, "distill", &insightID, fmt.Sprintf("Distilled %s:%s", entityType, topic))

	resMsg := fmt.Sprintf("Successfully distilled knowledge into [%s] %q (ID=%d).", entityType, topic, insightID)

	// Link source nodes if provided
	var linked int
	if srcIDs, ok := args["source_node_ids"].([]any); ok && len(srcIDs) > 0 {
		for _, rawID := range srcIDs {
			var id int64
			switch v := rawID.(type) {
			case float64:
				id = int64(v)
			case int:
				id = int64(v)
			case int64:
				id = v
			}

			if id > 0 {
				if err := t.db.UpsertEdge(t.agentID, insightID, id, "distilled_from", 1.0, "{}"); err == nil {
					linked++
				}
			}
		}
	}

	if linked > 0 {
		resMsg += fmt.Sprintf("\nLinked to %d source nodes.", linked)
	}

	return SilentResult(resMsg)
}
