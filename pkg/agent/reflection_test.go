package agent

import (
	"testing"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openReflectionTestDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestFormatLessonsContext_Empty(t *testing.T) {
	db := openReflectionTestDB(t)
	engine := NewReflectionEngine(db, "a1")

	ctx := engine.FormatLessonsContext(5)
	assert.Equal(t, "", ctx, "should return empty when no reflections exist")
}

func TestFormatLessonsContext_WithReflections(t *testing.T) {
	db := openReflectionTestDB(t)
	engine := NewReflectionEngine(db, "a1")

	// Save some reflections
	require.NoError(t, db.SaveReflection(memory.ReflectionRecord{
		AgentID:     "a1",
		SessionKey:  "s1",
		TaskSummary: "Helped with code review",
		WhatWorked:  "Used structured approach",
		WhatFailed:  "Missed one edge case",
		Lessons:     "Always check error paths in code review",
		Score:       0.8,
		ToolCount:   5,
		ErrorCount:  1,
	}))
	require.NoError(t, db.SaveReflection(memory.ReflectionRecord{
		AgentID:     "a1",
		SessionKey:  "s2",
		TaskSummary: "Set up a cron job",
		WhatWorked:  "Quick execution",
		WhatFailed:  "",
		Lessons:     "Verify cron expressions before scheduling",
		Score:       0.9,
		ToolCount:   3,
		ErrorCount:  0,
	}))

	ctx := engine.FormatLessonsContext(5)
	assert.Contains(t, ctx, "Past Lessons")
	assert.Contains(t, ctx, "Always check error paths")
	assert.Contains(t, ctx, "Verify cron expressions")
}

func TestFormatLessonsContext_NilDB(t *testing.T) {
	engine := NewReflectionEngine(nil, "a1")
	ctx := engine.FormatLessonsContext(5)
	assert.Equal(t, "", ctx)
}

func TestGetRelevantLessons(t *testing.T) {
	db := openReflectionTestDB(t)
	engine := NewReflectionEngine(db, "a1")

	require.NoError(t, db.SaveReflection(memory.ReflectionRecord{
		AgentID:     "a1",
		SessionKey:  "s1",
		TaskSummary: "Code refactoring task",
		Lessons:     "Break large refactors into smaller steps",
		Score:       0.7,
	}))
	require.NoError(t, db.SaveReflection(memory.ReflectionRecord{
		AgentID:     "a1",
		SessionKey:  "s2",
		TaskSummary: "Writing unit tests",
		Lessons:     "Test edge cases first",
		Score:       0.9,
	}))

	// Search for refactoring
	results, err := engine.GetRelevantLessons("refactor", 5)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0].Lessons, "smaller steps")

	// Search for tests
	results, err = engine.GetRelevantLessons("test", 5)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0].Lessons, "edge cases")
}

func TestGetRelevantLessons_NilDB(t *testing.T) {
	engine := NewReflectionEngine(nil, "a1")
	results, err := engine.GetRelevantLessons("anything", 5)
	assert.NoError(t, err)
	assert.Nil(t, results)
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain json",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "markdown wrapped",
			input:    "```json\n{\"key\": \"value\"}\n```",
			expected: `{"key": "value"}`,
		},
		{
			name:     "text with json",
			input:    "Here is the result:\n{\"score\": 0.8}\nDone.",
			expected: `{"score": 0.8}`,
		},
		{
			name:     "no json",
			input:    "no json here",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractJSON(tt.input))
		})
	}
}

func TestSaveFallbackReflection(t *testing.T) {
	db := openReflectionTestDB(t)
	engine := NewReflectionEngine(db, "a1")

	err := engine.saveFallbackReflection("session1", 10, 2, 5000)
	require.NoError(t, err)

	reflections, err := db.GetRecentReflections("a1", 1)
	require.NoError(t, err)
	require.Len(t, reflections, 1)

	r := reflections[0]
	assert.Equal(t, "a1", r.AgentID)
	assert.Equal(t, "session1", r.SessionKey)
	assert.Equal(t, 10, r.ToolCount)
	assert.Equal(t, 2, r.ErrorCount)
	assert.Greater(t, r.Score, 0.0, "fallback should compute a score")
}
