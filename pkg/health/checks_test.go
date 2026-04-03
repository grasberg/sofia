package health

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockDB struct {
	err error
}

func (m *mockDB) Ping() error { return m.err }

func TestDatabaseCheck_Healthy(t *testing.T) {
	check := DatabaseCheck(&mockDB{})
	ok, msg := check()
	assert.True(t, ok)
	assert.Equal(t, "ok", msg)
}

func TestDatabaseCheck_Unhealthy(t *testing.T) {
	check := DatabaseCheck(&mockDB{err: errors.New("connection closed")})
	ok, msg := check()
	assert.False(t, ok)
	assert.Contains(t, msg, "connection closed")
}

func TestDatabaseCheck_NilDB(t *testing.T) {
	check := DatabaseCheck(nil)
	ok, msg := check()
	assert.False(t, ok)
	assert.Contains(t, msg, "not initialized")
}

func TestDiskSpaceCheck_OK(t *testing.T) {
	// Use "/" which always exists and should have some free space.
	check := DiskSpaceCheck("/", 1) // 1 byte threshold — always passes
	ok, msg := check()
	assert.True(t, ok)
	assert.Contains(t, msg, "MB free")
}

func TestDiskSpaceCheck_NonExistentDir(t *testing.T) {
	// Should walk up to an existing parent directory.
	check := DiskSpaceCheck("/nonexistent/path/that/does/not/exist", 1)
	ok, msg := check()
	assert.True(t, ok)
	assert.Contains(t, msg, "MB free")
}
