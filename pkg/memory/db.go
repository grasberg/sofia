package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (CGO_ENABLED=0 compatible)

	"github.com/grasberg/sofia/pkg/providers"
)

const schemaVersion = 1

// MemoryDB is a shared SQLite database for session history and memory notes.
// It is opened once at AgentLoop startup and shared across all AgentInstances.
type MemoryDB struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at the given path.
// It runs schema migrations, enables WAL mode, and sets foreign_keys ON.
// Pass ":memory:" for an in-process database (useful in tests).
func Open(path string) (*MemoryDB, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("memory: create dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("memory: open db: %w", err)
	}

	// Single writer connection to avoid SQLITE_BUSY on concurrent writes.
	db.SetMaxOpenConns(1)

	if _, err = db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("memory: set WAL mode: %w", err)
	}
	if _, err = db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("memory: enable foreign keys: %w", err)
	}

	m := &MemoryDB{db: db}
	if err = m.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("memory: migrate: %w", err)
	}

	return m, nil
}

// Close closes the database connection.
func (m *MemoryDB) Close() error {
	return m.db.Close()
}

// ---------------------------------------------------------------------------
// Schema migration
// ---------------------------------------------------------------------------

func (m *MemoryDB) migrate() error {
	// Create schema_version table first if it doesn't exist.
	_, err := m.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`)
	if err != nil {
		return err
	}

	var current int
	row := m.db.QueryRow(`SELECT version FROM schema_version LIMIT 1`)
	if scanErr := row.Scan(&current); scanErr != nil {
		// No row yet — start at 0.
		current = 0
	}

	if current >= schemaVersion {
		return nil
	}

	// Version 1: create all tables.
	if current < 1 {
		if err = m.applyV1(); err != nil {
			return err
		}
	}

	// Upsert schema version.
	_, err = m.db.Exec(`DELETE FROM schema_version`)
	if err != nil {
		return err
	}
	_, err = m.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, schemaVersion)
	return err
}

func (m *MemoryDB) applyV1() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS sessions (
    key        TEXT PRIMARY KEY,
    agent_id   TEXT NOT NULL DEFAULT '',
    summary    TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    session_key       TEXT    NOT NULL REFERENCES sessions(key) ON DELETE CASCADE,
    position          INTEGER NOT NULL,
    role              TEXT    NOT NULL,
    content           TEXT    NOT NULL DEFAULT '',
    tool_calls        TEXT    NOT NULL DEFAULT '[]',
    tool_call_id      TEXT    NOT NULL DEFAULT '',
    images            TEXT    NOT NULL DEFAULT '[]',
    reasoning_content TEXT    NOT NULL DEFAULT '',
    created_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(session_key, position)
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_key, position);

CREATE TABLE IF NOT EXISTS memory_notes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id   TEXT NOT NULL DEFAULT '',
    kind       TEXT NOT NULL,
    date_key   TEXT NOT NULL DEFAULT '',
    content    TEXT NOT NULL DEFAULT '',
    updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_notes_key ON memory_notes(agent_id, kind, date_key);
`
	_, err := m.db.Exec(ddl)
	return err
}

// ---------------------------------------------------------------------------
// Session CRUD
// ---------------------------------------------------------------------------

// GetOrCreateSession ensures a session row exists for the given key and
// returns the current summary.  agentID is stored on creation only.
func (m *MemoryDB) GetOrCreateSession(key, agentID string) (summary string, err error) {
	now := time.Now().UTC()
	_, err = m.db.Exec(
		`INSERT INTO sessions (key, agent_id, summary, created_at, updated_at)
		 VALUES (?, ?, '', ?, ?)
		 ON CONFLICT(key) DO NOTHING`,
		key, agentID, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("memory: upsert session: %w", err)
	}
	row := m.db.QueryRow(`SELECT summary FROM sessions WHERE key = ?`, key)
	err = row.Scan(&summary)
	if err != nil {
		return "", fmt.Errorf("memory: get session: %w", err)
	}
	return summary, nil
}

// GetSummary returns the summary for a session key (empty string if not found).
func (m *MemoryDB) GetSummary(key string) string {
	var s string
	_ = m.db.QueryRow(`SELECT summary FROM sessions WHERE key = ?`, key).Scan(&s)
	return s
}

// SetSummary updates the summary for a session key.
func (m *MemoryDB) SetSummary(key, summary string) error {
	_, err := m.db.Exec(
		`UPDATE sessions SET summary = ?, updated_at = ? WHERE key = ?`,
		summary, time.Now().UTC(), key,
	)
	return err
}

// AppendMessage appends a single message at the next position in the session.
// The session row must already exist (call GetOrCreateSession first).
func (m *MemoryDB) AppendMessage(key string, msg providers.Message) error {
	toolCallsJSON, err := json.Marshal(msg.ToolCalls)
	if err != nil {
		return fmt.Errorf("memory: marshal tool_calls: %w", err)
	}
	imagesJSON, err := json.Marshal(msg.Images)
	if err != nil {
		return fmt.Errorf("memory: marshal images: %w", err)
	}

	_, err = m.db.Exec(
		`INSERT INTO messages
		    (session_key, position, role, content, tool_calls, tool_call_id, images, reasoning_content, created_at)
		 VALUES (
		    ?,
		    (SELECT COALESCE(MAX(position), -1) + 1 FROM messages WHERE session_key = ?),
		    ?, ?, ?, ?, ?, ?,
		    datetime('now')
		 )`,
		key, key,
		msg.Role, msg.Content, string(toolCallsJSON), msg.ToolCallID,
		string(imagesJSON), msg.ReasoningContent,
	)
	if err != nil {
		return fmt.Errorf("memory: append message: %w", err)
	}

	_, err = m.db.Exec(`UPDATE sessions SET updated_at = ? WHERE key = ?`, time.Now().UTC(), key)
	return err
}

