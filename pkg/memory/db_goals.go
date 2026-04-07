package memory

import (
	"time"
)

// ---------------------------------------------------------------------------
// Goal log CRUD
// ---------------------------------------------------------------------------

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

// DeleteGoalLog removes all log entries for a goal.
func (m *MemoryDB) DeleteGoalLog(goalID int64) error {
	_, err := m.db.Exec(`DELETE FROM goal_log WHERE goal_id = ?`, goalID)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
