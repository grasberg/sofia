package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHTTPClientTool_Name(t *testing.T) {
	tool := NewHTTPClientTool()
	assert.Equal(t, "http", tool.Name())
}

func TestHTTPClientTool_Validation(t *testing.T) {
	tool := NewHTTPClientTool()

	t.Run("missing method and url", func(t *testing.T) {
		result := tool.Execute(t.Context(), map[string]any{})
		assert.True(t, result.IsError)
	})

	t.Run("private URL blocked", func(t *testing.T) {
		result := tool.Execute(t.Context(), map[string]any{
			"method": "GET",
			"url":    "http://localhost:8080/secret",
		})
		assert.True(t, result.IsError)
		assert.Contains(t, result.ForLLM, "private")
	})

	t.Run("private IP blocked", func(t *testing.T) {
		result := tool.Execute(t.Context(), map[string]any{
			"method": "GET",
			"url":    "http://192.168.1.1/admin",
		})
		assert.True(t, result.IsError)
		assert.Contains(t, result.ForLLM, "private")
	})
}

func TestIsPrivateURL(t *testing.T) {
	assert.True(t, isPrivateURL("http://localhost:8080"))
	assert.True(t, isPrivateURL("http://127.0.0.1/api"))
	assert.True(t, isPrivateURL("http://192.168.1.1/"))
	assert.True(t, isPrivateURL("http://10.0.0.1/"))
	assert.True(t, isPrivateURL("http://172.16.0.1/"))
	assert.False(t, isPrivateURL("https://api.example.com/v1"))
	assert.False(t, isPrivateURL("https://google.com"))
}