// GetMessages returns all messages for a session, ordered by position.
func (m *MemoryDB) GetMessages(key string) ([]providers.Message, error) {
	rows, err := m.db.Query(
		`SELECT role, content, tool_calls, tool_call_id, images, reasoning_content
		 FROM messages WHERE session_key = ? ORDER BY position ASC`,
		key,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: query messages: %w", err)
	}
	defer rows.Close()

	var msgs []providers.Message
	for rows.Next() {
		var msg providers.Message
		var toolCallsJSON, imagesJSON string
		if err = rows.Scan(
			&msg.Role, &msg.Content, &toolCallsJSON, &msg.ToolCallID,
			&imagesJSON, &msg.ReasoningContent,
		); err != nil {
			return nil, fmt.Errorf("memory: scan message: %w", err)
		}
		if err = json.Unmarshal([]byte(toolCallsJSON), &msg.ToolCalls); err != nil {
			msg.ToolCalls = nil
		}
		if err = json.Unmarshal([]byte(imagesJSON), &msg.Images); err != nil {
			msg.Images = nil
		}
		msgs = append(msgs, msg)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: iterate messages: %w", err)
	}
	return msgs, nil
}

// SetMessages replaces all messages in a session with the provided slice.
func (m *MemoryDB) SetMessages(key string, msgs []providers.Message) error {
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("memory: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.Exec(`DELETE FROM messages WHERE session_key = ?`, key); err != nil {
		return fmt.Errorf("memory: delete messages: %w", err)
	}

	for i, msg := range msgs {
		toolCallsJSON, _ := json.Marshal(msg.ToolCalls)
		imagesJSON, _ := json.Marshal(msg.Images)
		_, err = tx.Exec(
			`INSERT INTO messages
			    (session_key, position, role, content, tool_calls, tool_call_id, images, reasoning_content, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
			key, i,
			msg.Role, msg.Content, string(toolCallsJSON), msg.ToolCallID,
			string(imagesJSON), msg.ReasoningContent,
		)
		if err != nil {
			return fmt.Errorf("memory: insert message at %d: %w", i, err)
		}
	}

	_, err = tx.Exec(`UPDATE sessions SET updated_at = ? WHERE key = ?`, time.Now().UTC(), key)
	if err != nil {
		return fmt.Errorf("memory: update session updated_at: %w", err)
	}

	return tx.Commit()
}

// TruncateMessages keeps only the last keepLast messages for a session.
// If keepLast <= 0, all messages are deleted.
func (m *MemoryDB) TruncateMessages(key string, keepLast int) error {
	if keepLast <= 0 {
		_, err := m.db.Exec(`DELETE FROM messages WHERE session_key = ?`, key)
		return err
	}

	_, err := m.db.Exec(
		`DELETE FROM messages
		 WHERE session_key = ?
		   AND position NOT IN (
		       SELECT position FROM messages WHERE session_key = ?
		       ORDER BY position DESC LIMIT ?
		   )`,
		key, key, keepLast,
	)
	return err
}

// DeleteSession deletes a session and all its messages (cascaded).
func (m *MemoryDB) DeleteSession(key string) error {
	_, err := m.db.Exec(`DELETE FROM sessions WHERE key = ?`, key)
	return err
}

// SessionRow holds the fields returned by ListSessions.
type SessionRow struct {
	Key       string
	AgentID   string
	Summary   string
	CreatedAt time.Time
	UpdatedAt time.Time
	MsgCount  int
	Preview   string
}

// ListSessions returns lightweight metadata for all sessions.
func (m *MemoryDB) ListSessions() ([]SessionRow, error) {
	const q = `
SELECT s.key, s.agent_id, s.summary, s.created_at, s.updated_at,
       COUNT(msg.id) AS msg_count,
       COALESCE((
           SELECT content FROM messages
           WHERE session_key = s.key AND role = 'user' AND content != ''
           ORDER BY position ASC LIMIT 1
       ), '') AS preview
FROM sessions s
LEFT JOIN messages msg ON msg.session_key = s.key
GROUP BY s.key
ORDER BY s.updated_at DESC`

	rows, err := m.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("memory: list sessions: %w", err)
	}
	defer rows.Close()

	var result []SessionRow
	for rows.Next() {
		var r SessionRow
		var createdStr, updatedStr string
		if err = rows.Scan(
			&r.Key, &r.AgentID, &r.Summary, &createdStr, &updatedStr,
			&r.MsgCount, &r.Preview,
		); err != nil {
			return nil, fmt.Errorf("memory: scan session row: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		result = append(result, r)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Memory notes CRUD
// ---------------------------------------------------------------------------

// GetNote returns the content of a memory note identified by (agentID, kind, dateKey).
// Returns "" if the note does not exist.
func (m *MemoryDB) GetNote(agentID, kind, dateKey string) string {
	var content string
	_ = m.db.QueryRow(
		`SELECT content FROM memory_notes WHERE agent_id = ? AND kind = ? AND date_key = ?`,
		agentID, kind, dateKey,
	).Scan(&content)
	return content
}

// SetNote upserts a memory note.
func (m *MemoryDB) SetNote(agentID, kind, dateKey, content string) error {
	now := time.Now().UTC()
	_, err := m.db.Exec(
		`INSERT INTO memory_notes (agent_id, kind, date_key, content, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(agent_id, kind, date_key) DO UPDATE SET content = excluded.content, updated_at = excluded.updated_at`,
		agentID, kind, dateKey, content, now,
	)
	return err
}
