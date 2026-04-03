package eval

import (
	"fmt"
	"strings"
)

// ScorerFunc is a custom scoring function. It receives the test case and output,
// returns a score 0.0-1.0 and optional error details.
type ScorerFunc func(tc TestCase, output string) (float64, []string)

// ExactMatchScorer performs a case-insensitive exact match between the output
// and the expected output. Returns 1.0 on match, 0.0 otherwise.
func ExactMatchScorer(tc TestCase, output string) (float64, []string) {
	if strings.EqualFold(strings.TrimSpace(output), strings.TrimSpace(tc.ExpectedOutput)) {
		return 1.0, nil
	}

	return 0.0, []string{
		fmt.Sprintf(
			"exact match failed: expected %q, got %q",
			tc.ExpectedOutput,
			output,
		),
	}
}
