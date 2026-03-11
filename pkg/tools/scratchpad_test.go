package tools

import (
	"context"
	"testing"
)

func TestSharedScratchpad_Write_Read(t *testing.T) {
	sp := NewSharedScratchpad()
	sp.Write("group1", "key1", "value1")
	
	val, ok := sp.Read("group1", "key1")
	if !ok {
		t.Error("expected key to be found")
	}
	if val != "value1" {
		t.Errorf("expected 'value1', got '%s'", val)
	}
}

func TestSharedScratchpad_Read_NotFound(t *testing.T) {
	sp := NewSharedScratchpad()
	val, ok := sp.Read("group1", "nonexistent")
	if ok {
		t.Error("expected key not to be found")
	}
	if val != "" {
		t.Errorf("expected empty string, got '%s'", val)
	}
}

func TestSharedScratchpad_Write_Overwrite(t *testing.T) {
	sp := NewSharedScratchpad()
	sp.Write("group1", "key1", "value1")
	sp.Write("group1", "key1", "value2")
	
	val, _ := sp.Read("group1", "key1")
	if val != "value2" {
		t.Errorf("expected 'value2', got '%s'", val)
	}
}

func TestSharedScratchpad_MultipleGroups(t *testing.T) {
	sp := NewSharedScratchpad()
	sp.Write("group1", "key1", "value1")
	sp.Write("group2", "key1", "value2")
	
	val1, _ := sp.Read("group1", "key1")
	val2, _ := sp.Read("group2", "key1")
	
	if val1 != "value1" {
		t.Errorf("expected 'value1' in group1, got '%s'", val1)
	}
	if val2 != "value2" {
		t.Errorf("expected 'value2' in group2, got '%s'", val2)
	}
}

func TestSharedScratchpad_List_Empty(t *testing.T) {
	sp := NewSharedScratchpad()
	keys := sp.List("group1")
	if len(keys) != 0 {
		t.Errorf("expected no keys, got %d", len(keys))
	}
}

func TestSharedScratchpad_List_Sorted(t *testing.T) {
	sp := NewSharedScratchpad()
	sp.Write("group1", "zebra", "1")
	sp.Write("group1", "apple", "2")
	sp.Write("group1", "banana", "3")
	
	keys := sp.List("group1")
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "apple" {
		t.Errorf("expected 'apple' first, got '%s'", keys[0])
	}
	if keys[1] != "banana" {
		t.Errorf("expected 'banana' second, got '%s'", keys[1])
	}
	if keys[2] != "zebra" {
		t.Errorf("expected 'zebra' third, got '%s'", keys[2])
	}
}

func TestScratchpadTool_Name(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewScratchpadTool(sp, "default")
	if tool.Name() != "scratchpad" {
		t.Errorf("expected name 'scratchpad', got '%s'", tool.Name())
	}
}

func TestScratchpadTool_Description(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewScratchpadTool(sp, "default")
	desc := tool.Description()
	if desc == "" {
		t.Error("expected non-empty description")
	}
	if !contains(desc, "scratchpad") {
		t.Error("expected 'scratchpad' in description")
	}
}

func TestScratchpadTool_Parameters(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewScratchpadTool(sp, "default")
	params := tool.Parameters()
	if params == nil {
		t.Error("expected parameters to be non-nil")
	}
	if _, ok := params["properties"]; !ok {
		t.Error("expected 'properties' in parameters")
	}
}

func TestScratchpadTool_Write(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewScratchpadTool(sp, "default")
	
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "write",
		"key":       "test_key",
		"value":     "test_value",
	})
	
	if result.IsError {
		t.Errorf("expected no error, got: %s", result.ForLLM)
	}
	
	// Verify the value was stored
	val, ok := sp.Read("default", "test_key")
	if !ok {
		t.Error("expected key to be stored")
	}
	if val != "test_value" {
		t.Errorf("expected 'test_value', got '%s'", val)
	}
}

func TestScratchpadTool_Write_CustomGroup(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewScratchpadTool(sp, "default")
	
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "write",
		"key":       "test_key",
		"value":     "test_value",
		"group":     "custom",
	})
	
	if result.IsError {
		t.Errorf("expected no error, got: %s", result.ForLLM)
	}
	
	// Verify in custom group
	val, ok := sp.Read("custom", "test_key")
	if !ok {
		t.Error("expected key to be stored in custom group")
	}
	if val != "test_value" {
		t.Errorf("expected 'test_value', got '%s'", val)
	}
}

func TestScratchpadTool_Write_MissingKey(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewScratchpadTool(sp, "default")
	
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "write",
		"value":     "test_value",
	})
	
	if !result.IsError {
		t.Error("expected error for missing key")
	}
}

func TestScratchpadTool_Read(t *testing.T) {
	sp := NewSharedScratchpad()
	sp.Write("default", "test_key", "test_value")
	tool := NewScratchpadTool(sp, "default")
	
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "read",
		"key":       "test_key",
	})
	
	if result.IsError {
		t.Errorf("expected no error, got: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "test_value") {
		t.Errorf("expected 'test_value' in result, got: %s", result.ForLLM)
	}
}

func TestScratchpadTool_Read_NotFound(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewScratchpadTool(sp, "default")
	
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "read",
		"key":       "nonexistent",
	})
	
	if result.IsError {
		t.Errorf("expected no error, got: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "not found") {
		t.Errorf("expected 'not found' in result, got: %s", result.ForLLM)
	}
}

func TestScratchpadTool_Read_MissingKey(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewScratchpadTool(sp, "default")
	
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "read",
	})
	
	if !result.IsError {
		t.Error("expected error for missing key")
	}
}

func TestScratchpadTool_List(t *testing.T) {
	sp := NewSharedScratchpad()
	sp.Write("default", "key1", "val1")
	sp.Write("default", "key2", "val2")
	tool := NewScratchpadTool(sp, "default")
	
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "list",
	})
	
	if result.IsError {
		t.Errorf("expected no error, got: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "key1") {
		t.Errorf("expected 'key1' in result")
	}
	if !contains(result.ForLLM, "key2") {
		t.Errorf("expected 'key2' in result")
	}
}

func TestScratchpadTool_List_Empty(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewScratchpadTool(sp, "default")
	
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "list",
	})
	
	if result.IsError {
		t.Errorf("expected no error, got: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "No keys") {
		t.Errorf("expected 'No keys' in result, got: %s", result.ForLLM)
	}
}

func TestScratchpadTool_InvalidOperation(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewScratchpadTool(sp, "default")
	
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "invalid",
	})
	
	if !result.IsError {
		t.Error("expected error for invalid operation")
	}
}

func TestScratchpadTool_DefaultGroup(t *testing.T) {
	sp := NewSharedScratchpad()
	tool := NewScratchpadTool(sp, "")
	
	// Should use "default" as defaultGroup
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "write",
		"key":       "test",
		"value":     "val",
	})
	
	if result.IsError {
		t.Errorf("expected no error, got: %s", result.ForLLM)
	}
	
	// Verify stored in "default"
	val, ok := sp.Read("default", "test")
	if !ok {
		t.Error("expected key to be stored in 'default' group")
	}
	if val != "val" {
		t.Errorf("expected 'val', got '%s'", val)
	}
}
