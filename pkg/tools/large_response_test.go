package tools

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleLargeResponse_SmallOutput(t *testing.T) {
	result := NewToolResult("small output")
	handled := HandleLargeResponse(result, "test")
	assert.Equal(t, "small output", handled.ForLLM)
}

func TestHandleLargeResponse_NilResult(t *testing.T) {
	assert.Nil(t, HandleLargeResponse(nil, "test"))
}

func TestHandleLargeResponse_ErrorResult(t *testing.T) {
	result := ErrorResult("some error")
	handled := HandleLargeResponse(result, "test")
	assert.Equal(t, "some error", handled.ForLLM)
}

func TestHandleLargeResponse_LargeOutput(t *testing.T) {
	// Create a string larger than the threshold
	large := strings.Repeat("x", LargeResponseThreshold+1000)
	result := NewToolResult(large)
	handled := HandleLargeResponse(result, "test_tool")

	// Should contain preview and reference
	assert.Contains(t, handled.ForLLM, "LARGE OUTPUT")
	assert.Contains(t, handled.ForLLM, "saved to")
	assert.Contains(t, handled.ForLLM, "read_file")
	// Should be much smaller than original
	assert.Less(t, len(handled.ForLLM), LargeResponseThreshold)
}
