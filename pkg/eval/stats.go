package eval

import (
	"math"
	"math/rand/v2"
	"sort"
)

// ConfidenceInterval represents a bootstrap confidence interval for a score
// delta between two evaluation runs.
type ConfidenceInterval struct {
	Lower       float64 `json:"lower"`
	Upper       float64 `json:"upper"`
	Level       float64 `json:"level"`       // e.g., 0.95
	Significant bool    `json:"significant"` // true if interval doesn't contain 0
}

// defaultBootstrapIterations is the number of bootstrap resampling iterations.
const defaultBootstrapIterations = 10000

// BootstrapConfidence computes a confidence interval for the score delta
// between two sets of per-test scores using bootstrap resampling.
//
// For each iteration it resamples both score arrays with replacement, computes
// the mean delta (candidate - baseline), then finds percentile bounds for the
// given confidence level. Significant is true when the interval doesn't cross
// zero.
//
// Pass nBootstrap <= 0 to use the default of 10000 iterations.
func BootstrapConfidence(baselineScores, candidateScores []float64, level float64, nBootstrap int) ConfidenceInterval {
	if nBootstrap <= 0 {
		nBootstrap = defaultBootstrapIterations
	}

	// Edge case: no scores to compare.
	if len(baselineScores) == 0 || len(candidateScores) == 0 {
		return ConfidenceInterval{
			Lower:       0,
			Upper:       0,
			Level:       level,
			Significant: false,
		}
	}

	// Single score in each: no variance to estimate, return point estimate.
	if len(baselineScores) == 1 && len(candidateScores) == 1 {
		delta := candidateScores[0] - baselineScores[0]

		return ConfidenceInterval{
			Lower:       delta,
			Upper:       delta,
			Level:       level,
			Significant: delta != 0,
		}
	}

	deltas := make([]float64, nBootstrap)
	nBase := len(baselineScores)
	nCand := len(candidateScores)

	for i := range nBootstrap {
		var baseSum, candSum float64

		for range nBase {
			baseSum += baselineScores[rand.IntN(nBase)]
		}

		for range nCand {
			candSum += candidateScores[rand.IntN(nCand)]
		}

		baseMean := baseSum / float64(nBase)
		candMean := candSum / float64(nCand)
		deltas[i] = candMean - baseMean
	}

	sort.Float64s(deltas)

	// Percentile bounds: for a 95% CI, we want the 2.5th and 97.5th percentiles.
	alpha := 1.0 - level
	lowerIdx := int(math.Floor(alpha / 2.0 * float64(nBootstrap)))
	upperIdx := int(math.Floor((1.0 - alpha/2.0) * float64(nBootstrap)))

	if lowerIdx < 0 {
		lowerIdx = 0
	}

	if upperIdx >= nBootstrap {
		upperIdx = nBootstrap - 1
	}

	lower := deltas[lowerIdx]
	upper := deltas[upperIdx]

	// The interval is significant if it doesn't contain zero.
	significant := (lower > 0 && upper > 0) || (lower < 0 && upper < 0)

	return ConfidenceInterval{
		Lower:       lower,
		Upper:       upper,
		Level:       level,
		Significant: significant,
	}
}
