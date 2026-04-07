package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/tools"
)

// toolCacheEntry holds a cached tool result with expiration.
type toolCacheEntry struct {
	result    *tools.ToolResult
	expiresAt time.Time
}

// checkToolResultCache returns a cached result if available and not expired.
// Returns (result, true) if cache hit, (nil, false) if cache miss.
func (al *AgentLoop) checkToolResultCache(toolName string, args map[string]any) (*tools.ToolResult, bool) {
	key := al.buildCacheKey(toolName, args)
	if entry, ok := al.toolResultCache.Load(key); ok {
		e := entry.(*toolCacheEntry)
		if time.Now().Before(e.expiresAt) {
			logger.DebugCF("tool_cache", "Cache hit", map[string]any{
				"tool": toolName,
				"key":  key,
			})
			return e.result, true
		}
		// Expired, remove from cache
		al.toolResultCache.Delete(key)
	}
	return nil, false
}

// storeToolResultCache stores a tool result in the cache with TTL.
func (al *AgentLoop) storeToolResultCache(toolName string, args map[string]any, result *tools.ToolResult) {
	if result == nil {
		return
	}
	// Only cache successful results or non-error results
	if result.IsError && result.Err != nil {
		return
	}

	key := al.buildCacheKey(toolName, args)
	al.toolResultCache.Store(key, &toolCacheEntry{
		result:    result,
		expiresAt: time.Now().Add(al.toolResultCacheTTL),
	})
}

// buildCacheKey creates a unique cache key from tool name and arguments.
func (al *AgentLoop) buildCacheKey(toolName string, args map[string]any) string {
	argsJSON, _ := json.Marshal(args)
	hash := sha256.Sum256(argsJSON)
	return fmt.Sprintf("%s:%x", toolName, hash[:8])
}

// clearToolResultCache removes all entries from the tool result cache.
func (al *AgentLoop) clearToolResultCache() {
	al.toolResultCache.Range(func(key, value any) bool {
		al.toolResultCache.Delete(key)
		return true
	})
}
