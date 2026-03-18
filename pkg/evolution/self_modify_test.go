package evolution

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeModifier_IsImmutable(t *testing.T) {
	sm := NewSafeModifier(t.TempDir(), nil, nil)

	// Default immutable paths must be blocked.
	assert.True(t, sm.IsImmutable("config.json"))
	assert.True(t, sm.IsImmutable("/home/user/.sofia/config.json"))
	assert.True(t, sm.IsImmutable("config.yaml"))
	assert.True(t, sm.IsImmutable(".env"))
	assert.True(t, sm.IsImmutable("pkg/agent/loop.go"))
	assert.True(t, sm.IsImmutable("pkg/evolution/engine.go"))
	assert.True(t, sm.IsImmutable("/some/path/evolution/foo.go"))

	// Non-immutable paths must be allowed.
	assert.False(t, sm.IsImmutable("workspace/AGENT.md"))
	assert.False(t, sm.IsImmutable("workspace/skills/test/SKILL.md"))
}

func TestSafeModifier_IsImmutable_ExtraPaths(t *testing.T) {
	sm := NewSafeModifier(t.TempDir(), []string{"secrets/"}, nil)

	assert.True(t, sm.IsImmutable("secrets/api_key.txt"))
	assert.False(t, sm.IsImmutable("workspace/notes.md"))
}

func TestSafeModifier_VersionFile(t *testing.T) {
	historyDir := filepath.Join(t.TempDir(), "history")
	sm := NewSafeModifier(historyDir, nil, nil)

	// Create a temp file to version.
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "test.md")
	original := "original content"
	require.NoError(t, os.WriteFile(srcFile, []byte(original), 0o644))

	backupPath, err := sm.VersionFile(srcFile)
	require.NoError(t, err)
	assert.Contains(t, backupPath, "test.md.")
	assert.True(t, strings.HasSuffix(backupPath, ".bak"))

	// Verify backup content matches original.
	backupData, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	assert.Equal(t, original, string(backupData))

	// Verify the history directory was created.
	info, err := os.Stat(historyDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestSafeModifier_ModifyFile(t *testing.T) {
	historyDir := filepath.Join(t.TempDir(), "history")
	sm := NewSafeModifier(historyDir, nil, nil) // nil provider skips safety check

	// Create an original file.
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "agent.md")
	original := "original agent prompt"
	require.NoError(t, os.WriteFile(srcFile, []byte(original), 0o644))

	// Modify it.
	newContent := "updated agent prompt"
	err := sm.ModifyFile(context.Background(), srcFile, newContent)
	require.NoError(t, err)

	// Verify the file has new content.
	data, err := os.ReadFile(srcFile)
	require.NoError(t, err)
	assert.Equal(t, newContent, string(data))

	// Verify a backup was created with the original content.
	backups, err := sm.ListBackups(srcFile)
	require.NoError(t, err)
	require.Len(t, backups, 1)

	backupData, err := os.ReadFile(backups[0])
	require.NoError(t, err)
	assert.Equal(t, original, string(backupData))
}

func TestSafeModifier_ModifyFile_Immutable(t *testing.T) {
	sm := NewSafeModifier(t.TempDir(), nil, nil)

	err := sm.ModifyFile(context.Background(), "config.json", "bad content")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "immutable")
}

func TestSafeModifier_ModifyFile_NewFile(t *testing.T) {
	historyDir := filepath.Join(t.TempDir(), "history")
	sm := NewSafeModifier(historyDir, nil, nil)

	// Modify a file that does not exist yet (no backup expected).
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "new_file.md")
	err := sm.ModifyFile(context.Background(), srcFile, "brand new")
	require.NoError(t, err)

	data, err := os.ReadFile(srcFile)
	require.NoError(t, err)
	assert.Equal(t, "brand new", string(data))

	// No backup should exist for a new file.
	backups, err := sm.ListBackups(srcFile)
	require.NoError(t, err)
	assert.Empty(t, backups)
}

func TestSafeModifier_RevertFile(t *testing.T) {
	historyDir := filepath.Join(t.TempDir(), "history")
	sm := NewSafeModifier(historyDir, nil, nil)

	// Create original, modify, then revert.
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "revert_me.md")
	original := "original content here"
	require.NoError(t, os.WriteFile(srcFile, []byte(original), 0o644))

	// Modify.
	err := sm.ModifyFile(context.Background(), srcFile, "modified content")
	require.NoError(t, err)

	// Verify modification took effect.
	data, err := os.ReadFile(srcFile)
	require.NoError(t, err)
	assert.Equal(t, "modified content", string(data))

	// Get backup path and revert.
	backups, err := sm.ListBackups(srcFile)
	require.NoError(t, err)
	require.Len(t, backups, 1)

	err = sm.RevertFile(srcFile, backups[0])
	require.NoError(t, err)

	// Verify revert.
	data, err = os.ReadFile(srcFile)
	require.NoError(t, err)
	assert.Equal(t, original, string(data))
}

func TestSafeModifier_ListBackups(t *testing.T) {
	historyDir := filepath.Join(t.TempDir(), "history")
	require.NoError(t, os.MkdirAll(historyDir, 0o755))

	sm := NewSafeModifier(historyDir, nil, nil)

	// Create multiple backups with distinct timestamps.
	baseName := "prompt.md"
	timestamps := []int64{1000000, 2000000, 3000000}
	for _, ts := range timestamps {
		name := filepath.Join(historyDir, fmt.Sprintf("%s.%d.bak", baseName, ts))
		require.NoError(t, os.WriteFile(name, []byte("backup"), 0o644))
	}

	backups, err := sm.ListBackups("/any/path/" + baseName)
	require.NoError(t, err)
	require.Len(t, backups, 3)

	// Verify descending order (newest first).
	assert.Contains(t, backups[0], "3000000")
	assert.Contains(t, backups[1], "2000000")
	assert.Contains(t, backups[2], "1000000")
}

func TestSafeModifier_ListBackups_Empty(t *testing.T) {
	historyDir := filepath.Join(t.TempDir(), "history")
	require.NoError(t, os.MkdirAll(historyDir, 0o755))

	sm := NewSafeModifier(historyDir, nil, nil)

	backups, err := sm.ListBackups("/some/nonexistent_file.md")
	require.NoError(t, err)
	assert.Empty(t, backups)
}

// Ensure VersionFile uses real timestamps (sleep briefly to distinguish).
func TestSafeModifier_VersionFile_UniqueTimestamps(t *testing.T) {
	historyDir := filepath.Join(t.TempDir(), "history")
	sm := NewSafeModifier(historyDir, nil, nil)

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "ts_test.md")
	require.NoError(t, os.WriteFile(srcFile, []byte("v1"), 0o644))

	backup1, err := sm.VersionFile(srcFile)
	require.NoError(t, err)

	// Wait to get a different unix timestamp.
	time.Sleep(1100 * time.Millisecond)

	require.NoError(t, os.WriteFile(srcFile, []byte("v2"), 0o644))
	backup2, err := sm.VersionFile(srcFile)
	require.NoError(t, err)

	assert.NotEqual(t, backup1, backup2)
}
