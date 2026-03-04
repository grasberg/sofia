package session

import (
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
	summary, _ := sm.db.GetOrCreateSession(key, sm.agentID)
	msgs, _ := sm.db.GetMessages(key)
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
	_, _ = sm.db.GetOrCreateSession(sessionKey, sm.agentID)
	_ = sm.db.AppendMessage(sessionKey, msg)
}

// GetHistory returns all messages for the session, ordered oldest first.
func (sm *SessionManager) GetHistory(key string) []providers.Message {
	msgs, _ := sm.db.GetMessages(key)
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
	_ = sm.db.SetSummary(key, summary)
}

// TruncateHistory keeps only the last keepLast messages.
// If keepLast <= 0, all messages are deleted.
func (sm *SessionManager) TruncateHistory(key string, keepLast int) {
	_ = sm.db.TruncateMessages(key, keepLast)
}

// Save is a no-op: writes are immediate via AddFullMessage.
// Kept to avoid changing callers.
func (sm *SessionManager) Save(_ string) error {
	return nil
}

// SetHistory replaces all messages in a session with the provided slice.
func (sm *SessionManager) SetHistory(key string, history []providers.Message) {
	// Ensure the session row exists before replacing messages.
	_, _ = sm.db.GetOrCreateSession(key, sm.agentID)
	_ = sm.db.SetMessages(key, history)
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
