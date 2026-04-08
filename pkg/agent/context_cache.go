package agent

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
)

// BuildSystemPromptWithCache returns the cached system prompt if available
// and source files haven't changed, otherwise builds and caches it.
// Source file changes are detected via mtime checks (cheap stat calls).
func (cb *ContextBuilder) BuildSystemPromptWithCache() string {
	// Fast path under read lock: if cache exists and either TTL hasn't expired
	// or source files haven't changed, return cached prompt.
	cb.systemPromptMutex.RLock()
	if cb.cachedSystemPrompt != "" {
		ttl := cb.cacheTTL
		if (ttl > 0 && time.Since(cb.lastCacheCheck) < ttl) || !cb.sourceFilesChangedLocked() {
			result := cb.cachedSystemPrompt
			cb.systemPromptMutex.RUnlock()
			return result
		}
	}
	cb.systemPromptMutex.RUnlock()

	// Acquire write lock for building
	cb.systemPromptMutex.Lock()
	defer cb.systemPromptMutex.Unlock()

	// Double-check: another goroutine may have rebuilt while we waited
	if cb.cachedSystemPrompt != "" && !cb.sourceFilesChangedLocked() {
		cb.lastCacheCheck = time.Now()
		return cb.cachedSystemPrompt
	}

	// Snapshot the baseline (existence + max mtime) BEFORE building the prompt.
	// This way cachedAt reflects the pre-build state: if a file is modified
	// during BuildSystemPrompt, its new mtime will be > baseline.maxMtime,
	// so the next sourceFilesChangedLocked check will correctly trigger a
	// rebuild. The alternative (baseline after build) risks caching stale
	// content with a too-new baseline, making the staleness invisible.
	baseline := cb.buildCacheBaseline()
	prompt := cb.BuildSystemPrompt()
	cb.cachedSystemPrompt = prompt
	cb.cachedAt = baseline.maxMtime
	cb.existedAtCache = baseline.existed
	cb.lastCacheCheck = time.Now()

	logger.DebugCF("agent", "System prompt cached",
		map[string]any{
			"length": len(prompt),
		})

	return prompt
}

// InvalidateCache clears the cached system prompt.
// Normally not needed because the cache auto-invalidates via mtime checks,
// but this is useful for tests or explicit reload commands.
func (cb *ContextBuilder) InvalidateCache() {
	cb.systemPromptMutex.Lock()
	defer cb.systemPromptMutex.Unlock()

	cb.cachedSystemPrompt = ""
	cb.cachedAt = time.Time{}
	cb.existedAtCache = nil

	logger.DebugCF("agent", "System prompt cache invalidated", nil)
}

// sourcePaths returns the workspace source file paths tracked for cache
// invalidation (bootstrap files + memory). The skills directory is handled
// separately in sourceFilesChangedLocked because it requires both directory-
// level and recursive file-level mtime checks.
func (cb *ContextBuilder) sourcePaths() []string {
	return []string{
		filepath.Join(cb.workspace, "AGENTS.md"),
		filepath.Join(cb.workspace, "SOUL.md"),
		filepath.Join(cb.workspace, "USER.md"),
		filepath.Join(cb.workspace, "IDENTITY.md"),
		filepath.Join(cb.workspace, "SELF_OPTIMIZATION.md"),
	}
}

// cacheBaseline holds the file existence snapshot and the latest observed
// mtime across all tracked paths. Used as the cache reference point.
type cacheBaseline struct {
	existed  map[string]bool
	maxMtime time.Time
}

// buildCacheBaseline records which tracked paths currently exist and computes
// the latest mtime across all tracked files + skills directory contents.
// Called under write lock when the cache is built.
func (cb *ContextBuilder) buildCacheBaseline() cacheBaseline {
	skillsDir := filepath.Join(cb.workspace, "skills")
	agentsDir := filepath.Join(cb.workspace, "agents")

	// All paths whose existence we track: source files + skills/agents dirs.
	allPaths := append(cb.sourcePaths(), skillsDir, agentsDir)

	existed := make(map[string]bool, len(allPaths))
	var maxMtime time.Time

	for _, p := range allPaths {
		info, err := os.Stat(p)
		existed[p] = err == nil
		if err == nil && info.ModTime().After(maxMtime) {
			maxMtime = info.ModTime()
		}
	}

	// Walk skills files to capture their mtimes too.
	// Use os.Stat (not d.Info) to match the stat method used in
	// fileChangedSince / skillFilesModifiedSince for consistency.
	_ = filepath.WalkDir(skillsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr == nil && !d.IsDir() {
			if info, err := os.Stat(path); err == nil && info.ModTime().After(maxMtime) {
				maxMtime = info.ModTime()
			}
		}
		return nil
	})

	// If no tracked files exist yet (empty workspace), maxMtime is zero.
	// Use a very old non-zero time so that:
	// 1. cachedAt.IsZero() won't trigger perpetual rebuilds.
	// 2. Any real file created afterwards has mtime > cachedAt, so it
	//    will be detected by fileChangedSince (unlike time.Now() which
	//    could race with a file whose mtime <= Now).
	if maxMtime.IsZero() {
		maxMtime = time.Unix(1, 0)
	}

	return cacheBaseline{existed: existed, maxMtime: maxMtime}
}

