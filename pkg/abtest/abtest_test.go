package abtest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
)

// mockProvider implements providers.LLMProvider for testing.
type mockProvider struct {
	responses map[string]string // model -> response
	usage     *providers.UsageInfo
	err       error
}

func (m *mockProvider) Chat(
	_ context.Context,
	_ []providers.Message,
	_ []providers.ToolDefinition,
	model string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	resp := &providers.LLMResponse{
		Content: m.responses[model],
		Usage:   m.usage,
	}
	if resp.Content == "" {
		resp.Content = "default response"
	}
	return resp, nil
}

func (m *mockProvider) GetDefaultModel() string {
	return "test-model"
}

func newTestDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck
	return db
}

func TestCreateExperiment(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	temp1 := 0.3
	temp2 := 0.9
	exp, err := mgr.CreateExperiment("test-exp", "Testing temps",
		map[string]VariantConfig{
			"low-temp":  {Temperature: &temp1},
			"high-temp": {Temperature: &temp2},
		})
	require.NoError(t, err)
	assert.Equal(t, "test-exp", exp.Name)
	assert.Equal(t, "active", exp.Status)
	assert.Len(t, exp.Variants, 2)
}

func TestCreateExperimentRequiresMinVariants(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	_, err := mgr.CreateExperiment("bad", "one variant",
		map[string]VariantConfig{"only": {}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 2 variants")
}

func TestCreateExperimentDuplicateName(t *testing.T) {
	mgr := NewManager(newTestDB(t))
	variants := map[string]VariantConfig{"a": {}, "b": {}}

	_, err := mgr.CreateExperiment("dup", "first", variants)
	require.NoError(t, err)

	_, err = mgr.CreateExperiment("dup", "second", variants)
	assert.Error(t, err)
}

func TestGetExperiment(t *testing.T) {
	mgr := NewManager(newTestDB(t))
	variants := map[string]VariantConfig{
		"v1": {Model: "model-a"},
		"v2": {Model: "model-b"},
	}

	_, err := mgr.CreateExperiment("get-test", "desc", variants)
	require.NoError(t, err)

	exp, err := mgr.GetExperiment("get-test")
	require.NoError(t, err)
	assert.Equal(t, "get-test", exp.Name)
	assert.Equal(t, "desc", exp.Description)
	assert.Len(t, exp.Variants, 2)
}

func TestGetExperimentNotFound(t *testing.T) {
	mgr := NewManager(newTestDB(t))
	_, err := mgr.GetExperiment("nonexistent")
	assert.Error(t, err)
}

func TestListExperiments(t *testing.T) {
	mgr := NewManager(newTestDB(t))
	variants := map[string]VariantConfig{"a": {}, "b": {}}

	_, err := mgr.CreateExperiment("exp1", "", variants)
	require.NoError(t, err)
	_, err = mgr.CreateExperiment("exp2", "", variants)
	require.NoError(t, err)

	exps, err := mgr.ListExperiments()
	require.NoError(t, err)
	assert.Len(t, exps, 2)
}

func TestRunTrial(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	variants := map[string]VariantConfig{
		"model-a": {Model: "gpt-4"},
		"model-b": {Model: "claude"},
	}
	_, err := mgr.CreateExperiment("trial-test", "test", variants)
	require.NoError(t, err)

	prov := &mockProvider{
		responses: map[string]string{
			"gpt-4":  "response from gpt-4",
			"claude": "response from claude",
		},
		usage: &providers.UsageInfo{
			PromptTokens:     10,
			CompletionTokens: 20,
		},
	}

	trials, err := mgr.RunTrial(
		context.Background(), "trial-test", "Hello",
		prov, "default-model",
	)
	require.NoError(t, err)
	assert.Len(t, trials, 2)

	for _, trial := range trials {
		assert.Equal(t, "Hello", trial.Prompt)
		assert.NotEmpty(t, trial.Response)
		assert.Greater(t, trial.ID, int64(0))
		assert.Equal(t, 10, trial.TokensIn)
		assert.Equal(t, 20, trial.TokensOut)
	}
}

func TestRunTrialOnConcludedExperiment(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	variants := map[string]VariantConfig{"a": {}, "b": {}}
	_, err := mgr.CreateExperiment("concluded", "", variants)
	require.NoError(t, err)

	err = mgr.ConcludeExperiment("concluded", "a")
	require.NoError(t, err)

	_, err = mgr.RunTrial(
		context.Background(), "concluded", "test",
		&mockProvider{}, "m",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not active")
}

func TestRunTrialWithPromptModifiers(t *testing.T) {
	db := newTestDB(t)
	mgr := NewManager(db)

	variants := map[string]VariantConfig{
		"plain":    {},
		"enhanced": {PromptPrefix: "Think step by step.", PromptSuffix: "Be concise."},
	}
	_, err := mgr.CreateExperiment("prefix-test", "", variants)
	require.NoError(t, err)

	prov := &mockProvider{responses: map[string]string{}}
	trials, err := mgr.RunTrial(
		context.Background(), "prefix-test", "What is 2+2?",
		prov, "model",
	)
	require.NoError(t, err)
	assert.Len(t, trials, 2)
}

func TestScoreTrial(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	variants := map[string]VariantConfig{"a": {}, "b": {}}
	_, err := mgr.CreateExperiment("score-test", "", variants)
	require.NoError(t, err)

	trials, err := mgr.RunTrial(
		context.Background(), "score-test", "test",
		&mockProvider{}, "m",
	)
	require.NoError(t, err)
	require.Len(t, trials, 2)

	err = mgr.ScoreTrial(trials[0].ID, 0.8)
	require.NoError(t, err)

	err = mgr.ScoreTrial(trials[1].ID, 0.6)
	require.NoError(t, err)
}

func TestScoreTrialOutOfRange(t *testing.T) {
	mgr := NewManager(newTestDB(t))
	assert.Error(t, mgr.ScoreTrial(1, -0.1))
	assert.Error(t, mgr.ScoreTrial(1, 1.1))
}

func TestAnalyze(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	variants := map[string]VariantConfig{
		"fast": {Model: "fast-model"},
		"slow": {Model: "slow-model"},
	}
	_, err := mgr.CreateExperiment("analyze-test", "", variants)
	require.NoError(t, err)

	prov := &mockProvider{
		responses: map[string]string{
			"fast-model": "quick answer",
			"slow-model": "detailed answer",
		},
		usage: &providers.UsageInfo{
			PromptTokens: 5, CompletionTokens: 10,
		},
	}

	// Run multiple trials.
	for i := 0; i < 3; i++ {
		trials, tErr := mgr.RunTrial(
			context.Background(), "analyze-test", "test prompt",
			prov, "default",
		)
		require.NoError(t, tErr)

		// Score: fast gets 0.7, slow gets 0.9.
		for _, trial := range trials {
			score := 0.7
			if trial.VariantName == "slow" {
				score = 0.9
			}
			require.NoError(t, mgr.ScoreTrial(trial.ID, score))
		}
	}

	analysis, err := mgr.Analyze("analyze-test")
	require.NoError(t, err)
	assert.Equal(t, "analyze-test", analysis.ExperimentName)
	assert.Equal(t, 6, analysis.TotalTrials)
	assert.Len(t, analysis.Stats, 2)
	assert.NotEmpty(t, analysis.Recommendation)
}

func TestAnalyzeNoScores(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	variants := map[string]VariantConfig{"a": {}, "b": {}}
	_, err := mgr.CreateExperiment("no-scores", "", variants)
	require.NoError(t, err)

	_, err = mgr.RunTrial(
		context.Background(), "no-scores", "test",
		&mockProvider{}, "m",
	)
	require.NoError(t, err)

	analysis, err := mgr.Analyze("no-scores")
	require.NoError(t, err)
	assert.Contains(t, analysis.Recommendation, "No scores yet")
}

func TestConcludeExperiment(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	variants := map[string]VariantConfig{"a": {}, "b": {}}
	_, err := mgr.CreateExperiment("conclude-test", "", variants)
	require.NoError(t, err)

	err = mgr.ConcludeExperiment("conclude-test", "a")
	require.NoError(t, err)

	exp, err := mgr.GetExperiment("conclude-test")
	require.NoError(t, err)
	assert.Equal(t, "concluded", exp.Status)
	assert.Equal(t, "a", exp.Winner)
	assert.NotNil(t, exp.ConcludedAt)
}

func TestDeleteExperiment(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	variants := map[string]VariantConfig{"a": {}, "b": {}}
	_, err := mgr.CreateExperiment("del-test", "", variants)
	require.NoError(t, err)

	err = mgr.DeleteExperiment("del-test")
	require.NoError(t, err)

	_, err = mgr.GetExperiment("del-test")
	assert.Error(t, err)
}

func TestComputeStats(t *testing.T) {
	score1 := 0.8
	score2 := 0.6
	trials := []Trial{
		{LatencyMs: 100, TokensIn: 10, TokensOut: 20, Score: &score1},
		{LatencyMs: 200, TokensIn: 15, TokensOut: 25, Score: &score2},
		{LatencyMs: 150, TokensIn: 12, TokensOut: 22, Error: "timeout"},
	}

	stats := computeStats("test", trials)
	assert.Equal(t, 3, stats.TrialCount)
	assert.Equal(t, 2, stats.ScoredCount)
	assert.Equal(t, 1, stats.ErrorCount)
	assert.InDelta(t, 0.7, stats.AvgScore, 0.001)
	assert.InDelta(t, 150.0, stats.AvgLatencyMs, 0.001)
	assert.InDelta(t, 0.333, stats.ErrorRate, 0.01)
	assert.Equal(t, 0.6, stats.MinScore)
	assert.Equal(t, 0.8, stats.MaxScore)
	assert.Greater(t, stats.StdDevScore, 0.0)
}

func TestComputeStatsEmpty(t *testing.T) {
	stats := computeStats("empty", nil)
	assert.Equal(t, 0, stats.TrialCount)
	assert.Equal(t, 0, stats.ScoredCount)
}

func TestRecommendNoData(t *testing.T) {
	r := recommend(nil)
	assert.Equal(t, "No data available.", r)
}

func TestRecommendClearWinner(t *testing.T) {
	stats := []VariantStats{
		{VariantName: "good", ScoredCount: 10, AvgScore: 0.9, AvgLatencyMs: 100},
		{VariantName: "bad", ScoredCount: 10, AvgScore: 0.5, AvgLatencyMs: 200},
	}
	r := recommend(stats)
	assert.Contains(t, r, "good")
	assert.Contains(t, r, "clear winner")
}

func TestRecommendCloseResults(t *testing.T) {
	stats := []VariantStats{
		{VariantName: "a", ScoredCount: 10, AvgScore: 0.82},
		{VariantName: "b", ScoredCount: 10, AvgScore: 0.80},
	}
	r := recommend(stats)
	assert.Contains(t, r, "close")
}

func TestRecommendNeedMoreTrials(t *testing.T) {
	stats := []VariantStats{
		{VariantName: "a", ScoredCount: 2, AvgScore: 0.9},
		{VariantName: "b", ScoredCount: 2, AvgScore: 0.5},
	}
	r := recommend(stats)
	assert.Contains(t, r, "more trials")
}

func TestRunTrialWithError(t *testing.T) {
	mgr := NewManager(newTestDB(t))

	variants := map[string]VariantConfig{"a": {}, "b": {}}
	_, err := mgr.CreateExperiment("err-test", "", variants)
	require.NoError(t, err)

	prov := &mockProvider{err: assert.AnError}
	trials, err := mgr.RunTrial(
		context.Background(), "err-test", "test",
		prov, "m",
	)
	require.NoError(t, err)
	for _, trial := range trials {
		assert.NotEmpty(t, trial.Error)
		assert.Empty(t, trial.Response)
	}
}
