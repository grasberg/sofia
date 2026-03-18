package tools

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
)

// ToolStats holds usage and performance statistics for a single tool.
type ToolStats struct {
	Name         string        `json:"name"`
	UsageCount   int           `json:"usage_count"`
	SuccessCount int           `json:"success_count"`
	ErrorCount   int           `json:"error_count"`
	TotalTimeMs  int64         `json:"total_time_ms"`
	AverageTime  time.Duration `json:"-"`
	SuccessRate  float64       `json:"-"`
}

func (ts *ToolStats) CalculateDerived() {
	if ts.UsageCount > 0 {
		ts.AverageTime = time.Duration(ts.TotalTimeMs/int64(ts.UsageCount)) * time.Millisecond
		ts.SuccessRate = float64(ts.SuccessCount) / float64(ts.UsageCount)
	}
}

// ToolTracker tracks execution metrics across all tools.
type ToolTracker struct {
	stats      map[string]*ToolStats
	mu         sync.RWMutex
	path       string
	dirtyCount int
	lastSave   time.Time
}

// NewToolTracker creates a new tracker, loading from persistence if available.
func NewToolTracker(path string) *ToolTracker {
	tracker := &ToolTracker{
		stats: make(map[string]*ToolStats),
		path:  path,
	}
	tracker.Load()
	return tracker
}

// Record records an execution for a tool.
func (t *ToolTracker) Record(name string, duration time.Duration, isError bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	stat, exists := t.stats[name]
	if !exists {
		stat = &ToolStats{Name: name}
		t.stats[name] = stat
	}

	stat.UsageCount++
	stat.TotalTimeMs += duration.Milliseconds()
	if isError {
		stat.ErrorCount++
	} else {
		stat.SuccessCount++
	}

	// Batch disk writes: save every 50 calls or 30 seconds
	t.dirtyCount++
	if t.dirtyCount >= 50 || time.Since(t.lastSave) > 30*time.Second {
		t.saveNoLock()
		t.dirtyCount = 0
		t.lastSave = time.Now()
	}
}

// Flush forces an immediate save of pending stats to disk.
func (t *ToolTracker) Flush() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dirtyCount > 0 {
		t.saveNoLock()
		t.dirtyCount = 0
		t.lastSave = time.Now()
	}
}

// GetStats returns copies of all current tool stats.
func (t *ToolTracker) GetStats() map[string]ToolStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]ToolStats, len(t.stats))
	for name, stat := range t.stats {
		stat.CalculateDerived()
		result[name] = *stat
	}
	return result
}

// GetStat returns statistics for a specific tool.
func (t *ToolTracker) GetStat(name string) (ToolStats, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stat, ok := t.stats[name]
	if !ok {
		return ToolStats{}, false
	}
	stat.CalculateDerived()
	return *stat, true
}

func (t *ToolTracker) saveNoLock() {
	if t.path == "" {
		return
	}

	data, err := json.MarshalIndent(t.stats, "", "  ")
	if err != nil {
		logger.ErrorCF("tool:tracker", "Failed to marshal tool stats", map[string]any{"error": err.Error()})
		return
	}

	err = os.WriteFile(t.path, data, 0644)
	if err != nil {
		logger.ErrorCF("tool:tracker", "Failed to save tool stats", map[string]any{"error": err.Error(), "path": t.path})
	}
}

// Load loads tool stats from disk.
func (t *ToolTracker) Load() {
	if t.path == "" {
		return
	}

	data, err := os.ReadFile(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return // Normal on first run
		}
		logger.ErrorCF("tool:tracker", "Failed to load tool stats", map[string]any{"error": err.Error(), "path": t.path})
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if err := json.Unmarshal(data, &t.stats); err != nil {
		logger.ErrorCF("tool:tracker", "Failed to unmarshal tool stats", map[string]any{"error": err.Error()})
	}
}
