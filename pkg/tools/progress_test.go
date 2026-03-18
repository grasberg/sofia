package tools

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProgressReporter_Lifecycle(t *testing.T) {
	var updates []ProgressUpdate
	var mu sync.Mutex

	reporter := NewProgressReporter("read_file", func(u ProgressUpdate) {
		mu.Lock()
		updates = append(updates, u)
		mu.Unlock()
	})

	reporter.Start("Reading file contents")
	reporter.Update("Processing line 50 of 100", 0.5)
	reporter.Update("Processing line 80 of 100", 0.8)
	reporter.Complete("File read successfully")

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, updates, 4)

	// Started
	assert.Equal(t, "read_file", updates[0].ToolName)
	assert.Equal(t, "started", updates[0].Status)
	assert.Equal(t, "Reading file contents", updates[0].Message)
	assert.Equal(t, float64(0), updates[0].Progress)
	assert.Equal(t, int64(0), updates[0].Elapsed)

	// First running update
	assert.Equal(t, "running", updates[1].Status)
	assert.Equal(t, "Processing line 50 of 100", updates[1].Message)
	assert.Equal(t, 0.5, updates[1].Progress)
	assert.GreaterOrEqual(t, updates[1].Elapsed, int64(0))

	// Second running update
	assert.Equal(t, "running", updates[2].Status)
	assert.Equal(t, 0.8, updates[2].Progress)

	// Completed
	assert.Equal(t, "completed", updates[3].Status)
	assert.Equal(t, "File read successfully", updates[3].Message)
	assert.Equal(t, 1.0, updates[3].Progress)
	assert.GreaterOrEqual(t, updates[3].Elapsed, int64(0))
}

func TestProgressReporter_Fail(t *testing.T) {
	var updates []ProgressUpdate
	var mu sync.Mutex

	reporter := NewProgressReporter("exec", func(u ProgressUpdate) {
		mu.Lock()
		updates = append(updates, u)
		mu.Unlock()
	})

	reporter.Start("Executing command")
	reporter.Fail("Command exited with code 1")

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, updates, 2)

	// Started
	assert.Equal(t, "started", updates[0].Status)
	assert.Equal(t, "Executing command", updates[0].Message)

	// Failed
	assert.Equal(t, "exec", updates[1].ToolName)
	assert.Equal(t, "failed", updates[1].Status)
	assert.Equal(t, "Command exited with code 1", updates[1].Message)
	assert.Equal(t, float64(-1), updates[1].Progress)
	assert.GreaterOrEqual(t, updates[1].Elapsed, int64(0))
}

func TestProgressReporter_CallbackInvoked(t *testing.T) {
	callCount := 0
	var receivedToolName string
	var receivedStatuses []string

	reporter := NewProgressReporter("write_file", func(u ProgressUpdate) {
		callCount++
		receivedToolName = u.ToolName
		receivedStatuses = append(receivedStatuses, u.Status)
	})

	reporter.Start("Writing data")
	reporter.Update("50% written", 0.5)
	reporter.Complete("Write finished")

	assert.Equal(t, 3, callCount)
	assert.Equal(t, "write_file", receivedToolName)
	assert.Equal(t, []string{"started", "running", "completed"}, receivedStatuses)
}

func TestProgressReporter_NilCallback(t *testing.T) {
	// Ensure nil callback does not panic.
	reporter := NewProgressReporter("safe_tool", nil)

	assert.NotPanics(t, func() {
		reporter.Start("Starting")
		reporter.Update("Running", 0.5)
		reporter.Complete("Done")
	})
}

func TestProgressReporter_NilCallbackFail(t *testing.T) {
	reporter := NewProgressReporter("safe_tool", nil)

	assert.NotPanics(t, func() {
		reporter.Start("Starting")
		reporter.Fail("Error occurred")
	})
}

func TestProgressReporter_IndeterminateProgress(t *testing.T) {
	var updates []ProgressUpdate

	reporter := NewProgressReporter("web_fetch", func(u ProgressUpdate) {
		updates = append(updates, u)
	})

	reporter.Start("Fetching URL")
	reporter.Update("Waiting for response", -1)
	reporter.Complete("Response received")

	require.Len(t, updates, 3)
	assert.Equal(t, float64(-1), updates[1].Progress)
}
