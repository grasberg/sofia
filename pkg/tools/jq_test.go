package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJqTool_Name(t *testing.T) {
	tool := NewJqTool()
	assert.Equal(t, "jq", tool.Name())
}

func TestEvaluateSimpleJq(t *testing.T) {
	obj := map[string]any{
		"name": "Alice",
		"age":  float64(30),
		"nested": map[string]any{
			"city": "NYC",
		},
	}

	arr := []any{"a", "b", "c"}

	t.Run("identity", func(t *testing.T) {
		result, err := evaluateSimpleJq(obj, ".")
		assert.NoError(t, err)
		assert.Contains(t, result, "Alice")
	})

	t.Run("field access", func(t *testing.T) {
		result, err := evaluateSimpleJq(obj, ".name")
		assert.NoError(t, err)
		assert.Equal(t, `"Alice"`, result)
	})

	t.Run("nested field", func(t *testing.T) {
		result, err := evaluateSimpleJq(obj, ".nested.city")
		assert.NoError(t, err)
		assert.Equal(t, `"NYC"`, result)
	})

	t.Run("missing field", func(t *testing.T) {
		result, err := evaluateSimpleJq(obj, ".nonexistent")
		assert.NoError(t, err)
		assert.Equal(t, "null", result)
	})

	t.Run("keys", func(t *testing.T) {
		result, err := evaluateSimpleJq(obj, "keys")
		assert.NoError(t, err)
		assert.Contains(t, result, "name")
		assert.Contains(t, result, "age")
	})

	t.Run("length of object", func(t *testing.T) {
		result, err := evaluateSimpleJq(obj, "length")
		assert.NoError(t, err)
		assert.Equal(t, "3", result)
	})

	t.Run("length of array", func(t *testing.T) {
		result, err := evaluateSimpleJq(arr, "length")
		assert.NoError(t, err)
		assert.Equal(t, "3", result)
	})

	t.Run("type of object", func(t *testing.T) {
		result, err := evaluateSimpleJq(obj, "type")
		assert.NoError(t, err)
		assert.Equal(t, `"object"`, result)
	})

	t.Run("type of array", func(t *testing.T) {
		result, err := evaluateSimpleJq(arr, "type")
		assert.NoError(t, err)
		assert.Equal(t, `"array"`, result)
	})

	t.Run("complex expression falls back to error", func(t *testing.T) {
		_, err := evaluateSimpleJq(obj, ".[] | select(.age > 30)")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too complex")
	})
}
