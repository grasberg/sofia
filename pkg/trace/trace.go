package trace

import (
	"time"

	"github.com/google/uuid"
)

// SpanKind classifies the type of work a span represents.
type SpanKind string

const (
	SpanRequest    SpanKind = "request"
	SpanRouting    SpanKind = "routing"
	SpanLLMCall    SpanKind = "llm_call"
	SpanToolCall   SpanKind = "tool_call"
	SpanDelegation SpanKind = "delegation"
	SpanReflection SpanKind = "reflection"
)

// SpanStatus indicates whether a span completed successfully.
type SpanStatus string

const (
	StatusRunning SpanStatus = "running"
	StatusOK      SpanStatus = "ok"
	StatusError   SpanStatus = "error"
)

// Span represents a single unit of work within an execution trace.
// Spans form a tree via TraceID (shared root) and ParentID.
type Span struct {
	ID         string             `json:"id"`
	TraceID    string             `json:"trace_id"`
	ParentID   string             `json:"parent_id,omitempty"`
	Kind       SpanKind           `json:"kind"`
	Name       string             `json:"name"`
	AgentID    string             `json:"agent_id"`
	SessionKey string             `json:"session_key,omitempty"`
	StartTime  time.Time          `json:"start_time"`
	EndTime    *time.Time         `json:"end_time,omitempty"`
	Status     SpanStatus         `json:"status"`
	Attributes map[string]any     `json:"attributes,omitempty"`
	Scores     map[string]float64 `json:"scores,omitempty"`
}

// Duration returns the span's wall-clock duration. Returns 0 if not yet ended.
func (s *Span) Duration() time.Duration {
	if s.EndTime == nil {
		return 0
	}
	return s.EndTime.Sub(s.StartTime)
}

// TraceSummary is a lightweight view used for listing/filtering traces.
type TraceSummary struct {
	TraceID    string             `json:"trace_id"`
	AgentID    string             `json:"agent_id"`
	SessionKey string             `json:"session_key"`
	Name       string             `json:"name"`
	StartTime  time.Time          `json:"start_time"`
	DurationMs int64              `json:"duration_ms"`
	Status     SpanStatus         `json:"status"`
	SpanCount  int                `json:"span_count"`
	Scores     map[string]float64 `json:"scores,omitempty"`
}

// TraceFilter specifies criteria for querying traces.
type TraceFilter struct {
	AgentID string
	Since   time.Time
	Until   time.Time
	Kind    SpanKind
	Status  SpanStatus
	Limit   int
}

// newSpanID generates a new random span ID.
func newSpanID() string {
	return uuid.New().String()
}
