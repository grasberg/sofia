package providers

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const (
	// poolCooldown429 is the key cooldown for HTTP 429 (rate-limited).
	// Quotas typically reset within an hour.
	poolCooldown429 = time.Hour

	// poolCooldownDefault is the key cooldown for auth/billing errors.
	poolCooldownDefault = 24 * time.Hour

	// PoolStrategyFillFirst always uses the first available key (default).
	PoolStrategyFillFirst = "fill_first"
	// PoolStrategyRoundRobin rotates through keys in sequence.
	PoolStrategyRoundRobin = "round_robin"
	// PoolStrategyRandom selects a random available key.
	PoolStrategyRandom = "random"
	// PoolStrategyLeastUsed selects the key with the fewest successful requests.
	PoolStrategyLeastUsed = "least_used"
)

type keyEntry struct {
	key            string
	useCount       int
	exhaustedUntil time.Time // zero value = available
}

func (e *keyEntry) isAvailable(now time.Time) bool {
	return e.exhaustedUntil.IsZero() || now.After(e.exhaustedUntil)
}

// KeyPool manages multiple API keys for a single provider endpoint, providing
// automatic rotation when keys are exhausted (rate-limited or auth errors).
//
// Strategies:
//   - fill_first  — always use the first available key (default)
//   - round_robin — rotate through keys in order
//   - random      — pick a random available key
//   - least_used  — use the key with the fewest successful requests
//
// Cooldowns:
//   - HTTP 429 (rate limit) → 1 hour
//   - Other errors (auth, billing) → 24 hours
type KeyPool struct {
	mu       sync.Mutex
	entries  []*keyEntry
	strategy string
	nowFn    func() time.Time // injectable for testing
}

// NewKeyPool creates a key pool from a list of API keys and a selection strategy.
// Strategy defaults to fill_first if empty or unrecognized.
func NewKeyPool(keys []string, strategy string) *KeyPool {
	entries := make([]*keyEntry, len(keys))
	for i, k := range keys {
		entries[i] = &keyEntry{key: k}
	}
	switch strategy {
	case PoolStrategyFillFirst, PoolStrategyRoundRobin, PoolStrategyRandom, PoolStrategyLeastUsed:
	default:
		strategy = PoolStrategyFillFirst
	}
	return &KeyPool{entries: entries, strategy: strategy, nowFn: time.Now}
}

// Select returns the next available key according to the pool's strategy.
// Returns "" when all keys are in cooldown.
func (p *KeyPool) Select() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.nowFn()
	available := p.availableEntries(now)
	if len(available) == 0 {
		return ""
	}

	switch p.strategy {
	case PoolStrategyRandom:
		return available[rand.Intn(len(available))].key //nolint:gosec // non-security use

	case PoolStrategyLeastUsed:
		best := available[0]
		for _, e := range available[1:] {
			if e.useCount < best.useCount {
				best = e
			}
		}
		return best.key

	case PoolStrategyRoundRobin:
		if len(available) == 1 {
			return available[0].key
		}
		// Rotate: move the first available entry to the end of the slice.
		entry := available[0]
		for i, e := range p.entries {
			if e == entry {
				p.entries = append(p.entries[:i], append(p.entries[i+1:], entry)...)
				break
			}
		}
		return entry.key

	default: // fill_first
		return available[0].key
	}
}

// MarkExhausted puts a key into cooldown. The duration depends on the HTTP status:
// 429 → 1 hour, anything else → 24 hours.
func (p *KeyPool) MarkExhausted(key string, statusCode int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.nowFn()
	cooldown := poolCooldownDefault
	if statusCode == 429 {
		cooldown = poolCooldown429
	}

	for _, e := range p.entries {
		if e.key == key {
			e.exhaustedUntil = now.Add(cooldown)
			return
		}
	}
}

// MarkSuccess clears a key's cooldown and increments its use count.
func (p *KeyPool) MarkSuccess(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, e := range p.entries {
		if e.key == key {
			e.exhaustedUntil = time.Time{}
			e.useCount++
			return
		}
	}
}

// Len returns the total number of keys (including exhausted ones).
func (p *KeyPool) Len() int {
	return len(p.entries)
}

// HasAvailable returns true if at least one key is not in cooldown.
func (p *KeyPool) HasAvailable() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.availableEntries(p.nowFn())) > 0
}

// Status returns a human-readable summary of pool state for diagnostics.
func (p *KeyPool) Status() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.nowFn()
	available := p.availableEntries(now)
	exhausted := len(p.entries) - len(available)

	var sb strings.Builder
	fmt.Fprintf(&sb, "key_pool: %d/%d available (strategy=%s)", len(available), len(p.entries), p.strategy)
	if exhausted > 0 {
		fmt.Fprintf(&sb, ", %d exhausted", exhausted)
	}
	return sb.String()
}

func (p *KeyPool) availableEntries(now time.Time) []*keyEntry {
	result := make([]*keyEntry, 0, len(p.entries))
	for _, e := range p.entries {
		if e.isAvailable(now) {
			result = append(result, e)
		}
	}
	return result
}
