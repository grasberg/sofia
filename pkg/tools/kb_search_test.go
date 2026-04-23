package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/grasberg/sofia/pkg/memory"
)

func openKBTestDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestKBSearchTool_BasicFields(t *testing.T) {
	tool := NewKBSearchTool(nil, "a")
	if tool.Name() != "kb_search" {
		t.Errorf("Name = %q", tool.Name())
	}
	if !strings.Contains(strings.ToLower(tool.Description()), "knowledge") {
		t.Errorf("Description should mention knowledge base: %q", tool.Description())
	}
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("params type = %v", params["type"])
	}
}

func TestKBSearchTool_MissingQueryIsError(t *testing.T) {
	tool := NewKBSearchTool(openKBTestDB(t), "a")
	res := tool.Execute(context.Background(), map[string]any{})
	if !res.IsError {
		t.Error("empty args should produce an error")
	}
}

func TestKBSearchTool_NilDBIsError(t *testing.T) {
	tool := NewKBSearchTool(nil, "a")
	res := tool.Execute(context.Background(), map[string]any{"query": "test"})
	if !res.IsError {
		t.Error("nil DB should produce an error")
	}
}

func TestKBSearchTool_NoMatchReturnsHint(t *testing.T) {
	db := openKBTestDB(t)
	tool := NewKBSearchTool(db, "a")
	res := tool.Execute(context.Background(), map[string]any{"query": "anything"})
	if res.IsError {
		t.Errorf("empty store should NOT be an error: %s", res.ForLLM)
	}
	if !strings.Contains(strings.ToLower(res.ForLLM), "no kb entries") {
		t.Errorf("expected 'no kb entries' hint, got %q", res.ForLLM)
	}
}

func TestKBSearchTool_HitsFormatInLLMOutput(t *testing.T) {
	db := openKBTestDB(t)
	_, err := db.UpsertKBEntry("a", "How do I reset my password",
		"Go to Settings → Security → Reset.", "", []string{"account"})
	if err != nil {
		t.Fatal(err)
	}
	tool := NewKBSearchTool(db, "a")
	res := tool.Execute(context.Background(), map[string]any{
		"query": "password reset",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	if !strings.Contains(res.ForLLM, "Settings → Security") {
		t.Errorf("result body missing: %q", res.ForLLM)
	}
	if res.StructuredData == nil {
		t.Error("StructuredData should be populated")
	}
}

func TestKBSearchTool_TopKClamp(t *testing.T) {
	db := openKBTestDB(t)
	_, _ = db.UpsertKBEntry("a", "q1 password", "a1", "", nil)
	_, _ = db.UpsertKBEntry("a", "q2 password", "a2", "", nil)
	_, _ = db.UpsertKBEntry("a", "q3 password", "a3", "", nil)

	tool := NewKBSearchTool(db, "a")
	res := tool.Execute(context.Background(), map[string]any{
		"query": "password",
		"top_k": float64(2),
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	hits, ok := res.StructuredData.([]memory.KBEntry)
	if !ok {
		t.Fatalf("StructuredData wrong type: %T", res.StructuredData)
	}
	if len(hits) != 2 {
		t.Errorf("top_k=2 should cap hits to 2, got %d", len(hits))
	}
}

func TestKBSearchTool_CrossAgentWhenEmptyScope(t *testing.T) {
	db := openKBTestDB(t)
	_, _ = db.UpsertKBEntry("agent-1", "q for one password", "a1", "", nil)
	_, _ = db.UpsertKBEntry("agent-2", "q for two password", "a2", "", nil)

	// Empty agent scope should find both.
	toolShared := NewKBSearchTool(db, "")
	res := toolShared.Execute(context.Background(), map[string]any{"query": "password", "top_k": float64(5)})
	hits := res.StructuredData.([]memory.KBEntry)
	if len(hits) != 2 {
		t.Errorf("shared scope should see both entries, got %d", len(hits))
	}

	// Scoped to agent-1 only: one hit.
	tool1 := NewKBSearchTool(db, "agent-1")
	res = tool1.Execute(context.Background(), map[string]any{"query": "password"})
	hits = res.StructuredData.([]memory.KBEntry)
	if len(hits) != 1 {
		t.Errorf("agent-1 scope should see 1 entry, got %d", len(hits))
	}
}
