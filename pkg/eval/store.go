package eval

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (CGO_ENABLED=0 compatible)
)

// EvalStore persists evaluation results in SQLite.
type EvalStore struct {
	mu sync.Mutex
	db *sql.DB
}

// EvalRunSummary is a high-level summary of a past evaluation run.
type EvalRunSummary struct {
	ID         int64     `json:"id"`
	SuiteName  string    `json:"suite_name"`
	AgentID    string    `json:"agent_id,omitempty"`
	Model      string    `json:"model,omitempty"`
	AvgScore   float64   `json:"avg_score"`
	PassRate   float64   `json:"pass_rate"`
	TotalTests int       `json:"total_tests"`
	Passed     int       `json:"passed"`
	Failed     int       `json:"failed"`
	DurationMs int64     `json:"duration_ms"`
	RunAt      time.Time `json:"run_at"`
}

// OpenEvalStore opens (or creates) the SQLite database at the given path and
// ensures the eval schema exists. Pass ":memory:" for in-process tests.
func OpenEvalStore(path string) (*EvalStore, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("eval store: create dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("eval store: open db: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err = db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("eval store: set WAL mode: %w", err)
	}

	if _, err = db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("eval store: enable foreign keys: %w", err)
	}

	s := &EvalStore{db: db}

	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return s, nil
}

// Close closes the underlying database connection.
func (s *EvalStore) Close() error {
	return s.db.Close()
}

// migrate creates the eval tables if they don't already exist.
func (s *EvalStore) migrate() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS eval_runs (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		suite_name  TEXT    NOT NULL,
		agent_id    TEXT    NOT NULL DEFAULT '',
		model       TEXT    NOT NULL DEFAULT '',
		avg_score   REAL    NOT NULL DEFAULT 0,
		pass_rate   REAL    NOT NULL DEFAULT 0,
		total_tests INTEGER NOT NULL DEFAULT 0,
		passed      INTEGER NOT NULL DEFAULT 0,
		failed      INTEGER NOT NULL DEFAULT 0,
		duration_ms INTEGER NOT NULL DEFAULT 0,
		run_at      TEXT    NOT NULL
	);

	CREATE TABLE IF NOT EXISTS eval_results (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id      INTEGER NOT NULL REFERENCES eval_runs(id) ON DELETE CASCADE,
		test_name   TEXT    NOT NULL,
		passed      INTEGER NOT NULL DEFAULT 0,
		score       REAL    NOT NULL DEFAULT 0,
		input       TEXT    NOT NULL DEFAULT '',
		output      TEXT    NOT NULL DEFAULT '',
		errors      TEXT    NOT NULL DEFAULT '[]',
		duration_ms INTEGER NOT NULL DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_eval_runs_suite ON eval_runs(suite_name, run_at);
	CREATE INDEX IF NOT EXISTS idx_eval_results_run ON eval_results(run_id);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("eval store: migrate: %w", err)
	}

	return nil
}

