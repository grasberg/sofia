package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (CGO_ENABLED=0 compatible)

	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/trace"
)

const schemaVersion = 14

// Encryptor defines the interface for encrypting/decrypting stored values.
// Implementations must be safe for concurrent use.
type Encryptor interface {
	Encrypt(plaintext string) string
	Decrypt(ciphertext string) string
	Active() bool
}

// MemoryDB is a shared SQLite database for session history and memory notes.
// It is opened once at AgentLoop startup and shared across all AgentInstances.
type MemoryDB struct {
	mu        sync.RWMutex
	db        *sql.DB
	path      string
	enc       Encryptor
	statCount atomic.Int64 // counter for periodic stats pruning
}

// Option configures optional MemoryDB settings.
type Option func(*MemoryDB)

// WithEncryptor injects a custom Encryptor. If not set, the default
// environment-based encryptor (SOFIA_DB_KEY) is used.
func WithEncryptor(enc Encryptor) Option {
	return func(m *MemoryDB) {
		m.enc = enc
	}
}

// Open opens (or creates) the SQLite database at the given path.
// It runs schema migrations, enables WAL mode, and sets foreign_keys ON.
// Pass ":memory:" for an in-process database (useful in tests).
// Optional Option values can be passed to configure encryption, etc.
func Open(path string, opts ...Option) (*MemoryDB, error) {
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

	m := &MemoryDB{
		db:   db,
		path: path,
		enc:  defaultEncryptor{},
	}
	for _, opt := range opts {
		opt(m)
	}
	if err = m.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("memory: migrate: %w", err)
	}

	return m, nil
}

// Path returns the filepath where the database is located.
func (m *MemoryDB) Path() string {
	return m.path
}

// Exec allows raw SQL execution, primarily useful for test setups and migrations.
func (m *MemoryDB) Exec(query string, args ...any) (sql.Result, error) {
	return m.db.Exec(query, args...)
}

// Query executes a query that returns rows.
func (m *MemoryDB) Query(query string, args ...any) (*sql.Rows, error) {
	return m.db.Query(query, args...)
}

// QueryRow executes a query that returns at most one row.
func (m *MemoryDB) QueryRow(query string, args ...any) *sql.Row {
	return m.db.QueryRow(query, args...)
}

// Ping verifies the database connection is alive by executing a trivial query.
func (m *MemoryDB) Ping() error {
	var n int
	return m.db.QueryRow(`SELECT 1`).Scan(&n)
}

// Close closes the database connection.
func (m *MemoryDB) Close() error {
	return m.db.Close()
}

// DB returns the underlying *sql.DB for use by subsystems that need direct
// database access (e.g. budget persistence). The caller must not close it.
func (m *MemoryDB) DB() *sql.DB {
	return m.db
}

// ---------------------------------------------------------------------------
// Schema migration
// ---------------------------------------------------------------------------

// columnExists checks if a column exists in a table using PRAGMA table_info.
func (m *MemoryDB) columnExists(table, column string) bool {
	rows, err := m.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			continue
		}
		if name == column {
			return true
		}
	}
	return false
}

// setVersion updates the schema_version table to the given version.
func (m *MemoryDB) setVersion(version int) error {
	if _, err := m.db.Exec(`DELETE FROM schema_version`); err != nil {
		return err
	}
	_, err := m.db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, version)
	return err
}

// runMigrationInTx wraps a migration function and its version bump in a transaction.
// The migration function receives the *sql.Tx to use for all DDL/DML.
func (m *MemoryDB) runMigrationInTx(version int, fn func(tx *sql.Tx) error) error {
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("migration v%d: begin tx: %w", version, err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx); err != nil {
		return fmt.Errorf("migration v%d: %w", version, err)
	}

	// Update version inside the transaction.
	if _, err := tx.Exec(`DELETE FROM schema_version`); err != nil {
		return fmt.Errorf("migration v%d: delete version: %w", version, err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, version); err != nil {
		return fmt.Errorf("migration v%d: insert version: %w", version, err)
	}

	return tx.Commit()
}

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

	type migration struct {
		version int
		fn      func(tx *sql.Tx) error
		// postTx runs after the transaction commits (for ALTER TABLE statements).
		postTx func() error
	}

	migrations := []migration{
		{1, m.applyV1tx, nil},
		{2, m.applyV2tx, nil},
		{3, m.applyV3tx, nil},
		{4, m.applyV4tx, nil},
		{5, m.applyV5tx, nil},
		{6, m.applyV6tx, nil},
		{7, m.applyV7tx, nil},
		{8, m.applyV8tx, nil},
		{9, nil, m.applyV9},
		{10, m.applyV10tx, nil},
		{11, m.applyV11tx, nil},
		{12, nil, m.applyV12},
		{13, m.applyV13tx, nil},
		{14, m.applyV14tx, nil},
	}

	for _, mig := range migrations {
		if current >= mig.version {
			continue
		}
		if mig.fn != nil {
			if err := m.runMigrationInTx(mig.version, mig.fn); err != nil {
				return err
			}
		}
		if mig.postTx != nil {
			if err := mig.postTx(); err != nil {
				return err
			}
			// Update version outside tx for ALTER TABLE migrations.
			if err := m.setVersion(mig.version); err != nil {
				return fmt.Errorf("migration v%d: set version: %w", mig.version, err)
			}
		}
	}

	return nil
}

func (m *MemoryDB) applyV1tx(tx *sql.Tx) error {
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
	_, err := tx.Exec(ddl)
	return err
}

// ---------------------------------------------------------------------------
// Session CRUD
// ---------------------------------------------------------------------------

// GetOrCreateSession ensures a session row exists for the given key and
// returns the current summary.  agentID is stored on creation only.
func (m *MemoryDB) GetOrCreateSession(key, agentID string) (summary string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

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
	m.mu.RLock()
	defer m.mu.RUnlock()

	var s string
	_ = m.db.QueryRow(`SELECT summary FROM sessions WHERE key = ?`, key).Scan(&s)
	return s
}

// SetSummary updates the summary for a session key.
func (m *MemoryDB) SetSummary(key, summary string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`UPDATE sessions SET summary = ?, updated_at = ? WHERE key = ?`,
		summary, time.Now().UTC(), key,
	)
	return err
}

