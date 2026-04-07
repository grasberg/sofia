package memory

import (
	"fmt"
	"time"
)

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
