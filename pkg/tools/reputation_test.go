package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/reputation"
)

func newTestReputationSetup(t *testing.T) *ReputationTool {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck

	mgr := reputation.NewManager(db)
	return NewReputationTool(mgr)
}

func TestReputationToolMeta(t *testing.T) {
	tool := newTestReputationSetup(t)
	assert.Equal(t, "agent_reputation", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.NotNil(t, tool.Parameters())
}

func TestReputationStatsEmpty(t *testing.T) {
	tool := newTestReputationSetup(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation": "stats",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "No reputation data")
}

func TestReputationStatsForAgent(t *testing.T) {
	tool := newTestReputationSetup(t)

	// Record some outcomes first.
	tool.Execute(context.Background(), map[string]any{
		"operation": "record",
		"agent_id":  "coder",
		"task":      "Fix login bug",
		"success":   true,
	})

	r := tool.Execute(context.Background(), map[string]any{
		"operation": "stats",
		"agent_id":  "coder",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "coder")
	assert.Contains(t, r.ForLLM, "Tasks: 1")
}

func TestReputationRecord(t *testing.T) {
	tool := newTestReputationSetup(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation": "record",
		"agent_id":  "coder",
		"task":      "Write tests",
		"success":   true,
		"category":  "coding",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "Recorded success")
	assert.Contains(t, r.ForLLM, "coder")
}

func TestReputationRecordFailure(t *testing.T) {
	tool := newTestReputationSetup(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation": "record",
		"agent_id":  "coder",
		"task":      "Deploy app",
		"success":   false,
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "Recorded failure")
}

func TestReputationRecordMissingAgentID(t *testing.T) {
	tool := newTestReputationSetup(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation": "record",
		"task":      "something",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "agent_id is required")
}

func TestReputationRecordMissingTask(t *testing.T) {
	tool := newTestReputationSetup(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation": "record",
		"agent_id":  "coder",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "task is required")
}

func TestReputationCategories(t *testing.T) {
	tool := newTestReputationSetup(t)

	tool.Execute(context.Background(), map[string]any{
		"operation": "record",
		"agent_id":  "coder",
		"task":      "Fix bug",
		"success":   true,
		"category":  "coding",
	})
	tool.Execute(context.Background(), map[string]any{
		"operation": "record",
		"agent_id":  "coder",
		"task":      "Deploy app",
		"success":   true,
		"category":  "devops",
	})

	r := tool.Execute(context.Background(), map[string]any{
		"operation": "categories",
		"agent_id":  "coder",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "coding")
	assert.Contains(t, r.ForLLM, "devops")
}

func TestReputationCategoriesMissingAgent(t *testing.T) {
	tool := newTestReputationSetup(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation": "categories",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "agent_id is required")
}

func TestReputationHistory(t *testing.T) {
	tool := newTestReputationSetup(t)

	for i := 0; i < 5; i++ {
		tool.Execute(context.Background(), map[string]any{
			"operation": "record",
			"agent_id":  "coder",
			"task":      "task",
			"success":   true,
		})
	}

	r := tool.Execute(context.Background(), map[string]any{
		"operation": "history",
		"agent_id":  "coder",
		"limit":     float64(3),
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "3 outcome(s)")
}

func TestReputationHistoryMissingAgent(t *testing.T) {
	tool := newTestReputationSetup(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation": "history",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "agent_id is required")
}

func TestReputationRank(t *testing.T) {
	tool := newTestReputationSetup(t)

	// Record outcomes for two agents.
	for i := 0; i < 10; i++ {
		tool.Execute(context.Background(), map[string]any{
			"operation": "record",
			"agent_id":  "coder",
			"task":      "code task",
			"success":   true,
			"category":  "coding",
		})
		tool.Execute(context.Background(), map[string]any{
			"operation": "record",
			"agent_id":  "writer",
			"task":      "code task",
			"success":   false,
			"category":  "coding",
		})
	}

	r := tool.Execute(context.Background(), map[string]any{
		"operation": "rank",
		"category":  "coding",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "coder")
	assert.Contains(t, r.ForLLM, "writer")
}

func TestReputationRankMissingCategory(t *testing.T) {
	tool := newTestReputationSetup(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation": "rank",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "category is required")
}

func TestReputationScore(t *testing.T) {
	tool := newTestReputationSetup(t)

	// Record and score an outcome.
	tool.Execute(context.Background(), map[string]any{
		"operation": "record",
		"agent_id":  "coder",
		"task":      "Write tests",
		"success":   true,
	})

	r := tool.Execute(context.Background(), map[string]any{
		"operation":   "score",
		"outcome_id":  float64(1),
		"score_value": float64(0.95),
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "Scored outcome #1")
}

func TestReputationScoreMissingOutcomeID(t *testing.T) {
	tool := newTestReputationSetup(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation":   "score",
		"score_value": float64(0.5),
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "outcome_id is required")
}

func TestReputationScoreMissingValue(t *testing.T) {
	tool := newTestReputationSetup(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation":  "score",
		"outcome_id": float64(1),
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "score_value is required")
}

func TestReputationUnknownOperation(t *testing.T) {
	tool := newTestReputationSetup(t)
	r := tool.Execute(context.Background(), map[string]any{
		"operation": "invalid",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "unknown operation")
}