// sourceFilesChangedLocked checks whether any workspace source file has been
// modified, created, or deleted since the cache was last built.
//
// IMPORTANT: The caller MUST hold at least a read lock on systemPromptMutex.
// Go's sync.RWMutex is not reentrant, so this function must NOT acquire the
// lock itself (it would deadlock when called from BuildSystemPromptWithCache
// which already holds RLock or Lock).
func (cb *ContextBuilder) sourceFilesChangedLocked() bool {
	if cb.cachedAt.IsZero() {
		return true
	}

	// Check tracked source files (bootstrap + memory).
	for _, p := range cb.sourcePaths() {
		if cb.fileChangedSince(p) {
			return true
		}
	}

	// --- Skills directory (handled separately from sourcePaths) ---
	//
	// 1. Creation/deletion: tracked via existedAtCache, same as bootstrap files.
	skillsDir := filepath.Join(cb.workspace, "skills")
	agentsDir := filepath.Join(cb.workspace, "agents")
	if cb.fileChangedSince(skillsDir) || cb.fileChangedSince(agentsDir) {
		return true
	}

	if skillFilesModifiedSince(skillsDir, cb.cachedAt) {
		return true
	}
	if skillFilesModifiedSince(agentsDir, cb.cachedAt) {
		return true
	}

	return false
}

// fileChangedSince returns true if a tracked source file has been modified,
// newly created, or deleted since the cache was built.
//
// Four cases:
//   - existed at cache time, exists now -> check mtime
//   - existed at cache time, gone now   -> changed (deleted)
//   - absent at cache time,  exists now -> changed (created)
//   - absent at cache time,  gone now   -> no change
func (cb *ContextBuilder) fileChangedSince(path string) bool {
	// Defensive: if existedAtCache was never initialized, treat as changed
	// so the cache rebuilds rather than silently serving stale data.
	if cb.existedAtCache == nil {
		return true
	}

	existedBefore := cb.existedAtCache[path]
	info, err := os.Stat(path)
	existsNow := err == nil

	if existedBefore != existsNow {
		return true // file was created or deleted
	}
	if !existsNow {
		return false // didn't exist before, doesn't exist now
	}
	return info.ModTime().After(cb.cachedAt)
}

// errWalkStop is a sentinel error used to stop filepath.WalkDir early.
// Using a dedicated error (instead of fs.SkipAll) makes the early-exit
// intent explicit and avoids the nilerr linter warning that would fire
// if the callback returned nil when its err parameter is non-nil.
var errWalkStop = errors.New("walk stop")

// skillFilesModifiedSince recursively walks the skills directory and checks
// whether any file was modified after t. This catches content-only edits at
// any nesting depth (e.g. skills/name/docs/extra.md) that don't update
// parent directory mtimes.
func skillFilesModifiedSince(skillsDir string, t time.Time) bool {
	// Quick check: if the directory doesn't exist, nothing could have changed.
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return false
	}

	changed := false
	err := filepath.WalkDir(skillsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr == nil && !d.IsDir() {
			if info, statErr := os.Stat(path); statErr == nil && info.ModTime().After(t) {
				changed = true
				return errWalkStop // stop walking
			}
		}
		return nil
	})
	// errWalkStop is expected (early exit on first changed file).
	// os.IsNotExist means the skills dir doesn't exist yet — not an error.
	// Any other error is unexpected and worth logging.
	if err != nil && !errors.Is(err, errWalkStop) && !os.IsNotExist(err) {
		logger.DebugCF("agent", "skills walk error", map[string]any{"error": err.Error()})
	}
	return changed
}
