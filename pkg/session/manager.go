package session

import (
	"log"
	"sort"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
)

// SessionMeta is a lightweight summary of a session used for listing history.
type SessionMeta struct {
	Key          string    `json:"key"`
	Channel      string    `json:"channel"`
	Preview      string    `json:"preview"`
	MessageCount int       `json:"message_count"`
	Created      time.Time `json:"created"`
	Updated      time.Time `json:"updated"`
}

// Session is kept as a type for callers that use it directly (e.g. web handler).
// It is no longer persisted as a struct; the DB is the source of truth.
type Session struct {
	Key      string              `json:"key"`
	Messages []providers.Message `json:"messages"`
	Summary  string              `json:"summary,omitempty"`
	Created  time.Time           `json:"created"`
	Updated  time.Time           `json:"updated"`
}

// SessionManager manages conversation sessions backed by a shared MemoryDB.
type SessionManager struct {
	db      *memory.MemoryDB
	agentID string
}

// NewSessionManager creates a SessionManager using the given MemoryDB.
// agentID is stored with newly created sessions and memory notes.
func NewSessionManager(db *memory.MemoryDB, agentID string) *SessionManager {
	return &SessionManager{db: db, agentID: agentID}
}

// GetOrCreate ensures a session exists for the given key and returns it.
// The returned Session has its Messages populated from the DB.
func (sm *SessionManager) GetOrCreate(key string) *Session {
	summary, err := sm.db.GetOrCreateSession(key, sm.agentID)
	if err != nil {
		log.Printf("session: GetOrCreate(%q): %v", key, err)
	}
	msgs, err := sm.db.GetMessages(key)
	if err != nil {
		log.Printf("session: GetOrCreate(%q) messages: %v", key, err)
	}
	if msgs == nil {
		msgs = []providers.Message{}
	}
	return &Session{
		Key:      key,
		Messages: msgs,
		Summary:  summary,
	}
}

// AddMessage adds a simple role/content message to the session.
func (sm *SessionManager) AddMessage(sessionKey, role, content string) {
	sm.AddFullMessage(sessionKey, providers.Message{
		Role:    role,
		Content: content,
	})
}

// AddFullMessage appends a complete message (including tool calls and images)
// directly to the database.
func (sm *SessionManager) AddFullMessage(sessionKey string, msg providers.Message) {
	// Ensure the session row exists before appending.
	if _, err := sm.db.GetOrCreateSession(sessionKey, sm.agentID); err != nil {
		log.Printf("session: AddFullMessage(%q) ensure session: %v", sessionKey, err)
	}
	if err := sm.db.AppendMessage(sessionKey, msg); err != nil {
		log.Printf("session: AddFullMessage(%q) append: %v", sessionKey, err)
	}
}

// GetHistory returns all messages for the session, ordered oldest first.
func (sm *SessionManager) GetHistory(key string) []providers.Message {
	msgs, err := sm.db.GetMessages(key)
	if err != nil {
		log.Printf("session: GetHistory(%q): %v", key, err)
	}
	if msgs == nil {
		return []providers.Message{}
	}
	return msgs
}

// GetSummary returns the compression summary for a session.
func (sm *SessionManager) GetSummary(key string) string {
	return sm.db.GetSummary(key)
}

// SetSummary updates the compression summary for a session.
func (sm *SessionManager) SetSummary(key string, summary string) {
	if err := sm.db.SetSummary(key, summary); err != nil {
		log.Printf("session: SetSummary(%q): %v", key, err)
	}
}

// TruncateHistory keeps only the last keepLast messages.
// If keepLast <= 0, all messages are deleted.
func (sm *SessionManager) TruncateHistory(key string, keepLast int) {
	if err := sm.db.TruncateMessages(key, keepLast); err != nil {
		log.Printf("session: TruncateHistory(%q, %d): %v", key, keepLast, err)
	}
}

// Save is a no-op: writes are immediate via AddFullMessage.
// Kept to avoid changing callers.
func (sm *SessionManager) Save(_ string) error {
	return nil
}

