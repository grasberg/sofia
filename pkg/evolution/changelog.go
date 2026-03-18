package evolution

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/grasberg/sofia/pkg/memory"
)

// ChangelogEntry represents a single evolution changelog record.
type ChangelogEntry struct {
	ID           string         `json:"id"`
	Timestamp    time.Time      `json:"timestamp"`
	Action       string         `json:"action"`
	Summary      string         `json:"summary"`
	Details      map[string]any `json:"details,omitempty"`
	Outcome      string         `json:"outcome,omitempty"`
	VerifiedAt   *time.Time     `json:"verified_at,omitempty"`
	MetricBefore float64        `json:"metric_before,omitempty"`
	MetricAfter  float64        `json:"metric_after,omitempty"`
}

// ActionOutcome captures the measured result of a changelog action.
type ActionOutcome struct {
	Result       string  `json:"result"` // improved, no_change, degraded, reverted
	MetricBefore float64 `json:"metric_before"`
	MetricAfter  float64 `json:"metric_after"`
}

// ChangelogWriter reads and writes evolution_changelog records in SQLite.
type ChangelogWriter struct {
	db *memory.MemoryDB
}

// NewChangelogWriter creates a new ChangelogWriter backed by the given MemoryDB.
func NewChangelogWriter(db *memory.MemoryDB) *ChangelogWriter {
	return &ChangelogWriter{db: db}
}

// Write inserts a new changelog entry. If entry.ID is empty, one is generated.
// The entry is taken by pointer so the caller can read back the generated ID.
func (cw *ChangelogWriter) Write(entry *ChangelogEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	detailJSON, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("evolution: marshal changelog entry: %w", err)
	}
	ts := entry.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	_, err = cw.db.Exec(
		`INSERT INTO evolution_changelog (agent_id, action, detail, created_at) VALUES (?, ?, ?, ?)`,
		entry.ID, entry.Action, string(detailJSON), ts.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return fmt.Errorf("evolution: write changelog: %w", err)
	}
	return nil
}

// Get retrieves a single changelog entry by its ID.
func (cw *ChangelogWriter) Get(id string) (*ChangelogEntry, error) {
	row := cw.db.QueryRow(
		`SELECT detail FROM evolution_changelog WHERE agent_id = ? LIMIT 1`,
		id,
	)
	var detailJSON string
	if err := row.Scan(&detailJSON); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("evolution: get changelog %s: %w", id, err)
	}
	var entry ChangelogEntry
	if err := json.Unmarshal([]byte(detailJSON), &entry); err != nil {
		return nil, fmt.Errorf("evolution: unmarshal changelog: %w", err)
	}
	return &entry, nil
}

// Query returns changelog entries since the given time, ordered newest first, up to limit.
func (cw *ChangelogWriter) Query(since time.Time, limit int) ([]ChangelogEntry, error) {
	rows, err := cw.db.Query(
		`SELECT detail FROM evolution_changelog WHERE created_at >= ? ORDER BY created_at DESC LIMIT ?`,
		since.Format("2006-01-02 15:04:05"), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("evolution: query changelog: %w", err)
	}
	defer rows.Close()

	var result []ChangelogEntry
	for rows.Next() {
		var detailJSON string
		if err := rows.Scan(&detailJSON); err != nil {
			return nil, fmt.Errorf("evolution: scan changelog: %w", err)
		}
		var entry ChangelogEntry
		if err := json.Unmarshal([]byte(detailJSON), &entry); err != nil {
			return nil, fmt.Errorf("evolution: unmarshal changelog row: %w", err)
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

// UpdateOutcome updates the outcome fields of an existing changelog entry.
func (cw *ChangelogWriter) UpdateOutcome(id string, outcome ActionOutcome) error {
	// Read existing entry.
	existing, err := cw.Get(id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("evolution: changelog entry %s not found", id)
	}

	// Patch outcome fields.
	now := time.Now().UTC()
	existing.Outcome = outcome.Result
	existing.VerifiedAt = &now
	existing.MetricBefore = outcome.MetricBefore
	existing.MetricAfter = outcome.MetricAfter

	detailJSON, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("evolution: marshal updated changelog: %w", err)
	}

	_, err = cw.db.Exec(
		`UPDATE evolution_changelog SET detail = ? WHERE agent_id = ?`,
		string(detailJSON), id,
	)
	if err != nil {
		return fmt.Errorf("evolution: update changelog outcome %s: %w", id, err)
	}
	return nil
}

// QueryUnverified returns entries that have no outcome set, ordered newest first, up to limit.
func (cw *ChangelogWriter) QueryUnverified(limit int) ([]ChangelogEntry, error) {
	// We store outcome in the detail JSON blob, so we filter with JSON extraction.
	// Entries without an outcome have "outcome":"" or no outcome key in the JSON.
	rows, err := cw.db.Query(
		`SELECT detail FROM evolution_changelog ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("evolution: query unverified: %w", err)
	}
	defer rows.Close()

	var result []ChangelogEntry
	for rows.Next() {
		var detailJSON string
		if err := rows.Scan(&detailJSON); err != nil {
			return nil, fmt.Errorf("evolution: scan unverified: %w", err)
		}
		var entry ChangelogEntry
		if err := json.Unmarshal([]byte(detailJSON), &entry); err != nil {
			return nil, fmt.Errorf("evolution: unmarshal unverified: %w", err)
		}
		if entry.Outcome == "" {
			result = append(result, entry)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, rows.Err()
}
