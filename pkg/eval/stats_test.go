package eval

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBootstrapConfidence_ClearlyDifferent(t *testing.T) {
	// Candidate is clearly better (~0.85) vs baseline (~0.25), with some variance.
	baseline := []float64{0.2, 0.3, 0.2, 0.3, 0.2, 0.3, 0.2, 0.3, 0.2, 0.3}
	candidate := []float64{0.8, 0.9, 0.8, 0.9, 0.8, 0.9, 0.8, 0.9, 0.8, 0.9}

	ci := BootstrapConfidence(baseline, candidate, 0.95, 10000)

	assert.Equal(t, 0.95, ci.Level)
	assert.True(t, ci.Significant, "clearly different distributions should be significant")
	assert.Greater(t, ci.Lower, 0.0, "lower bound should be above zero when candidate is better")
	assert.GreaterOrEqual(t, ci.Upper, ci.Lower, "upper bound should be at least the lower bound")
}

func TestBootstrapConfidence_IdenticalDistributions(t *testing.T) {
	// Same scores in both groups: no significant difference expected.
	scores := []float64{0.5, 0.6, 0.7, 0.5, 0.6, 0.7, 0.5, 0.6, 0.7, 0.5}

	ci := BootstrapConfidence(scores, scores, 0.95, 10000)

	assert.Equal(t, 0.95, ci.Level)
	assert.False(t, ci.Significant, "identical distributions should not be significant")
	assert.LessOrEqual(t, ci.Lower, 0.0, "lower bound should be at or below zero")
	assert.GreaterOrEqual(t, ci.Upper, 0.0, "upper bound should be at or above zero")
}

func TestBootstrapConfidence_EmptyBaseline(t *testing.T) {
	ci := BootstrapConfidence(nil, []float64{0.8, 0.9}, 0.95, 1000)

	assert.False(t, ci.Significant)
	assert.Equal(t, 0.0, ci.Lower)
	assert.Equal(t, 0.0, ci.Upper)
}

func TestBootstrapConfidence_EmptyCandidate(t *testing.T) {
	ci := BootstrapConfidence([]float64{0.8, 0.9}, nil, 0.95, 1000)

	assert.False(t, ci.Significant)
	assert.Equal(t, 0.0, ci.Lower)
	assert.Equal(t, 0.0, ci.Upper)
}

func TestBootstrapConfidence_BothEmpty(t *testing.T) {
	ci := BootstrapConfidence(nil, nil, 0.95, 1000)

	assert.False(t, ci.Significant)
	assert.Equal(t, 0.0, ci.Lower)
	assert.Equal(t, 0.0, ci.Upper)
}

func TestBootstrapConfidence_SingleScore(t *testing.T) {
	t.Run("different single scores", func(t *testing.T) {
		ci := BootstrapConfidence([]float64{0.3}, []float64{0.9}, 0.95, 1000)

		assert.True(t, ci.Significant, "single different scores should be significant")
		assert.InDelta(t, 0.6, ci.Lower, 0.001)
		assert.InDelta(t, 0.6, ci.Upper, 0.001)
	})

	t.Run("identical single scores", func(t *testing.T) {
		ci := BootstrapConfidence([]float64{0.5}, []float64{0.5}, 0.95, 1000)

		assert.False(t, ci.Significant, "identical single scores should not be significant")
		assert.Equal(t, 0.0, ci.Lower)
		assert.Equal(t, 0.0, ci.Upper)
	})
}

func TestBootstrapConfidence_DefaultIterations(t *testing.T) {
	// Pass 0 for nBootstrap to use the default.
	baseline := []float64{0.1, 0.2, 0.3}
	candidate := []float64{0.8, 0.9, 1.0}

	ci := BootstrapConfidence(baseline, candidate, 0.95, 0)

	assert.True(t, ci.Significant)
	assert.Greater(t, ci.Lower, 0.0)
}

func TestBootstrapConfidence_DifferentLevels(t *testing.T) {
	// Use distributions with variance so the CI widths differ across levels.
	baseline := []float64{0.2, 0.3, 0.4, 0.2, 0.3, 0.4, 0.2, 0.3, 0.4, 0.3}
	candidate := []float64{0.7, 0.8, 0.9, 0.7, 0.8, 0.9, 0.7, 0.8, 0.9, 0.8}

	ci90 := BootstrapConfidence(baseline, candidate, 0.90, 10000)
	ci99 := BootstrapConfidence(baseline, candidate, 0.99, 10000)

	assert.Equal(t, 0.90, ci90.Level)
	assert.Equal(t, 0.99, ci99.Level)

	// Both should be significant with such clearly different distributions.
	assert.True(t, ci90.Significant)
	assert.True(t, ci99.Significant)

	// The 99% CI should be wider than (or equal to) the 90% CI.
	ci90Width := ci90.Upper - ci90.Lower
	ci99Width := ci99.Upper - ci99.Lower
	assert.GreaterOrEqual(t, ci99Width, ci90Width, "99%% CI should be at least as wide as 90%% CI")
}

func TestMean(t *testing.T) {
	assert.Equal(t, 0.0, mean(nil))
	assert.Equal(t, 0.0, mean([]float64{}))
	assert.Equal(t, 5.0, mean([]float64{5.0}))
	assert.InDelta(t, 2.0, mean([]float64{1.0, 2.0, 3.0}), 0.001)
}
