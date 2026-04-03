package trace

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStore implements TraceStore for testing.
type mockStore struct {
	spans []mockSpanRow
}

type mockSpanRow struct {
	id, traceID, parentID, kind, name, agentID, sessionKey, status string
	startTime                                                      time.Time
	endTime                                                        *time.Time
	attributes                                                     map[string]any
	scores                                                         map[string]float64
}

func (m *mockStore) InsertTraceSpan(
	id, traceID, parentID, kind, name, agentID, sessionKey string,
	startTime time.Time, endTime *time.Time, status string,
	attributes map[string]any, scores map[string]float64,
) error {
	m.spans = append(m.spans, mockSpanRow{
		id: id, traceID: traceID, parentID: parentID,
		kind: kind, name: name, agentID: agentID,
		sessionKey: sessionKey, status: status,
		startTime: startTime, endTime: endTime,
		attributes: attributes, scores: scores,
	})
	return nil
}

func (m *mockStore) UpdateTraceScores(string, map[string]float64) error { return nil }
func (m *mockStore) GetTraceSpans(string) ([]Span, error)               { return nil, nil }
func (m *mockStore) QueryTraceSummaries(string, time.Time, time.Time, int) ([]TraceSummary, error) {
	return nil, nil
}
func (m *mockStore) PruneTraces(int) error { return nil }

func TestTracerSpanHierarchy(t *testing.T) {
	store := &mockStore{}
	tracer := NewTracer(store)

	// Create a root span
	root := tracer.StartTrace("agent-1", "session-1", "processMessage")
	require.NotNil(t, root)
	assert.Equal(t, root.ID, root.TraceID, "root span ID should equal trace ID")
	assert.Equal(t, SpanRequest, root.Kind)
	assert.Equal(t, "agent-1", root.AgentID)

	// Create child spans
	child1 := tracer.StartSpan(root, SpanLLMCall, "runLLMIteration")
	require.NotNil(t, child1)
	assert.Equal(t, root.TraceID, child1.TraceID)
	assert.Equal(t, root.ID, child1.ParentID)
	assert.Equal(t, SpanLLMCall, child1.Kind)

	child2 := tracer.StartSpan(child1, SpanToolCall, "read_file")
	require.NotNil(t, child2)
	assert.Equal(t, root.TraceID, child2.TraceID)
	assert.Equal(t, child1.ID, child2.ParentID)
	assert.Equal(t, SpanToolCall, child2.Kind)

	// Nil parent returns nil span
	assert.Nil(t, tracer.StartSpan(nil, SpanToolCall, "test"))

	// Set scores
	tracer.SetScore(root, "task_completion", 0.85)
	assert.Equal(t, 0.85, root.Scores["task_completion"])

	// End spans (they get flushed to store)
	tracer.EndSpan(child2, StatusOK, map[string]any{"result": "ok"})
	tracer.EndSpan(child1, StatusOK, nil)
	tracer.EndSpan(root, StatusOK, map[string]any{"model": "gpt-4o"})

	// Wait for flush
	tracer.Close()

	// Verify spans were persisted
	assert.Len(t, store.spans, 3)

	// Check root span
	rootRow := store.spans[2]
	assert.Equal(t, root.ID, rootRow.id)
	assert.Equal(t, "request", rootRow.kind)
	assert.Equal(t, "ok", rootRow.status)
	assert.Equal(t, "gpt-4o", rootRow.attributes["model"])
	assert.Equal(t, 0.85, rootRow.scores["task_completion"])

	// Check tool span
	toolRow := store.spans[0]
	assert.Equal(t, child2.ID, toolRow.id)
	assert.Equal(t, child1.ID, toolRow.parentID)
	assert.Equal(t, "tool_call", toolRow.kind)
}

func TestSpanDuration(t *testing.T) {
	span := &Span{StartTime: time.Now()}
	assert.Equal(t, time.Duration(0), span.Duration())

	now := time.Now().Add(100 * time.Millisecond)
	span.EndTime = &now
	assert.Greater(t, span.Duration(), time.Duration(0))
}

func TestEndSpanNilSafe(t *testing.T) {
	store := &mockStore{}
	tracer := NewTracer(store)
	defer tracer.Close()

	// Should not panic
	tracer.EndSpan(nil, StatusOK, nil)
	tracer.SetScore(nil, "test", 1.0)
}
