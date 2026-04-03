package eval

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/grasberg/sofia/pkg/cron"
)

// ScheduledEval defines an evaluation suite that runs on a cron schedule.
type ScheduledEval struct {
	SuitePath          string `json:"suite_path"`
	CronExpr           string `json:"cron_expr"`
	AgentID            string `json:"agent_id"`
	NotifyOnRegression bool   `json:"notify_on_regression"`
}

// EvalScheduler registers eval suites with the cron system and runs them on
// schedule. It loads suites, validates their structure, saves results to the
// store, and detects regressions by comparing against the previous run.
type EvalScheduler struct {
	store     *EvalStore
	suiteDir  string
	schedules []ScheduledEval
}

// NewEvalScheduler creates a scheduler with the given store, suite directory,
// and list of scheduled evaluations.
func NewEvalScheduler(store *EvalStore, suiteDir string, schedules []ScheduledEval) *EvalScheduler {
	return &EvalScheduler{
		store:     store,
		suiteDir:  suiteDir,
		schedules: schedules,
	}
}

// RegisterWithCron registers each scheduled eval as a cron job. The jobs use
// the existing CronService infrastructure so no separate ticker loop is needed.
func (es *EvalScheduler) RegisterWithCron(cronService *cron.CronService) {
	for _, sched := range es.schedules {
		schedule := cron.CronSchedule{
			Kind: "cron",
			Expr: sched.CronExpr,
		}

		name := fmt.Sprintf("eval:%s", sched.SuitePath)

		_, err := cronService.AddJob(name, schedule, "eval-run", false, "", "")
		if err != nil {
			log.Printf("[eval-scheduler] failed to register cron job for %s: %v", sched.SuitePath, err)
		}
	}
}

// RunScheduledEval loads a suite, validates it, generates a structural report,
// saves results to the store, and checks for regression against the previous run.
func (es *EvalScheduler) RunScheduledEval(ctx context.Context, schedule ScheduledEval) (*EvalReport, error) {
	suite, err := LoadSuite(schedule.SuitePath)
	if err != nil {
		return nil, fmt.Errorf("load suite: %w", err)
	}

	// Without an actual agent function we validate the suite structure and
	// generate a report based on structural checks. Each test case is
	// "executed" with an empty output — real agent execution requires the
	// harness to inject an agent function at runtime.
	runner := NewEvalRunner()
	start := time.Now()

	var results []TestResult
	for _, tc := range suite.Cases {
		// Structural validation pass: the test case loaded and parsed
		// correctly, so we mark it as a structural pass with a neutral score.
		result := runner.RunTest(tc, "", time.Since(start))
		results = append(results, result)
	}

	totalDuration := time.Since(start)
	report := runner.GenerateReportWithCases(results, suite.Cases, totalDuration)

	// Persist the run.
	suiteName := suite.Name
	if suiteName == "" {
		suiteName = schedule.SuitePath
	}

	_, err = es.store.SaveRun(suiteName, schedule.AgentID, "", report)
	if err != nil {
		return &report, fmt.Errorf("save run: %w", err)
	}

	// Check for regression compared to the previous run.
	if schedule.NotifyOnRegression {
		if regressed, details := es.checkRegression(suiteName, &report); regressed {
			log.Printf("[eval-scheduler] REGRESSION detected for suite %q: %s", suiteName, details)
		}
	}

	return &report, nil
}

// checkRegression compares the current report against the most recent
// historical run for the same suite. Returns true if avg_score dropped by more
// than 5 percentage points or pass_rate declined.
func (es *EvalScheduler) checkRegression(suiteName string, current *EvalReport) (bool, string) {
	history, err := es.store.GetRunHistory(suiteName, 2)
	if err != nil || len(history) < 2 {
		return false, ""
	}

	// history[0] is the run we just saved; history[1] is the previous one.
	prev := history[1]

	var currentPassRate float64
	if current.TotalTests > 0 {
		currentPassRate = float64(current.Passed) / float64(current.TotalTests)
	}

	scoreDrop := prev.AvgScore - current.AvgScore
	passRateDrop := prev.PassRate - currentPassRate

	const threshold = 0.05

	if scoreDrop > threshold {
		return true, fmt.Sprintf("avg_score dropped %.1f%% (%.2f -> %.2f)",
			scoreDrop*100, prev.AvgScore, current.AvgScore)
	}

	if passRateDrop > threshold {
		return true, fmt.Sprintf("pass_rate dropped %.1f%% (%.2f -> %.2f)",
			passRateDrop*100, prev.PassRate, currentPassRate)
	}

	return false, ""
}
