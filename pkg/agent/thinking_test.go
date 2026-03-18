package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidThinkingLevel(t *testing.T) {
	assert.True(t, IsValidThinkingLevel(ThinkingOff))
	assert.True(t, IsValidThinkingLevel(ThinkingMinimal))
	assert.True(t, IsValidThinkingLevel(ThinkingLow))
	assert.True(t, IsValidThinkingLevel(ThinkingMedium))
	assert.True(t, IsValidThinkingLevel(ThinkingHigh))
	assert.False(t, IsValidThinkingLevel("xhigh"))
	assert.False(t, IsValidThinkingLevel(""))
	assert.False(t, IsValidThinkingLevel("invalid"))
}

func TestThinkingBudgetTokens(t *testing.T) {
	assert.Equal(t, 0, ThinkingBudgetTokens(ThinkingOff))
	assert.Equal(t, 1024, ThinkingBudgetTokens(ThinkingMinimal))
	assert.Equal(t, 4096, ThinkingBudgetTokens(ThinkingLow))
	assert.Equal(t, 10000, ThinkingBudgetTokens(ThinkingMedium))
	assert.Equal(t, 32000, ThinkingBudgetTokens(ThinkingHigh))
	assert.Equal(t, 0, ThinkingBudgetTokens("unknown"))
}
