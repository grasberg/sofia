package memory

import (
	"fmt"
	"time"
)

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
