// Sofia - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Sofia contributors

package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
)

// MemoryStore manages persistent memory for the agent via the shared MemoryDB.
// - Long-term memory: stored as kind="longterm", date_key=""
// - Daily notes:      stored as kind="daily",    date_key="YYYYMMDD"
type MemoryStore struct {
	db       *memory.MemoryDB
	agentID  string
	semantic *SemanticMemory
}

// NewMemoryStore creates a new MemoryStore backed by the given MemoryDB.
func NewMemoryStore(db *memory.MemoryDB, agentID string) *MemoryStore {
	return &MemoryStore{
		db:       db,
		agentID:  agentID,
		semantic: NewSemanticMemory(db, agentID),
	}
}

// todayKey returns today's date formatted as "YYYYMMDD".
func todayKey() string {
	return time.Now().Format("20060102")
}

// ReadLongTerm reads the long-term memory.
// Returns empty string if no note has been stored yet.
func (ms *MemoryStore) ReadLongTerm() string {
	if ms.db == nil {
		return ""
	}
	return ms.db.GetNote(ms.agentID, "longterm", "")
}

// WriteLongTerm writes content to the long-term memory.
func (ms *MemoryStore) WriteLongTerm(content string) error {
	if ms.db == nil {
		return nil
	}
	return ms.db.SetNote(ms.agentID, "longterm", "", content)
}

// ReadToday reads today's daily note.
// Returns empty string if no note exists for today.
func (ms *MemoryStore) ReadToday() string {
	if ms.db == nil {
		return ""
	}
	return ms.db.GetNote(ms.agentID, "daily", todayKey())
}

// AppendToday appends content to today's daily note.
// If no note exists for today, a date header is prepended.
func (ms *MemoryStore) AppendToday(content string) error {
	if ms.db == nil {
		return nil
	}
	key := todayKey()
	existing := ms.db.GetNote(ms.agentID, "daily", key)

	var newContent string
	if existing == "" {
		header := fmt.Sprintf("# %s\n\n", time.Now().Format("2006-01-02"))
		newContent = header + content
	} else {
		newContent = existing + "\n" + content
	}

	return ms.db.SetNote(ms.agentID, "daily", key, newContent)
}

// GetRecentDailyNotes returns daily notes from the last N days.
// Contents are joined with "---" separator.
func (ms *MemoryStore) GetRecentDailyNotes(days int) string {
	if ms.db == nil {
		return ""
	}
	var sb strings.Builder
	first := true

	for i := 0; i < days; i++ {
		date := time.Now().AddDate(0, 0, -i)
		key := date.Format("20060102")
		content := ms.db.GetNote(ms.agentID, "daily", key)
		if content != "" {
			if !first {
				sb.WriteString("\n\n---\n\n")
			}
			sb.WriteString(content)
			first = false
		}
	}

	return sb.String()
}

// GetMemoryContext returns formatted memory context for the agent prompt.
// Includes long-term memory, recent daily notes, knowledge graph, and reflection lessons.
func (ms *MemoryStore) GetMemoryContext() string {
	longTerm := ms.ReadLongTerm()
	recentNotes := ms.GetRecentDailyNotes(3)
	graphContext := ""
	if ms.semantic != nil {
		graphContext = ms.semantic.GetContext(10)
	}

	// Reflection lessons from past self-evaluations
	reflectionContext := ""
	if ms.db != nil {
		engine := NewReflectionEngine(ms.db, ms.agentID)
		reflectionContext = engine.FormatLessonsContext(5)
	}

	if longTerm == "" && recentNotes == "" && graphContext == "" && reflectionContext == "" {
		return ""
	}

	var sb strings.Builder

	if longTerm != "" {
		sb.WriteString("## Long-term Memory\n\n")
		sb.WriteString(longTerm)
	}

	if recentNotes != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n---\n\n")
		}
		sb.WriteString("## Recent Daily Notes\n\n")
		sb.WriteString(recentNotes)
	}

	if graphContext != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n---\n\n")
		}
		sb.WriteString(graphContext)
	}

	if reflectionContext != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n---\n\n")
		}
		sb.WriteString(reflectionContext)
	}

	return sb.String()
}

// GetRelevantLessonsFormatted returns formatted lessons relevant to the given query.
// This complements GetMemoryContext (which returns the N most recent lessons) by
// returning lessons that semantically match the current user message.
func (ms *MemoryStore) GetRelevantLessonsFormatted(query string, limit int) string {
	if ms.db == nil || query == "" {
		return ""
	}
	if limit <= 0 {
		limit = 3
	}
	engine := NewReflectionEngine(ms.db, ms.agentID)
	records, err := engine.GetRelevantLessons(query, limit)
	if err != nil || len(records) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Relevant Past Lessons\n\n")
	for _, r := range records {
		if r.Lessons == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("- (score=%.1f) %s\n", r.Score, r.Lessons))
	}
	result := sb.String()
	if result == "## Relevant Past Lessons\n\n" {
		return ""
	}
	return result
}
