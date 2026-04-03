package eval

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/grasberg/sofia/pkg/eval"
)

// NewEvalCommand returns the "sofia eval" subcommand.
func NewEvalCommand() *cobra.Command {
	var (
		listOnly bool
		suite    string
		suiteDir string
		builtin  string
		tags     string
		agentID  string
		model    string
		dbPath   string
		filter   string
	)

	cmd := &cobra.Command{
		Use:   "eval [test-file.json]",
		Short: "Run evaluation tests against agent responses",
		Long: `Load test suites from JSON files and evaluate agent outputs.

A test suite JSON file contains: name, description, agent_id (optional),
model (optional), and a cases array. Each case needs at least name and input.

Examples:
  sofia eval --suite tests/basic.json
  sofia eval --suite-dir tests/ --tags safety,accuracy
  sofia eval --suite tests/basic.json --db ~/.sofia/eval.db
  sofia eval --builtin all
  sofia eval --builtin tool_use --tags safety`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			// Resolve suite source: positional arg, --suite flag, or --suite-dir.
			if len(args) == 1 && suite == "" {
				suite = args[0]
			}

			suites, err := loadSuites(suite, suiteDir, builtin)
			if err != nil {
				return err
			}

			if len(suites) == 0 {
				return fmt.Errorf("no test suites found (use --suite, --suite-dir, or --builtin)")
			}

			if listOnly {
				return printSuiteList(suites, tags, filter)
			}

			// Collect all cases (with suite context for reporting).
			type suiteCase struct {
				suiteName string
				agentID   string
				model     string
				tc        eval.TestCase
			}

			var allCases []suiteCase

			for _, s := range suites {
				aid := s.AgentID
				if agentID != "" {
					aid = agentID
				}

				mdl := s.Model
				if model != "" {
					mdl = model
				}

				cases := s.Cases

				// Apply tag filter.
				if tags != "" {
					tagList := strings.Split(tags, ",")
					cases = eval.FilterByTags(cases, tagList)
				}

				// Apply name filter.
				if filter != "" {
					cases = eval.FilterByName(cases, filter)
				}

				for _, tc := range cases {
					allCases = append(allCases, suiteCase{
						suiteName: s.Name,
						agentID:   aid,
						model:     mdl,
						tc:        tc,
					})
				}
			}

			if len(allCases) == 0 {
				return fmt.Errorf("no test cases match the given filters")
			}

			// Group cases by suite for running and reporting.
			type suiteRun struct {
				name    string
				agentID string
				model   string
				cases   []eval.TestCase
			}

			suiteMap := make(map[string]*suiteRun)
			var suiteOrder []string

			for _, sc := range allCases {
				sr, ok := suiteMap[sc.suiteName]
				if !ok {
					sr = &suiteRun{name: sc.suiteName, agentID: sc.agentID, model: sc.model}
					suiteMap[sc.suiteName] = sr
					suiteOrder = append(suiteOrder, sc.suiteName)
				}

				sr.cases = append(sr.cases, sc.tc)
			}

			// Open store if persistence requested.
			var store *eval.EvalStore
			if dbPath != "" {
				store, err = eval.OpenEvalStore(dbPath)
				if err != nil {
					return fmt.Errorf("open eval store: %w", err)
				}

				defer func() { _ = store.Close() }()
			}

			runner := eval.NewEvalRunner()
			globalStart := time.Now()
			var globalPassed, globalFailed, globalTotal int

			for _, name := range suiteOrder {
				sr := suiteMap[name]

				report := runSuite(runner, sr.name, sr.cases)
				globalPassed += report.Passed
				globalFailed += report.Failed
				globalTotal += report.TotalTests

				// Persist if store is available.
				if store != nil {
					runID, err := store.SaveRun(sr.name, sr.agentID, sr.model, report)
					if err != nil {
						fmt.Fprintf(os.Stderr, "  warning: failed to save run: %v\n", err)
					} else {
						fmt.Fprintf(os.Stdout, "  Saved run #%d to %s\n", runID, dbPath)

						trend, err := store.GetTrend(sr.name)
						if err == nil && trend != eval.TrendInsufficientData {
							fmt.Fprintf(os.Stdout, "  Trend: %s\n", trend)
						}
					}
				}

				fmt.Fprintln(os.Stdout)
			}

			totalDuration := time.Since(globalStart)

			// Print global summary if multiple suites were run.
			if len(suiteOrder) > 1 {
				fmt.Fprintf(os.Stdout, "Overall: %d/%d passed across %d suite(s), duration: %s\n",
					globalPassed, globalTotal, len(suiteOrder), totalDuration.Round(time.Millisecond))
			}

			if globalFailed > 0 {
				return fmt.Errorf("%d test(s) failed", globalFailed)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&listOnly, "list", false, "List test cases without running them")
	cmd.Flags().StringVar(&suite, "suite", "", "Path to a single test suite JSON file")
	cmd.Flags().StringVar(&suiteDir, "suite-dir", "", "Directory containing test suite JSON files")
	cmd.Flags().StringVar(&builtin, "builtin", "", "Load builtin benchmark suite by name (e.g., tool_use, reasoning) or \"all\"")
	cmd.Flags().StringVar(&tags, "tags", "", "Comma-separated tag filter (matches cases with any listed tag)")
	cmd.Flags().StringVar(&filter, "filter", "", "Regex filter on test case names")
	cmd.Flags().StringVar(&agentID, "agent", "", "Override agent ID for all suites")
	cmd.Flags().StringVar(&model, "model", "", "Override model for all suites")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite path for result persistence")

	return cmd
}

// loadSuites resolves suite sources from flags and returns all loaded suites.
func loadSuites(suitePath, suiteDir, builtin string) ([]*eval.TestSuite, error) {
	var suites []*eval.TestSuite

	if suitePath != "" {
		s, err := eval.LoadSuite(suitePath)
		if err != nil {
			return nil, err
		}

		suites = append(suites, s)
	}

	if suiteDir != "" {
		dirSuites, err := eval.LoadSuitesFromDir(suiteDir)
		if err != nil {
			return nil, err
		}

		suites = append(suites, dirSuites...)
	}

	if builtin != "" {
		if builtin == "all" {
			builtinSuites, err := eval.LoadBuiltinSuites()
			if err != nil {
				return nil, fmt.Errorf("load builtin benchmarks: %w", err)
			}

			suites = append(suites, builtinSuites...)
		} else {
			// Load specific builtin suites (comma-separated).
			for _, name := range strings.Split(builtin, ",") {
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}

				s, err := eval.LoadBuiltinSuite(name)
				if err != nil {
					return nil, fmt.Errorf("load builtin benchmark %q: %w", name, err)
				}

				suites = append(suites, s)
			}
		}
	}

	return suites, nil
}

