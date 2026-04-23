package memory

import (
	"database/sql"
	"fmt"
)

// applyV19tx creates the email_ingested table used by the email channel to
// deduplicate inbound messages across polls and restarts.
func (m *MemoryDB) applyV19tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS email_ingested (
    message_id   TEXT    PRIMARY KEY,
    thread_id    TEXT    NOT NULL DEFAULT '',
    from_addr    TEXT    NOT NULL DEFAULT '',
    subject      TEXT    NOT NULL DEFAULT '',
    ingested_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_email_ingested_thread ON email_ingested(thread_id);
CREATE INDEX IF NOT EXISTS idx_email_ingested_from ON email_ingested(from_addr, ingested_at);
`
	_, err := tx.Exec(ddl)
	return err
}

// IsEmailIngested reports whether a message ID has already been ingested.
func (m *MemoryDB) IsEmailIngested(messageID string) (bool, error) {
	if messageID == "" {
		return false, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var seen int
	err := m.db.QueryRow(
		`SELECT 1 FROM email_ingested WHERE message_id = ? LIMIT 1`,
		messageID,
	).Scan(&seen)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("memory: is_email_ingested: %w", err)
	}
	return true, nil
}

// MarkEmailIngested records that a message has been delivered to the bus.
// Safe to call with duplicates — INSERT OR IGNORE makes it idempotent.
func (m *MemoryDB) MarkEmailIngested(messageID, threadID, fromAddr, subject string) error {
	if messageID == "" {
		return fmt.Errorf("memory: mark_email_ingested: empty message_id")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`INSERT OR IGNORE INTO email_ingested (message_id, thread_id, from_addr, subject)
		 VALUES (?, ?, ?, ?)`,
		messageID, threadID, fromAddr, subject,
	)
	if err != nil {
		return fmt.Errorf("memory: mark_email_ingested: %w", err)
	}
	return nil
}

// PruneEmailIngestedBefore deletes ingestion records older than the given
// cutoff expressed as an SQL datetime modifier (e.g. "-30 days"). Returns the
// number of rows removed.
func (m *MemoryDB) PruneEmailIngestedBefore(cutoffModifier string) (int64, error) {
	if cutoffModifier == "" {
		cutoffModifier = "-30 days"
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	res, err := m.db.Exec(
		`DELETE FROM email_ingested WHERE ingested_at < datetime('now', ?)`,
		cutoffModifier,
	)
	if err != nil {
		return 0, fmt.Errorf("memory: prune_email_ingested: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
