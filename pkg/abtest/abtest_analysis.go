package abtest

import (
	"fmt"
	"math"
	"sort"
)

// Analyze produces aggregate statistics for an experiment.
func (m *Manager) Analyze(experimentName string) (*Analysis, error) {
	exp, err := m.GetExperiment(experimentName)
	if err != nil {
		return nil, err
	}

	analysis := &Analysis{
		ExperimentName: exp.Name,
		Status:         exp.Status,
	}

	for _, v := range exp.Variants {
		trials, tErr := m.getTrials(exp.ID, v.ID)
		if tErr != nil {
			return nil, tErr
		}

		stats := computeStats(v.Name, trials)
		analysis.TotalTrials += stats.TrialCount
		analysis.Stats = append(analysis.Stats, stats)
	}

	analysis.Recommendation = recommend(analysis.Stats)
	return analysis, nil
}

func computeStats(variantName string, trials []Trial) VariantStats {
	s := VariantStats{VariantName: variantName, TrialCount: len(trials)}
	if len(trials) == 0 {
		return s
	}

	var (
		totalLatency int64
		totalIn      int
		totalOut     int
		scores       []float64
	)

	for _, t := range trials {
		totalLatency += t.LatencyMs
		totalIn += t.TokensIn
		totalOut += t.TokensOut
		if t.Error != "" {
			s.ErrorCount++
		}
		if t.Score != nil {
			scores = append(scores, *t.Score)
		}
	}

	n := float64(len(trials))
	s.AvgLatencyMs = float64(totalLatency) / n
	s.AvgTokensIn = float64(totalIn) / n
	s.AvgTokensOut = float64(totalOut) / n
	s.ErrorRate = float64(s.ErrorCount) / n

	if len(scores) > 0 {
		s.ScoredCount = len(scores)
		sum := 0.0
		s.MinScore = scores[0]
		s.MaxScore = scores[0]
		for _, sc := range scores {
			sum += sc
			if sc < s.MinScore {
				s.MinScore = sc
			}
			if sc > s.MaxScore {
				s.MaxScore = sc
			}
		}
		s.AvgScore = sum / float64(len(scores))

		if len(scores) > 1 {
			variance := 0.0
			for _, sc := range scores {
				diff := sc - s.AvgScore
				variance += diff * diff
			}
			s.StdDevScore = math.Sqrt(
				variance / float64(len(scores)),
			)
		}
	}

	return s
}

func recommend(stats []VariantStats) string {
	if len(stats) == 0 {
		return "No data available."
	}

	hasScoredData := false
	for _, s := range stats {
		if s.ScoredCount > 0 {
			hasScoredData = true
			break
		}
	}

	if !hasScoredData {
		best := stats[0]
		for _, s := range stats[1:] {
			if s.ErrorRate < best.ErrorRate {
				best = s
			} else if s.ErrorRate == best.ErrorRate &&
				s.AvgLatencyMs < best.AvgLatencyMs {
				best = s
			}
		}
		return fmt.Sprintf(
			"No scores yet. Based on error rate and latency, "+
				"%q looks best (%.0fms avg, %.0f%% errors). "+
				"Score trials for a quality-based recommendation.",
			best.VariantName, best.AvgLatencyMs, best.ErrorRate*100,
		)
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].AvgScore > stats[j].AvgScore
	})
	best := stats[0]

	minTrials := 5
	if best.ScoredCount < minTrials {
		return fmt.Sprintf(
			"%q leads with avg score %.2f (%d trials scored). "+
				"Run at least %d more trials for "+
				"statistical confidence.",
			best.VariantName, best.AvgScore, best.ScoredCount,
			minTrials-best.ScoredCount,
		)
	}

	if len(stats) > 1 {
		second := stats[1]
		gap := best.AvgScore - second.AvgScore
		if gap < 0.05 {
			return fmt.Sprintf(
				"Results are close: %q (%.2f) vs %q (%.2f). "+
					"Run more trials to establish significance.",
				best.VariantName, best.AvgScore,
				second.VariantName, second.AvgScore,
			)
		}
	}

	return fmt.Sprintf(
		"%q is the clear winner with avg score %.2f "+
			"(%.0fms avg latency, %d trials). "+
			"Consider concluding the experiment.",
		best.VariantName, best.AvgScore,
		best.AvgLatencyMs, best.ScoredCount,
	)
}
