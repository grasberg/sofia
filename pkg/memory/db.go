package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (CGO_ENABLED=0 compatible)

	"github.com/grasberg/sofia/pkg/logger"
)

const schemaVersion = 16

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

	// Allow multiple concurrent readers while WAL mode serialises writes.
	db.SetMaxOpenConns(4)

	if _, err = db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("memory: set WAL mode: %w", err)
	}
	if _, err = db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("memory: enable foreign keys: %w", err)
	}
	// Reduce fsync frequency for better write performance (safe with WAL mode).
	if _, err = db.Exec(`PRAGMA synchronous = NORMAL`); err != nil {
		logger.WarnCF("memory", "Failed to set synchronous pragma", map[string]any{"error": err.Error()})
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
		{15, m.applyV15tx, nil},
		{16, m.applyV16tx, nil},
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
CREATE INDEX IF NOT EXISTS idx_messages_role ON messages(session_key, role);

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
// Session CRUD — see db_sessions.go
// Notes CRUD — see db_notes.go
// Semantic memory — see db_semantic.go
// Reflections & Templates — see db_reflections.go
// Checkpoints — see db_checkpoints.go
// Execution traces — see db_traces.go
// Goal log — see db_goals.go
// ---------------------------------------------------------------------------
