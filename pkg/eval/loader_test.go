package eval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSuite_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "basic.json")

	data := `{
		"name": "basic-suite",
		"description": "A simple test suite",
		"agent_id": "default",
		"model": "gpt-4o",
		"cases": [
			{
				"name": "greeting",
				"input": "Hello",
				"expected_output": "Hi",
				"tags": ["basic", "greeting"]
			},
			{
				"name": "math",
				"input": "What is 2+2?",
				"expect_contains": ["4"]
			}
		]
	}`

	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))

	suite, err := LoadSuite(path)
	require.NoError(t, err)

	assert.Equal(t, "basic-suite", suite.Name)
	assert.Equal(t, "A simple test suite", suite.Description)
	assert.Equal(t, "default", suite.AgentID)
	assert.Equal(t, "gpt-4o", suite.Model)
	assert.Len(t, suite.Cases, 2)
	assert.Equal(t, "greeting", suite.Cases[0].Name)
	assert.Equal(t, []string{"basic", "greeting"}, suite.Cases[0].Tags)
}

func TestLoadSuite_MissingName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-name.json")

	data := `{
		"cases": [{"name": "test", "input": "hello"}]
	}`

	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))

	_, err := LoadSuite(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field \"name\"")
}

func TestLoadSuite_NoCases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")

	data := `{
		"name": "empty-suite",
		"cases": []
	}`

	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))

	_, err := LoadSuite(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no test cases")
}

func TestLoadSuite_CaseMissingInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-input.json")

	data := `{
		"name": "bad-suite",
		"cases": [{"name": "test-no-input"}]
	}`

	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))

	_, err := LoadSuite(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field \"input\"")
}

func TestLoadSuite_CaseMissingName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-case-name.json")

	data := `{
		"name": "bad-suite",
		"cases": [{"input": "hello"}]
	}`

	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))

	_, err := LoadSuite(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "case 0 missing required field \"name\"")
}

func TestLoadSuite_FileNotFound(t *testing.T) {
	_, err := LoadSuite("/nonexistent/path.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read suite file")
}

func TestLoadSuite_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")

	require.NoError(t, os.WriteFile(path, []byte("{invalid json}"), 0o644))

	_, err := LoadSuite(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse suite")
}

func TestLoadSuitesFromDir(t *testing.T) {
	dir := t.TempDir()

	suite1 := `{"name": "suite-a", "cases": [{"name": "a1", "input": "hello"}]}`
	suite2 := `{"name": "suite-b", "cases": [{"name": "b1", "input": "world"}]}`

	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"), []byte(suite1), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.json"), []byte(suite2), 0o644))

	// Non-JSON file should be ignored.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0o644))

	suites, err := LoadSuitesFromDir(dir)
	require.NoError(t, err)
	assert.Len(t, suites, 2)

	names := make(map[string]bool)
	for _, s := range suites {
		names[s.Name] = true
	}

	assert.True(t, names["suite-a"])
	assert.True(t, names["suite-b"])
}

func TestLoadSuitesFromDir_Empty(t *testing.T) {
	dir := t.TempDir()

	suites, err := LoadSuitesFromDir(dir)
	require.NoError(t, err)
	assert.Empty(t, suites)
}

func TestLoadSuitesFromDir_InvalidFile(t *testing.T) {
	dir := t.TempDir()

	// One valid, one invalid.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"),
		[]byte(`{"name": "ok", "cases": [{"name": "t", "input": "x"}]}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.json"),
		[]byte(`{invalid}`), 0o644))

	_, err := LoadSuitesFromDir(dir)
	require.Error(t, err)
}

func TestLoadSuitesFromDir_NotFound(t *testing.T) {
	_, err := LoadSuitesFromDir("/nonexistent/dir")
	require.Error(t, err)
}

func TestLoadSuitesFromDir_SkipsSubdirectories(t *testing.T) {
	dir := t.TempDir()

	// Create a subdirectory that should be skipped.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "subdir", "nested.json"),
		[]byte(`{"name": "nested", "cases": [{"name": "n1", "input": "x"}]}`), 0o644))

	// One valid suite in the top level.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "top.json"),
		[]byte(`{"name": "top", "cases": [{"name": "t1", "input": "y"}]}`), 0o644))

	suites, err := LoadSuitesFromDir(dir)
	require.NoError(t, err)
	assert.Len(t, suites, 1)
	assert.Equal(t, "top", suites[0].Name)
}
