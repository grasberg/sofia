package budget

import (
	"database/sql"
	"fmt"
	"time"
)

// Store defines the persistence interface for budget state.
// Implementations must be safe for concurrent use.
type Store interface {
	// Load retrieves all persisted spend entries.
	Load() (map[string]*spendEntry, error)
	// Save persists the current spend entries.
	Save(entries map[string]*spendEntry) error
}

// SQLiteStore persists budget spend state to a SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a SQLiteStore using the provided database connection
// and ensures the budget_spend table exists.
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	const ddl = `
CREATE TABLE IF NOT EXISTS budget_spend (
    agent_id     TEXT PRIMARY KEY,
    amount       REAL NOT NULL DEFAULT 0,
    period_start DATETIME NOT NULL
);`
	if _, err := db.Exec(ddl); err != nil {
		return nil, fmt.Errorf("budget: create table: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// Load retrieves all persisted spend entries from the database.
func (s *SQLiteStore) Load() (map[string]*spendEntry, error) {
	rows, err := s.db.Query(`SELECT agent_id, amount, period_start FROM budget_spend`)
	if err != nil {
		return nil, fmt.Errorf("budget: load: %w", err)
	}
	defer rows.Close()

	entries := make(map[string]*spendEntry)
	for rows.Next() {
		var agentID string
		var amount float64
		var periodStart time.Time
		if err := rows.Scan(&agentID, &amount, &periodStart); err != nil {
			return nil, fmt.Errorf("budget: scan: %w", err)
		}
		entries[agentID] = &spendEntry{
			Amount:      amount,
			PeriodStart: periodStart,
		}
	}
	return entries, rows.Err()
}

// Save persists all spend entries to the database using an upsert.
func (s *SQLiteStore) Save(entries map[string]*spendEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("budget: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO budget_spend (agent_id, amount, period_start)
		VALUES (?, ?, ?)
		ON CONFLICT(agent_id) DO UPDATE SET
			amount = excluded.amount,
			period_start = excluded.period_start`)
	if err != nil {
		return fmt.Errorf("budget: prepare: %w", err)
	}
	defer stmt.Close()

	for agentID, entry := range entries {
		if _, err := stmt.Exec(agentID, entry.Amount, entry.PeriodStart.UTC()); err != nil {
			return fmt.Errorf("budget: upsert %s: %w", agentID, err)
		}
	}

	return tx.Commit()
}
