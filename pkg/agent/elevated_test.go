package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestElevatedManager_ElevateAndCheck(t *testing.T) {
	em := NewElevatedManager()
	em.Elevate("session-1", "user-1", "telegram", 10*time.Minute)

	assert.True(t, em.IsElevated("session-1"))
}

func TestElevatedManager_Revoke(t *testing.T) {
	em := NewElevatedManager()
	em.Elevate("session-1", "user-1", "telegram", 10*time.Minute)
	assert.True(t, em.IsElevated("session-1"))

	em.Revoke("session-1")
	assert.False(t, em.IsElevated("session-1"))
}

func TestElevatedManager_AutoExpire(t *testing.T) {
	em := NewElevatedManager()
	em.Elevate("session-1", "user-1", "cli", 1*time.Millisecond)

	time.Sleep(5 * time.Millisecond)
	assert.False(t, em.IsElevated("session-1"))
}

func TestElevatedManager_NotElevated(t *testing.T) {
	em := NewElevatedManager()
	assert.False(t, em.IsElevated("unknown-session"))
}

func TestElevatedManager_GetState(t *testing.T) {
	em := NewElevatedManager()
	em.Elevate("session-1", "user-42", "discord", 30*time.Minute)

	state := em.GetState("session-1")
	require.NotNil(t, state)
	assert.True(t, state.Active)
	assert.Equal(t, "user-42", state.GrantedBy)
	assert.Equal(t, "discord", state.Channel)
	assert.False(t, state.ExpiresAt.IsZero())

	// Unknown session returns nil
	assert.Nil(t, em.GetState("no-such-session"))
}
