package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (CGO_ENABLED=0 compatible)

	"github.com/grasberg/sofia/pkg/providers"
)

const schemaVersion = 2

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

	if current < 2 {
		if err = m.applyV2(); err != nil {
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

// ---------------------------------------------------------------------------
// Schema migration v2: semantic memory tables
// ---------------------------------------------------------------------------

func (m *MemoryDB) applyV2() error {
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
	_, err := m.db.Exec(ddl)
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
	row := m.db.QueryRow(
		`SELECT id, agent_id, label, name, properties, access_count, last_accessed, created_at, updated_at
		 FROM semantic_nodes WHERE agent_id = ? AND label = ? AND name = ?`,
		agentID, label, name,
	)
	return scanNode(row)
}

// GetNodeByID returns a node by its ID.
func (m *MemoryDB) GetNodeByID(nodeID int64) (*SemanticNode, error) {
	row := m.db.QueryRow(
		`SELECT id, agent_id, label, name, properties, access_count, last_accessed, created_at, updated_at
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
		&n.AccessCount, &lastAccessed, &created, &updated)
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

// FindNodes searches nodes by agent and optional filters.
// namePattern uses SQL LIKE syntax (% for wildcard).
func (m *MemoryDB) FindNodes(agentID, label, namePattern string, limit int) ([]SemanticNode, error) {
	var args []any
	query := `SELECT id, agent_id, label, name, properties, access_count, last_accessed, created_at, updated_at
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
			&n.AccessCount, &lastAccessed, &created, &updated); err != nil {
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
	_, err := m.db.Exec(`DELETE FROM semantic_nodes WHERE id = ?`, nodeID)
	if err != nil {
		return fmt.Errorf("memory: delete node: %w", err)
	}
	return nil
}

// DeleteNodes batch-deletes nodes by IDs.
func (m *MemoryDB) DeleteNodes(nodeIDs []int64) error {
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
func (m *MemoryDB) TouchNode(nodeID int64) {
	_, _ = m.db.Exec(
		`UPDATE semantic_nodes SET access_count = access_count + 1, last_accessed = ? WHERE id = ?`,
		time.Now().UTC(), nodeID,
	)
}

// CountNodes returns the number of semantic nodes for an agent.
func (m *MemoryDB) CountNodes(agentID string) int {
	var count int
	_ = m.db.QueryRow(`SELECT COUNT(*) FROM semantic_nodes WHERE agent_id = ?`, agentID).Scan(&count)
	return count
}

// ---------------------------------------------------------------------------
// Semantic edges CRUD
// ---------------------------------------------------------------------------

// UpsertEdge inserts or updates an edge between two nodes.
func (m *MemoryDB) UpsertEdge(agentID string, sourceID, targetID int64, relation string, weight float64, properties string) error {
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
		agentID, sourceID, targetID, relation, weight, properties, now, now,
	)
	if err != nil {
		return fmt.Errorf("memory: upsert edge: %w", err)
	}
	return nil
}

// GetEdges returns all edges for a node (as source or target), with joined names.
func (m *MemoryDB) GetEdges(agentID string, nodeID int64) ([]SemanticEdge, error) {
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

// ReinforceEdge increases an edge's weight by delta (capped at 1.0).
func (m *MemoryDB) ReinforceEdge(edgeID int64, delta float64) {
	_, _ = m.db.Exec(
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
	if limit <= 0 {
		limit = 10
	}
	pattern := "%" + query + "%"

	nodes, err := m.FindNodes(agentID, "", pattern, limit)
	if err != nil {
		return nil, err
	}

	// Also search by label
	labelNodes, err := m.db.Query(
		`SELECT id, agent_id, label, name, properties, access_count, last_accessed, created_at, updated_at
		 FROM semantic_nodes WHERE agent_id = ? AND label LIKE ? ORDER BY access_count DESC LIMIT ?`,
		agentID, pattern, limit,
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
				&n.AccessCount, &lastAccessed, &created, &updated); err == nil {
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
		m.TouchNode(n.ID)

		edges, edgeErr := m.GetEdges(agentID, n.ID)
		if edgeErr != nil {
			edges = nil
		}

		// Reinforce traversed edges
		for _, e := range edges {
			m.ReinforceEdge(e.ID, 0.01)
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
	cutoff := time.Now().UTC().Add(-maxAge)
	rows, err := m.db.Query(
		`SELECT id, agent_id, label, name, properties, access_count, last_accessed, created_at, updated_at
		 FROM semantic_nodes
		 WHERE agent_id = ?
		   AND access_count < ?
		   AND (last_accessed IS NULL OR last_accessed < ?)
		 ORDER BY access_count ASC, last_accessed ASC`,
		agentID, minAccessCount, cutoff,
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
			&n.AccessCount, &lastAccessed, &created, &updated); err != nil {
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
func (m *MemoryDB) RecordStat(agentID, eventType string, nodeID *int64, details string) error {
	_, err := m.db.Exec(
		`INSERT INTO memory_stats (agent_id, event_type, node_id, details, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		agentID, eventType, nodeID, details, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("memory: record stat: %w", err)
	}
	return nil
}

// GetNodeStats returns aggregated access statistics for an agent's nodes.
func (m *MemoryDB) GetNodeStats(agentID string) ([]NodeStatSummary, error) {
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
	// Find exact label+name duplicates first (shouldn't exist due to unique index,
	// but handles edge cases), then group by label for fuzzy matching.
	nodes, err := m.FindNodes(agentID, "", "", 0)
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
	_, err := m.db.Exec(`DELETE FROM semantic_edges WHERE id = ?`, edgeID)
	return err
}

// MergeNodes merges secondary nodes into the primary node.
// All edges pointing to/from secondary nodes are redirected to the primary.
// Secondary nodes are then deleted.
func (m *MemoryDB) MergeNodes(primaryID int64, secondaryIDs []int64) error {
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