// AppendMessage appends a single message at the next position in the session.
// The session row must already exist (call GetOrCreateSession first).
// The INSERT and session UPDATE are wrapped in a single transaction.
func (m *MemoryDB) AppendMessage(key string, msg providers.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	toolCallsJSON, err := json.Marshal(msg.ToolCalls)
	if err != nil {
		return fmt.Errorf("memory: marshal tool_calls: %w", err)
	}
	imagesJSON, err := json.Marshal(msg.Images)
	if err != nil {
		return fmt.Errorf("memory: marshal images: %w", err)
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("memory: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(
		`INSERT INTO messages
		    (session_key, position, role, content, tool_calls, tool_call_id, tool_name, images, reasoning_content, created_at)
		 VALUES (
		    ?,
		    (SELECT COALESCE(MAX(position), -1) + 1 FROM messages WHERE session_key = ?),
		    ?, ?, ?, ?, ?, ?, ?,
		    datetime('now')
		 )`,
		key, key,
		msg.Role, m.enc.Encrypt(msg.Content), string(toolCallsJSON), msg.ToolCallID, msg.ToolName,
		string(imagesJSON), m.enc.Encrypt(msg.ReasoningContent),
	)
	if err != nil {
		return fmt.Errorf("memory: append message: %w", err)
	}

	_, err = tx.Exec(`UPDATE sessions SET updated_at = ? WHERE key = ?`, time.Now().UTC(), key)
	if err != nil {
		return fmt.Errorf("memory: update session updated_at: %w", err)
	}

	return tx.Commit()
}

// GetMessages returns all messages for a session, ordered by position.
func (m *MemoryDB) GetMessages(key string) ([]providers.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(
		`SELECT role, content, tool_calls, tool_call_id, tool_name, images, reasoning_content
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
			&msg.Role, &msg.Content, &toolCallsJSON, &msg.ToolCallID, &msg.ToolName,
			&imagesJSON, &msg.ReasoningContent,
		); err != nil {
			return nil, fmt.Errorf("memory: scan message: %w", err)
		}
		msg.Content = m.enc.Decrypt(msg.Content)
		msg.ReasoningContent = m.enc.Decrypt(msg.ReasoningContent)
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
	m.mu.Lock()
	defer m.mu.Unlock()

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
			    (session_key, position, role, content, tool_calls, tool_call_id, tool_name, images, reasoning_content, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
			key, i,
			msg.Role, m.enc.Encrypt(msg.Content), string(toolCallsJSON), msg.ToolCallID, msg.ToolName,
			string(imagesJSON), m.enc.Encrypt(msg.ReasoningContent),
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
	m.mu.Lock()
	defer m.mu.Unlock()

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
	m.mu.Lock()
	defer m.mu.Unlock()

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
	m.mu.RLock()
	defer m.mu.RUnlock()

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

// GetSessionMeta returns metadata for a single session by key, or nil if not found.
func (m *MemoryDB) GetSessionMeta(key string) *SessionRow {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var r SessionRow
	var createdStr, updatedStr string
	err := m.db.QueryRow(
		`SELECT s.key, s.agent_id, s.summary, s.created_at, s.updated_at, COUNT(msg.id)
		 FROM sessions s LEFT JOIN messages msg ON msg.session_key = s.key
		 WHERE s.key = ? GROUP BY s.key`,
		key,
	).Scan(&r.Key, &r.AgentID, &r.Summary, &createdStr, &updatedStr, &r.MsgCount)
	if err != nil {
		return nil
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &r
}

// SearchMessageRow holds a single message result from SearchMessages.
type SearchMessageRow struct {
	SessionKey string
	Role       string
	Content    string
	CreatedAt  string
}

// SearchMessages returns user and assistant messages whose content contains the query substring
// (case-insensitive). Results are ordered by recency. Pass limit <= 0 for no limit.
// When encryption is active, all qualifying rows are fetched and filtered in Go
// because SQL LIKE cannot match against encrypted content.
func (m *MemoryDB) SearchMessages(query string, limit int) ([]SearchMessageRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	// When encryption is active, fetch all candidate rows and filter in Go.
	if m.enc.Active() {
		return m.searchMessagesEncrypted(query, limit)
	}

	q := `SELECT m.session_key, m.role, m.content, m.created_at
	      FROM messages m
	      WHERE m.role IN ('user', 'assistant')
	        AND m.content != ''
	        AND LOWER(m.content) LIKE '%' || LOWER(?) || '%'
	      ORDER BY m.created_at DESC
	      LIMIT ?`

	rows, err := m.db.Query(q, query, limit)
	if err != nil {
		return nil, fmt.Errorf("memory: search messages: %w", err)
	}
	defer rows.Close()

	var result []SearchMessageRow
	for rows.Next() {
		var r SearchMessageRow
		if err = rows.Scan(&r.SessionKey, &r.Role, &r.Content, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("memory: scan search row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// searchMessagesEncrypted fetches all user/assistant messages, decrypts them,
// and filters by query in Go. This is needed because SQL LIKE cannot operate
// on encrypted content.
func (m *MemoryDB) searchMessagesEncrypted(query string, limit int) ([]SearchMessageRow, error) {
	q := `SELECT m.session_key, m.role, m.content, m.created_at
	      FROM messages m
	      WHERE m.role IN ('user', 'assistant')
	        AND m.content != ''
	      ORDER BY m.created_at DESC`

	rows, err := m.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("memory: search messages (encrypted): %w", err)
	}
	defer rows.Close()

	lowerQuery := strings.ToLower(query)
	var result []SearchMessageRow
	for rows.Next() {
		var r SearchMessageRow
		if err = rows.Scan(&r.SessionKey, &r.Role, &r.Content, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("memory: scan search row: %w", err)
		}
		r.Content = m.enc.Decrypt(r.Content)
		if strings.Contains(strings.ToLower(r.Content), lowerQuery) {
			result = append(result, r)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Memory notes CRUD
// ---------------------------------------------------------------------------

// GetNote returns the content of a memory note identified by (agentID, kind, dateKey).
// Returns "" if the note does not exist.
func (m *MemoryDB) GetNote(agentID, kind, dateKey string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var content string
	_ = m.db.QueryRow(
		`SELECT content FROM memory_notes WHERE agent_id = ? AND kind = ? AND date_key = ?`,
		agentID, kind, dateKey,
	).Scan(&content)
	return content
}

// SetNote upserts a memory note.
func (m *MemoryDB) SetNote(agentID, kind, dateKey, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	_, err := m.db.Exec(
		`INSERT INTO memory_notes (agent_id, kind, date_key, content, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(agent_id, kind, date_key) DO UPDATE SET content = excluded.content, updated_at = excluded.updated_at`,
		agentID, kind, dateKey, content, now,
	)
	return err
}

// NoteRow holds the fields returned by ListNotes.
type NoteRow struct {
	AgentID   string
	Kind      string
	DateKey   string
	Content   string
	UpdatedAt time.Time
}

// ListNotes returns all memory notes, ordered by agent_id, kind, date_key.
func (m *MemoryDB) ListNotes() ([]NoteRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	const q = `SELECT agent_id, kind, date_key, content, updated_at
	           FROM memory_notes ORDER BY agent_id, kind, date_key`
	rows, err := m.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("memory: list notes: %w", err)
	}
	defer rows.Close()

	var result []NoteRow
	for rows.Next() {
		var r NoteRow
		var updatedStr string
		if err = rows.Scan(&r.AgentID, &r.Kind, &r.DateKey, &r.Content, &updatedStr); err != nil {
			return nil, fmt.Errorf("memory: scan note row: %w", err)
		}
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		result = append(result, r)
	}
	return result, rows.Err()
}

// ListNotesByKind returns all memory notes of a given kind, ordered by
// updated_at descending. This is useful for finding all handoff records.
func (m *MemoryDB) ListNotesByKind(kind string) ([]NoteRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	const q = `SELECT agent_id, kind, date_key, content, updated_at
	           FROM memory_notes WHERE kind = ? ORDER BY updated_at DESC`
	rows, err := m.db.Query(q, kind)
	if err != nil {
		return nil, fmt.Errorf("memory: list notes by kind: %w", err)
	}
	defer rows.Close()

	var result []NoteRow
	for rows.Next() {
		var r NoteRow
		var updatedStr string
		if err = rows.Scan(&r.AgentID, &r.Kind, &r.DateKey, &r.Content, &updatedStr); err != nil {
			return nil, fmt.Errorf("memory: scan note row: %w", err)
		}
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		result = append(result, r)
	}
	return result, rows.Err()
}

// DeleteNote removes a memory note identified by (agentID, kind, dateKey).
func (m *MemoryDB) DeleteNote(agentID, kind, dateKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`DELETE FROM memory_notes WHERE agent_id = ? AND kind = ? AND date_key = ?`,
		agentID, kind, dateKey,
	)
	return err
}

// ---------------------------------------------------------------------------
// Schema migration v2: semantic memory tables
// ---------------------------------------------------------------------------

func (m *MemoryDB) applyV2tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS semantic_nodes (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id      TEXT    NOT NULL DEFAULT '',
    label         TEXT    NOT NULL DEFAULT '',
    name          TEXT    NOT NULL DEFAULT '',
    properties    TEXT    NOT NULL DEFAULT '{}',
    access_count  INTEGER NOT NULL DEFAULT 0,
    last_accessed DATETIME,
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_semantic_nodes_key ON semantic_nodes(agent_id, label, name);
CREATE INDEX IF NOT EXISTS idx_semantic_nodes_agent ON semantic_nodes(agent_id);

CREATE TABLE IF NOT EXISTS semantic_edges (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id    TEXT    NOT NULL DEFAULT '',
    source_id   INTEGER NOT NULL REFERENCES semantic_nodes(id) ON DELETE CASCADE,
    target_id   INTEGER NOT NULL REFERENCES semantic_nodes(id) ON DELETE CASCADE,
    relation    TEXT    NOT NULL DEFAULT '',
    weight      REAL    NOT NULL DEFAULT 1.0,
    properties  TEXT    NOT NULL DEFAULT '{}',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_semantic_edges_key ON semantic_edges(agent_id, source_id, target_id, relation);
CREATE INDEX IF NOT EXISTS idx_semantic_edges_source ON semantic_edges(source_id);
CREATE INDEX IF NOT EXISTS idx_semantic_edges_target ON semantic_edges(target_id);

CREATE TABLE IF NOT EXISTS memory_stats (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id    TEXT    NOT NULL DEFAULT '',
    event_type  TEXT    NOT NULL DEFAULT '',
    node_id     INTEGER,
    details     TEXT    NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_memory_stats_agent ON memory_stats(agent_id, event_type);
`
	_, err := tx.Exec(ddl)
	return err
}

// ---------------------------------------------------------------------------
// Semantic node types
// ---------------------------------------------------------------------------

// SemanticNode represents an entity in the knowledge graph.
type SemanticNode struct {
	ID           int64
	AgentID      string
	Label        string
	Name         string
	Properties   string // JSON
	AccessCount  int
	LastAccessed *time.Time
	QualityScore float64 // 0.0-1.0, computed by memory quality scorer
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SemanticEdge represents a relationship between two nodes.
type SemanticEdge struct {
	ID         int64
	AgentID    string
	SourceID   int64
	TargetID   int64
	Relation   string
	Weight     float64
	Properties string // JSON
	CreatedAt  time.Time
	UpdatedAt  time.Time
	// Populated by join queries
	SourceName  string
	SourceLabel string
	TargetName  string
	TargetLabel string
}

// GraphResult is a combined node+edges result for graph queries.
type GraphResult struct {
	Node  SemanticNode
	Edges []SemanticEdge
}

// NodeStatSummary holds aggregated access stats for a node.
type NodeStatSummary struct {
	NodeID      int64
	Name        string
	Label       string
	AccessCount int
	QueryCount  int
	HitCount    int
}

// ---------------------------------------------------------------------------
// Semantic nodes CRUD
// ---------------------------------------------------------------------------

// UpsertNode inserts or updates a semantic node.
// Returns the node ID.
func (m *MemoryDB) UpsertNode(agentID, label, name, properties string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if properties == "" {
		properties = "{}"
	}
	now := time.Now().UTC()
	result, err := m.db.Exec(
		`INSERT INTO semantic_nodes (agent_id, label, name, properties, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(agent_id, label, name) DO UPDATE SET
		   properties = excluded.properties,
		   updated_at = excluded.updated_at`,
		agentID, label, name, properties, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("memory: upsert node: %w", err)
	}

	// If it was an update, get the existing ID
	id, insertErr := result.LastInsertId()
	if insertErr != nil || id == 0 {
		var nodeID int64
		err = m.db.QueryRow(
			`SELECT id FROM semantic_nodes WHERE agent_id = ? AND label = ? AND name = ?`,
			agentID, label, name,
		).Scan(&nodeID)
		if err != nil {
			return 0, fmt.Errorf("memory: get node id after upsert: %w", err)
		}
		return nodeID, nil
	}
	return id, nil
}

// GetNode returns a single semantic node by agent, label, and name.
// Returns nil if not found.
func (m *MemoryDB) GetNode(agentID, label, name string) (*SemanticNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	row := m.db.QueryRow(
		`SELECT id, agent_id, label, name, properties, access_count, last_accessed, quality_score, created_at, updated_at
		 FROM semantic_nodes WHERE agent_id = ? AND label = ? AND name = ?`,
		agentID,
		label,
		name,
	)
	return scanNode(row)
}

// GetNodeByID returns a node by its ID.
func (m *MemoryDB) GetNodeByID(nodeID int64) (*SemanticNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	row := m.db.QueryRow(
		`SELECT id, agent_id, label, name, properties, access_count, last_accessed, quality_score, created_at, updated_at
		 FROM semantic_nodes WHERE id = ?`,
		nodeID,
	)
	return scanNode(row)
}

func scanNode(row *sql.Row) (*SemanticNode, error) {
	var n SemanticNode
	var lastAccessed sql.NullString
	var created, updated string
	err := row.Scan(&n.ID, &n.AgentID, &n.Label, &n.Name, &n.Properties,
		&n.AccessCount, &lastAccessed, &n.QualityScore, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memory: scan node: %w", err)
	}
	n.CreatedAt, _ = time.Parse(time.RFC3339, created)
	n.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	if lastAccessed.Valid {
		t, _ := time.Parse(time.RFC3339, lastAccessed.String)
		n.LastAccessed = &t
	}
	return &n, nil
}

// UpdateNodeQuality updates the quality score for a node.
func (m *MemoryDB) UpdateNodeQuality(nodeID int64, score float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(`UPDATE semantic_nodes SET quality_score = ? WHERE id = ?`, score, nodeID)
	return err
}

// FindNodes searches nodes by agent and optional filters.
// namePattern uses SQL LIKE syntax (% for wildcard).
func (m *MemoryDB) FindNodes(agentID, label, namePattern string, limit int) ([]SemanticNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.findNodesLocked(agentID, label, namePattern, limit)
}

// findNodesLocked is the lock-free version of FindNodes; caller must hold mu.
func (m *MemoryDB) findNodesLocked(agentID, label, namePattern string, limit int) ([]SemanticNode, error) {
	var args []any
	query := `SELECT id, agent_id, label, name, properties, access_count, last_accessed, quality_score, created_at, updated_at
		 FROM semantic_nodes WHERE agent_id = ?`
	args = append(args, agentID)

	if label != "" {
		query += ` AND label = ?`
		args = append(args, label)
	}
	if namePattern != "" {
		query += ` AND name LIKE ?`
		args = append(args, namePattern)
	}
	query += ` ORDER BY access_count DESC, updated_at DESC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: find nodes: %w", err)
	}
	defer rows.Close()

	var nodes []SemanticNode
	for rows.Next() {
		var n SemanticNode
		var lastAccessed sql.NullString
		var created, updated string
		if err = rows.Scan(&n.ID, &n.AgentID, &n.Label, &n.Name, &n.Properties,
			&n.AccessCount, &lastAccessed, &n.QualityScore, &created, &updated); err != nil {
			return nil, fmt.Errorf("memory: scan node row: %w", err)
		}
		n.CreatedAt, _ = time.Parse(time.RFC3339, created)
		n.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		if lastAccessed.Valid {
			t, _ := time.Parse(time.RFC3339, lastAccessed.String)
			n.LastAccessed = &t
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// DeleteNode deletes a node and cascades to its edges.
func (m *MemoryDB) DeleteNode(nodeID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`DELETE FROM semantic_nodes WHERE id = ?`, nodeID)
	if err != nil {
		return fmt.Errorf("memory: delete node: %w", err)
	}
	return nil
}

// DeleteNodes batch-deletes nodes by IDs.
func (m *MemoryDB) DeleteNodes(nodeIDs []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(nodeIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(nodeIDs))
	args := make([]any, len(nodeIDs))
	for i, id := range nodeIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`DELETE FROM semantic_nodes WHERE id IN (%s)`, strings.Join(placeholders, ","))
	_, err := m.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("memory: delete nodes batch: %w", err)
	}
	return nil
}

// TouchNode increments access_count and updates last_accessed.
// Errors are intentionally ignored — this is a best-effort update.
func (m *MemoryDB) TouchNode(nodeID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.touchNodeLocked(nodeID)
}

// touchNodeLocked is the lock-free version of TouchNode; caller must hold mu.
func (m *MemoryDB) touchNodeLocked(nodeID int64) {
	_, _ = m.db.Exec( //nolint:errcheck // best-effort
		`UPDATE semantic_nodes SET access_count = access_count + 1, last_accessed = ? WHERE id = ?`,
		time.Now().UTC(), nodeID,
	)
}

// CountNodes returns the number of semantic nodes for an agent.
func (m *MemoryDB) CountNodes(agentID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int
	_ = m.db.QueryRow(`SELECT COUNT(*) FROM semantic_nodes WHERE agent_id = ?`, agentID).Scan(&count)
	return count
}

// ---------------------------------------------------------------------------
// Semantic edges CRUD
// ---------------------------------------------------------------------------

// UpsertEdge inserts or updates an edge between two nodes.
func (m *MemoryDB) UpsertEdge(
	agentID string,
	sourceID, targetID int64,
	relation string,
	weight float64,
	properties string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if properties == "" {
		properties = "{}"
	}
	if weight <= 0 {
		weight = 1.0
	}
	if weight > 1.0 {
		weight = 1.0
	}
	now := time.Now().UTC()
	_, err := m.db.Exec(
		`INSERT INTO semantic_edges (agent_id, source_id, target_id, relation, weight, properties, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(agent_id, source_id, target_id, relation) DO UPDATE SET
		   weight = excluded.weight,
		   properties = excluded.properties,
		   updated_at = excluded.updated_at`,
		agentID,
		sourceID,
		targetID,
		relation,
		weight,
		properties,
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("memory: upsert edge: %w", err)
	}
	return nil
}

// GetEdges returns all edges for a node (as source or target), with joined names.
func (m *MemoryDB) GetEdges(agentID string, nodeID int64) ([]SemanticEdge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.getEdgesLocked(agentID, nodeID)
}

// getEdgesLocked is the lock-free version of GetEdges; caller must hold mu.
func (m *MemoryDB) getEdgesLocked(agentID string, nodeID int64) ([]SemanticEdge, error) {
	rows, err := m.db.Query(
		`SELECT e.id, e.agent_id, e.source_id, e.target_id, e.relation, e.weight, e.properties,
		        e.created_at, e.updated_at,
		        s.name, s.label, t.name, t.label
		 FROM semantic_edges e
		 JOIN semantic_nodes s ON s.id = e.source_id
		 JOIN semantic_nodes t ON t.id = e.target_id
		 WHERE e.agent_id = ? AND (e.source_id = ? OR e.target_id = ?)
		 ORDER BY e.weight DESC`,
		agentID, nodeID, nodeID,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get edges: %w", err)
	}
	defer rows.Close()

	var edges []SemanticEdge
	for rows.Next() {
		var e SemanticEdge
		var created, updated string
		if err = rows.Scan(&e.ID, &e.AgentID, &e.SourceID, &e.TargetID, &e.Relation,
			&e.Weight, &e.Properties, &created, &updated,
			&e.SourceName, &e.SourceLabel, &e.TargetName, &e.TargetLabel); err != nil {
			return nil, fmt.Errorf("memory: scan edge: %w", err)
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, created)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// ListEdges returns all edges for an agent, with source/target names populated.
func (m *MemoryDB) ListEdges(agentID string, limit int) ([]SemanticEdge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 500
	}
	rows, err := m.db.Query(
		`SELECT e.id, e.agent_id, e.source_id, e.target_id, e.relation, e.weight, e.properties,
		        e.created_at, e.updated_at,
		        s.name, s.label, t.name, t.label
		 FROM semantic_edges e
		 JOIN semantic_nodes s ON s.id = e.source_id
		 JOIN semantic_nodes t ON t.id = e.target_id
		 WHERE e.agent_id = ?
		 ORDER BY e.weight DESC
		 LIMIT ?`,
		agentID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: list edges: %w", err)
	}
	defer rows.Close()

	var edges []SemanticEdge
	for rows.Next() {
		var e SemanticEdge
		var created, updated string
		if err = rows.Scan(&e.ID, &e.AgentID, &e.SourceID, &e.TargetID, &e.Relation,
			&e.Weight, &e.Properties, &created, &updated,
			&e.SourceName, &e.SourceLabel, &e.TargetName, &e.TargetLabel); err != nil {
			return nil, fmt.Errorf("memory: scan edge: %w", err)
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, created)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// ReinforceEdge increases an edge's weight by delta (capped at 1.0).
// Errors are intentionally ignored — this is a best-effort update.
func (m *MemoryDB) ReinforceEdge(edgeID int64, delta float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.reinforceEdgeLocked(edgeID, delta)
}

// reinforceEdgeLocked is the lock-free version of ReinforceEdge; caller must hold mu.
func (m *MemoryDB) reinforceEdgeLocked(edgeID int64, delta float64) {
	_, _ = m.db.Exec( //nolint:errcheck // best-effort
		`UPDATE semantic_edges SET weight = MIN(1.0, weight + ?), updated_at = ? WHERE id = ?`,
		delta, time.Now().UTC(), edgeID,
	)
}

// ---------------------------------------------------------------------------
// Graph queries
// ---------------------------------------------------------------------------

// QueryGraph searches across nodes and their edges using LIKE matching.
// Returns up to limit results with their connected edges.
func (m *MemoryDB) QueryGraph(agentID, query string, limit int) ([]GraphResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if limit <= 0 {
		limit = 10
	}
	pattern := "%" + query + "%"

	nodes, err := m.findNodesLocked(agentID, "", pattern, limit)
	if err != nil {
		return nil, err
	}

	// Also search by label
	labelNodes, err := m.db.Query(
		`SELECT id, agent_id, label, name, properties, access_count, last_accessed, quality_score, created_at, updated_at
		 FROM semantic_nodes WHERE agent_id = ? AND label LIKE ? ORDER BY access_count DESC LIMIT ?`,
		agentID,
		pattern,
		limit,
	)
	if err == nil {
		defer labelNodes.Close()
		seen := make(map[int64]bool)
		for _, n := range nodes {
			seen[n.ID] = true
		}
		for labelNodes.Next() {
			var n SemanticNode
			var lastAccessed sql.NullString
			var created, updated string
			if err = labelNodes.Scan(&n.ID, &n.AgentID, &n.Label, &n.Name, &n.Properties,
				&n.AccessCount, &lastAccessed, &n.QualityScore, &created, &updated); err == nil {
				n.CreatedAt, _ = time.Parse(time.RFC3339, created)
				n.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
				if lastAccessed.Valid {
					t, _ := time.Parse(time.RFC3339, lastAccessed.String)
					n.LastAccessed = &t
				}
				if !seen[n.ID] {
					nodes = append(nodes, n)
					seen[n.ID] = true
				}
			}
		}
	}

	// Cap results
	if len(nodes) > limit {
		nodes = nodes[:limit]
	}

	var results []GraphResult
	for _, n := range nodes {
		// Touch node for self-evolution tracking
		m.touchNodeLocked(n.ID)

		edges, edgeErr := m.getEdgesLocked(agentID, n.ID)
		if edgeErr != nil {
			edges = nil
		}

		// Reinforce traversed edges
		for _, e := range edges {
			m.reinforceEdgeLocked(e.ID, 0.01)
		}

		results = append(results, GraphResult{Node: n, Edges: edges})
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// Strategic forgetting: stale node detection
// ---------------------------------------------------------------------------

// GetStaleNodes returns nodes that haven't been accessed recently and have low usage.
func (m *MemoryDB) GetStaleNodes(agentID string, maxAge time.Duration, minAccessCount int) ([]SemanticNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cutoff := time.Now().UTC().Add(-maxAge)
	rows, err := m.db.Query(
		`SELECT id, agent_id, label, name, properties, access_count, last_accessed, quality_score, created_at, updated_at
		 FROM semantic_nodes
		 WHERE agent_id = ?
		   AND access_count < ?
		   AND (last_accessed IS NULL OR last_accessed < ?)
		 ORDER BY access_count ASC, last_accessed ASC`,
		agentID,
		minAccessCount,
		cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get stale nodes: %w", err)
	}
	defer rows.Close()

	var nodes []SemanticNode
	for rows.Next() {
		var n SemanticNode
		var lastAccessed sql.NullString
		var created, updated string
		if err = rows.Scan(&n.ID, &n.AgentID, &n.Label, &n.Name, &n.Properties,
			&n.AccessCount, &lastAccessed, &n.QualityScore, &created, &updated); err != nil {
			return nil, fmt.Errorf("memory: scan stale node: %w", err)
		}
		n.CreatedAt, _ = time.Parse(time.RFC3339, created)
		n.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		if lastAccessed.Valid {
			t, _ := time.Parse(time.RFC3339, lastAccessed.String)
			n.LastAccessed = &t
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// ---------------------------------------------------------------------------
// Memory stats (self-evolution tracking)
// ---------------------------------------------------------------------------

// RecordStat logs an event in memory_stats.
// Every 100th call, old stats (>30 days) are pruned automatically.
func (m *MemoryDB) RecordStat(agentID, eventType string, nodeID *int64, details string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`INSERT INTO memory_stats (agent_id, event_type, node_id, details, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		agentID, eventType, nodeID, details, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("memory: record stat: %w", err)
	}

	// Prune old stats every 100 calls.
	if m.statCount.Add(1)%100 == 0 {
		m.pruneStatsLocked()
	}

	return nil
}

// pruneStatsLocked deletes memory_stats entries older than 30 days.
// Caller must hold mu.
func (m *MemoryDB) pruneStatsLocked() {
	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	_, _ = m.db.Exec(`DELETE FROM memory_stats WHERE created_at < ?`, cutoff) //nolint:errcheck // best-effort
}

// GetNodeStats returns aggregated access statistics for an agent's nodes.
func (m *MemoryDB) GetNodeStats(agentID string) ([]NodeStatSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(
		`SELECT n.id, n.name, n.label, n.access_count,
		        COALESCE(SUM(CASE WHEN s.event_type = 'query' THEN 1 ELSE 0 END), 0) AS query_count,
		        COALESCE(SUM(CASE WHEN s.event_type = 'hit' THEN 1 ELSE 0 END), 0) AS hit_count
		 FROM semantic_nodes n
		 LEFT JOIN memory_stats s ON s.node_id = n.id AND s.agent_id = n.agent_id
		 WHERE n.agent_id = ?
		 GROUP BY n.id
		 ORDER BY n.access_count DESC`,
		agentID,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get node stats: %w", err)
	}
	defer rows.Close()

	var stats []NodeStatSummary
	for rows.Next() {
		var s NodeStatSummary
		if err = rows.Scan(&s.NodeID, &s.Name, &s.Label, &s.AccessCount, &s.QueryCount, &s.HitCount); err != nil {
			return nil, fmt.Errorf("memory: scan node stat: %w", err)
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// FindDuplicateNodes returns groups of nodes with the same label and similar names.
// Used by the consolidation process to merge duplicates.
func (m *MemoryDB) FindDuplicateNodes(agentID string) ([][]SemanticNode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Find exact label+name duplicates first (shouldn't exist due to unique index,
	// but handles edge cases), then group by label for fuzzy matching.
	nodes, err := m.findNodesLocked(agentID, "", "", 0)
	if err != nil {
		return nil, err
	}

	// Group by label
	byLabel := make(map[string][]SemanticNode)
	for _, n := range nodes {
		byLabel[n.Label] = append(byLabel[n.Label], n)
	}

	var duplicates [][]SemanticNode
	for _, group := range byLabel {
		if len(group) < 2 {
			continue
		}
		// Find similar names within the same label group
		used := make(map[int]bool)
		for i := 0; i < len(group); i++ {
			if used[i] {
				continue
			}
			cluster := []SemanticNode{group[i]}
			for j := i + 1; j < len(group); j++ {
				if used[j] {
					continue
				}
				if isSimilarName(group[i].Name, group[j].Name) {
					cluster = append(cluster, group[j])
					used[j] = true
				}
			}
			if len(cluster) > 1 {
				duplicates = append(duplicates, cluster)
			}
		}
	}
	return duplicates, nil
}

// isSimilarName checks if two names are similar enough to be considered duplicates.
// Uses a simple case-insensitive comparison plus prefix/substring matching.
func isSimilarName(a, b string) bool {
	la := strings.ToLower(strings.TrimSpace(a))
	lb := strings.ToLower(strings.TrimSpace(b))
	if la == lb {
		return true
	}
	// One is a prefix of the other (e.g., "Magnus" and "Magnus Grasberg")
	if strings.HasPrefix(la, lb) || strings.HasPrefix(lb, la) {
		return true
	}
	// Simple Levenshtein-ish: if names differ by at most 2 characters and are short
	if len(la) <= 20 && len(lb) <= 20 && levenshteinDistance(la, lb) <= 2 {
		return true
	}
	return false
}

// levenshteinDistance computes the edit distance between two strings.
func levenshteinDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, min(curr[j-1]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// GetConflictingEdges returns edges between the same source+target with different relations.
func (m *MemoryDB) GetConflictingEdges(agentID string) ([][]SemanticEdge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(
		`SELECT e.id, e.agent_id, e.source_id, e.target_id, e.relation, e.weight, e.properties,
		        e.created_at, e.updated_at,
		        s.name, s.label, t.name, t.label
		 FROM semantic_edges e
		 JOIN semantic_nodes s ON s.id = e.source_id
		 JOIN semantic_nodes t ON t.id = e.target_id
		 WHERE e.agent_id = ?
		 ORDER BY e.source_id, e.target_id, e.weight DESC`,
		agentID,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get conflicting edges: %w", err)
	}
	defer rows.Close()

	type edgeKey struct {
		source, target int64
	}
	grouped := make(map[edgeKey][]SemanticEdge)
	var order []edgeKey

	for rows.Next() {
		var e SemanticEdge
		var created, updated string
		if err = rows.Scan(&e.ID, &e.AgentID, &e.SourceID, &e.TargetID, &e.Relation,
			&e.Weight, &e.Properties, &created, &updated,
			&e.SourceName, &e.SourceLabel, &e.TargetName, &e.TargetLabel); err != nil {
			return nil, fmt.Errorf("memory: scan conflict edge: %w", err)
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, created)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		key := edgeKey{e.SourceID, e.TargetID}
		if _, exists := grouped[key]; !exists {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], e)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	var conflicts [][]SemanticEdge
	for _, key := range order {
		if edges := grouped[key]; len(edges) > 1 {
			conflicts = append(conflicts, edges)
		}
	}
	return conflicts, nil
}

// DeleteEdge deletes a single edge by ID.
func (m *MemoryDB) DeleteEdge(edgeID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`DELETE FROM semantic_edges WHERE id = ?`, edgeID)
	return err
}

// MergeNodes merges secondary nodes into the primary node.
// All edges pointing to/from secondary nodes are redirected to the primary.
// Secondary nodes are then deleted.
func (m *MemoryDB) MergeNodes(primaryID int64, secondaryIDs []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(secondaryIDs) == 0 {
		return nil
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("memory: begin merge tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, secID := range secondaryIDs {
		// Redirect source edges: secID -> X becomes primaryID -> X
		// Skip if it would create a duplicate (same agent, source, target, relation)
		_, _ = tx.Exec(
			`UPDATE OR IGNORE semantic_edges SET source_id = ?, updated_at = ? WHERE source_id = ?`,
			primaryID, time.Now().UTC(), secID,
		)
		// Redirect target edges: X -> secID becomes X -> primaryID
		_, _ = tx.Exec(
			`UPDATE OR IGNORE semantic_edges SET target_id = ?, updated_at = ? WHERE target_id = ?`,
			primaryID, time.Now().UTC(), secID,
		)
		// Delete remaining edges (duplicates that couldn't be redirected)
		_, _ = tx.Exec(`DELETE FROM semantic_edges WHERE source_id = ? OR target_id = ?`, secID, secID)
		// Delete the secondary node
		_, _ = tx.Exec(`DELETE FROM semantic_nodes WHERE id = ?`, secID)
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Schema migration v3: reflections table
// ---------------------------------------------------------------------------

func (m *MemoryDB) applyV3tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS reflections (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id     TEXT NOT NULL DEFAULT '',
    session_key  TEXT NOT NULL DEFAULT '',
    task_summary TEXT NOT NULL DEFAULT '',
    what_worked  TEXT NOT NULL DEFAULT '',
    what_failed  TEXT NOT NULL DEFAULT '',
    lessons      TEXT NOT NULL DEFAULT '',
    score        REAL NOT NULL DEFAULT 0.0,
    tool_count   INTEGER NOT NULL DEFAULT 0,
    error_count  INTEGER NOT NULL DEFAULT 0,
    duration_ms  INTEGER NOT NULL DEFAULT 0,
    created_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_reflections_agent ON reflections(agent_id, created_at);
`
	_, err := tx.Exec(ddl)
	return err
}

// ---------------------------------------------------------------------------
// Reflections CRUD
// ---------------------------------------------------------------------------

// ReflectionRecord holds a single post-task self-evaluation.
type ReflectionRecord struct {
	ID          int64
	AgentID     string
	SessionKey  string
	TaskSummary string
	WhatWorked  string
	WhatFailed  string
	Lessons     string
	Score       float64
	ToolCount   int
	ErrorCount  int
	DurationMs  int64
	CreatedAt   time.Time
}

// SaveReflection inserts a new reflection record.
func (m *MemoryDB) SaveReflection(r ReflectionRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`INSERT INTO reflections
		    (agent_id, session_key, task_summary, what_worked, what_failed, lessons,
		     score, tool_count, error_count, duration_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.AgentID, r.SessionKey, r.TaskSummary, r.WhatWorked, r.WhatFailed, r.Lessons,
		r.Score, r.ToolCount, r.ErrorCount, r.DurationMs, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("memory: save reflection: %w", err)
	}
	return nil
}

// GetRecentReflections returns the last N reflections for an agent.
func (m *MemoryDB) GetRecentReflections(agentID string, limit int) ([]ReflectionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}
	rows, err := m.db.Query(
		`SELECT id, agent_id, session_key, task_summary, what_worked, what_failed, lessons,
		        score, tool_count, error_count, duration_ms, created_at
		 FROM reflections WHERE agent_id = ?
		 ORDER BY created_at DESC LIMIT ?`,
		agentID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get recent reflections: %w", err)
	}
	defer rows.Close()

	var records []ReflectionRecord
	for rows.Next() {
		var r ReflectionRecord
		var created string
		if err = rows.Scan(&r.ID, &r.AgentID, &r.SessionKey, &r.TaskSummary,
			&r.WhatWorked, &r.WhatFailed, &r.Lessons,
			&r.Score, &r.ToolCount, &r.ErrorCount, &r.DurationMs, &created); err != nil {
			return nil, fmt.Errorf("memory: scan reflection: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		records = append(records, r)
	}
	return records, rows.Err()
}

// GetFailedReflections returns reflections with a score below 0.5.
func (m *MemoryDB) GetFailedReflections(agentID string, limit int) ([]ReflectionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}
	rows, err := m.db.Query(
		`SELECT id, agent_id, session_key, task_summary, what_worked, what_failed, lessons,
		        score, tool_count, error_count, duration_ms, created_at
		 FROM reflections WHERE agent_id = ? AND score < 0.5
		 ORDER BY created_at DESC LIMIT ?`,
		agentID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get failed reflections: %w", err)
	}
	defer rows.Close()

	var records []ReflectionRecord
	for rows.Next() {
		var r ReflectionRecord
		var created string
		if err = rows.Scan(&r.ID, &r.AgentID, &r.SessionKey, &r.TaskSummary,
			&r.WhatWorked, &r.WhatFailed, &r.Lessons,
			&r.Score, &r.ToolCount, &r.ErrorCount, &r.DurationMs, &created); err != nil {
			return nil, fmt.Errorf("memory: scan reflection: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		records = append(records, r)
	}
	return records, rows.Err()
}

// SearchReflections searches past reflections matching a text query against lessons and what_failed.
func (m *MemoryDB) SearchReflections(agentID, query string, limit int) ([]ReflectionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 5
	}
	pattern := "%" + query + "%"
	rows, err := m.db.Query(
		`SELECT id, agent_id, session_key, task_summary, what_worked, what_failed, lessons,
		        score, tool_count, error_count, duration_ms, created_at
		 FROM reflections
		 WHERE agent_id = ? AND (lessons LIKE ? OR what_failed LIKE ? OR task_summary LIKE ?)
		 ORDER BY created_at DESC LIMIT ?`,
		agentID, pattern, pattern, pattern, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: search reflections: %w", err)
	}
	defer rows.Close()

	var records []ReflectionRecord
	for rows.Next() {
		var r ReflectionRecord
		var created string
		if err = rows.Scan(&r.ID, &r.AgentID, &r.SessionKey, &r.TaskSummary,
			&r.WhatWorked, &r.WhatFailed, &r.Lessons,
			&r.Score, &r.ToolCount, &r.ErrorCount, &r.DurationMs, &created); err != nil {
			return nil, fmt.Errorf("memory: scan reflection: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		records = append(records, r)
	}
	return records, rows.Err()
}

// ReflectionStats holds aggregated performance metrics.
type ReflectionStats struct {
	TotalReflections int
	AvgScore         float64
	AvgToolCount     float64
	AvgErrorCount    float64
	AvgDurationMs    float64
}

// GetReflectionStats returns aggregate performance stats for an agent over the last N days.
func (m *MemoryDB) GetReflectionStats(agentID string, days int) (ReflectionStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if days <= 0 {
		days = 30
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	var s ReflectionStats
	err := m.db.QueryRow(
		`SELECT COUNT(*), COALESCE(AVG(score), 0), COALESCE(AVG(tool_count), 0),
		        COALESCE(AVG(error_count), 0), COALESCE(AVG(duration_ms), 0)
		 FROM reflections WHERE agent_id = ? AND created_at >= ?`,
		agentID, cutoff,
	).Scan(&s.TotalReflections, &s.AvgScore, &s.AvgToolCount, &s.AvgErrorCount, &s.AvgDurationMs)
	if err != nil {
		return s, fmt.Errorf("memory: get reflection stats: %w", err)
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// Schema migration v4: plan templates table
// ---------------------------------------------------------------------------

func (m *MemoryDB) applyV4tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS plan_templates (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT    NOT NULL UNIQUE,
    goal         TEXT    NOT NULL DEFAULT '',
    steps        TEXT    NOT NULL DEFAULT '[]',
    tags         TEXT    NOT NULL DEFAULT '',
    use_count    INTEGER NOT NULL DEFAULT 0,
    success_rate REAL    NOT NULL DEFAULT 0.0,
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_plan_templates_name ON plan_templates(name);
`
	_, err := tx.Exec(ddl)
	return err
}

// ---------------------------------------------------------------------------
// Plan templates CRUD
// ---------------------------------------------------------------------------

// PlanTemplate represents a reusable plan structure.
type PlanTemplate struct {
	ID          int64
	Name        string
	Goal        string
	Steps       []string
	Tags        string
	UseCount    int
	SuccessRate float64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SavePlanTemplate upserts a plan template.
func (m *MemoryDB) SavePlanTemplate(name, goal string, steps []string, tags string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	stepsJSON, err := json.Marshal(steps)
	if err != nil {
		return fmt.Errorf("memory: marshal template steps: %w", err)
	}
	now := time.Now().UTC()
	_, err = m.db.Exec(
		`INSERT INTO plan_templates (name, goal, steps, tags, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   goal = excluded.goal,
		   steps = excluded.steps,
		   tags = excluded.tags,
		   updated_at = excluded.updated_at`,
		name, goal, string(stepsJSON), tags, now, now,
	)
	if err != nil {
		return fmt.Errorf("memory: save plan template: %w", err)
	}
	return nil
}

// GetPlanTemplate returns a single template by name.
func (m *MemoryDB) GetPlanTemplate(name string) (*PlanTemplate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var t PlanTemplate
	var stepsJSON, created, updated string
	err := m.db.QueryRow(
		`SELECT id, name, goal, steps, tags, use_count, success_rate, created_at, updated_at
		 FROM plan_templates WHERE name = ?`, name,
	).Scan(&t.ID, &t.Name, &t.Goal, &stepsJSON, &t.Tags,
		&t.UseCount, &t.SuccessRate, &created, &updated)
	if err != nil {
		return nil, fmt.Errorf("memory: get plan template: %w", err)
	}
	if err = json.Unmarshal([]byte(stepsJSON), &t.Steps); err != nil {
		t.Steps = nil
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, created)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &t, nil
}

// FindPlanTemplates searches templates by name, goal, or tags using LIKE.
func (m *MemoryDB) FindPlanTemplates(query string, limit int) ([]PlanTemplate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}
	pattern := "%" + query + "%"
	rows, err := m.db.Query(
		`SELECT id, name, goal, steps, tags, use_count, success_rate, created_at, updated_at
		 FROM plan_templates
		 WHERE name LIKE ? OR goal LIKE ? OR tags LIKE ?
		 ORDER BY use_count DESC, updated_at DESC
		 LIMIT ?`,
		pattern, pattern, pattern, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: find plan templates: %w", err)
	}
	defer rows.Close()

	var templates []PlanTemplate
	for rows.Next() {
		var t PlanTemplate
		var stepsJSON, created, updated string
		if err = rows.Scan(&t.ID, &t.Name, &t.Goal, &stepsJSON, &t.Tags,
			&t.UseCount, &t.SuccessRate, &created, &updated); err != nil {
			return nil, fmt.Errorf("memory: scan plan template: %w", err)
		}
		if err = json.Unmarshal([]byte(stepsJSON), &t.Steps); err != nil {
			t.Steps = nil
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, created)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

// IncrementTemplateUseCount bumps the use_count for a template.
func (m *MemoryDB) IncrementTemplateUseCount(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`UPDATE plan_templates SET use_count = use_count + 1, updated_at = ? WHERE name = ?`,
		time.Now().UTC(), name,
	)
	return err
}

// ---------------------------------------------------------------------------
// Schema V5: Checkpoints
// ---------------------------------------------------------------------------

func (m *MemoryDB) applyV5tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS checkpoints (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_key TEXT    NOT NULL REFERENCES sessions(key) ON DELETE CASCADE,
    agent_id    TEXT    NOT NULL DEFAULT '',
    name        TEXT    NOT NULL DEFAULT '',
    iteration   INTEGER NOT NULL DEFAULT 0,
    msg_count   INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_checkpoints_session ON checkpoints(session_key, created_at);
`
	_, err := tx.Exec(ddl)
	return err
}

// ---------------------------------------------------------------------------
// Checkpoint CRUD
// ---------------------------------------------------------------------------

// CountMessages returns the number of messages in a session.
func (m *MemoryDB) CountMessages(sessionKey string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int
	err := m.db.QueryRow(
		`SELECT COUNT(*) FROM messages WHERE session_key = ?`, sessionKey,
	).Scan(&count)
	return count, err
}

// CreateCheckpoint inserts a new checkpoint row and returns its ID.
func (m *MemoryDB) CreateCheckpoint(sessionKey, agentID, name string, iteration, msgCount int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var summary string
	_ = m.db.QueryRow(
		`SELECT COALESCE(summary, '') FROM sessions WHERE key = ?`,
		sessionKey,
	).Scan(&summary)

	res, err := m.db.Exec(
		`INSERT INTO checkpoints (session_key, agent_id, name, iteration, msg_count, summary, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sessionKey, agentID, name, iteration, msgCount, summary, time.Now().UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("memory: create checkpoint: %w", err)
	}
	return res.LastInsertId()
}

// CheckpointRow is the data returned by checkpoint queries.
type CheckpointRow struct {
	ID         int64
	SessionKey string
	AgentID    string
	Name       string
	Iteration  int
	MsgCount   int
	Summary    string
	CreatedAt  time.Time
}

// GetCheckpoint retrieves a single checkpoint by ID.
func (m *MemoryDB) GetCheckpoint(id int64) (*CheckpointRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	row := m.db.QueryRow(
		`SELECT id, session_key, agent_id, name, iteration, msg_count, summary, created_at
		 FROM checkpoints WHERE id = ?`, id,
	)
	var cp CheckpointRow
	var created string
	err := row.Scan(
		&cp.ID,
		&cp.SessionKey,
		&cp.AgentID,
		&cp.Name,
		&cp.Iteration,
		&cp.MsgCount,
		&cp.Summary,
		&created,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get checkpoint: %w", err)
	}
	cp.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return &cp, nil
}

// ListCheckpoints returns all checkpoints for a session, newest first.
func (m *MemoryDB) ListCheckpoints(sessionKey string) ([]CheckpointRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(
		`SELECT id, session_key, agent_id, name, iteration, msg_count, summary, created_at
		 FROM checkpoints WHERE session_key = ? ORDER BY created_at DESC, id DESC`,
		sessionKey,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: list checkpoints: %w", err)
	}
	defer rows.Close()

	var result []CheckpointRow
	for rows.Next() {
		var cp CheckpointRow
		var created string
		if err := rows.Scan(
			&cp.ID,
			&cp.SessionKey,
			&cp.AgentID,
			&cp.Name,
			&cp.Iteration,
			&cp.MsgCount,
			&cp.Summary,
			&created,
		); err != nil {
			return nil, fmt.Errorf("memory: scan checkpoint: %w", err)
		}
		cp.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		result = append(result, cp)
	}
	return result, rows.Err()
}

// TruncateMessagesToCount keeps only the first `count` messages in a session
// (by position order), deleting the rest.
func (m *MemoryDB) TruncateMessagesToCount(sessionKey string, count int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`DELETE FROM messages
		 WHERE session_key = ?
		   AND position >= ?`,
		sessionKey, count,
	)
	return err
}

// DeleteCheckpointsAfter removes all checkpoints for a session with ID > the given ID.
func (m *MemoryDB) DeleteCheckpointsAfter(sessionKey string, afterID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`DELETE FROM checkpoints WHERE session_key = ? AND id > ?`,
		sessionKey, afterID,
	)
	return err
}

// DeleteAllCheckpoints removes all checkpoints for a session.
func (m *MemoryDB) DeleteAllCheckpoints(sessionKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`DELETE FROM checkpoints WHERE session_key = ?`, sessionKey)
	return err
}

// ---------------------------------------------------------------------------
// Schema V6: A/B Testing
// ---------------------------------------------------------------------------

func (m *MemoryDB) applyV6tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS ab_experiments (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL UNIQUE,
    description   TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'active',
    winner        TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    concluded_at  DATETIME
);

CREATE TABLE IF NOT EXISTS ab_variants (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    experiment_id  INTEGER NOT NULL
        REFERENCES ab_experiments(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    config         TEXT NOT NULL DEFAULT '{}',
    UNIQUE(experiment_id, name)
);

CREATE TABLE IF NOT EXISTS ab_trials (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    experiment_id  INTEGER NOT NULL
        REFERENCES ab_experiments(id) ON DELETE CASCADE,
    variant_id     INTEGER NOT NULL
        REFERENCES ab_variants(id) ON DELETE CASCADE,
    prompt         TEXT NOT NULL,
    response       TEXT NOT NULL DEFAULT '',
    score          REAL,
    latency_ms     INTEGER NOT NULL DEFAULT 0,
    tokens_in      INTEGER NOT NULL DEFAULT 0,
    tokens_out     INTEGER NOT NULL DEFAULT 0,
    error          TEXT NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_ab_trials_experiment
    ON ab_trials(experiment_id);
CREATE INDEX IF NOT EXISTS idx_ab_trials_variant
    ON ab_trials(variant_id);
`
	_, err := tx.Exec(ddl)
	return err
}

// ---------------------------------------------------------------------------
// Schema V7: Dynamic Tools
// ---------------------------------------------------------------------------

func (m *MemoryDB) applyV7tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS dynamic_tools (
    name        TEXT PRIMARY KEY,
    definition  TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
`
	_, err := tx.Exec(ddl)
	return err
}

func (m *MemoryDB) applyV8tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS agent_reputation (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id   TEXT    NOT NULL,
    category   TEXT    NOT NULL DEFAULT 'general',
    task       TEXT    NOT NULL,
    success    INTEGER NOT NULL DEFAULT 0,
    score      REAL,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    tokens_in  INTEGER NOT NULL DEFAULT 0,
    tokens_out INTEGER NOT NULL DEFAULT 0,
    error      TEXT    NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_reputation_agent
    ON agent_reputation(agent_id);
CREATE INDEX IF NOT EXISTS idx_reputation_agent_category
    ON agent_reputation(agent_id, category);
CREATE INDEX IF NOT EXISTS idx_reputation_created
    ON agent_reputation(created_at DESC);
`
	_, err := tx.Exec(ddl)
	return err
}

// Schema migration v9: add tool_name column to messages for Gemini compatibility.
// Gemini requires function_response.name to be non-empty.
// Runs outside a transaction (ALTER TABLE) with idempotency check.
func (m *MemoryDB) applyV9() error {
	if m.columnExists("messages", "tool_name") {
		return nil
	}
	_, err := m.db.Exec(`ALTER TABLE messages ADD COLUMN tool_name TEXT NOT NULL DEFAULT ''`)
	return err
}

func (m *MemoryDB) applyV10tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS evolution_agents (
    id          TEXT PRIMARY KEY,
    agent_id    TEXT NOT NULL,
    parent_id   TEXT NOT NULL DEFAULT '',
    reason      TEXT NOT NULL DEFAULT '',
    config_json TEXT NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    retired_at  DATETIME
);
CREATE INDEX IF NOT EXISTS idx_evolution_agents_status ON evolution_agents(status);

CREATE TABLE IF NOT EXISTS evolution_changelog (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id   TEXT NOT NULL DEFAULT '',
    action     TEXT NOT NULL,
    detail     TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_evolution_changelog_agent ON evolution_changelog(agent_id, created_at);

CREATE INDEX IF NOT EXISTS idx_reputation_agent_created ON agent_reputation(agent_id, created_at);
`
	_, err := tx.Exec(ddl)
	return err
}

func (m *MemoryDB) applyV11tx(tx *sql.Tx) error {
	const ddl = `ALTER TABLE semantic_nodes ADD COLUMN quality_score REAL NOT NULL DEFAULT 0.5`
	_, err := tx.Exec(ddl)
	return err
}

func (m *MemoryDB) applyV12() error {
	if m.columnExists("checkpoints", "summary") {
		return nil
	}
	_, err := m.db.Exec(`ALTER TABLE checkpoints ADD COLUMN summary TEXT NOT NULL DEFAULT ''`)
	return err
}

// Schema migration v13: execution traces table.
func (m *MemoryDB) applyV13tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS execution_traces (
    id          TEXT PRIMARY KEY,
    trace_id    TEXT NOT NULL,
    parent_id   TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    agent_id    TEXT NOT NULL DEFAULT '',
    session_key TEXT NOT NULL DEFAULT '',
    start_time  DATETIME NOT NULL,
    end_time    DATETIME,
    status      TEXT NOT NULL DEFAULT 'running',
    attributes  TEXT NOT NULL DEFAULT '{}',
    scores      TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_traces_trace_id ON execution_traces(trace_id);
CREATE INDEX IF NOT EXISTS idx_traces_agent ON execution_traces(agent_id, start_time);
CREATE INDEX IF NOT EXISTS idx_traces_kind ON execution_traces(kind, start_time);
`
	_, err := tx.Exec(ddl)
	return err
}

// ---------------------------------------------------------------------------
// Execution trace CRUD
// ---------------------------------------------------------------------------

// InsertTraceSpan persists a single trace span.
func (m *MemoryDB) InsertTraceSpan(
	id, traceID, parentID, kind, name, agentID, sessionKey string,
	startTime time.Time, endTime *time.Time, status string,
	attributes map[string]any, scores map[string]float64,
) error {
	attrsJSON, err := json.Marshal(attributes)
	if err != nil {
		attrsJSON = []byte("{}")
	}
	scoresJSON, err := json.Marshal(scores)
	if err != nil {
		scoresJSON = []byte("{}")
	}

	_, err = m.db.Exec(`
		INSERT OR REPLACE INTO execution_traces
			(id, trace_id, parent_id, kind, name, agent_id, session_key,
			 start_time, end_time, status, attributes, scores)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, traceID, parentID, kind, name, agentID, sessionKey,
		startTime, endTime, status, string(attrsJSON), string(scoresJSON),
	)
	return err
}

// UpdateTraceScores merges scores into the root span of a trace.
func (m *MemoryDB) UpdateTraceScores(traceID string, scores map[string]float64) error {
	// Read existing scores, merge, write back.
	var existing string
	err := m.db.QueryRow(
		`SELECT scores FROM execution_traces WHERE id = ?`, traceID,
	).Scan(&existing)
	if err != nil {
		return err
	}

	merged := make(map[string]float64)
	_ = json.Unmarshal([]byte(existing), &merged)
	for k, v := range scores {
		merged[k] = v
	}

	data, _ := json.Marshal(merged)
	_, err = m.db.Exec(
		`UPDATE execution_traces SET scores = ? WHERE id = ?`,
		string(data), traceID,
	)
	return err
}

// GetTraceSpans returns all spans for a trace, ordered by start_time.
func (m *MemoryDB) GetTraceSpans(traceID string) ([]trace.Span, error) {
	rows, err := m.db.Query(`
		SELECT id, trace_id, parent_id, kind, name, agent_id, session_key,
		       start_time, end_time, status, attributes, scores
		FROM execution_traces
		WHERE trace_id = ?
		ORDER BY start_time ASC`, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var spans []trace.Span
	for rows.Next() {
		var s trace.Span
		var endTime sql.NullTime
		var attrsStr, scoresStr string
		if err := rows.Scan(
			&s.ID, &s.TraceID, &s.ParentID, &s.Kind, &s.Name,
			&s.AgentID, &s.SessionKey, &s.StartTime, &endTime,
			&s.Status, &attrsStr, &scoresStr,
		); err != nil {
			return nil, err
		}
		if endTime.Valid {
			s.EndTime = &endTime.Time
		}
		s.Attributes = make(map[string]any)
		_ = json.Unmarshal([]byte(attrsStr), &s.Attributes)
		s.Scores = make(map[string]float64)
		_ = json.Unmarshal([]byte(scoresStr), &s.Scores)
		spans = append(spans, s)
	}
	return spans, rows.Err()
}

// QueryTraceSummaries returns lightweight summaries for root spans matching filters.
func (m *MemoryDB) QueryTraceSummaries(
	agentID string, since, until time.Time, limit int,
) ([]trace.TraceSummary, error) {
	query := `
		SELECT t.trace_id, t.agent_id, t.session_key, t.name,
		       t.start_time, t.end_time, t.status, t.scores,
		       (SELECT COUNT(*) FROM execution_traces c WHERE c.trace_id = t.trace_id) AS span_count
		FROM execution_traces t
		WHERE t.kind = 'request'`

	var args []any
	if agentID != "" {
		query += ` AND t.agent_id = ?`
		args = append(args, agentID)
	}
	if !since.IsZero() {
		query += ` AND t.start_time >= ?`
		args = append(args, since)
	}
	if !until.IsZero() {
		query += ` AND t.start_time <= ?`
		args = append(args, until)
	}
	query += ` ORDER BY t.start_time DESC`
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []trace.TraceSummary
	for rows.Next() {
		var ts trace.TraceSummary
		var endTime sql.NullTime
		var scoresStr string
		if err := rows.Scan(
			&ts.TraceID, &ts.AgentID, &ts.SessionKey, &ts.Name,
			&ts.StartTime, &endTime, &ts.Status, &scoresStr, &ts.SpanCount,
		); err != nil {
			return nil, err
		}
		if endTime.Valid {
			ts.DurationMs = endTime.Time.Sub(ts.StartTime).Milliseconds()
		}
		ts.Scores = make(map[string]float64)
		_ = json.Unmarshal([]byte(scoresStr), &ts.Scores)
		summaries = append(summaries, ts)
	}
	return summaries, rows.Err()
}

// PruneTraces deletes traces older than the given retention period.
func (m *MemoryDB) PruneTraces(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	_, err := m.db.Exec(
		`DELETE FROM execution_traces WHERE start_time < ?`, cutoff,
	)
	return err
}

// GetModelTraceScores returns average scores grouped by model for adaptive provider ranking.
func (m *MemoryDB) GetModelTraceScores(
	since time.Time,
	minTraces int,
) (map[string]map[string]float64, map[string]int, error) {
	rows, err := m.db.Query(`
		SELECT json_extract(attributes, '$.model') AS model, scores
		FROM execution_traces
		WHERE kind = 'request' AND start_time >= ? AND scores != '{}'`,
		since,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	// Accumulate scores per model.
	type accum struct {
		sums  map[string]float64
		count int
	}
	models := make(map[string]*accum)
	for rows.Next() {
		var model sql.NullString
		var scoresStr string
		if err := rows.Scan(&model, &scoresStr); err != nil {
			continue
		}
		if !model.Valid || model.String == "" {
			continue
		}
		scores := make(map[string]float64)
		_ = json.Unmarshal([]byte(scoresStr), &scores)
		if len(scores) == 0 {
			continue
		}
		a, ok := models[model.String]
		if !ok {
			a = &accum{sums: make(map[string]float64)}
			models[model.String] = a
		}
		a.count++
		for k, v := range scores {
			a.sums[k] += v
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// Average and filter by minimum trace count.
	avgScores := make(map[string]map[string]float64)
	counts := make(map[string]int)
	for model, a := range models {
		if a.count < minTraces {
			continue
		}
		avg := make(map[string]float64)
		for k, sum := range a.sums {
			avg[k] = sum / float64(a.count)
		}
		avgScores[model] = avg
		counts[model] = a.count
	}
	return avgScores, counts, nil
}

// ---------------------------------------------------------------------------
// Migration v14: goal_log table for persisting goal step results
// ---------------------------------------------------------------------------

func (m *MemoryDB) applyV14tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS goal_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    goal_id    INTEGER NOT NULL,
    agent_id   TEXT    NOT NULL DEFAULT '',
    step       TEXT    NOT NULL DEFAULT '',
    result     TEXT    NOT NULL DEFAULT '',
    success    INTEGER NOT NULL DEFAULT 1,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_goal_log_goal ON goal_log(goal_id, created_at);
`
	_, err := tx.Exec(ddl)
	return err
}

// GoalLogEntry represents a single step result in a goal's execution history.
type GoalLogEntry struct {
	ID         int64     `json:"id"`
	GoalID     int64     `json:"goal_id"`
	AgentID    string    `json:"agent_id"`
	Step       string    `json:"step"`
	Result     string    `json:"result"`
	Success    bool      `json:"success"`
	DurationMs int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// InsertGoalLog adds a step result to the goal log.
func (m *MemoryDB) InsertGoalLog(goalID int64, agentID, step, result string, success bool, durationMs int64) error {
	_, err := m.db.Exec(`
		INSERT INTO goal_log (goal_id, agent_id, step, result, success, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		goalID, agentID, step, result, boolToInt(success), durationMs, time.Now().UTC(),
	)
	return err
}

// GetGoalLog returns all log entries for a goal, ordered by creation time.
func (m *MemoryDB) GetGoalLog(goalID int64) ([]GoalLogEntry, error) {
	rows, err := m.db.Query(`
		SELECT id, goal_id, agent_id, step, result, success, duration_ms, created_at
		FROM goal_log
		WHERE goal_id = ?
		ORDER BY created_at ASC`, goalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []GoalLogEntry
	for rows.Next() {
		var e GoalLogEntry
		var successInt int
		if err := rows.Scan(&e.ID, &e.GoalID, &e.AgentID, &e.Step, &e.Result,
			&successInt, &e.DurationMs, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Success = successInt != 0
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
