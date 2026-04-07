package memory

import (
	"encoding/json"
	"fmt"
	"time"
)

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
