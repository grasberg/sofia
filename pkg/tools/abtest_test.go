package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/abtest"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
)

type abMockProvider struct{}

func (p *abMockProvider) Chat(
	_ context.Context,
	msgs []providers.Message,
	_ []providers.ToolDefinition,
	model string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content: "response from " + model,
		Usage: &providers.UsageInfo{
			PromptTokens: 5, CompletionTokens: 10,
		},
	}, nil
}

func (p *abMockProvider) GetDefaultModel() string { return "test-model" }

func newABTestTool(t *testing.T) *ABTestTool {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mgr := abtest.NewManager(db)
	return NewABTestTool(mgr, &abMockProvider{}, "test-model")
}

func TestABTestToolName(t *testing.T) {
	tool := newABTestTool(t)
	assert.Equal(t, "ab_test", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.Parameters())
}

func TestABTestToolUnknownOperation(t *testing.T) {
	tool := newABTestTool(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation": "invalid",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "unknown operation")
}

func TestABTestToolCreate(t *testing.T) {
	tool := newABTestTool(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation":   "create",
		"experiment":  "test-exp",
		"description": "Test experiment",
		"variants": map[string]any{
			"low-temp":  map[string]any{"temperature": 0.2},
			"high-temp": map[string]any{"temperature": 0.9},
		},
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "Created experiment")
	assert.Contains(t, r.ForLLM, "2 variants")
}

func TestABTestToolCreateMissingName(t *testing.T) {
	tool := newABTestTool(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation": "create",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "experiment name is required")
}

func TestABTestToolCreateTooFewVariants(t *testing.T) {
	tool := newABTestTool(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation":  "create",
		"experiment": "bad",
		"variants":   map[string]any{"only-one": map[string]any{}},
	})
	assert.True(t, r.IsError)
}

func TestABTestToolRun(t *testing.T) {
	tool := newABTestTool(t)

	// Create first.
	tool.Execute(context.Background(), map[string]any{
		"operation":  "create",
		"experiment": "run-test",
		"variants": map[string]any{
			"a": map[string]any{"model": "model-a"},
			"b": map[string]any{"model": "model-b"},
		},
	})

	r := tool.Execute(context.Background(), map[string]any{
		"operation":  "run",
		"experiment": "run-test",
		"prompt":     "Hello",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "Ran 2 trials")
	assert.Contains(t, r.ForLLM, "trial #")
}

func TestABTestToolRunMissingPrompt(t *testing.T) {
	tool := newABTestTool(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation":  "run",
		"experiment": "test",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "prompt is required")
}

func TestABTestToolScore(t *testing.T) {
	tool := newABTestTool(t)

	tool.Execute(context.Background(), map[string]any{
		"operation":  "create",
		"experiment": "score-test",
		"variants": map[string]any{
			"a": map[string]any{},
			"b": map[string]any{},
		},
	})
	tool.Execute(context.Background(), map[string]any{
		"operation":  "run",
		"experiment": "score-test",
		"prompt":     "test",
	})

	r := tool.Execute(context.Background(), map[string]any{
		"operation": "score",
		"trial_id":  float64(1),
		"score":     0.85,
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "Scored trial #1 with 0.85")
}

func TestABTestToolScoreMissingArgs(t *testing.T) {
	tool := newABTestTool(t)

	r := tool.Execute(context.Background(), map[string]any{
		"operation": "score",
	})
	assert.True(t, r.IsError)

	r = tool.Execute(context.Background(), map[string]any{
		"operation": "score",
		"trial_id":  float64(1),
	})
	assert.True(t, r.IsError)
}

func TestABTestToolAnalyze(t *testing.T) {
	tool := newABTestTool(t)

	tool.Execute(context.Background(), map[string]any{
		"operation":  "create",
		"experiment": "analyze-test",
		"variants": map[string]any{
			"a": map[string]any{},
			"b": map[string]any{},
		},
	})
	tool.Execute(context.Background(), map[string]any{
		"operation":  "run",
		"experiment": "analyze-test",
		"prompt":     "test",
	})

	r := tool.Execute(context.Background(), map[string]any{
		"operation":  "analyze",
		"experiment": "analyze-test",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "Analysis for")
	assert.Contains(t, r.ForLLM, "Recommendation:")
}

func TestABTestToolList(t *testing.T) {
	tool := newABTestTool(t)

	// Empty list.
	r := tool.Execute(context.Background(), map[string]any{
		"operation": "list",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "No experiments")

	// After creating.
	tool.Execute(context.Background(), map[string]any{
		"operation":  "create",
		"experiment": "listed",
		"variants": map[string]any{
			"a": map[string]any{},
			"b": map[string]any{},
		},
	})
	r = tool.Execute(context.Background(), map[string]any{
		"operation": "list",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "listed")
}

func TestABTestToolConclude(t *testing.T) {
	tool := newABTestTool(t)

	tool.Execute(context.Background(), map[string]any{
		"operation":  "create",
		"experiment": "conclude-test",
		"variants": map[string]any{
			"a": map[string]any{},
			"b": map[string]any{},
		},
	})

	r := tool.Execute(context.Background(), map[string]any{
		"operation":  "conclude",
		"experiment": "conclude-test",
		"winner":     "a",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "concluded")
	assert.Contains(t, r.ForLLM, "Winner: a")
}

func TestABTestToolConcludeMissingArgs(t *testing.T) {
	tool := newABTestTool(t)

	r := tool.Execute(context.Background(), map[string]any{
		"operation": "conclude",
	})
	assert.True(t, r.IsError)

	r = tool.Execute(context.Background(), map[string]any{
		"operation":  "conclude",
		"experiment": "test",
	})
	assert.True(t, r.IsError)
}

func TestABTestToolDelete(t *testing.T) {
	tool := newABTestTool(t)

	tool.Execute(context.Background(), map[string]any{
		"operation":  "create",
		"experiment": "del-test",
		"variants": map[string]any{
			"a": map[string]any{},
			"b": map[string]any{},
		},
	})

	r := tool.Execute(context.Background(), map[string]any{
		"operation":  "delete",
		"experiment": "del-test",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "Deleted experiment")
}
