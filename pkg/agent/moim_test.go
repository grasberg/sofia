package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadMOIM_EnvText(t *testing.T) {
	t.Setenv("SOFIA_MOIM_TEXT", "Always use Go for new code")
	t.Setenv("SOFIA_MOIM_FILE", "")

	result := LoadMOIM("")
	assert.Contains(t, result, "Always use Go for new code")
}

func TestLoadMOIM_EnvFile(t *testing.T) {
	dir := t.TempDir()
	moimFile := filepath.Join(dir, "moim.txt")
	require.NoError(t, os.WriteFile(moimFile, []byte("Prefer functional style"), 0o644))

	t.Setenv("SOFIA_MOIM_TEXT", "")
	t.Setenv("SOFIA_MOIM_FILE", moimFile)

	result := LoadMOIM("")
	assert.Contains(t, result, "Prefer functional style")
}

func TestLoadMOIM_WorkspaceFile(t *testing.T) {
	dir := t.TempDir()
	moimFile := filepath.Join(dir, ".moim")
	require.NoError(t, os.WriteFile(moimFile, []byte("This project uses React"), 0o644))

	t.Setenv("SOFIA_MOIM_TEXT", "")
	t.Setenv("SOFIA_MOIM_FILE", "")

	result := LoadMOIM(dir)
	assert.Contains(t, result, "This project uses React")
}

func TestLoadMOIM_Empty(t *testing.T) {
	t.Setenv("SOFIA_MOIM_TEXT", "")
	t.Setenv("SOFIA_MOIM_FILE", "")

	result := LoadMOIM("/nonexistent/path")
	assert.Equal(t, "", result)
}

func TestLoadMOIM_CombinesMultipleSources(t *testing.T) {
	dir := t.TempDir()
	moimFile := filepath.Join(dir, ".moim")
	require.NoError(t, os.WriteFile(moimFile, []byte("workspace rule"), 0o644))

	t.Setenv("SOFIA_MOIM_TEXT", "env rule")
	t.Setenv("SOFIA_MOIM_FILE", "")

	result := LoadMOIM(dir)
	assert.Contains(t, result, "env rule")
	assert.Contains(t, result, "workspace rule")
}
