package tools

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/memory"
)

func newTestDynamicSetup(t *testing.T) (*DynamicToolCreator, *ToolRegistry) {
	t.Helper()
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck

	registry := NewToolRegistry()
	creator := NewDynamicToolCreator(db, registry, "")
	return creator, registry
}

func TestDynamicToolCreatorMeta(t *testing.T) {
	creator, _ := newTestDynamicSetup(t)
	assert.Equal(t, "dynamic_tool", creator.Name())
	assert.NotEmpty(t, creator.Description())
	assert.NotNil(t, creator.Parameters())
}

func TestDynamicToolCreateTemplate(t *testing.T) {
	creator, registry := newTestDynamicSetup(t)

	r := creator.Execute(context.Background(), map[string]any{
		"operation":   "create",
		"name":        "greet",
		"description": "Greet someone by name",
		"template":    "Hello, {{.name}}! Welcome.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Name to greet",
				},
			},
			"required": []string{"name"},
		},
	})
	assert.False(t, r.IsError, r.ForLLM)
	assert.Contains(t, r.ForLLM, "Created tool")

	// Tool should be registered.
	tool, ok := registry.Get("greet")
	require.True(t, ok)
	assert.Equal(t, "greet", tool.Name())
	assert.Equal(t, "Greet someone by name", tool.Description())

	// Execute the new tool.
	result := tool.Execute(context.Background(), map[string]any{
		"name": "Alice",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "Hello, Alice! Welcome.")
}

func TestDynamicToolCreateCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell commands use sh -c")
	}

	creator, registry := newTestDynamicSetup(t)

	r := creator.Execute(context.Background(), map[string]any{
		"operation":   "create",
		"name":        "echo_tool",
		"description": "Echo a message",
		"command":     "echo '{{.message}}'",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type": "string",
				},
			},
		},
	})
	assert.False(t, r.IsError, r.ForLLM)

	tool, ok := registry.Get("echo_tool")
	require.True(t, ok)

	result := tool.Execute(context.Background(), map[string]any{
		"message": "hello world",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "hello world")
}

