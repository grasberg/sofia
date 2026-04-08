package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
)

func TestForkSession(t *testing.T) {
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	sm := NewSessionManager(db, "test-agent")

	// Create source session
	sm.AddMessage("source", "user", "Hello")
	sm.AddMessage("source", "assistant", "Hi there!")
	sm.AddMessage("source", "user", "Do X")

	// Fork
	forkKey, err := sm.ForkSession("source")
	require.NoError(t, err)
	assert.Contains(t, forkKey, "source-fork-")

	// Verify fork has same messages
	forkHistory := sm.GetHistory(forkKey)
	assert.Len(t, forkHistory, 3)
	assert.Equal(t, "Hello", forkHistory[0].Content)
	assert.Equal(t, "Hi there!", forkHistory[1].Content)

	// Verify independence
	sm.AddFullMessage(forkKey, providers.Message{Role: "assistant", Content: "Fork response"})
	assert.Len(t, sm.GetHistory(forkKey), 4)
	assert.Len(t, sm.GetHistory("source"), 3) // unchanged
}

func TestForkSessionAt(t *testing.T) {
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	sm := NewSessionManager(db, "test-agent")

	sm.AddMessage("source", "user", "msg1")
	sm.AddMessage("source", "assistant", "msg2")
	sm.AddMessage("source", "user", "msg3")
	sm.AddMessage("source", "assistant", "msg4")

	forkKey, err := sm.ForkSessionAt("source", 2)
	require.NoError(t, err)

	forkHistory := sm.GetHistory(forkKey)
	assert.Len(t, forkHistory, 2)
	assert.Equal(t, "msg1", forkHistory[0].Content)
}

func TestForkSession_EmptySource(t *testing.T) {
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	defer db.Close()

	sm := NewSessionManager(db, "test-agent")

	_, err = sm.ForkSession("nonexistent")
	assert.Error(t, err)
}
