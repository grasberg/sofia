package tools

import (
	"sync"
	"time"
)

// ProgressUpdate represents a status update during tool execution.
type ProgressUpdate struct {
	ToolName  string    `json:"tool_name"`
	Status    string    `json:"status"`     // "started", "running", "completed", "failed"
	Message   string    `json:"message"`    // human-readable progress
	Progress  float64   `json:"progress"`   // 0.0 to 1.0, -1 for indeterminate
	Elapsed   int64     `json:"elapsed_ms"` // milliseconds since start
	StartedAt time.Time `json:"-"`
}

// ProgressReporter provides progress updates during tool execution.
// It is safe for concurrent use.
type ProgressReporter struct {
	mu        sync.Mutex
	toolName  string
	callback  func(ProgressUpdate)
	startedAt time.Time
}

// NewProgressReporter creates a new ProgressReporter for the given tool.
// The callback is invoked on every status change (start, update, complete, fail).
func NewProgressReporter(toolName string, callback func(ProgressUpdate)) *ProgressReporter {
	return &ProgressReporter{
		toolName: toolName,
		callback: callback,
	}
}

// Start marks the tool execution as started and emits a "started" update.
func (pr *ProgressReporter) Start(message string) {
	pr.mu.Lock()
	pr.startedAt = time.Now()
	pr.mu.Unlock()

	pr.emit(ProgressUpdate{
		ToolName:  pr.toolName,
		Status:    "started",
		Message:   message,
		Progress:  0,
		Elapsed:   0,
		StartedAt: pr.startedAt,
	})
}

// Update emits a "running" update with the given message and progress value.
// Progress should be between 0.0 and 1.0, or -1 for indeterminate.
func (pr *ProgressReporter) Update(message string, progress float64) {
	pr.mu.Lock()
	elapsed := time.Since(pr.startedAt).Milliseconds()
	startedAt := pr.startedAt
	pr.mu.Unlock()

	pr.emit(ProgressUpdate{
		ToolName:  pr.toolName,
		Status:    "running",
		Message:   message,
		Progress:  progress,
		Elapsed:   elapsed,
		StartedAt: startedAt,
	})
}

// Complete marks the tool execution as completed and emits a "completed" update.
func (pr *ProgressReporter) Complete(message string) {
	pr.mu.Lock()
	elapsed := time.Since(pr.startedAt).Milliseconds()
	startedAt := pr.startedAt
	pr.mu.Unlock()

	pr.emit(ProgressUpdate{
		ToolName:  pr.toolName,
		Status:    "completed",
		Message:   message,
		Progress:  1.0,
		Elapsed:   elapsed,
		StartedAt: startedAt,
	})
}

// Fail marks the tool execution as failed and emits a "failed" update.
func (pr *ProgressReporter) Fail(message string) {
	pr.mu.Lock()
	elapsed := time.Since(pr.startedAt).Milliseconds()
	startedAt := pr.startedAt
	pr.mu.Unlock()

	pr.emit(ProgressUpdate{
		ToolName:  pr.toolName,
		Status:    "failed",
		Message:   message,
		Progress:  -1,
		Elapsed:   elapsed,
		StartedAt: startedAt,
	})
}

// emit invokes the callback if it is non-nil.
func (pr *ProgressReporter) emit(update ProgressUpdate) {
	if pr.callback != nil {
		pr.callback(update)
	}
}