// SetHistory replaces all messages in a session with the provided slice.
func (sm *SessionManager) SetHistory(key string, history []providers.Message) {
	// Ensure the session row exists before replacing messages.
	if _, err := sm.db.GetOrCreateSession(key, sm.agentID); err != nil {
		log.Printf("session: SetHistory(%q) ensure session: %v", key, err)
	}
	if err := sm.db.SetMessages(key, history); err != nil {
		log.Printf("session: SetHistory(%q) set messages: %v", key, err)
	}
}

// inferChannel extracts a human-readable channel name from a session key.
// Example keys: "web:ui:2026-03-04T10:00:00Z", "agent:main:telegram:direct:123", "web:ui".
func inferChannel(key string) string {
	switch {
	case strings.HasPrefix(key, "web:"):
		return "web"
	case strings.Contains(key, ":telegram:"):
		return "telegram"
	case strings.Contains(key, ":discord:"):
		return "discord"
	case strings.Contains(key, ":slack:"):
		return "slack"
	case strings.HasPrefix(key, "subagent:"):
		return "subagent"
	case key == "heartbeat":
		return "heartbeat"
	default:
		return "cli"
	}
}

// ListSessions returns lightweight metadata for all sessions, sorted
// by Updated descending (most recent first).
func (sm *SessionManager) ListSessions() []SessionMeta {
	rows, err := sm.db.ListSessions()
	if err != nil {
		log.Printf("session: ListSessions: %v", err)
		return nil
	}

	metas := make([]SessionMeta, 0, len(rows))
	for _, r := range rows {
		preview := r.Preview
		if len(preview) > 80 {
			preview = preview[:80] + "…"
		}
		metas = append(metas, SessionMeta{
			Key:          r.Key,
			Channel:      inferChannel(r.Key),
			Preview:      preview,
			MessageCount: r.MsgCount,
			Created:      r.CreatedAt,
			Updated:      r.UpdatedAt,
		})
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Updated.After(metas[j].Updated)
	})

	return metas
}

// DeleteSession removes a session and all its messages from the database.
func (sm *SessionManager) DeleteSession(key string) error {
	return sm.db.DeleteSession(key)
}

// SessionRotationPolicy defines when a session should be rotated to a fresh one.
type SessionRotationPolicy struct {
	MaxTokenEstimate int           // Rotate when estimated tokens exceed this (0 = disabled)
	MaxAge           time.Duration // Rotate when session age exceeds this (0 = disabled)
	MaxMessages      int           // Rotate when message count exceeds this (0 = disabled)
}

// ShouldRotate checks if a session should be rotated based on the given policy.
// Returns true if any threshold is exceeded.
func (sm *SessionManager) ShouldRotate(key string, policy SessionRotationPolicy) bool {
	if policy.MaxTokenEstimate <= 0 && policy.MaxAge <= 0 && policy.MaxMessages <= 0 {
		return false
	}

	msgs := sm.GetHistory(key)

	// Check message count
	if policy.MaxMessages > 0 && len(msgs) > policy.MaxMessages {
		return true
	}

	// Check token estimate (rough: 1 token ≈ 4 chars)
	if policy.MaxTokenEstimate > 0 {
		totalChars := 0
		for _, m := range msgs {
			totalChars += len(m.Content)
		}
		estimatedTokens := totalChars / 4
		if estimatedTokens > policy.MaxTokenEstimate {
			return true
		}
	}

	// Check age
	if policy.MaxAge > 0 {
		meta := sm.db.GetSessionMeta(key)
		if meta != nil && !meta.CreatedAt.IsZero() && time.Since(meta.CreatedAt) > policy.MaxAge {
			return true
		}
	}

	return false
}

// RotateSession archives the current session and creates a fresh one.
// The old session's summary is carried forward to the new session as context.
func (sm *SessionManager) RotateSession(oldKey, newKey string) {
	// Carry forward the summary from the old session
	summary := sm.GetSummary(oldKey)
	if summary == "" {
		// Generate a minimal summary from the last few messages
		msgs := sm.GetHistory(oldKey)
		if len(msgs) > 0 {
			last := msgs[len(msgs)-1]
			if len(last.Content) > 200 {
				summary = "Previous session context: " + last.Content[:200] + "..."
			} else if last.Content != "" {
				summary = "Previous session context: " + last.Content
			}
		}
	}

	// Create new session with carried-forward summary
	sm.GetOrCreate(newKey)
	if summary != "" {
		sm.SetSummary(newKey, summary)
	}
}
