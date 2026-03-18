package agent

import (
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/providers/protocoltypes"
)

// SessionUsage tracks accumulated token usage for a single session.
type SessionUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	CallCount        int
	StartTime        time.Time
}

// UsageTracker accumulates token usage per session.
type UsageTracker struct {
	mu       sync.Mutex
	sessions map[string]*SessionUsage
}

func NewUsageTracker() *UsageTracker {
	return &UsageTracker{
		sessions: make(map[string]*SessionUsage),
	}
}

// Record adds a single LLM response's usage to the session accumulator.
func (ut *UsageTracker) Record(sessionKey string, usage *protocoltypes.UsageInfo) {
	if usage == nil {
		return
	}
	ut.mu.Lock()
	defer ut.mu.Unlock()

	s, ok := ut.sessions[sessionKey]
	if !ok {
		s = &SessionUsage{StartTime: time.Now()}
		ut.sessions[sessionKey] = s
	}
	s.PromptTokens += int64(usage.PromptTokens)
	s.CompletionTokens += int64(usage.CompletionTokens)
	s.TotalTokens += int64(usage.TotalTokens)
	s.CallCount++
}

// GetSession returns accumulated usage for a session, or nil if no data.
func (ut *UsageTracker) GetSession(sessionKey string) *SessionUsage {
	ut.mu.Lock()
	defer ut.mu.Unlock()
	s, ok := ut.sessions[sessionKey]
	if !ok {
		return nil
	}
	// Return a copy
	cp := *s
	return &cp
}

// Reset clears usage data for a session.
func (ut *UsageTracker) Reset(sessionKey string) {
	ut.mu.Lock()
	defer ut.mu.Unlock()
	delete(ut.sessions, sessionKey)
}
