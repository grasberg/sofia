package memory

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/providers"
)

// ---------------------------------------------------------------------------
// Session CRUD
// ---------------------------------------------------------------------------

// GetOrCreateSession ensures a session row exists for the given key and
// returns the current summary.  agentID is stored on creation only.
func (m *MemoryDB) GetOrCreateSession(key, agentID string) (summary string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	_, err = m.db.Exec(
		`INSERT INTO sessions (key, agent_id, summary, created_at, updated_at)
		 VALUES (?, ?, '', ?, ?)
		 ON CONFLICT(key) DO NOTHING`,
		key, agentID, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("memory: upsert session: %w", err)
	}
	row := m.db.QueryRow(`SELECT summary FROM sessions WHERE key = ?`, key)
	err = row.Scan(&summary)
	if err != nil {
		return "", fmt.Errorf("memory: get session: %w", err)
	}
	return summary, nil
}

// GetSummary returns the summary for a session key (empty string if not found).
func (m *MemoryDB) GetSummary(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var s string
	_ = m.db.QueryRow(`SELECT summary FROM sessions WHERE key = ?`, key).Scan(&s)
	return s
}

// SetSummary updates the summary for a session key.
func (m *MemoryDB) SetSummary(key, summary string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`UPDATE sessions SET summary = ?, updated_at = ? WHERE key = ?`,
		summary, time.Now().UTC(), key,
	)
	return err
}

