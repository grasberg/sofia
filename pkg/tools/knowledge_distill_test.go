package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grasberg/sofia/pkg/memory"
)

func TestDistillKnowledgeTool_Execute(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "memory.db")
	db, err := memory.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open memory db: %v", err)
	}
	defer db.Close()

	tool := NewDistillKnowledgeTool(db, "test_agent")

	if tool.Name() != "distill_knowledge" {
		t.Errorf("expected distill_knowledge, got %s", tool.Name())
	}

	// 1. Create a source node
	srcID, err := db.UpsertNode("test_agent", "Error", "Test Error", `{"msg": "test"}`)
	if err != nil {
		t.Fatalf("Failed to create source node: %v", err)
	}

	// 2. Run the tool
	args := map[string]any{
		"topic":           "Test Insight",
		"type":            "Insight",
		"learnings":       "Always test your code.",
		"source_node_ids": []any{srcID},
	}

	res := tool.Execute(context.Background(), args)
	if !res.Silent {
		t.Fatalf("expected silent result, got error: %v, message: %s", res.IsError, res.ForLLM)
	}

	if !strings.Contains(res.ForLLM, "Successfully distilled knowledge") {
		t.Errorf("unexpected output: %s", res.ForLLM)
	}
	if !strings.Contains(res.ForLLM, "Linked to 1 source nodes") {
		t.Errorf("unexpected output (should link source): %s", res.ForLLM)
	}

	// 3. Verify node was created
	node, err := db.GetNode("test_agent", "Insight", "Test Insight")
	if err != nil || node == nil {
		t.Fatalf("Failed to get created insight node: %v", err)
	}

	if !strings.Contains(node.Properties, "Always test your code.") {
		t.Errorf("unexpected properties: %s", node.Properties)
	}

	// 4. Verify edge was created
	edges, err := db.GetEdges("test_agent", node.ID)
	if err != nil {
		t.Fatalf("Failed to get edges: %v", err)
	}

	if len(edges) != 1 {
		t.Fatalf("Expected 1 edge, got %d", len(edges))
	}

	if edges[0].Relation != "distilled_from" || edges[0].TargetID != srcID {
		t.Errorf("Unexpected edge: %+v", edges[0])
	}

	// Test missing args
	missingArgs := map[string]any{
		"topic": "missing",
	}
	res2 := tool.Execute(context.Background(), missingArgs)
	if !strings.Contains(res2.ForLLM, "are required") {
		t.Errorf("expected error result for missing args, got error: %v, message: %s", res2.IsError, res2.ForLLM)
	}
}
