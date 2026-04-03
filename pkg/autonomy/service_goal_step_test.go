package autonomy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
)

func TestBuildGoalsSummary(t *testing.T) {
	summary, refs := buildGoalsSummary([]any{
		map[string]any{
			"id":          float64(7),
			"name":        "Goal 7",
			"description": "Ship the feature",
			"priority":    "high",
		},
		map[string]any{"name": "missing-id"},
	})

	require.Len(t, refs, 1)
	assert.Equal(t, int64(7), refs[0].id)
	assert.Equal(t, "Goal 7", refs[0].name)
	assert.Contains(t, summary, "[ID:7] Goal 7")
	assert.Contains(t, summary, "Ship the feature")
}

func TestParseGoalPlannerResponse(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		wantNoAction bool
		wantComplete bool
		wantGoalID   int64
		wantStep     string
		wantErr      bool
	}{
		{name: "no action", content: "NO_ACTION", wantNoAction: true},
		{name: "goal complete", content: "GOAL_COMPLETE: 42", wantComplete: true, wantGoalID: 42},
		{
			name:       "json fenced",
			content:    "```json\n{\"goal_id\": 5, \"goal_name\": \"Goal\", \"step\": \"Do work\"}\n```",
			wantGoalID: 5,
			wantStep:   "Do work",
		},
		{name: "invalid json", content: "{not json}", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := parseGoalPlannerResponse(tt.content)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantNoAction, decision.NoAction)
			assert.Equal(t, tt.wantComplete, decision.MarkComplete)
			assert.Equal(t, tt.wantGoalID, maxInt64(decision.CompleteGoalID, decision.Plan.GoalID))
			if tt.wantStep != "" {
				assert.Equal(t, tt.wantStep, decision.Plan.Step)
			}
		})
	}
}

func TestExecuteOneGoalStep_GoalComplete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	goalAny, err := gm.AddGoal("agent-1", "Learn Rust", "Read the book", "high")
	require.NoError(t, err)

	var goal Goal
	require.NoError(t, decodeGoal(goalAny, &goal))

	svc := NewService(
		&config.AutonomyConfig{Enabled: true, Goals: true},
		db,
		bus.NewMessageBus(),
		&MockProvider{ResponseContent: fmt.Sprintf("GOAL_COMPLETE: %d", goal.ID)},
		nil,
		"agent-1",
		"mock-model",
		"test-workspace",
		nil,
	)

	outcome := svc.executeOneGoalStep(context.Background(), gm, []any{goalAny}, 0)
	assert.Equal(t, stepResultDone, outcome)

	activeGoals, err := gm.ListActiveGoals("agent-1")
	require.NoError(t, err)
	assert.Len(t, activeGoals, 0)
}

func TestExecuteOneGoalStep_UsesTaskRunner(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	goalAny, err := gm.AddGoal("agent-1", "Ship Feature", "Implement the endpoint", "high")
	require.NoError(t, err)

	var goal Goal
	require.NoError(t, decodeGoal(goalAny, &goal))

	provider := &MockProvider{ResponseContent: fmt.Sprintf(
		"{\"goal_id\": %d, \"goal_name\": \"Ship Feature\", \"step\": \"Write the endpoint\"}",
		goal.ID,
	)}
	svc := NewService(
		&config.AutonomyConfig{Enabled: true, Goals: true},
		db,
		bus.NewMessageBus(),
		provider,
		nil,
		"agent-1",
		"mock-model",
		"test-workspace",
		nil,
	)

	called := false
	var capturedSessionKey string
	var capturedTask string
	svc.SetTaskRunner(
		func(ctx context.Context, agentID, sessionKey, task, originChannel, originChatID string) (string, error) {
			called = true
			capturedSessionKey = sessionKey
			capturedTask = task
			return "task finished", nil
		},
	)

	outcome := svc.executeOneGoalStep(context.Background(), gm, []any{goalAny}, 0)
	assert.Equal(t, stepResultSuccess, outcome)
	assert.True(t, called)
	assert.Equal(t, fmt.Sprintf("goal:%d", goal.ID), capturedSessionKey)
	assert.Contains(t, capturedTask, "Your next step: Write the endpoint")
	assert.True(t, strings.Contains(capturedTask, "Ship Feature"))
}

func decodeGoal(src any, dst *Goal) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, dst)
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
