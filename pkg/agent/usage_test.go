package agent

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/providers/protocoltypes"
)

func TestUsageTracker_Record(t *testing.T) {
	ut := NewUsageTracker()

	ut.Record("session1", &protocoltypes.UsageInfo{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	})
	ut.Record("session1", &protocoltypes.UsageInfo{
		PromptTokens:     200,
		CompletionTokens: 80,
		TotalTokens:      280,
	})

	s := ut.GetSession("session1")
	require.NotNil(t, s)
	assert.Equal(t, int64(300), s.PromptTokens)
	assert.Equal(t, int64(130), s.CompletionTokens)
	assert.Equal(t, int64(430), s.TotalTokens)
	assert.Equal(t, 2, s.CallCount)
}

func TestUsageTracker_GetSession_NotFound(t *testing.T) {
	ut := NewUsageTracker()
	s := ut.GetSession("nonexistent")
	assert.Nil(t, s)
}

func TestUsageTracker_Reset(t *testing.T) {
	ut := NewUsageTracker()

	ut.Record("session1", &protocoltypes.UsageInfo{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	})
	require.NotNil(t, ut.GetSession("session1"))

	ut.Reset("session1")
	assert.Nil(t, ut.GetSession("session1"))
}

func TestUsageTracker_Concurrent(t *testing.T) {
	ut := NewUsageTracker()
	var wg sync.WaitGroup
	goroutines := 100

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			ut.Record("concurrent-session", &protocoltypes.UsageInfo{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			})
		}()
	}
	wg.Wait()

	s := ut.GetSession("concurrent-session")
	require.NotNil(t, s)
	assert.Equal(t, int64(1000), s.PromptTokens)
	assert.Equal(t, int64(500), s.CompletionTokens)
	assert.Equal(t, int64(1500), s.TotalTokens)
	assert.Equal(t, goroutines, s.CallCount)
}

func TestEstimateCost(t *testing.T) {
	usage := &SessionUsage{
		PromptTokens:     1_000_000,
		CompletionTokens: 500_000,
	}
	// claude-sonnet: $3.00/1M input, $15.00/1M output
	cost := EstimateCost(usage, "claude-sonnet")
	expected := 3.00 + 7.50 // 1M * 3.00/1M + 0.5M * 15.00/1M
	assert.InDelta(t, expected, cost, 0.001)
}

func TestEstimateCost_UnknownModel(t *testing.T) {
	usage := &SessionUsage{
		PromptTokens:     1_000_000,
		CompletionTokens: 500_000,
	}
	cost := EstimateCost(usage, "totally-unknown-model-xyz")
	assert.Equal(t, 0.0, cost)
}

func TestEstimateCost_NilUsage(t *testing.T) {
	cost := EstimateCost(nil, "claude-sonnet")
	assert.Equal(t, 0.0, cost)
}

func TestGetPricing_SubstringMatch(t *testing.T) {
	p := GetPricing("claude-sonnet-4-20260101")
	assert.Equal(t, 3.00, p.InputPer1M)
	assert.Equal(t, 15.00, p.OutputPer1M)
}

func TestGetPricing_ExactMatch(t *testing.T) {
	p := GetPricing("gpt-4o")
	assert.Equal(t, 2.50, p.InputPer1M)
	assert.Equal(t, 10.00, p.OutputPer1M)
}

func TestGetPricing_LongestSubstringWins(t *testing.T) {
	// "gpt-4o-mini" should match "gpt-4o-mini" not "gpt-4o"
	p := GetPricing("gpt-4o-mini")
	assert.Equal(t, 0.15, p.InputPer1M)
	assert.Equal(t, 0.60, p.OutputPer1M)
}

func TestUsageTracker_RecordNilUsage(t *testing.T) {
	ut := NewUsageTracker()
	ut.Record("session1", nil)
	assert.Nil(t, ut.GetSession("session1"))
}
