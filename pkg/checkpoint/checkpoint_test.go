package checkpoint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
)

func setupTestDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func seedSession(t *testing.T, db *memory.MemoryDB, key string, msgs []providers.Message) {
	t.Helper()
	_, err := db.GetOrCreateSession(key, "test-agent")
	require.NoError(t, err)
	for _, m := range msgs {
		require.NoError(t, db.AppendMessage(key, m))
	}
}

func TestCreate(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewManager(db)

	seedSession(t, db, "sess1", []providers.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	})

	cp, err := mgr.Create("sess1", "agent1", "before-edit", 3)
	require.NoError(t, err)
	assert.Equal(t, "before-edit", cp.Name)
	assert.Equal(t, 3, cp.Iteration)
	assert.Equal(t, 2, cp.MsgCount)
	assert.True(t, cp.ID > 0)
}

func TestList(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewManager(db)

	seedSession(t, db, "sess1", []providers.Message{
		{Role: "user", Content: "a"},
	})

	_, err := mgr.Create("sess1", "agent1", "cp1", 1)
	require.NoError(t, err)

	// Add more messages
	require.NoError(t, db.AppendMessage("sess1", providers.Message{Role: "assistant", Content: "b"}))

	_, err = mgr.Create("sess1", "agent1", "cp2", 2)
	require.NoError(t, err)

	list, err := mgr.List("sess1")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	// Newest first
	assert.Equal(t, "cp2", list[0].Name)
	assert.Equal(t, 2, list[0].MsgCount)
	assert.Equal(t, "cp1", list[1].Name)
	assert.Equal(t, 1, list[1].MsgCount)
}

func TestRollback(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewManager(db)

	seedSession(t, db, "sess1", []providers.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	})

	cp1, err := mgr.Create("sess1", "agent1", "cp1", 1)
	require.NoError(t, err)

	// Add more messages after checkpoint
	require.NoError(t, db.AppendMessage("sess1", providers.Message{Role: "user", Content: "do something"}))
	require.NoError(t, db.AppendMessage("sess1", providers.Message{Role: "assistant", Content: "done"}))

	// Create second checkpoint
	_, err = mgr.Create("sess1", "agent1", "cp2", 2)
	require.NoError(t, err)

	// Rollback to first checkpoint
	restored, err := mgr.Rollback("sess1", cp1.ID)
	require.NoError(t, err)
	assert.Equal(t, cp1.ID, restored.ID)

	// Verify messages truncated
	msgs, err := db.GetMessages("sess1")
	require.NoError(t, err)
	assert.Len(t, msgs, 2) // Only the original 2 messages

	// Verify cp2 was deleted
	list, err := mgr.List("sess1")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "cp1", list[0].Name)
}

func TestRollbackToLatest(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewManager(db)

	seedSession(t, db, "sess1", []providers.Message{
		{Role: "user", Content: "hello"},
	})

	_, err := mgr.Create("sess1", "agent1", "cp1", 1)
	require.NoError(t, err)

	// Add more messages
	require.NoError(t, db.AppendMessage("sess1", providers.Message{Role: "assistant", Content: "result"}))
	require.NoError(t, db.AppendMessage("sess1", providers.Message{Role: "user", Content: "more"}))

	_, err = mgr.Create("sess1", "agent1", "cp2", 2)
	require.NoError(t, err)

	// Add even more
	require.NoError(t, db.AppendMessage("sess1", providers.Message{Role: "assistant", Content: "failed"}))

	cp, msgs, err := mgr.RollbackToLatest("sess1")
	require.NoError(t, err)
	assert.Equal(t, "cp2", cp.Name)
	assert.Len(t, msgs, 3) // 3 messages at cp2
}

func TestRollbackToLatestEmpty(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewManager(db)

	seedSession(t, db, "sess1", nil)

	cp, msgs, err := mgr.RollbackToLatest("sess1")
	require.NoError(t, err)
	assert.Nil(t, cp)
	assert.Nil(t, msgs)
}

func TestCleanup(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewManager(db)

	seedSession(t, db, "sess1", []providers.Message{
		{Role: "user", Content: "hello"},
	})

	_, err := mgr.Create("sess1", "agent1", "cp1", 1)
	require.NoError(t, err)
	_, err = mgr.Create("sess1", "agent1", "cp2", 2)
	require.NoError(t, err)

	err = mgr.Cleanup("sess1")
	require.NoError(t, err)

	list, err := mgr.List("sess1")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestRollbackSessionMismatch(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewManager(db)

	seedSession(t, db, "sess1", []providers.Message{
		{Role: "user", Content: "hello"},
	})
	seedSession(t, db, "sess2", nil)

	cp, err := mgr.Create("sess1", "agent1", "cp1", 1)
	require.NoError(t, err)

	_, err = mgr.Rollback("sess2", cp.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session mismatch")
}
