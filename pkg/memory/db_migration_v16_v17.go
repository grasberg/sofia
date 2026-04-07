package memory

import "fmt"

// migrationV16 adds quality_score and archived columns to semantic_nodes.
// This enables graduated memory forgetting (soft vs. hard delete).
const migrationV16 = `
-- Add quality_score column for memory quality tracking
ALTER TABLE semantic_nodes ADD COLUMN quality_score REAL NOT NULL DEFAULT 0.5;

-- Add archived column for soft-delete (graduated forgetting)
-- Archived nodes are excluded from default queries but remain in database
ALTER TABLE semantic_nodes ADD COLUMN archived BOOLEAN NOT NULL DEFAULT 0;

-- Create index on archived for efficient filtering
CREATE INDEX IF NOT EXISTS idx_semantic_nodes_archived ON semantic_nodes(archived);

-- Create index on quality_score for quality-based pruning
CREATE INDEX IF NOT EXISTS idx_semantic_nodes_quality ON semantic_nodes(agent_id, quality_score, access_count, last_accessed);
`

// migrationV17 adds progress_history table for tool execution progress tracking.
const migrationV17 = `
CREATE TABLE IF NOT EXISTS progress_history (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id        TEXT    NOT NULL DEFAULT '',
    session_key     TEXT    NOT NULL DEFAULT '',
    tool_name       TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL DEFAULT '', -- started, completed, failed
    message         TEXT    NOT NULL DEFAULT '',
    progress        REAL    NOT NULL DEFAULT 0.0, -- 0.0 to 1.0
    elapsed_ms      INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_progress_session ON progress_history(session_key);
CREATE INDEX IF NOT EXISTS idx_progress_agent ON progress_history(agent_id);
CREATE INDEX IF NOT EXISTS idx_progress_tool ON progress_history(agent_id, tool_name);
`

// ApplyMigrationV16 applies the graduated memory forgetting migration.
func (m *MemoryDB) ApplyMigrationV16() error {
	_, err := m.db.Exec(migrationV16)
	if err != nil {
		return fmt.Errorf("memory: migration v16 failed: %w", err)
	}
	return nil
}

// ApplyMigrationV17 applies the progress history migration.
func (m *MemoryDB) ApplyMigrationV17() error {
	_, err := m.db.Exec(migrationV17)
	if err != nil {
		return fmt.Errorf("memory: migration v17 failed: %w", err)
	}
	return nil
}

// ArchiveNode marks a node as archived (soft delete).
// Archived nodes are excluded from GetContext and default queries.
func (m *MemoryDB) ArchiveNode(nodeID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`UPDATE semantic_nodes SET archived = 1, updated_at = datetime('now') WHERE id = ?`, nodeID)
	if err != nil {
		return fmt.Errorf("memory: archive node: %w", err)
	}
	return nil
}

// RestoreNode unarchives a node (reverses soft delete).
func (m *MemoryDB) RestoreNode(nodeID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`UPDATE semantic_nodes SET archived = 0, updated_at = datetime('now') WHERE id = ?`, nodeID)
	if err != nil {
		return fmt.Errorf("memory: restore node: %w", err)
	}
	return nil
}

// RecordProgress stores a tool execution progress entry for post-mortem analysis.
func (m *MemoryDB) RecordProgress(agentID, sessionKey, toolName, status, message string, progress float64, elapsedMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`INSERT INTO progress_history (agent_id, session_key, tool_name, status, message, progress, elapsed_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		agentID, sessionKey, toolName, status, message, progress, elapsedMs,
	)
	if err != nil {
		return fmt.Errorf("memory: record progress: %w", err)
	}
	return nil
}

// GetProgressHistory retrieves progress entries for a session.
func (m *MemoryDB) GetProgressHistory(sessionKey string, limit int) ([]map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	rows, err := m.db.Query(
		`SELECT tool_name, status, message, progress, elapsed_ms, created_at
		 FROM progress_history
		 WHERE session_key = ?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		sessionKey, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: query progress: %w", err)
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var toolName, status, message, createdAt string
		var progress float64
		var elapsedMs int

		if err := rows.Scan(&toolName, &status, &message, &progress, &elapsedMs, &createdAt); err != nil {
			return nil, fmt.Errorf("memory: scan progress: %w", err)
		}

		results = append(results, map[string]any{
			"tool_name":  toolName,
			"status":     status,
			"message":    message,
			"progress":   progress,
			"elapsed_ms": elapsedMs,
			"created_at": createdAt,
		})
	}

	return results, rows.Err()
}
