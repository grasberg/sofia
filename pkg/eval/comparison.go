package eval

import (
	"fmt"
	"math"
)

// ComparisonResult holds the outcome of comparing two evaluation runs
// (baseline vs candidate).
type ComparisonResult struct {
	BaselineReport  EvalReport       `json:"baseline"`
	CandidateReport EvalReport       `json:"candidate"`
	ScoreDelta      float64          `json:"score_delta"`
	PassRateDelta   float64          `json:"pass_rate_delta"`
	CostDelta       float64          `json:"cost_delta_usd"`
	SpeedDelta      float64          `json:"speed_delta_ms"`
	Winner          ComparisonWinner `json:"winner"`
	Confidence      float64          `json:"confidence"` // 0.0-1.0
	CI              ConfidenceInterval `json:"confidence_interval"`
	Summary         string           `json:"summary"`
	PerTest         []TestComparison `json:"per_test"`
}

// TestComparison holds the per-test score comparison between baseline and
// candidate runs.
type TestComparison struct {
	TestName        string  `json:"test_name"`
	BaselineScore   float64 `json:"baseline_score"`
	CandidateScore  float64 `json:"candidate_score"`
	Delta           float64 `json:"delta"`
	BaselinePassed  bool    `json:"baseline_passed"`
	CandidatePassed bool    `json:"candidate_passed"`
}

// ComparisonWinner indicates which run performed better.
type ComparisonWinner string

const (
	WinnerTie       ComparisonWinner = "tie"
	WinnerCandidate ComparisonWinner = "candidate"
	WinnerBaseline  ComparisonWinner = "baseline"
)

// tieThreshold is the minimum absolute score delta to declare a winner.
const tieThreshold = 0.05

// Compare compares a baseline evaluation report against a candidate report and
// returns a detailed comparison result with per-test breakdowns, aggregate
// deltas, winner determination, and bootstrap confidence intervals.
func Compare(baseline, candidate EvalReport) ComparisonResult {
	result := ComparisonResult{
		BaselineReport:  baseline,
		CandidateReport: candidate,
	}

	// --- Per-test comparison ---
	// Index baseline results by name for O(1) lookup.
	baselineByName := make(map[string]TestResult, len(baseline.Results))
	for _, r := range baseline.Results {
		baselineByName[r.Name] = r
	}

	// Track which baseline tests were matched so we can include unmatched ones.
	matched := make(map[string]bool, len(baseline.Results))

	// Match candidate results to baseline by name.
	for _, cr := range candidate.Results {
		tc := TestComparison{
			TestName:        cr.Name,
			CandidateScore:  cr.Score,
			CandidatePassed: cr.Passed,
		}

		if br, ok := baselineByName[cr.Name]; ok {
			tc.BaselineScore = br.Score
			tc.BaselinePassed = br.Passed
			tc.Delta = cr.Score - br.Score
			matched[cr.Name] = true
		} else {
			tc.Delta = cr.Score
		}

		result.PerTest = append(result.PerTest, tc)
	}

	// Add baseline-only tests (not present in candidate).
	for _, br := range baseline.Results {
		if matched[br.Name] {
			continue
		}

		result.PerTest = append(result.PerTest, TestComparison{
			TestName:       br.Name,
			BaselineScore:  br.Score,
			BaselinePassed: br.Passed,
			Delta:          -br.Score,
		})
	}

	// --- Aggregate deltas ---
	result.ScoreDelta = candidate.AvgScore - baseline.AvgScore

	baselinePassRate := passRate(baseline)
	candidatePassRate := passRate(candidate)
	result.PassRateDelta = candidatePassRate - baselinePassRate

	result.CostDelta = totalCost(candidate) - totalCost(baseline)
	result.SpeedDelta = float64(candidate.Duration.Milliseconds()) - float64(baseline.Duration.Milliseconds())

	// --- Winner determination ---
	if math.Abs(result.ScoreDelta) < tieThreshold {
		result.Winner = WinnerTie
	} else if result.ScoreDelta > 0 {
		result.Winner = WinnerCandidate
	} else {
		result.Winner = WinnerBaseline
	}

	// --- Bootstrap confidence interval ---
	baselineScores := extractScores(baseline)
	candidateScores := extractScores(candidate)

	if len(baselineScores) > 0 && len(candidateScores) > 0 {
		ci := BootstrapConfidence(baselineScores, candidateScores, 0.95, 0)
		result.CI = ci
		result.Confidence = ci.Level
	}

	// --- Summary ---
	result.Summary = buildSummary(result, baselinePassRate, candidatePassRate)

	return result
}

// passRate returns the pass rate for a report, or 0 if there are no tests.
func passRate(report EvalReport) float64 {
	if report.TotalTests == 0 {
		return 0
	}

	return float64(report.Passed) / float64(report.TotalTests)
}

// totalCost sums the CostUSD across all test results.
func totalCost(report EvalReport) float64 {
	var total float64
	for _, r := range report.Results {
		total += r.CostUSD
	}

	return total
}

// extractScores collects per-test scores into a slice.
func extractScores(report EvalReport) []float64 {
	scores := make([]float64, len(report.Results))
	for i, r := range report.Results {
		scores[i] = r.Score
	}

	return scores
}

// buildSummary generates a human-readable summary of the comparison.
func buildSummary(result ComparisonResult, basePassRate, candPassRate float64) string {
	switch result.Winner {
	case WinnerTie:
		return fmt.Sprintf(
			"Tie: avg score delta %.4f is within threshold (+-%.2f). Baseline %.2f vs Candidate %.2f. Pass rates: %.0f%% vs %.0f%%.",
			result.ScoreDelta, tieThreshold,
			result.BaselineReport.AvgScore, result.CandidateReport.AvgScore,
			basePassRate*100, candPassRate*100,
		)
	case WinnerCandidate:
		s := fmt.Sprintf(
			"Candidate wins: avg score %.2f vs %.2f (delta +%.4f). Pass rates: %.0f%% vs %.0f%%.",
			result.CandidateReport.AvgScore, result.BaselineReport.AvgScore,
			result.ScoreDelta,
			candPassRate*100, basePassRate*100,
		)

		if result.CI.Significant {
			s += fmt.Sprintf(" Statistically significant (%.0f%% CI: [%.4f, %.4f]).", result.CI.Level*100, result.CI.Lower, result.CI.Upper)
		}

		return s
	default: // baseline
		s := fmt.Sprintf(
			"Baseline wins: avg score %.2f vs %.2f (delta %.4f). Pass rates: %.0f%% vs %.0f%%.",
			result.BaselineReport.AvgScore, result.CandidateReport.AvgScore,
			result.ScoreDelta,
			basePassRate*100, candPassRate*100,
		)

		if result.CI.Significant {
			s += fmt.Sprintf(" Statistically significant (%.0f%% CI: [%.4f, %.4f]).", result.CI.Level*100, result.CI.Lower, result.CI.Upper)
		}

		return s
	}
}
