package agent

import (
	"sync"
	"time"
)

// ElevationState holds the state for a single elevated session.
type ElevationState struct {
	Active    bool
	GrantedBy string
	Channel   string
	ExpiresAt time.Time
}

// ElevatedManager manages per-session elevated permissions that relax
// some shell deny-patterns (e.g. package managers, docker, git push)
// while keeping destructive patterns always enforced.
type ElevatedManager struct {
	mu       sync.RWMutex
	sessions map[string]*ElevationState
}

// NewElevatedManager creates a new ElevatedManager.
func NewElevatedManager() *ElevatedManager {
	return &ElevatedManager{
		sessions: make(map[string]*ElevationState),
	}
}

// Elevate grants elevated permissions to the given session for the specified duration.
func (em *ElevatedManager) Elevate(sessionKey, grantedBy, channel string, duration time.Duration) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.sessions[sessionKey] = &ElevationState{
		Active:    true,
		GrantedBy: grantedBy,
		Channel:   channel,
		ExpiresAt: time.Now().Add(duration),
	}
}

// Revoke removes elevated permissions from the given session.
func (em *ElevatedManager) Revoke(sessionKey string) {
	em.mu.Lock()
	defer em.mu.Unlock()
	delete(em.sessions, sessionKey)
}

// IsElevated returns true if the given session currently has elevated permissions.
func (em *ElevatedManager) IsElevated(sessionKey string) bool {
	em.mu.RLock()
	defer em.mu.RUnlock()
	s, ok := em.sessions[sessionKey]
	if !ok || !s.Active {
		return false
	}
	if time.Now().After(s.ExpiresAt) {
		return false
	}
	return true
}

// GetState returns a copy of the elevation state for the given session, or nil.
func (em *ElevatedManager) GetState(sessionKey string) *ElevationState {
	em.mu.RLock()
	defer em.mu.RUnlock()
	s, ok := em.sessions[sessionKey]
	if !ok {
		return nil
	}
	cp := *s
	return &cp
}
