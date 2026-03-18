package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/grasberg/sofia/pkg/eval"
)

// NewEvalCommand returns the "sofia eval" subcommand.
func NewEvalCommand() *cobra.Command {
	var listOnly bool

	cmd := &cobra.Command{
		Use:   "eval <test-file.json>",
		Short: "Run evaluation tests against agent responses",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			testFile := args[0]

			cases, err := loadTestCases(testFile)
			if err != nil {
				return fmt.Errorf("load test cases: %w", err)
			}

			if listOnly {
				return printTestList(cases)
			}

			return runEval(cases)
		},
	}

	cmd.Flags().BoolVar(&listOnly, "list", false, "List test cases without running them")

	return cmd
}

// loadTestCases reads and parses a JSON file containing test cases.
func loadTestCases(path string) ([]eval.TestCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", path, err)
	}

	var cases []eval.TestCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	return cases, nil
}

// printTestList prints all test cases in the file.
func printTestList(cases []eval.TestCase) error {
	fmt.Fprintf(os.Stdout, "Found %d test case(s):\n\n", len(cases))

	for i, tc := range cases {
		fmt.Fprintf(os.Stdout, "  %d. %s\n", i+1, tc.Name)
		fmt.Fprintf(os.Stdout, "     Input: %s\n", tc.Input)

		if len(tc.Tags) > 0 {
			fmt.Fprintf(os.Stdout, "     Tags:  %v\n", tc.Tags)
		}
	}

	return nil
}

// runEval executes all test cases using the response checker and prints results.
func runEval(cases []eval.TestCase) error {
	runner := eval.NewEvalRunner()
	start := time.Now()

	var results []eval.TestResult

	fmt.Fprintf(os.Stdout, "Running %d eval test(s)...\n\n", len(cases))

	for _, tc := range cases {
		tcStart := time.Now()

		// For the framework mode, we use the input as the output to check against.
		// Users can extend this to call the actual agent loop.
		output := tc.Input
		duration := time.Since(tcStart)

		result := runner.RunTest(tc, output, duration)
		results = append(results, result)

		status := "PASS"
		if !result.Passed {
			status = "FAIL"
		}
		fmt.Fprintf(os.Stdout, "  [%s] %s (score: %.2f)\n", status, result.Name, result.Score)

		for _, e := range result.Errors {
			fmt.Fprintf(os.Stdout, "         - %s\n", e)
		}
	}

	totalDuration := time.Since(start)
	report := runner.GenerateReport(results, totalDuration)

	fmt.Fprintln(os.Stdout)
	fmt.Fprintf(os.Stdout, "Results: %d/%d passed, avg score: %.2f, duration: %s\n",
		report.Passed, report.TotalTests, report.AvgScore, report.Duration.Round(time.Millisecond))

	if report.Failed > 0 {
		return fmt.Errorf("%d test(s) failed", report.Failed)
	}

	return nil
}