func TestDynamicToolCreateMissingName(t *testing.T) {
	creator, _ := newTestDynamicSetup(t)
	r := creator.Execute(context.Background(), map[string]any{
		"operation": "create",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "name is required")
}

func TestDynamicToolCreateInvalidName(t *testing.T) {
	creator, _ := newTestDynamicSetup(t)
	r := creator.Execute(context.Background(), map[string]any{
		"operation":   "create",
		"name":        "Bad-Name",
		"description": "test",
		"template":    "test",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "lowercase")
}

func TestDynamicToolCreateMissingImpl(t *testing.T) {
	creator, _ := newTestDynamicSetup(t)
	r := creator.Execute(context.Background(), map[string]any{
		"operation":   "create",
		"name":        "empty",
		"description": "no impl",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "command or template is required")
}

func TestDynamicToolCreateBothImpl(t *testing.T) {
	creator, _ := newTestDynamicSetup(t)
	r := creator.Execute(context.Background(), map[string]any{
		"operation":   "create",
		"name":        "both",
		"description": "both",
		"command":     "echo hi",
		"template":    "hi",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "mutually exclusive")
}

func TestDynamicToolCreateInvalidTemplate(t *testing.T) {
	creator, _ := newTestDynamicSetup(t)
	r := creator.Execute(context.Background(), map[string]any{
		"operation":   "create",
		"name":        "bad_tmpl",
		"description": "test",
		"template":    "{{.bad",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "invalid template")
}

func TestDynamicToolCannotOverwriteBuiltin(t *testing.T) {
	creator, registry := newTestDynamicSetup(t)

	// Register a fake built-in tool.
	registry.Register(&stubTool{name: "read_file"})

	r := creator.Execute(context.Background(), map[string]any{
		"operation":   "create",
		"name":        "read_file",
		"description": "test",
		"template":    "test",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "cannot overwrite built-in")
}

func TestDynamicToolCanUpdateDynamic(t *testing.T) {
	creator, registry := newTestDynamicSetup(t)

	// Create first version.
	creator.Execute(context.Background(), map[string]any{
		"operation":   "create",
		"name":        "my_tool",
		"description": "v1",
		"template":    "version 1",
	})

	// Update it.
	r := creator.Execute(context.Background(), map[string]any{
		"operation":   "create",
		"name":        "my_tool",
		"description": "v2",
		"template":    "version 2",
	})
	assert.False(t, r.IsError)

	tool, _ := registry.Get("my_tool")
	result := tool.Execute(context.Background(), nil)
	assert.Contains(t, result.ForLLM, "version 2")
}

func TestDynamicToolList(t *testing.T) {
	creator, _ := newTestDynamicSetup(t)

	// Empty list.
	r := creator.Execute(context.Background(), map[string]any{
		"operation": "list",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "No dynamic tools")

	// After create.
	creator.Execute(context.Background(), map[string]any{
		"operation":   "create",
		"name":        "my_tool",
		"description": "A test tool",
		"template":    "test",
	})

	r = creator.Execute(context.Background(), map[string]any{
		"operation": "list",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "my_tool")
	assert.Contains(t, r.ForLLM, "template")
}

func TestDynamicToolRemove(t *testing.T) {
	creator, registry := newTestDynamicSetup(t)

	creator.Execute(context.Background(), map[string]any{
		"operation":   "create",
		"name":        "removable",
		"description": "will be removed",
		"template":    "test",
	})

	_, ok := registry.Get("removable")
	require.True(t, ok)

	r := creator.Execute(context.Background(), map[string]any{
		"operation": "remove",
		"name":      "removable",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "Removed tool")

	_, ok = registry.Get("removable")
	assert.False(t, ok)
}

func TestDynamicToolRemoveBuiltinFails(t *testing.T) {
	creator, registry := newTestDynamicSetup(t)
	registry.Register(&stubTool{name: "read_file"})

	r := creator.Execute(context.Background(), map[string]any{
		"operation": "remove",
		"name":      "read_file",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "cannot remove built-in")
}

func TestDynamicToolGet(t *testing.T) {
	creator, _ := newTestDynamicSetup(t)

	creator.Execute(context.Background(), map[string]any{
		"operation":   "create",
		"name":        "my_tool",
		"description": "A test tool",
		"template":    "Hello {{.name}}",
	})

	r := creator.Execute(context.Background(), map[string]any{
		"operation": "get",
		"name":      "my_tool",
	})
	assert.False(t, r.IsError)
	assert.Contains(t, r.ForLLM, "my_tool")
	assert.Contains(t, r.ForLLM, "A test tool")
	assert.Contains(t, r.ForLLM, "template")
}

func TestDynamicToolGetNotFound(t *testing.T) {
	creator, _ := newTestDynamicSetup(t)

	r := creator.Execute(context.Background(), map[string]any{
		"operation": "get",
		"name":      "nonexistent",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "not found")
}

func TestDynamicToolUnknownOp(t *testing.T) {
	creator, _ := newTestDynamicSetup(t)
	r := creator.Execute(context.Background(), map[string]any{
		"operation": "invalid",
	})
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "unknown operation")
}

func TestDynamicToolNoCommandOrTemplate(t *testing.T) {
	def := DynamicToolDef{Name: "empty", Description: "no impl"}
	tool := NewDynamicTool(def, "")
	r := tool.Execute(context.Background(), nil)
	assert.True(t, r.IsError)
	assert.Contains(t, r.ForLLM, "no command or template")
}

func TestDynamicToolDefaultParameters(t *testing.T) {
	def := DynamicToolDef{
		Name:        "no_params",
		Description: "test",
		Template:    "hello",
	}
	tool := NewDynamicTool(def, "")
	params := tool.Parameters()
	assert.Equal(t, "object", params["type"])
}

func TestLoadDynamicTools(t *testing.T) {
	db, err := memory.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck

	// Insert a persisted tool.
	def := DynamicToolDef{
		Name:        "persisted_tool",
		Description: "A persisted tool",
		Template:    "Persisted: {{.input}}",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
	defJSON, _ := json.Marshal(def) //nolint:errcheck
	_, err = db.Exec(
		`INSERT INTO dynamic_tools (name, definition) VALUES (?, ?)`,
		"persisted_tool", string(defJSON),
	)
	require.NoError(t, err)

	registry := NewToolRegistry()
	LoadDynamicTools(db, registry, "")

	tool, ok := registry.Get("persisted_tool")
	require.True(t, ok)
	assert.Equal(t, "persisted_tool", tool.Name())

	result := tool.Execute(context.Background(), map[string]any{
		"input": "test",
	})
	assert.Contains(t, result.ForLLM, "Persisted: test")
}

// stubTool is a minimal Tool implementation for testing.
type stubTool struct {
	name string
}

func (s *stubTool) Name() string        { return s.name }
func (s *stubTool) Description() string { return "stub" }

func (s *stubTool) Parameters() map[string]any {
	return map[string]any{"type": "object"}
}

func (s *stubTool) Execute(
	_ context.Context, _ map[string]any,
) *ToolResult {
	return NewToolResult("stub")
}
