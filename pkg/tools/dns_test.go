package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDNSTool_Name(t *testing.T) {
	tool := NewDNSTool()
	assert.Equal(t, "dns", tool.Name())
}

func TestDNSTool_Validation(t *testing.T) {
	tool := NewDNSTool()

	t.Run("missing domain and ip", func(t *testing.T) {
		result := tool.Execute(t.Context(), map[string]any{})
		assert.True(t, result.IsError)
		assert.Contains(t, result.ForLLM, "domain or ip is required")
	})
}
