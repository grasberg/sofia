package eval

import (
	"context"
	"sync"
	"time"
)

// AgentFunc is the function the harness calls to get agent output for a given input.
// This decouples the harness from the agent loop implementation.
type AgentFunc func(ctx context.Context, input string) (output string, usage UsageInfo, err error)

// UsageInfo holds token and cost metrics from a single agent invocation.
type UsageInfo struct {
	TokensUsed int
	CostUSD    float64
}

// HarnessConfig configures an evaluation harness.
type HarnessConfig struct {
	AgentFn     AgentFunc
	Timeout     time.Duration          // per-test timeout, default 60s
	Concurrency int                    // max parallel tests, default 1
	JudgeFn     JudgeFunc              // optional LLM-as-judge function
	Scorers     map[string]ScorerFunc  // optional custom scoring functions
}

// Harness executes test suites against an agent function.
type Harness struct {
	cfg    HarnessConfig
	runner *EvalRunner
}

// NewHarness creates a new evaluation harness with the given configuration.
func NewHarness(cfg HarnessConfig) *Harness {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}

	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}

	return &Harness{
		cfg:    cfg,
		runner: NewEvalRunner(),
	}
}

// RunSuite executes all test cases in a suite against the agent function.
// Returns an EvalReport with all results populated including cost/token fields.
func (h *Harness) RunSuite(ctx context.Context, suite *TestSuite) EvalReport {
	start := time.Now()
	cases := suite.Cases

	results := make([]TestResult, len(cases))

	if h.cfg.Concurrency <= 1 {
		for i, tc := range cases {
			results[i] = h.runSingleTest(ctx, tc)
		}
	} else {
		sem := make(chan struct{}, h.cfg.Concurrency)

		var wg sync.WaitGroup

		for i, tc := range cases {
			wg.Add(1)

			go func(idx int, tc TestCase) {
				defer wg.Done()

				sem <- struct{}{}
				defer func() { <-sem }()

				results[idx] = h.runSingleTest(ctx, tc)
			}(i, tc)
		}

		wg.Wait()
	}

	return h.runner.GenerateReportWithCases(results, cases, time.Since(start))
}

// runSingleTest executes one test case: calls the agent, scores the output,
// optionally applies custom scorers and LLM-as-judge.
func (h *Harness) runSingleTest(ctx context.Context, tc TestCase) TestResult {
	timeout := h.cfg.Timeout
	if tc.Timeout > 0 {
		timeout = time.Duration(tc.Timeout) * time.Second
	}

	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()

	output, usage, err := h.cfg.AgentFn(testCtx, tc.Input)

	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Name:       tc.Name,
			Passed:     false,
			Input:      tc.Input,
			Output:     "",
			Duration:   duration,
			Errors:     []string{err.Error()},
			Score:      0.0,
			TokensUsed: usage.TokensUsed,
			CostUSD:    usage.CostUSD,
		}
	}

	// Check if a custom scorer is configured for this test case.
	if tc.CustomScorer != "" && h.cfg.Scorers != nil {
		if scorer, ok := h.cfg.Scorers[tc.CustomScorer]; ok {
			score, errors := scorer(tc, output)

			if score < 0.0 {
				score = 0.0
			}

			if score > 1.0 {
				score = 1.0
			}

			return TestResult{
				Name:       tc.Name,
				Passed:     score >= 0.5,
				Input:      tc.Input,
				Output:     output,
				Duration:   duration,
				Errors:     errors,
				Score:      score,
				TokensUsed: usage.TokensUsed,
				CostUSD:    usage.CostUSD,
			}
		}
	}

	// Run structural scoring via EvalRunner.
	result := h.runner.RunTest(tc, output, duration)
	result.TokensUsed = usage.TokensUsed
	result.CostUSD = usage.CostUSD

	// Apply LLM-as-judge if criteria are set and a judge function is configured.
	if tc.JudgeCriteria != "" && h.cfg.JudgeFn != nil {
		judgeScore, judgeErr := h.cfg.JudgeFn(ctx, tc.Input, output, tc.JudgeCriteria)
		if judgeErr == nil {
			if judgeScore < 0.0 {
				judgeScore = 0.0
			}

			if judgeScore > 1.0 {
				judgeScore = 1.0
			}

			// Blend structural and judge scores equally.
			result.Score = 0.5*result.Score + 0.5*judgeScore
			result.Passed = result.Score >= 0.5
		}
	}

	return result
}
