package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSummarizationConfig_Defaults(t *testing.T) {
	s := SummarizationConfig{}
	assert.Equal(t, 75, s.ContextTriggerPctOrDefault())
	assert.Equal(t, 90, s.ForceTriggerPctOrDefault())
	assert.Equal(t, 2, s.ProtectHeadOrDefault())
	assert.Equal(t, 30, s.ProtectTailPctOrDefault())
	assert.Equal(t, 4, s.MinTailOrDefault())
	assert.Equal(t, 200, s.ToolResultTruncateCharsOrDefault())
}

func TestSummarizationConfig_UsesConfiguredValues(t *testing.T) {
	s := SummarizationConfig{ContextTriggerPct: 50, MinTail: 8}
	assert.Equal(t, 50, s.ContextTriggerPctOrDefault())
	assert.Equal(t, 8, s.MinTailOrDefault())
	// unset fields still use defaults
	assert.Equal(t, 90, s.ForceTriggerPctOrDefault())
}
