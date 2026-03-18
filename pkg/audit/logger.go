package audit

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	ID         int64     `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	AgentID    string    `json:"agent_id"`
	SessionKey string    `json:"session_key,omitempty"`
	Channel    string    `json:"channel,omitempty"`
	Action     string    `json:"action"`
	Detail     string    `json:"detail"`
	Input      string    `json:"input,omitempty"`
	Output     string    `json:"output,omitempty"`
	Duration   int64     `json:"duration_ms,omitempty"`
	Success    bool      `json:"success"`
	Metadata   string    `json:"metadata,omitempty"`
}

// QueryOpts controls filtering and pagination for audit log queries.
type QueryOpts struct {
	AgentID string
	Action  string
	Since   time.Time
	Until   time.Time
	Limit   int
	Offset  int
}

// AuditLogger writes structured audit entries to SQLite.
type AuditLogger struct {
	db *sql.DB
	mu sync.Mutex
}

// NewAuditLogger opens (or creates) the SQLite database at dbPath and ensures the
// audit_log table and indexes exist. Pass ":memory:" for an in-process database
// (useful in tests).
func NewAuditLogger(dbPath string) (*AuditLogger, error) {
	if dbPath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			return nil, fmt.Errorf("audit: create dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("audit: open db: %w", err)
	}

	// Single writer connection to avoid SQLITE_BUSY on concurrent writes.
	db.SetMaxOpenConns(1)

	if _, err = db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("audit: set WAL mode: %w", err)
	}

	al := &AuditLogger{db: db}
	if err = al.createSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("audit: create schema: %w", err)
	}

	return al, nil
}

func (al *AuditLogger) createSchema() error {
	schema := `
CREATE TABLE IF NOT EXISTS audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp   DATETIME DEFAULT CURRENT_TIMESTAMP,
    agent_id    TEXT,
    session_key TEXT,
    channel     TEXT,
    action      TEXT NOT NULL,
    detail      TEXT,
    input       TEXT,
    output      TEXT,
    duration_ms INTEGER,
    success     BOOLEAN DEFAULT 1,
    metadata    TEXT
);
CREATE INDEX IF NOT EXISTS idx_audit_action    ON audit_log(action);
CREATE INDEX IF NOT EXISTS idx_audit_agent     ON audit_log(agent_id);
CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp);
`
	_, err := al.db.Exec(schema)
	return err
}

// Log inserts a single audit entry into the database. The entry's Timestamp is
// set to time.Now().UTC() if it is zero. The returned entry's ID is populated
// after a successful insert.
func (al *AuditLogger) Log(entry AuditEntry) error {
	al.mu.Lock()
	defer al.mu.Unlock()

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	// Validate metadata is valid JSON when non-empty.
	if entry.Metadata != "" {
		if !json.Valid([]byte(entry.Metadata)) {
			return fmt.Errorf("audit: metadata is not valid JSON")
		}
	}

	_, err := al.db.Exec(
		`INSERT INTO audit_log
			(timestamp, agent_id, session_key, channel, action, detail,
			 input, output, duration_ms, success, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.Timestamp.UTC().Format(time.RFC3339Nano),
		entry.AgentID,
		entry.SessionKey,
		entry.Channel,
		entry.Action,
		entry.Detail,
		entry.Input,
		entry.Output,
		entry.Duration,
		entry.Success,
		entry.Metadata,
	)
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}

	return nil
}

// Query returns audit entries matching the given options. Results are ordered
// by timestamp descending (newest first). A zero Limit defaults to 100.
func (al *AuditLogger) Query(opts QueryOpts) ([]AuditEntry, error) {
	al.mu.Lock()
	defer al.mu.Unlock()

	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	query := `SELECT id, timestamp, agent_id, session_key, channel, action,
	                  detail, input, output, duration_ms, success, metadata
	           FROM audit_log WHERE 1=1`
	args := []any{}

	if opts.AgentID != "" {
		query += " AND agent_id = ?"
		args = append(args, opts.AgentID)
	}
	if opts.Action != "" {
		query += " AND action = ?"
		args = append(args, opts.Action)
	}
	if !opts.Since.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, opts.Since.UTC().Format(time.RFC3339Nano))
	}
	if !opts.Until.IsZero() {
		query += " AND timestamp <= ?"
		args = append(args, opts.Until.UTC().Format(time.RFC3339Nano))
	}

	query += " ORDER BY timestamp DESC LIMIT ? OFFSET ?"
	args = append(args, opts.Limit, opts.Offset)

	rows, err := al.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("audit: query: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var ts string
		var agentID, sessionKey, channel, detail, input, output, metadata sql.NullString
		var durationMs sql.NullInt64
		var success sql.NullBool

		if err := rows.Scan(
			&e.ID, &ts, &agentID, &sessionKey, &channel, &e.Action,
			&detail, &input, &output, &durationMs, &success, &metadata,
		); err != nil {
			return nil, fmt.Errorf("audit: scan: %w", err)
		}

		e.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		e.AgentID = agentID.String
		e.SessionKey = sessionKey.String
		e.Channel = channel.String
		e.Detail = detail.String
		e.Input = input.String
		e.Output = output.String
		e.Duration = durationMs.Int64
		e.Success = !success.Valid || success.Bool
		e.Metadata = metadata.String

		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: rows: %w", err)
	}

	return entries, nil
}

// Close closes the underlying database connection.
func (al *AuditLogger) Close() error {
	return al.db.Close()
}
