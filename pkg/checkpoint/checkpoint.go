package checkpoint

import (
	"fmt"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
)

// Checkpoint represents a saved state snapshot during task execution.
type Checkpoint struct {
	ID         int64     `json:"id"`
	SessionKey string    `json:"session_key"`
	AgentID    string    `json:"agent_id"`
	Name       string    `json:"name"`
	Iteration  int       `json:"iteration"`
	MsgCount   int       `json:"msg_count"`
	CreatedAt  time.Time `json:"created_at"`
}

func fromRow(row memory.CheckpointRow) Checkpoint {
	return Checkpoint{
		ID:         row.ID,
		SessionKey: row.SessionKey,
		AgentID:    row.AgentID,
		Name:       row.Name,
		Iteration:  row.Iteration,
		MsgCount:   row.MsgCount,
		CreatedAt:  row.CreatedAt,
	}
}

// Manager handles creating, listing, and rolling back checkpoints.
type Manager struct {
	db *memory.MemoryDB
}

// NewManager creates a new checkpoint Manager.
func NewManager(db *memory.MemoryDB) *Manager {
	return &Manager{db: db}
}

// Create saves a checkpoint capturing the current message count for the session.
func (m *Manager) Create(sessionKey, agentID, name string, iteration int) (*Checkpoint, error) {
	msgCount, err := m.db.CountMessages(sessionKey)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: count messages: %w", err)
	}

	id, err := m.db.CreateCheckpoint(sessionKey, agentID, name, iteration, msgCount)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: create: %w", err)
	}

	cp := Checkpoint{
		ID:         id,
		SessionKey: sessionKey,
		AgentID:    agentID,
		Name:       name,
		Iteration:  iteration,
		MsgCount:   msgCount,
		CreatedAt:  time.Now(),
	}
	return &cp, nil
}

// List returns all checkpoints for a session, ordered newest first.
func (m *Manager) List(sessionKey string) ([]Checkpoint, error) {
	rows, err := m.db.ListCheckpoints(sessionKey)
	if err != nil {
		return nil, err
	}
	result := make([]Checkpoint, len(rows))
	for i, r := range rows {
		result[i] = fromRow(r)
	}
	return result, nil
}

// Rollback restores the session to the state captured by the given checkpoint.
// It truncates messages back to the checkpoint's message count and removes
// all checkpoints created after the target one.
func (m *Manager) Rollback(sessionKey string, checkpointID int64) (*Checkpoint, error) {
	row, err := m.db.GetCheckpoint(checkpointID)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: get: %w", err)
	}
	if row.SessionKey != sessionKey {
		return nil, fmt.Errorf("checkpoint: session mismatch")
	}

	if err := m.db.TruncateMessagesToCount(sessionKey, row.MsgCount); err != nil {
		return nil, fmt.Errorf("checkpoint: truncate messages: %w", err)
	}

	if err := m.db.DeleteCheckpointsAfter(sessionKey, checkpointID); err != nil {
		return nil, fmt.Errorf("checkpoint: delete later checkpoints: %w", err)
	}

	cp := fromRow(*row)
	return &cp, nil
}

// RollbackToLatest rolls back to the most recent checkpoint for the session.
// Returns the checkpoint rolled back to and the restored messages, or nil if none exist.
func (m *Manager) RollbackToLatest(sessionKey string) (*Checkpoint, []providers.Message, error) {
	rows, err := m.db.ListCheckpoints(sessionKey)
	if err != nil {
		return nil, nil, fmt.Errorf("checkpoint: list: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}

	latest := rows[0] // sorted newest first
	cp, err := m.Rollback(sessionKey, latest.ID)
	if err != nil {
		return nil, nil, err
	}

	msgs, err := m.db.GetMessages(sessionKey)
	if err != nil {
		return cp, nil, fmt.Errorf("checkpoint: get restored messages: %w", err)
	}

	return cp, msgs, nil
}

// Cleanup removes all checkpoints for a session.
func (m *Manager) Cleanup(sessionKey string) error {
	return m.db.DeleteAllCheckpoints(sessionKey)
}
