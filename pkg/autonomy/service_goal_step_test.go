package autonomy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGoalPlanResponse_Valid(t *testing.T) {
	input := `{"goal_id": 42, "goal_name": "Deploy stack", "plan": {"steps": [{"description": "Research", "depends_on": []}, {"description": "Build", "depends_on": [0]}]}}`
	resp, err := parseGoalPlanResponse(input)
	require.NoError(t, err)
	assert.Equal(t, int64(42), resp.GoalID)
	assert.Equal(t, "Deploy stack", resp.GoalName)
	require.Len(t, resp.Plan.Steps, 2)
	assert.Equal(t, "Research", resp.Plan.Steps[0].Description)
	assert.Empty(t, resp.Plan.Steps[0].DependsOn)
	assert.Equal(t, "Build", resp.Plan.Steps[1].Description)
	assert.Equal(t, []int{0}, resp.Plan.Steps[1].DependsOn)
}

func TestParseGoalPlanResponse_CodeFenced(t *testing.T) {
	input := "```json\n{\"goal_id\": 1, \"goal_name\": \"Test\", \"plan\": {\"steps\": [{\"description\": \"Do it\", \"depends_on\": []}]}}\n```"
	resp, err := parseGoalPlanResponse(input)
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.GoalID)
	require.Len(t, resp.Plan.Steps, 1)
}

func TestParseGoalPlanResponse_TopLevelSteps(t *testing.T) {
	input := `{"goal_id": 1, "goal_name": "Test", "steps": [{"description": "Step 1", "depends_on": []}]}`
	resp, err := parseGoalPlanResponse(input)
	require.NoError(t, err)
	require.Len(t, resp.Plan.Steps, 1)
	assert.Equal(t, "Step 1", resp.Plan.Steps[0].Description)
}

func TestParseGoalPlanResponse_Invalid(t *testing.T) {
	_, err := parseGoalPlanResponse("{bad json}")
	require.Error(t, err)
}

func TestParseGoalPlanResponse_NoSteps(t *testing.T) {
	input := `{"goal_id": 1, "goal_name": "Test", "plan": {"steps": []}}`
	_, err := parseGoalPlanResponse(input)
	require.Error(t, err)
}

func TestParseGoalResultResponse_Valid(t *testing.T) {
	input := `{"summary": "Done", "artifacts": ["/a.txt"], "next_steps": ["Deploy it"]}`
	result, err := parseGoalResultResponse(input)
	require.NoError(t, err)
	assert.Equal(t, "Done", result.Summary)
	assert.Equal(t, []string{"/a.txt"}, result.Artifacts)
	assert.Equal(t, []string{"Deploy it"}, result.NextSteps)
}

func TestParseGoalResultResponse_Invalid(t *testing.T) {
	_, err := parseGoalResultResponse("{bad}")
	require.Error(t, err)
}