// AppendMessage appends a single message at the next position in the session.
// The session row must already exist (call GetOrCreateSession first).
// The INSERT and session UPDATE are wrapped in a single transaction.
func (m *MemoryDB) AppendMessage(key string, msg providers.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	toolCallsJSON, err := json.Marshal(msg.ToolCalls)
	if err != nil {
		return fmt.Errorf("memory: marshal tool_calls: %w", err)
	}
	imagesJSON, err := json.Marshal(msg.Images)
	if err != nil {
		return fmt.Errorf("memory: marshal images: %w", err)
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("memory: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(
		`INSERT INTO messages
		    (session_key, position, role, content, tool_calls, tool_call_id, tool_name, images, reasoning_content, created_at)
		 VALUES (
		    ?,
		    (SELECT COALESCE(MAX(position), -1) + 1 FROM messages WHERE session_key = ?),
		    ?, ?, ?, ?, ?, ?, ?,
		    datetime('now')
		 )`,
		key, key,
		msg.Role, m.enc.Encrypt(msg.Content), string(toolCallsJSON), msg.ToolCallID, msg.ToolName,
		string(imagesJSON), m.enc.Encrypt(msg.ReasoningContent),
	)
	if err != nil {
		return fmt.Errorf("memory: append message: %w", err)
	}

	_, err = tx.Exec(`UPDATE sessions SET updated_at = ? WHERE key = ?`, time.Now().UTC(), key)
	if err != nil {
		return fmt.Errorf("memory: update session updated_at: %w", err)
	}

	return tx.Commit()
}

// GetMessages returns all messages for a session, ordered by position.
func (m *MemoryDB) GetMessages(key string) ([]providers.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(
		`SELECT role, content, tool_calls, tool_call_id, tool_name, images, reasoning_content
		 FROM messages WHERE session_key = ? ORDER BY position ASC`,
		key,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: query messages: %w", err)
	}
	defer rows.Close()

	var msgs []providers.Message
	for rows.Next() {
		var msg providers.Message
		var toolCallsJSON, imagesJSON string
		if err = rows.Scan(
			&msg.Role, &msg.Content, &toolCallsJSON, &msg.ToolCallID, &msg.ToolName,
			&imagesJSON, &msg.ReasoningContent,
		); err != nil {
			return nil, fmt.Errorf("memory: scan message: %w", err)
		}
		msg.Content = m.enc.Decrypt(msg.Content)
		msg.ReasoningContent = m.enc.Decrypt(msg.ReasoningContent)
		if err = json.Unmarshal([]byte(toolCallsJSON), &msg.ToolCalls); err != nil {
			msg.ToolCalls = nil
		}
		if err = json.Unmarshal([]byte(imagesJSON), &msg.Images); err != nil {
			msg.Images = nil
		}
		msgs = append(msgs, msg)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: iterate messages: %w", err)
	}
	return msgs, nil
}

// GetMessageCount returns the number of messages in a session without loading them.
func (m *MemoryDB) GetMessageCount(key string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int
	err := m.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_key = ?`, key).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("memory: count messages: %w", err)
	}
	return count, nil
}

// SetMessages replaces all messages in a session with the provided slice.
func (m *MemoryDB) SetMessages(key string, msgs []providers.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("memory: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.Exec(`DELETE FROM messages WHERE session_key = ?`, key); err != nil {
		return fmt.Errorf("memory: delete messages: %w", err)
	}

	if len(msgs) > 0 {
		// Use batch INSERT for efficiency with large message sets
		// Build placeholders: (?, ?, ?, ...), (?, ?, ?, ...), ...
		placeholders := make([]string, len(msgs))
		values := make([]any, 0, len(msgs)*9)
		
		for i, msg := range msgs {
			toolCallsJSON, _ := json.Marshal(msg.ToolCalls)
			imagesJSON, _ := json.Marshal(msg.Images)
			
			placeholders[i] = "(?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))"
			values = append(values,
				key, i,
				msg.Role, m.enc.Encrypt(msg.Content), string(toolCallsJSON),
				msg.ToolCallID, msg.ToolName,
				string(imagesJSON), m.enc.Encrypt(msg.ReasoningContent),
			)
		}
		
		query := fmt.Sprintf(
			`INSERT INTO messages
			    (session_key, position, role, content, tool_calls, tool_call_id, tool_name, images, reasoning_content, created_at)
			 VALUES %s`,
			strings.Join(placeholders, ","),
		)
		
		if _, err = tx.Exec(query, values...); err != nil {
			return fmt.Errorf("memory: batch insert messages: %w", err)
		}
	}

	_, err = tx.Exec(`UPDATE sessions SET updated_at = ? WHERE key = ?`, time.Now().UTC(), key)
	if err != nil {
		return fmt.Errorf("memory: update session updated_at: %w", err)
	}

	return tx.Commit()
}

// TruncateMessages keeps only the last keepLast messages for a session.
// If keepLast <= 0, all messages are deleted.
func (m *MemoryDB) TruncateMessages(key string, keepLast int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if keepLast <= 0 {
		_, err := m.db.Exec(`DELETE FROM messages WHERE session_key = ?`, key)
		return err
	}

	_, err := m.db.Exec(
		`DELETE FROM messages
		 WHERE session_key = ?
		   AND position NOT IN (
		       SELECT position FROM messages WHERE session_key = ?
		       ORDER BY position DESC LIMIT ?
		   )`,
		key, key, keepLast,
	)
	return err
}

// DeleteSession deletes a session and all its messages (cascaded).
func (m *MemoryDB) DeleteSession(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`DELETE FROM sessions WHERE key = ?`, key)
	return err
}

// SessionRow holds the fields returned by ListSessions.
type SessionRow struct {
	Key       string
	AgentID   string
	Summary   string
	CreatedAt time.Time
	UpdatedAt time.Time
	MsgCount  int
	Preview   string
}

// ListSessions returns lightweight metadata for all sessions.
func (m *MemoryDB) ListSessions() ([]SessionRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	const q = `
SELECT s.key, s.agent_id, s.summary, s.created_at, s.updated_at,
       COUNT(msg.id) AS msg_count,
       COALESCE((
           SELECT content FROM messages
           WHERE session_key = s.key AND role = 'user' AND content != ''
           ORDER BY position ASC LIMIT 1
       ), '') AS preview
FROM sessions s
LEFT JOIN messages msg ON msg.session_key = s.key
GROUP BY s.key
ORDER BY s.updated_at DESC`

	rows, err := m.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("memory: list sessions: %w", err)
	}
	defer rows.Close()

	var result []SessionRow
	for rows.Next() {
		var r SessionRow
		var createdStr, updatedStr string
		if err = rows.Scan(
			&r.Key, &r.AgentID, &r.Summary, &createdStr, &updatedStr,
			&r.MsgCount, &r.Preview,
		); err != nil {
			return nil, fmt.Errorf("memory: scan session row: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetSessionMeta returns metadata for a single session by key, or nil if not found.
func (m *MemoryDB) GetSessionMeta(key string) *SessionRow {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var r SessionRow
	var createdStr, updatedStr string
	err := m.db.QueryRow(
		`SELECT s.key, s.agent_id, s.summary, s.created_at, s.updated_at, COUNT(msg.id)
		 FROM sessions s LEFT JOIN messages msg ON msg.session_key = s.key
		 WHERE s.key = ? GROUP BY s.key`,
		key,
	).Scan(&r.Key, &r.AgentID, &r.Summary, &createdStr, &updatedStr, &r.MsgCount)
	if err != nil {
		return nil
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &r
}

// SearchMessageRow holds a single message result from SearchMessages.
type SearchMessageRow struct {
	SessionKey string
	Role       string
	Content    string
	CreatedAt  string
}

// SearchMessages returns user and assistant messages whose content contains the query substring
// (case-insensitive). Results are ordered by recency. Pass limit <= 0 for no limit.
// When encryption is active, all qualifying rows are fetched and filtered in Go
// because SQL LIKE cannot match against encrypted content.
func (m *MemoryDB) SearchMessages(query string, limit int) ([]SearchMessageRow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	// When encryption is active, fetch all candidate rows and filter in Go.
	if m.enc.Active() {
		return m.searchMessagesEncrypted(query, limit)
	}

	q := `SELECT m.session_key, m.role, m.content, m.created_at
	      FROM messages m
	      WHERE m.role IN ('user', 'assistant')
	        AND m.content != ''
	        AND LOWER(m.content) LIKE '%' || LOWER(?) || '%'
	      ORDER BY m.created_at DESC
	      LIMIT ?`

	rows, err := m.db.Query(q, query, limit)
	if err != nil {
		return nil, fmt.Errorf("memory: search messages: %w", err)
	}
	defer rows.Close()

	var result []SearchMessageRow
	for rows.Next() {
		var r SearchMessageRow
		if err = rows.Scan(&r.SessionKey, &r.Role, &r.Content, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("memory: scan search row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// searchMessagesEncrypted fetches all user/assistant messages, decrypts them,
// and filters by query in Go. This is needed because SQL LIKE cannot operate
// on encrypted content.
func (m *MemoryDB) searchMessagesEncrypted(query string, limit int) ([]SearchMessageRow, error) {
	// Add reasonable upper bound to prevent unbounded memory loading
	// while still allowing Go-side filtering to find matches
	maxRows := limit * 10
	if maxRows < 1000 {
		maxRows = 1000
	}
	if maxRows > 10000 {
		maxRows = 10000
	}
	
	q := fmt.Sprintf(`SELECT m.session_key, m.role, m.content, m.created_at
	      FROM messages m
	      WHERE m.role IN ('user', 'assistant')
	        AND m.content != ''
	      ORDER BY m.created_at DESC
	      LIMIT %d`, maxRows)

	rows, err := m.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("memory: search messages (encrypted): %w", err)
	}
	defer rows.Close()

	lowerQuery := strings.ToLower(query)
	var result []SearchMessageRow
	for rows.Next() {
		var r SearchMessageRow
		if err = rows.Scan(&r.SessionKey, &r.Role, &r.Content, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("memory: scan search row: %w", err)
		}
		r.Content = m.enc.Decrypt(r.Content)
		if strings.Contains(strings.ToLower(r.Content), lowerQuery) {
			result = append(result, r)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, rows.Err()
}
