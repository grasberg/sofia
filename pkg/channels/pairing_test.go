package channels

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPairingManager_GenerateAndApprove(t *testing.T) {
	dir := t.TempDir()
	pm := NewPairingManager(dir)

	code := pm.GenerateCode("telegram", "user123")
	require.NotEmpty(t, code)
	assert.Len(t, code, 6)

	ch, sender, err := pm.Approve(code)
	require.NoError(t, err)
	assert.Equal(t, "telegram", ch)
	assert.Equal(t, "user123", sender)

	assert.True(t, pm.IsApproved("telegram", "user123"))
	assert.False(t, pm.IsApproved("telegram", "other"))
	assert.False(t, pm.IsApproved("discord", "user123"))
}

func TestPairingManager_ExpiredCode(t *testing.T) {
	dir := t.TempDir()
	pm := NewPairingManager(dir)

	code := pm.GenerateCode("telegram", "user456")
	require.NotEmpty(t, code)

	// Manually set expiry to the past
	pm.mu.Lock()
	pm.pendingCodes[code].Expires = time.Now().Add(-1 * time.Minute)
	pm.mu.Unlock()

	_, _, err := pm.Approve(code)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")

	assert.False(t, pm.IsApproved("telegram", "user456"))
}

func TestPairingManager_InvalidCode(t *testing.T) {
	dir := t.TempDir()
	pm := NewPairingManager(dir)

	_, _, err := pm.Approve("badcode")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestPairingManager_ListPending(t *testing.T) {
	dir := t.TempDir()
	pm := NewPairingManager(dir)

	code1 := pm.GenerateCode("telegram", "userA")
	code2 := pm.GenerateCode("discord", "userB")
	require.NotEmpty(t, code1)
	require.NotEmpty(t, code2)

	pending := pm.ListPending()
	assert.Len(t, pending, 2)

	// Verify both codes are present
	codes := make(map[string]bool)
	for _, p := range pending {
		codes[p.Code] = true
	}
	assert.True(t, codes[code1])
	assert.True(t, codes[code2])
}

func TestPairingManager_Persistence(t *testing.T) {
	dir := t.TempDir()
	pm := NewPairingManager(dir)

	code := pm.GenerateCode("telegram", "persist_user")
	_, _, err := pm.Approve(code)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(filepath.Join(dir, "pairing.json"))
	require.NoError(t, err)

	// Create a new manager from the same path and verify state was loaded
	pm2 := NewPairingManager(dir)
	assert.True(t, pm2.IsApproved("telegram", "persist_user"))
	assert.False(t, pm2.IsApproved("telegram", "other_user"))
}
