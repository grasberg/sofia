package export

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
)

// openTestDB creates an in-memory SQLite database for testing.
func openTestDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedSession creates a session with the given messages in the test DB.
func seedSession(t *testing.T, db *memory.MemoryDB, key, agentID string, msgs []providers.Message) {
	t.Helper()
	_, err := db.GetOrCreateSession(key, agentID)
	require.NoError(t, err)
	for _, m := range msgs {
		require.NoError(t, db.AppendMessage(key, m))
	}
}

func TestExportSingleSession(t *testing.T) {
	db := openTestDB(t)

	msgs := []providers.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	seedSession(t, db, "test-session", "agent1", msgs)

	data, err := buildExportData(db, "test-session", false, false)
	require.NoError(t, err)

	assert.Equal(t, 1, data.Version)
	assert.False(t, data.ExportedAt.IsZero())

	require.Len(t, data.Sessions, 1)
	s := data.Sessions[0]
	assert.Equal(t, "test-session", s.Key)
	assert.Equal(t, "agent1", s.AgentID)
	require.Len(t, s.Messages, 2)
	assert.Equal(t, "user", s.Messages[0].Role)
	assert.Equal(t, "hello", s.Messages[0].Content)
	assert.Equal(t, "assistant", s.Messages[1].Role)
	assert.Equal(t, "hi there", s.Messages[1].Content)
	assert.Equal(t, 2, s.Metadata.MessageCount)
	assert.Nil(t, data.Memory)
}

func TestExportAllSessions(t *testing.T) {
	db := openTestDB(t)

	seedSession(t, db, "s1", "a1", []providers.Message{
		{Role: "user", Content: "q1"},
	})
	seedSession(t, db, "s2", "a2", []providers.Message{
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
	})

	data, err := buildExportData(db, "", true, false)
	require.NoError(t, err)

	assert.Len(t, data.Sessions, 2)

	keys := []string{data.Sessions[0].Key, data.Sessions[1].Key}
	assert.True(t, slices.Contains(keys, "s1"))
	assert.True(t, slices.Contains(keys, "s2"))
}

func TestExportNotFound(t *testing.T) {
	db := openTestDB(t)

	_, err := buildExportData(db, "nonexistent", false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestExportWithMemory(t *testing.T) {
	db := openTestDB(t)

	seedSession(t, db, "s1", "a1", []providers.Message{
		{Role: "user", Content: "hi"},
	})
	require.NoError(t, db.SetNote("a1", "longterm", "", "user likes tea"))
	require.NoError(t, db.SetNote("a1", "daily", "20260318", "had a meeting"))

	data, err := buildExportData(db, "", true, true)
	require.NoError(t, err)

	require.Len(t, data.Memory, 2)
	assert.Equal(t, "a1", data.Memory[0].AgentID)
}

func TestExportJSONFormat(t *testing.T) {
	db := openTestDB(t)

	seedSession(t, db, "web:ui:test", "main", []providers.Message{
		{Role: "user", Content: "test message"},
	})

	data, err := buildExportData(db, "web:ui:test", false, false)
	require.NoError(t, err)

	raw, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)

	// Re-parse and verify round-trip.
	var parsed ExportData
	require.NoError(t, json.Unmarshal(raw, &parsed))
	assert.Equal(t, 1, parsed.Version)
	require.Len(t, parsed.Sessions, 1)
	assert.Equal(t, "web:ui:test", parsed.Sessions[0].Key)
	assert.Equal(t, "test message", parsed.Sessions[0].Messages[0].Content)
}

func TestImportNewSessions(t *testing.T) {
	db := openTestDB(t)

	data := &ExportData{
		Version: 1,
		Sessions: []ExportSession{
			{
				Key:     "imported-1",
				AgentID: "bot",
				Messages: []providers.Message{
					{Role: "user", Content: "hello from import"},
					{Role: "assistant", Content: "welcome"},
				},
				Summary: "greeting session",
			},
		},
	}

	require.NoError(t, importData(db, data))

	msgs, err := db.GetMessages("imported-1")
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "hello from import", msgs[0].Content)

	summary := db.GetSummary("imported-1")
	assert.Equal(t, "greeting session", summary)
}

func TestImportSkipsDuplicates(t *testing.T) {
	db := openTestDB(t)

	// Pre-create a session.
	seedSession(t, db, "existing", "a1", []providers.Message{
		{Role: "user", Content: "original"},
	})

	data := &ExportData{
		Version: 1,
		Sessions: []ExportSession{
			{
				Key:     "existing",
				AgentID: "a1",
				Messages: []providers.Message{
					{Role: "user", Content: "should not overwrite"},
				},
			},
			{
				Key:     "new-session",
				AgentID: "a2",
				Messages: []providers.Message{
					{Role: "user", Content: "brand new"},
				},
			},
		},
	}

	require.NoError(t, importData(db, data))

	// Original session should be unchanged.
	msgs, err := db.GetMessages("existing")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "original", msgs[0].Content)

	// New session should have been imported.
	msgs, err = db.GetMessages("new-session")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "brand new", msgs[0].Content)
}

func TestImportWithMemoryNotes(t *testing.T) {
	db := openTestDB(t)

	data := &ExportData{
		Version:  1,
		Sessions: []ExportSession{},
		Memory: []ExportMemory{
			{AgentID: "a1", Kind: "longterm", DateKey: "", Content: "test note"},
		},
	}

	require.NoError(t, importData(db, data))

	content := db.GetNote("a1", "longterm", "")
	assert.Equal(t, "test note", content)
}

func TestExportRoundTrip(t *testing.T) {
	srcDB := openTestDB(t)
	dstDB := openTestDB(t)

	seedSession(t, srcDB, "rt-session", "agent", []providers.Message{
		{Role: "user", Content: "round trip"},
		{Role: "assistant", Content: "confirmed"},
	})
	require.NoError(t, srcDB.SetNote("agent", "longterm", "", "important fact"))

	// Export from source.
	exported, err := buildExportData(srcDB, "", true, true)
	require.NoError(t, err)

	// Import into destination.
	require.NoError(t, importData(dstDB, exported))

	// Verify.
	msgs, err := dstDB.GetMessages("rt-session")
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "round trip", msgs[0].Content)
	assert.Equal(t, "confirmed", msgs[1].Content)

	note := dstDB.GetNote("agent", "longterm", "")
	assert.Equal(t, "important fact", note)
}

func TestNewDataCommand(t *testing.T) {
	cmd := NewDataCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "data", cmd.Use)
	assert.Equal(t, "Export and import session data", cmd.Short)

	assert.True(t, cmd.HasSubCommands())

	allowedCommands := []string{"export", "import"}
	subcommands := cmd.Commands()
	assert.Len(t, subcommands, len(allowedCommands))

	for _, subcmd := range subcommands {
		found := slices.Contains(allowedCommands, subcmd.Name())
		assert.True(t, found, "unexpected subcommand %q", subcmd.Name())
		assert.False(t, subcmd.Hidden)
		assert.Nil(t, subcmd.Run)
		assert.NotNil(t, subcmd.RunE)
	}
}
