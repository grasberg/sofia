package agent

import (
	"regexp"
	"sync"
)

// promptInjectionPatterns are pre-compiled regex patterns for detecting potential
// prompt injection attempts. Compiled once at package init, not per-message.
var promptInjectionPatterns = func() []*regexp.Regexp {
	patterns := []string{
		`(?i)ignore (all )?previous instructions`,
		`(?i)ignore (all )?above`,
		`(?i)disregard (all )?previous`,
		`(?i)forget (all )?previous`,
		`(?i)system prompt`,
		`(?i)you are an assistant that`,
		`(?i)new instructions:`,
	}
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		compiled = append(compiled, regexp.MustCompile(p))
	}
	return compiled
}()

// regexCache provides thread-safe caching for user-configured regex patterns
// (deny patterns, redact patterns) that would otherwise be recompiled on every use.
var regexCache = struct {
	mu    sync.RWMutex
	cache map[string]*regexp.Regexp
}{
	cache: make(map[string]*regexp.Regexp),
}

// getCachedRegex returns a compiled regex for the given pattern, using a cache
// to avoid recompilation. Returns nil if the pattern is invalid.
func getCachedRegex(pattern string) *regexp.Regexp {
	regexCache.mu.RLock()
	if re, ok := regexCache.cache[pattern]; ok {
		regexCache.mu.RUnlock()
		return re
	}
	regexCache.mu.RUnlock()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}

	regexCache.mu.Lock()
	regexCache.cache[pattern] = re
	regexCache.mu.Unlock()
	return re
}