// SaveRun persists a complete evaluation run (summary + per-test results) and
// returns the run ID. The agentID and model are optional context fields.
func (s *EvalStore) SaveRun(suite, agentID, model string, report EvalReport) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("eval store: begin tx: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	var passRate float64
	if report.TotalTests > 0 {
		passRate = float64(report.Passed) / float64(report.TotalTests)
	}

	res, err := tx.Exec(
		`INSERT INTO eval_runs (suite_name, agent_id, model, avg_score, pass_rate, total_tests, passed, failed, duration_ms, run_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		suite, agentID, model, report.AvgScore, passRate,
		report.TotalTests, report.Passed, report.Failed,
		report.Duration.Milliseconds(), report.RunAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("eval store: insert run: %w", err)
	}

	runID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("eval store: last insert id: %w", err)
	}

	stmt, err := tx.Prepare(
		`INSERT INTO eval_results (run_id, test_name, passed, score, input, output, errors, duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return 0, fmt.Errorf("eval store: prepare result insert: %w", err)
	}

	defer func() { _ = stmt.Close() }()

	for _, r := range report.Results {
		passedInt := 0
		if r.Passed {
			passedInt = 1
		}

		errJSON, _ := json.Marshal(r.Errors)

		if _, err := stmt.Exec(
			runID, r.Name, passedInt, r.Score,
			r.Input, r.Output, string(errJSON),
			r.Duration.Milliseconds(),
		); err != nil {
			return 0, fmt.Errorf("eval store: insert result %q: %w", r.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("eval store: commit: %w", err)
	}

	return runID, nil
}

// GetRunHistory returns the most recent runs for a given suite, ordered by
// most recent first. Pass limit <= 0 to get all runs.
func (s *EvalStore) GetRunHistory(suite string, limit int) ([]EvalRunSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `SELECT id, suite_name, agent_id, model, avg_score, pass_rate,
	                 total_tests, passed, failed, duration_ms, run_at
	          FROM eval_runs
	          WHERE suite_name = ?
	          ORDER BY run_at DESC`

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query, suite)
	if err != nil {
		return nil, fmt.Errorf("eval store: query runs: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var summaries []EvalRunSummary

	for rows.Next() {
		var rs EvalRunSummary
		var runAtStr string

		if err := rows.Scan(
			&rs.ID, &rs.SuiteName, &rs.AgentID, &rs.Model,
			&rs.AvgScore, &rs.PassRate,
			&rs.TotalTests, &rs.Passed, &rs.Failed,
			&rs.DurationMs, &runAtStr,
		); err != nil {
			return nil, fmt.Errorf("eval store: scan run: %w", err)
		}

		if t, err := time.Parse(time.RFC3339, runAtStr); err == nil {
			rs.RunAt = t
		}

		summaries = append(summaries, rs)
	}

	return summaries, rows.Err()
}

// GetTrend compares the last 3 runs for a suite and returns a trend label:
//   - "improving"         — avg score is increasing across runs
//   - "declining"         — avg score is decreasing across runs
//   - "stable"            — avg score is flat (within +-0.01)
//   - "insufficient_data" — fewer than 2 runs available
func (s *EvalStore) GetTrend(suite string) (string, error) {
	history, err := s.GetRunHistory(suite, 3)
	if err != nil {
		return "", err
	}

	if len(history) < 2 {
		return "insufficient_data", nil
	}

	// history is ordered newest-first; compare newest vs oldest available.
	newest := history[0].AvgScore
	oldest := history[len(history)-1].AvgScore

	const threshold = 0.01

	diff := newest - oldest
	if diff > threshold {
		return "improving", nil
	}

	if diff < -threshold {
		return "declining", nil
	}

	return "stable", nil
}

// EvalRunDetail contains the run summary plus per-test results.
type EvalRunDetail struct {
	EvalRunSummary
	Results []EvalResultRow `json:"results"`
}

// EvalResultRow is a single test result as persisted in the database.
type EvalResultRow struct {
	ID         int64    `json:"id"`
	RunID      int64    `json:"run_id"`
	TestName   string   `json:"test_name"`
	Passed     bool     `json:"passed"`
	Score      float64  `json:"score"`
	Input      string   `json:"input"`
	Output     string   `json:"output"`
	Errors     []string `json:"errors"`
	DurationMs int64    `json:"duration_ms"`
}

// GetRunByID returns a single run summary by ID, or nil if not found.
func (s *EvalStore) GetRunByID(id int64) (*EvalRunSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var rs EvalRunSummary
	var runAtStr string

	err := s.db.QueryRow(
		`SELECT id, suite_name, agent_id, model, avg_score, pass_rate,
		        total_tests, passed, failed, duration_ms, run_at
		 FROM eval_runs WHERE id = ?`, id,
	).Scan(
		&rs.ID, &rs.SuiteName, &rs.AgentID, &rs.Model,
		&rs.AvgScore, &rs.PassRate,
		&rs.TotalTests, &rs.Passed, &rs.Failed,
		&rs.DurationMs, &runAtStr,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("eval store: get run by id: %w", err)
	}

	if t, err := time.Parse(time.RFC3339, runAtStr); err == nil {
		rs.RunAt = t
	}

	return &rs, nil
}

// GetRunResults returns all per-test results for a given run ID.
func (s *EvalStore) GetRunResults(runID int64) ([]EvalResultRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(
		`SELECT id, run_id, test_name, passed, score, input, output, errors, duration_ms
		 FROM eval_results WHERE run_id = ? ORDER BY id`, runID,
	)
	if err != nil {
		return nil, fmt.Errorf("eval store: query results: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var results []EvalResultRow

	for rows.Next() {
		var r EvalResultRow
		var passedInt int
		var errorsJSON string

		if err := rows.Scan(
			&r.ID, &r.RunID, &r.TestName, &passedInt,
			&r.Score, &r.Input, &r.Output, &errorsJSON, &r.DurationMs,
		); err != nil {
			return nil, fmt.Errorf("eval store: scan result: %w", err)
		}

		r.Passed = passedInt != 0

		if errorsJSON != "" && errorsJSON != "[]" {
			_ = json.Unmarshal([]byte(errorsJSON), &r.Errors)
		}

		if r.Errors == nil {
			r.Errors = []string{}
		}

		results = append(results, r)
	}

	return results, rows.Err()
}

// GetAllSuiteNames returns distinct suite names that have at least one run.
func (s *EvalStore) GetAllSuiteNames() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT DISTINCT suite_name FROM eval_runs ORDER BY suite_name`)
	if err != nil {
		return nil, fmt.Errorf("eval store: query suite names: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var names []string

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("eval store: scan suite name: %w", err)
		}

		names = append(names, name)
	}

	return names, rows.Err()
}

// GetRecentRuns returns the most recent runs across all suites, ordered by
// most recent first. Pass limit <= 0 to get all runs.
func (s *EvalStore) GetRecentRuns(limit int) ([]EvalRunSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `SELECT id, suite_name, agent_id, model, avg_score, pass_rate,
	                 total_tests, passed, failed, duration_ms, run_at
	          FROM eval_runs
	          ORDER BY run_at DESC`

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("eval store: query recent runs: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var summaries []EvalRunSummary

	for rows.Next() {
		var rs EvalRunSummary
		var runAtStr string

		if err := rows.Scan(
			&rs.ID, &rs.SuiteName, &rs.AgentID, &rs.Model,
			&rs.AvgScore, &rs.PassRate,
			&rs.TotalTests, &rs.Passed, &rs.Failed,
			&rs.DurationMs, &runAtStr,
		); err != nil {
			return nil, fmt.Errorf("eval store: scan run: %w", err)
		}

		if t, err := time.Parse(time.RFC3339, runAtStr); err == nil {
			rs.RunAt = t
		}

		summaries = append(summaries, rs)
	}

	return summaries, rows.Err()
}
