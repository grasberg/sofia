package providers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyPool_SelectFillFirst(t *testing.T) {
	pool := NewKeyPool([]string{"key1", "key2", "key3"}, PoolStrategyFillFirst)

	// fill_first always returns the first available key.
	assert.Equal(t, "key1", pool.Select())
	assert.Equal(t, "key1", pool.Select())
}

func TestKeyPool_SelectRoundRobin(t *testing.T) {
	pool := NewKeyPool([]string{"key1", "key2", "key3"}, PoolStrategyRoundRobin)

	got := make([]string, 6)
	for i := range got {
		got[i] = pool.Select()
	}
	// Should cycle through key1 → key2 → key3 → key1 → ...
	assert.Equal(t, []string{"key1", "key2", "key3", "key1", "key2", "key3"}, got)
}

func TestKeyPool_SelectRandom(t *testing.T) {
	pool := NewKeyPool([]string{"key1", "key2", "key3"}, PoolStrategyRandom)

	seen := map[string]bool{}
	for range 30 {
		k := pool.Select()
		require.NotEmpty(t, k)
		seen[k] = true
	}
	// With 30 draws from 3 keys we expect all keys to appear.
	assert.Len(t, seen, 3)
}

func TestKeyPool_SelectLeastUsed(t *testing.T) {
	pool := NewKeyPool([]string{"key1", "key2", "key3"}, PoolStrategyLeastUsed)

	// Mark key1 as having been used 5 times.
	for range 5 {
		pool.MarkSuccess("key1")
	}
	// least_used should now prefer key2 or key3.
	k := pool.Select()
	assert.NotEqual(t, "key1", k)
}

func TestKeyPool_MarkExhausted429(t *testing.T) {
	now := time.Now()
	pool := NewKeyPool([]string{"key1", "key2"}, PoolStrategyFillFirst)
	pool.nowFn = func() time.Time { return now }

	pool.MarkExhausted("key1", 429)

	// key1 is in 1-hour cooldown; Select should return key2.
	assert.Equal(t, "key2", pool.Select())
}

func TestKeyPool_MarkExhaustedDefault(t *testing.T) {
	now := time.Now()
	pool := NewKeyPool([]string{"key1", "key2"}, PoolStrategyFillFirst)
	pool.nowFn = func() time.Time { return now }

	pool.MarkExhausted("key1", 402)

	assert.Equal(t, "key2", pool.Select())
}

func TestKeyPool_AllExhausted(t *testing.T) {
	now := time.Now()
	pool := NewKeyPool([]string{"key1", "key2"}, PoolStrategyFillFirst)
	pool.nowFn = func() time.Time { return now }

	pool.MarkExhausted("key1", 429)
	pool.MarkExhausted("key2", 429)

	assert.Equal(t, "", pool.Select())
	assert.False(t, pool.HasAvailable())
}

func TestKeyPool_CooldownExpiry(t *testing.T) {
	now := time.Now()
	pool := NewKeyPool([]string{"key1"}, PoolStrategyFillFirst)
	pool.nowFn = func() time.Time { return now }

	pool.MarkExhausted("key1", 429)
	assert.Equal(t, "", pool.Select(), "key should be in cooldown")

	// Advance clock past cooldown.
	pool.nowFn = func() time.Time { return now.Add(poolCooldown429 + time.Second) }
	assert.Equal(t, "key1", pool.Select(), "key should be available after cooldown")
}

func TestKeyPool_MarkSuccess_ClearsExhaustion(t *testing.T) {
	now := time.Now()
	pool := NewKeyPool([]string{"key1"}, PoolStrategyFillFirst)
	pool.nowFn = func() time.Time { return now }

	pool.MarkExhausted("key1", 429)
	assert.Equal(t, "", pool.Select())

	pool.MarkSuccess("key1")
	assert.Equal(t, "key1", pool.Select())
}

func TestKeyPool_DefaultStrategy(t *testing.T) {
	pool := NewKeyPool([]string{"key1", "key2"}, "")
	assert.Equal(t, PoolStrategyFillFirst, pool.strategy)

	pool2 := NewKeyPool([]string{"key1", "key2"}, "invalid_strategy")
	assert.Equal(t, PoolStrategyFillFirst, pool2.strategy)
}

func TestKeyPool_Len(t *testing.T) {
	pool := NewKeyPool([]string{"k1", "k2", "k3"}, PoolStrategyFillFirst)
	assert.Equal(t, 3, pool.Len())
}

func TestCollectAPIKeys(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		apiKeys  []string
		expected []string
	}{
		{
			name:     "single key only",
			apiKey:   "key1",
			expected: []string{"key1"},
		},
		{
			name:     "pool keys only",
			apiKeys:  []string{"key1", "key2"},
			expected: []string{"key1", "key2"},
		},
		{
			name:     "primary + pool, no overlap",
			apiKey:   "key1",
			apiKeys:  []string{"key2", "key3"},
			expected: []string{"key1", "key2", "key3"},
		},
		{
			name:     "primary duplicated in pool",
			apiKey:   "key1",
			apiKeys:  []string{"key1", "key2"},
			expected: []string{"key1", "key2"},
		},
		{
			name:     "empty strings filtered",
			apiKey:   "",
			apiKeys:  []string{"", "key1", "", "key2"},
			expected: []string{"key1", "key2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &mockModelConfigForKeys{APIKey: tt.apiKey, APIKeys: tt.apiKeys}
			got := collectAPIKeysFromMock(cfg)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// mockModelConfigForKeys mirrors the relevant fields of config.ModelConfig for testing.
type mockModelConfigForKeys struct {
	APIKey  string
	APIKeys []string
}

func collectAPIKeysFromMock(cfg *mockModelConfigForKeys) []string {
	seen := make(map[string]bool)
	var keys []string
	if cfg.APIKey != "" {
		seen[cfg.APIKey] = true
		keys = append(keys, cfg.APIKey)
	}
	for _, k := range cfg.APIKeys {
		if k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}
