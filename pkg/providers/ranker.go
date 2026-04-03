package providers

import (
	"math"
	"sort"
	"time"
)

// ScoreQuerier retrieves aggregated trace scores per model.
// Implemented by memory.MemoryDB.
type ScoreQuerier interface {
	GetModelTraceScores(since time.Time, minTraces int) (map[string]map[string]float64, map[string]int, error)
}

// ProviderRanker reorders FallbackCandidates based on historical quality scores.
type ProviderRanker struct {
	querier    ScoreQuerier
	window     time.Duration
	minTraces  int
	weights    map[string]float64
	alpha      float64 // EMA smoothing factor (lower = more stable)
	priorScore float64 // Bayesian prior for models with few samples
}

// NewProviderRanker creates a ranker with sensible defaults.
func NewProviderRanker(querier ScoreQuerier) *ProviderRanker {
	return &ProviderRanker{
		querier:   querier,
		window:    7 * 24 * time.Hour, // 7 days
		minTraces: 5,
		weights: map[string]float64{
			"task_completion":   0.40,
			"efficiency":        0.20,
			"error_rate":        0.20,
			"user_satisfaction": 0.20,
		},
		alpha:      0.1,
		priorScore: 0.5, // neutral prior
	}
}

// Rank reorders candidates based on weighted quality scores from traces.
// Returns the reordered list. Candidates without sufficient data keep their
// original relative order (stable sort with high default score).
func (r *ProviderRanker) Rank(candidates []FallbackCandidate) []FallbackCandidate {
	if r.querier == nil || len(candidates) <= 1 {
		return candidates
	}

	since := time.Now().Add(-r.window)
	avgScores, counts, err := r.querier.GetModelTraceScores(since, r.minTraces)
	if err != nil || len(avgScores) == 0 {
		return candidates
	}

	type scored struct {
		candidate FallbackCandidate
		score     float64
		origIndex int
	}

	items := make([]scored, len(candidates))
	for i, c := range candidates {
		key := ModelKey(c.Provider, c.Model)
		dims, ok := avgScores[key]
		if !ok {
			// Also try just the model name (no provider prefix)
			dims, ok = avgScores[c.Model]
		}

		if !ok {
			// No data — assign prior score to preserve original order
			items[i] = scored{candidate: c, score: r.priorScore, origIndex: i}
			continue
		}

		// Compute weighted composite score
		composite := 0.0
		totalWeight := 0.0
		for dim, w := range r.weights {
			if v, exists := dims[dim]; exists {
				composite += v * w
				totalWeight += w
			}
		}
		if totalWeight > 0 {
			composite /= totalWeight
		}

		// Bayesian blend: confidence ramp based on sample count
		n := float64(counts[key])
		if n == 0 {
			n = float64(counts[c.Model])
		}
		confidence := 1.0 - math.Exp(-n/10.0) // sigmoid-like ramp
		blended := confidence*composite + (1-confidence)*r.priorScore

		items[i] = scored{candidate: c, score: blended, origIndex: i}
	}

	// Stable sort by score descending, breaking ties by original order
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		return items[i].origIndex < items[j].origIndex
	})

	result := make([]FallbackCandidate, len(items))
	for i, item := range items {
		result[i] = item.candidate
	}
	return result
}
