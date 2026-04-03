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

		// --- Multilingual injection attempts ---
		`忽略之前的指令`,                              // Chinese: "ignore previous instructions"
		`忽略以上(所有)?内容`,                            // Chinese: "ignore all above content"
		`(?i)ignoriere die vorherigen Anweisungen`, // German: "ignore previous instructions"
		`(?i)ignorez les instructions précédentes`, // French: "ignore previous instructions"
		`(?i)ignora le istruzioni precedenti`,      // Italian: "ignore previous instructions"
		`(?i)предыдущие инструкции`,                // Russian: "previous instructions"

		// --- Role-play / persona hijacking ---
		`(?i)pretend (that )?you are`,
		`(?i)you are now `,
		`(?i)act as if you have no restrictions`,
		`(?i)roleplay as`,
		`(?i)you must obey`,
		`(?i)you have been reprogrammed`,

		// --- Encoding tricks ---
		`(?i)base64 decode the following`,
		`(?i)rot13 (decode|the following|this)`,
		`(?i)decode this (hex|base64|binary)`,

		// --- Instruction override ---
		`(?i)^override:`,
		`(?i)from now on,? (you will|ignore|always)`,
		`(?i)new system prompt`,
		`(?i)updated instructions:`,
		`(?i)disregard (your|all) (safety|guidelines|rules)`,

		// --- Delimiter / boundary injection ---
		`(?i)---\s*END\s+SYSTEM\s*---`,
		`(?i)</system>`,
		`(?i)<\|im_end\|>`,
		`\[INST\]`,
		`\[/INST\]`,
		`(?i)<\|system\|>`,
		`(?i)<<\s*SYS\s*>>`,

		// --- Output manipulation / exfiltration ---
		`(?i)print your (system|initial) prompt`,
		`(?i)repeat the (above|system|initial) (text|instructions|prompt)`,
		`(?i)show me your (instructions|rules|prompt|system message)`,
		`(?i)what are your (secret|hidden|system) instructions`,
		`(?i)output everything (above|before) this`,
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

// maxRegexCacheSize is the maximum number of entries in the regex cache.
// When exceeded, the cache is cleared to prevent unbounded growth.
const maxRegexCacheSize = 100

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
	if len(regexCache.cache) >= maxRegexCacheSize {
		regexCache.cache = make(map[string]*regexp.Regexp)
	}
	regexCache.cache[pattern] = re
	regexCache.mu.Unlock()
	return re
}
