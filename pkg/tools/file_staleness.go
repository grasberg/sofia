package tools

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// FileStalenessTracker records file modification times when files are read,
// and warns when a write targets a file that has changed since it was last read.
type FileStalenessTracker struct {
	mu     sync.RWMutex
	mtimes map[string]time.Time // path -> mtime at last read
}

// NewFileStalenessTracker creates a new tracker.
func NewFileStalenessTracker() *FileStalenessTracker {
	return &FileStalenessTracker{
		mtimes: make(map[string]time.Time),
	}
}

// RecordRead records the current mtime of a file after a successful read.
func (t *FileStalenessTracker) RecordRead(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	t.mu.Lock()
	// Cap at 500 entries to prevent unbounded growth in long sessions
	if len(t.mtimes) >= 500 {
		// Evict oldest entry (arbitrary — just prevent unbounded growth)
		for k := range t.mtimes {
			delete(t.mtimes, k)
			break
		}
	}
	t.mtimes[path] = info.ModTime()
	t.mu.Unlock()
}

// CheckBeforeWrite returns a warning string if the file has been modified
// since it was last read. Returns empty string if safe to write.
func (t *FileStalenessTracker) CheckBeforeWrite(path string) string {
	t.mu.RLock()
	lastRead, tracked := t.mtimes[path]
	t.mu.RUnlock()

	if !tracked {
		return "" // Never read — no staleness to check
	}

	info, err := os.Stat(path)
	if err != nil {
		return "" // File doesn't exist anymore — write will create it
	}

	if info.ModTime().After(lastRead) {
		return fmt.Sprintf(
			"[WARNING: %s was modified since you last read it (read at %s, now %s). "+
				"Re-read the file to see the latest version before writing.]",
			path,
			lastRead.Format("15:04:05"),
			info.ModTime().Format("15:04:05"),
		)
	}
	return ""
}

// UpdateAfterWrite updates the recorded mtime after a successful write.
func (t *FileStalenessTracker) UpdateAfterWrite(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	t.mu.Lock()
	t.mtimes[path] = info.ModTime()
	t.mu.Unlock()
}
