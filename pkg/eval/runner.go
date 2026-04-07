package eval

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FilterByTags returns only test cases that have at least one tag matching any
// of the provided tags. Matching is case-insensitive. An empty tags slice
// returns all cases unchanged.
func FilterByTags(cases []TestCase, tags []string) []TestCase {
	if len(tags) == 0 {
		return cases
	}

	tagSet := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		tagSet[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
	}

	var filtered []TestCase

	for _, tc := range cases {
		for _, t := range tc.Tags {
			if _, ok := tagSet[strings.ToLower(strings.TrimSpace(t))]; ok {
				filtered = append(filtered, tc)

				break
			}
		}
	}

	return filtered
}

// FilterByName returns only test cases whose Name matches the given regex
// pattern. An empty pattern returns all cases unchanged.
func FilterByName(cases []TestCase, pattern string) []TestCase {
	if pattern == "" {
		return cases
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		// If the pattern is invalid, return nothing rather than panicking.
		return nil
	}

	var filtered []TestCase

	for _, tc := range cases {
		if re.MatchString(tc.Name) {
			filtered = append(filtered, tc)
		}
	}

	return filtered
}

// TestCase defines a single evaluation test.
type TestCase struct {
	Name           string   `json:"name"`
	Input          string   `json:"input"`
	ExpectedOutput string   `json:"expected_output,omitempty"`
	ExpectPattern  string   `json:"expect_pattern,omitempty"`
	ExpectContains []string `json:"expect_contains,omitempty"`
	NotContains    []string `json:"not_contains,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Timeout        int      `json:"timeout_sec,omitempty"`
	Weight         float64  `json:"weight,omitempty"`
	JudgeCriteria  string   `json:"judge_criteria,omitempty"`
	CustomScorer   string   `json:"custom_scorer,omitempty"`
}

// TestResult holds the outcome of a single test.
type TestResult struct {
	Name       string        `json:"name"`
	Passed     bool          `json:"passed"`
	Input      string        `json:"input"`
	Output     string        `json:"output"`
	Duration   time.Duration `json:"duration"`
	Errors     []string      `json:"errors,omitempty"`
	Score      float64       `json:"score"`
	TokensUsed int           `json:"tokens_used,omitempty"`
	CostUSD    float64       `json:"cost_usd,omitempty"`
}

// EvalReport is the summary of an evaluation run.
type EvalReport struct {
	TotalTests   int           `json:"total_tests"`
	Passed       int           `json:"passed"`
	Failed       int           `json:"failed"`
	TotalScore   float64       `json:"total_score"`
	AvgScore     float64       `json:"avg_score"`
	Duration     time.Duration `json:"duration"`
	Results      []TestResult  `json:"results"`
	RunAt        time.Time     `json:"run_at"`
	TotalTokens  int           `json:"total_tokens,omitempty"`
	TotalCostUSD float64       `json:"total_cost_usd,omitempty"`
}

// EvalRunner executes test cases against an agent.
type EvalRunner struct{}

// NewEvalRunner creates a new EvalRunner instance.
func NewEvalRunner() *EvalRunner {
	return &EvalRunner{}
}

// RunTest executes a single test case against a response and returns the result.
func (er *EvalRunner) RunTest(tc TestCase, output string, duration time.Duration) TestResult {
	score := 1.0
	var errors []string

	// Weight multiplier for score deductions (defaults to 1.0 if unset).
	w := tc.Weight
	if w <= 0 {
		w = 1.0
	}

	if tc.ExpectedOutput != "" {
		if !strings.Contains(output, tc.ExpectedOutput) {
			score -= 0.5 * w
			errors = append(errors, fmt.Sprintf(
				"expected substring %q not found in output", tc.ExpectedOutput,
			))
		}
	}

	if tc.ExpectPattern != "" {
		re, err := regexp.Compile(tc.ExpectPattern)
		if err != nil {
			score -= 0.5 * w
			errors = append(errors, fmt.Sprintf("invalid regex pattern %q: %v", tc.ExpectPattern, err))
		} else if !re.MatchString(output) {
			score -= 0.5 * w
			errors = append(errors, fmt.Sprintf("pattern %q did not match output", tc.ExpectPattern))
		}
	}

	for _, s := range tc.ExpectContains {
		if !strings.Contains(output, s) {
			score -= 0.25 * w
			errors = append(errors, fmt.Sprintf("expected substring %q not found", s))
		}
	}

	for _, s := range tc.NotContains {
		if strings.Contains(output, s) {
			score -= 0.25 * w
			errors = append(errors, fmt.Sprintf("prohibited substring %q found in output", s))
		}
	}

	// Clamp score to [0.0, 1.0].
	if score < 0.0 {
		score = 0.0
	}
	if score > 1.0 {
		score = 1.0
	}

	return TestResult{
		Name:     tc.Name,
		Passed:   score >= 0.5,
		Input:    tc.Input,
		Output:   output,
		Duration: duration,
		Errors:   errors,
		Score:    score,
	}
}

// GenerateReport compiles results into a report. When TestCases use weighted
// scoring (Weight > 0), the average score is computed as a weighted average.
func (er *EvalRunner) GenerateReport(results []TestResult, totalDuration time.Duration) EvalReport {
	return er.GenerateReportWithCases(results, nil, totalDuration)
}

// GenerateReportWithCases compiles results into a report, using corresponding
// test cases (if provided) to compute a weighted average score.
func (er *EvalRunner) GenerateReportWithCases(
	results []TestResult,
	cases []TestCase,
	totalDuration time.Duration,
) EvalReport {
	report := EvalReport{
		TotalTests: len(results),
		Duration:   totalDuration,
		Results:    results,
		RunAt:      time.Now().UTC(),
	}

	var weightedSum, totalWeight float64
	hasWeights := false

	for i, r := range results {
		report.TotalScore += r.Score
		report.TotalTokens += r.TokensUsed
		report.TotalCostUSD += r.CostUSD

		if r.Passed {
			report.Passed++
		} else {
			report.Failed++
		}

		// Accumulate weighted scores if cases with weights are provided.
		w := 1.0
		if cases != nil && i < len(cases) && cases[i].Weight > 0 {
			w = cases[i].Weight
			hasWeights = true
		}
		weightedSum += r.Score * w
		totalWeight += w
	}

	if report.TotalTests > 0 {
		if hasWeights && totalWeight > 0 {
			report.AvgScore = weightedSum / totalWeight
		} else {
			report.AvgScore = report.TotalScore / float64(report.TotalTests)
		}
	}

	return report
}
