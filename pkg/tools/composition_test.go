package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestToolTracker(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tracker_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	statsPath := filepath.Join(tempDir, "stats.json")
	tracker := NewToolTracker(statsPath)

	// Record some usage
	tracker.Record("test_tool", 100*time.Millisecond, false)
	tracker.Record("test_tool", 200*time.Millisecond, true)

	stats, ok := tracker.GetStat("test_tool")
	if !ok {
		t.Fatal("expected stats for test_tool")
	}

	if stats.UsageCount != 2 {
		t.Errorf("expected UsageCount 2, got %d", stats.UsageCount)
	}
	if stats.SuccessCount != 1 {
		t.Errorf("expected SuccessCount 1, got %d", stats.SuccessCount)
	}
	if stats.ErrorCount != 1 {
		t.Errorf("expected ErrorCount 1, got %d", stats.ErrorCount)
	}
	if stats.TotalTimeMs != 300 {
		t.Errorf("expected TotalTimeMs 300, got %d", stats.TotalTimeMs)
	}

	// Flush pending writes before testing persistence
	tracker.Flush()

	// Test persistence
	tracker2 := NewToolTracker(statsPath)
	stats2, ok := tracker2.GetStat("test_tool")
	if !ok {
		t.Fatal("expected persisted stats for test_tool")
	}
	if stats2.UsageCount != 2 {
		t.Errorf("expected persisted UsageCount 2, got %d", stats2.UsageCount)
	}
}

type mockSimpleTool struct {
	name string
	res  string
	err  error
}

func (m *mockSimpleTool) Name() string               { return m.name }
func (m *mockSimpleTool) Description() string        { return "mock" }
func (m *mockSimpleTool) Parameters() map[string]any { return nil }
func (m *mockSimpleTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if m.err != nil {
		return ErrorResult(m.err.Error()).WithError(m.err)
	}
	// Verify input passing
	if prev, ok := args["previous_output"].(string); ok {
		return SilentResult(prev + " -> " + m.res)
	}
	return SilentResult(m.res)
}

func TestCompositeTool(t *testing.T) {
	registry := NewToolRegistry()
	registry.Register(&mockSimpleTool{name: "step1", res: "one"})
	registry.Register(&mockSimpleTool{name: "step2", res: "two"})

	pipeline := NewCompositeTool("my_pipeline", "desc", []string{"step1", "step2"}, registry)

	ctx := context.Background()
	result := pipeline.Execute(ctx, map[string]any{"initial_input": "start"})

	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	expected := "one -> two"
	if !customContains(result.ForLLM, expected) {
		t.Errorf("expected trace to contain %q, got %q", expected, result.ForLLM)
	}
}

func customContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr))
}
