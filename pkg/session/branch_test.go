package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBranchManager_Branch(t *testing.T) {
	db := testDB(t)
	sm := NewSessionManager(db, "agent1")
	bm := NewBranchManager()

	// Seed parent session with messages.
	sm.AddMessage("parent", "user", "hello")
	sm.AddMessage("parent", "assistant", "hi there")
	sm.AddMessage("parent", "user", "how are you?")

	info, err := bm.Branch(sm, "parent", "test-branch")
	require.NoError(t, err)

	assert.Equal(t, "parent", info.ParentKey)
	assert.True(t, strings.HasPrefix(info.BranchKey, "parent:branch:"))
	assert.Equal(t, "test-branch", info.Label)
	assert.Equal(t, 3, info.MessageCount)
	assert.False(t, info.BranchedAt.IsZero())

	// Verify messages were copied to the branch session.
	branchHistory := sm.GetHistory(info.BranchKey)
	require.Len(t, branchHistory, 3)
	assert.Equal(t, "hello", branchHistory[0].Content)
	assert.Equal(t, "hi there", branchHistory[1].Content)
	assert.Equal(t, "how are you?", branchHistory[2].Content)

	// Verify parent session is unchanged.
	parentHistory := sm.GetHistory("parent")
	require.Len(t, parentHistory, 3)
}

func TestBranchManager_BranchCopiesSummary(t *testing.T) {
	db := testDB(t)
	sm := NewSessionManager(db, "agent1")
	bm := NewBranchManager()

	sm.AddMessage("parent", "user", "hello")
	sm.SetSummary("parent", "conversation about greetings")

	info, err := bm.Branch(sm, "parent", "")
	require.NoError(t, err)

	assert.Equal(t, "conversation about greetings", sm.GetSummary(info.BranchKey))
}

func TestBranchManager_ListBranches(t *testing.T) {
	db := testDB(t)
	sm := NewSessionManager(db, "agent1")
	bm := NewBranchManager()

	sm.AddMessage("parent", "user", "msg1")

	info1, err := bm.Branch(sm, "parent", "branch-one")
	require.NoError(t, err)

	info2, err := bm.Branch(sm, "parent", "branch-two")
	require.NoError(t, err)

	branches := bm.ListBranches("parent")
	require.Len(t, branches, 2)
	assert.Equal(t, info1.BranchKey, branches[0].BranchKey)
	assert.Equal(t, "branch-one", branches[0].Label)
	assert.Equal(t, info2.BranchKey, branches[1].BranchKey)
	assert.Equal(t, "branch-two", branches[1].Label)

	// Listing branches for a key with no branches returns empty slice.
	none := bm.ListBranches("nonexistent")
	assert.Empty(t, none)
}

func TestBranchManager_GetParent(t *testing.T) {
	db := testDB(t)
	sm := NewSessionManager(db, "agent1")
	bm := NewBranchManager()

	sm.AddMessage("parent", "user", "msg")

	info, err := bm.Branch(sm, "parent", "child")
	require.NoError(t, err)

	parent, ok := bm.GetParent(info.BranchKey)
	assert.True(t, ok)
	assert.Equal(t, "parent", parent)

	// Unknown branch key returns false.
	_, ok = bm.GetParent("unknown:key")
	assert.False(t, ok)
}

func TestBranchManager_BranchEmptySession(t *testing.T) {
	db := testDB(t)
	sm := NewSessionManager(db, "agent1")
	bm := NewBranchManager()

	// Branch from a session with zero messages.
	info, err := bm.Branch(sm, "empty-session", "empty")
	require.NoError(t, err)

	assert.Equal(t, 0, info.MessageCount)
	assert.True(t, strings.HasPrefix(info.BranchKey, "empty-session:branch:"))

	branchHistory := sm.GetHistory(info.BranchKey)
	assert.Empty(t, branchHistory)

	// Listing branches still works.
	branches := bm.ListBranches("empty-session")
	require.Len(t, branches, 1)
	assert.Equal(t, info.BranchKey, branches[0].BranchKey)
}
