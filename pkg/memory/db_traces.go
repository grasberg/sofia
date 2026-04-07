package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/grasberg/sofia/pkg/trace"
)

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
		query += fmt.Sprintf(` LIMIT %d`, limit) //nolint:gosec // limit is an int, not user input
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
