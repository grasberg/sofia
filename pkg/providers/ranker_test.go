package providers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockScoreQuerier struct {
	scores map[string]map[string]float64
	counts map[string]int
}

func (m *mockScoreQuerier) GetModelTraceScores(
	since time.Time,
	minTraces int,
) (map[string]map[string]float64, map[string]int, error) {
	// Filter by minTraces
	filtered := make(map[string]map[string]float64)
	filteredCounts := make(map[string]int)
	for model, scores := range m.scores {
		if m.counts[model] >= minTraces {
			filtered[model] = scores
			filteredCounts[model] = m.counts[model]
		}
	}
	return filtered, filteredCounts, nil
}

func TestProviderRankerReorders(t *testing.T) {
	querier := &mockScoreQuerier{
		scores: map[string]map[string]float64{
			"openai/gpt-4o": {
				"task_completion": 0.9,
				"efficiency":      0.8,
				"error_rate":      0.95,
			},
			"anthropic/claude-3": {
				"task_completion": 0.7,
				"efficiency":      0.6,
				"error_rate":      0.8,
			},
		},
		counts: map[string]int{
			"openai/gpt-4o":      20,
			"anthropic/claude-3": 15,
		},
	}

	ranker := NewProviderRanker(querier)

	candidates := []FallbackCandidate{
		{Provider: "anthropic", Model: "claude-3"},
		{Provider: "openai", Model: "gpt-4o"},
	}

	ranked := ranker.Rank(candidates)
	require.Len(t, ranked, 2)
	// gpt-4o should be ranked first due to higher scores
	assert.Equal(t, "gpt-4o", ranked[0].Model)
	assert.Equal(t, "claude-3", ranked[1].Model)
}

func TestProviderRankerPreservesOrderWithNoData(t *testing.T) {
	querier := &mockScoreQuerier{
		scores: map[string]map[string]float64{},
		counts: map[string]int{},
	}

	ranker := NewProviderRanker(querier)

	candidates := []FallbackCandidate{
		{Provider: "anthropic", Model: "claude-3"},
		{Provider: "openai", Model: "gpt-4o"},
	}

	ranked := ranker.Rank(candidates)
	require.Len(t, ranked, 2)
	// Original order preserved
	assert.Equal(t, "claude-3", ranked[0].Model)
	assert.Equal(t, "gpt-4o", ranked[1].Model)
}

func TestProviderRankerSingleCandidate(t *testing.T) {
	ranker := NewProviderRanker(&mockScoreQuerier{})

	candidates := []FallbackCandidate{
		{Provider: "openai", Model: "gpt-4o"},
	}

	ranked := ranker.Rank(candidates)
	assert.Equal(t, candidates, ranked)
}

func TestProviderRankerNilQuerier(t *testing.T) {
	ranker := NewProviderRanker(nil)

	candidates := []FallbackCandidate{
		{Provider: "openai", Model: "gpt-4o"},
		{Provider: "anthropic", Model: "claude-3"},
	}

	ranked := ranker.Rank(candidates)
	assert.Equal(t, candidates, ranked)
}