// printSuiteList prints all loaded suites and their (filtered) test cases.
func printSuiteList(suites []*eval.TestSuite, tags, filter string) error {
	for _, s := range suites {
		fmt.Fprintf(os.Stdout, "Suite: %s\n", s.Name)

		if s.Description != "" {
			fmt.Fprintf(os.Stdout, "  %s\n", s.Description)
		}

		cases := s.Cases

		if tags != "" {
			cases = eval.FilterByTags(cases, strings.Split(tags, ","))
		}

		if filter != "" {
			cases = eval.FilterByName(cases, filter)
		}

		fmt.Fprintf(os.Stdout, "  %d test case(s):\n\n", len(cases))

		for i, tc := range cases {
			fmt.Fprintf(os.Stdout, "    %d. %s\n", i+1, tc.Name)
			fmt.Fprintf(os.Stdout, "       Input: %s\n", tc.Input)

			if len(tc.Tags) > 0 {
				fmt.Fprintf(os.Stdout, "       Tags:  %v\n", tc.Tags)
			}
		}

		fmt.Fprintln(os.Stdout)
	}

	return nil
}

// runSuite executes a single suite's test cases and prints a formatted report.
func runSuite(runner *eval.EvalRunner, name string, cases []eval.TestCase) eval.EvalReport {
	fmt.Fprintf(os.Stdout, "Suite: %s (%d tests)\n", name, len(cases))
	fmt.Fprintf(os.Stdout, "%s\n", strings.Repeat("-", 60))

	start := time.Now()

	var results []eval.TestResult

	for _, tc := range cases {
		tcStart := time.Now()

		// Framework mode: use input as mock output. Real agent execution
		// can be wired in by extending this to call the agent loop.
		output := tc.Input
		duration := time.Since(tcStart)

		result := runner.RunTest(tc, output, duration)
		results = append(results, result)

		status := "PASS"
		if !result.Passed {
			status = "FAIL"
		}

		fmt.Fprintf(os.Stdout, "  [%s] %-40s score: %.2f  %s\n",
			status, result.Name, result.Score, result.Duration.Round(time.Microsecond))

		for _, e := range result.Errors {
			fmt.Fprintf(os.Stdout, "         - %s\n", e)
		}
	}

	totalDuration := time.Since(start)
	report := runner.GenerateReportWithCases(results, cases, totalDuration)

	fmt.Fprintf(os.Stdout, "%s\n", strings.Repeat("-", 60))
	fmt.Fprintf(os.Stdout, "Results: %d/%d passed | avg score: %.2f | duration: %s\n",
		report.Passed, report.TotalTests, report.AvgScore,
		report.Duration.Round(time.Millisecond))

	return report
}
