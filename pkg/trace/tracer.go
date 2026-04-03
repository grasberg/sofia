package trace

import (
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
)

const (
	flushInterval = 100 * time.Millisecond
	flushBatch    = 50
	pruneEvery    = 200 // prune check every N writes
	retentionDays = 30
)

// TraceStore is the persistence interface the Tracer needs.
// Implemented by memory.MemoryDB.
type TraceStore interface {
	InsertTraceSpan(id, traceID, parentID string, kind, name, agentID, sessionKey string,
		startTime time.Time, endTime *time.Time, status string,
		attributes map[string]any, scores map[string]float64) error
	UpdateTraceScores(traceID string, scores map[string]float64) error
	GetTraceSpans(traceID string) ([]Span, error)
	QueryTraceSummaries(agentID string, since, until time.Time, limit int) ([]TraceSummary, error)
	PruneTraces(retentionDays int) error
}

// Tracer creates and manages execution trace spans.
// Spans are buffered and flushed asynchronously to SQLite.
type Tracer struct {
	store      TraceStore
	buf        chan *Span
	done       chan struct{}
	wg         sync.WaitGroup
	closeOnce  sync.Once
	writeCount int
	mu         sync.Mutex
}

// NewTracer creates a tracer backed by the given store.
// Call Close() to flush remaining spans on shutdown.
func NewTracer(store TraceStore) *Tracer {
	t := &Tracer{
		store: store,
		buf:   make(chan *Span, 256),
		done:  make(chan struct{}),
	}
	t.wg.Add(1)
	go t.flushLoop()
	return t
}

// StartTrace creates a root span for a new request.
func (t *Tracer) StartTrace(agentID, sessionKey, name string) *Span {
	traceID := newSpanID()
	return &Span{
		ID:         traceID,
		TraceID:    traceID,
		Kind:       SpanRequest,
		Name:       name,
		AgentID:    agentID,
		SessionKey: sessionKey,
		StartTime:  time.Now(),
		Status:     StatusRunning,
		Attributes: make(map[string]any),
		Scores:     make(map[string]float64),
	}
}

// StartSpan creates a child span under the given parent.
func (t *Tracer) StartSpan(parent *Span, kind SpanKind, name string) *Span {
	if parent == nil {
		return nil
	}
	return &Span{
		ID:         newSpanID(),
		TraceID:    parent.TraceID,
		ParentID:   parent.ID,
		Kind:       kind,
		Name:       name,
		AgentID:    parent.AgentID,
		SessionKey: parent.SessionKey,
		StartTime:  time.Now(),
		Status:     StatusRunning,
		Attributes: make(map[string]any),
		Scores:     make(map[string]float64),
	}
}

// EndSpan finalizes a span with a status and optional attributes, then enqueues it for persistence.
func (t *Tracer) EndSpan(span *Span, status SpanStatus, attrs map[string]any) {
	if span == nil {
		return
	}
	now := time.Now()
	span.EndTime = &now
	span.Status = status
	for k, v := range attrs {
		span.Attributes[k] = v
	}
	t.enqueue(span)
}

// SetScore attaches a named score to a span (in-memory only; persisted when the span is ended).
func (t *Tracer) SetScore(span *Span, dimension string, value float64) {
	if span == nil {
		return
	}
	span.Scores[dimension] = value
}

// SetScoreByTraceID updates scores on an already-persisted trace root span.
func (t *Tracer) SetScoreByTraceID(traceID string, scores map[string]float64) {
	if t.store == nil || len(scores) == 0 {
		return
	}
	if err := t.store.UpdateTraceScores(traceID, scores); err != nil {
		logger.WarnCF("trace", "Failed to update trace scores",
			map[string]any{"trace_id": traceID, "error": err.Error()})
	}
}

// GetTrace returns all spans belonging to a trace, ordered by start_time.
func (t *Tracer) GetTrace(traceID string) ([]Span, error) {
	if t.store == nil {
		return nil, nil
	}
	return t.store.GetTraceSpans(traceID)
}

// QueryTraces returns trace summaries matching the given filter.
func (t *Tracer) QueryTraces(filter TraceFilter) ([]TraceSummary, error) {
	if t.store == nil {
		return nil, nil
	}
	return t.store.QueryTraceSummaries(filter.AgentID, filter.Since, filter.Until, filter.Limit)
}

// Close flushes remaining spans and stops the background goroutine.
// Safe to call multiple times.
func (t *Tracer) Close() {
	t.closeOnce.Do(func() {
		close(t.done)
		t.wg.Wait()
	})
}

func (t *Tracer) enqueue(span *Span) {
	select {
	case t.buf <- span:
	default:
		// Buffer full — write synchronously to avoid data loss.
		t.persist(span)
	}
}

func (t *Tracer) flushLoop() {
	defer t.wg.Done()
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	batch := make([]*Span, 0, flushBatch)
	for {
		select {
		case span := <-t.buf:
			batch = append(batch, span)
			if len(batch) >= flushBatch {
				t.persistBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				t.persistBatch(batch)
				batch = batch[:0]
			}
		case <-t.done:
			// Drain remaining
			for {
				select {
				case span := <-t.buf:
					batch = append(batch, span)
				default:
					if len(batch) > 0 {
						t.persistBatch(batch)
					}
					return
				}
			}
		}
	}
}

func (t *Tracer) persistBatch(batch []*Span) {
	for _, s := range batch {
		t.persist(s)
	}
}

func (t *Tracer) persist(span *Span) {
	if t.store == nil {
		return
	}
	if err := t.store.InsertTraceSpan(
		span.ID, span.TraceID, span.ParentID,
		string(span.Kind), span.Name, span.AgentID, span.SessionKey,
		span.StartTime, span.EndTime, string(span.Status),
		span.Attributes, span.Scores,
	); err != nil {
		logger.WarnCF("trace", "Failed to persist span",
			map[string]any{"span_id": span.ID, "error": err.Error()})
		return
	}

	t.mu.Lock()
	t.writeCount++
	shouldPrune := t.writeCount%pruneEvery == 0
	t.mu.Unlock()

	if shouldPrune {
		if err := t.store.PruneTraces(retentionDays); err != nil {
			logger.WarnCF("trace", "Failed to prune old traces",
				map[string]any{"error": err.Error()})
		}
	}
}
