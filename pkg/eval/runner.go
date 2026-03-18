package eval

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

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
}

// TestResult holds the outcome of a single test.
type TestResult struct {
	Name     string        `json:"name"`
	Passed   bool          `json:"passed"`
	Input    string        `json:"input"`
	Output   string        `json:"output"`
	Duration time.Duration `json:"duration"`
	Errors   []string      `json:"errors,omitempty"`
	Score    float64       `json:"score"`
}

// EvalReport is the summary of an evaluation run.
type EvalReport struct {
	TotalTests int           `json:"total_tests"`
	Passed     int           `json:"passed"`
	Failed     int           `json:"failed"`
	TotalScore float64       `json:"total_score"`
	AvgScore   float64       `json:"avg_score"`
	Duration   time.Duration `json:"duration"`
	Results    []TestResult  `json:"results"`
	RunAt      time.Time     `json:"run_at"`
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

	if tc.ExpectedOutput != "" {
		if !strings.Contains(output, tc.ExpectedOutput) {
			score -= 0.5
			errors = append(errors, fmt.Sprintf(
				"expected substring %q not found in output", tc.ExpectedOutput,
			))
		}
	}

	if tc.ExpectPattern != "" {
		re, err := regexp.Compile(tc.ExpectPattern)
		if err != nil {
			score -= 0.5
			errors = append(errors, fmt.Sprintf("invalid regex pattern %q: %v", tc.ExpectPattern, err))
		} else if !re.MatchString(output) {
			score -= 0.5
			errors = append(errors, fmt.Sprintf("pattern %q did not match output", tc.ExpectPattern))
		}
	}

	for _, s := range tc.ExpectContains {
		if !strings.Contains(output, s) {
			score -= 0.25
			errors = append(errors, fmt.Sprintf("expected substring %q not found", s))
		}
	}

	for _, s := range tc.NotContains {
		if strings.Contains(output, s) {
			score -= 0.25
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

// GenerateReport compiles results into a report.
func (er *EvalRunner) GenerateReport(results []TestResult, totalDuration time.Duration) EvalReport {
	report := EvalReport{
		TotalTests: len(results),
		Duration:   totalDuration,
		Results:    results,
		RunAt:      time.Now().UTC(),
	}

	for _, r := range results {
		report.TotalScore += r.Score
		if r.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
	}

	if report.TotalTests > 0 {
		report.AvgScore = report.TotalScore / float64(report.TotalTests)
	}

	return report
}
