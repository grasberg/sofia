package tools

import (
	"testing"
)

func TestMessageStructure(t *testing.T) {
	msg := Message{
		Role:    "user",
		Content: "hello",
	}
	if msg.Role != "user" {
		t.Errorf("expected role 'user', got '%s'", msg.Role)
	}
	if msg.Content != "hello" {
		t.Errorf("expected content 'hello', got '%s'", msg.Content)
	}
}

func TestMessageWithToolCalls(t *testing.T) {
	toolCall := ToolCall{
		ID:   "call_123",
		Type: "function",
		Name: "test_tool",
	}
	msg := Message{
		Role:      "assistant",
		ToolCalls: []ToolCall{toolCall},
	}
	if len(msg.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].ID != "call_123" {
		t.Errorf("expected ID 'call_123', got '%s'", msg.ToolCalls[0].ID)
	}
}

func TestToolCallWithFunctionCall(t *testing.T) {
	funcCall := &FunctionCall{
		Name:      "my_function",
		Arguments: `{"arg1": "value1"}`,
	}
	toolCall := ToolCall{
		ID:       "call_456",
		Type:     "function",
		Function: funcCall,
	}
	if toolCall.Function.Name != "my_function" {
		t.Errorf("expected function name 'my_function', got '%s'", toolCall.Function.Name)
	}
}

func TestToolCallWithArguments(t *testing.T) {
	args := map[string]any{
		"key1": "value1",
		"key2": 42,
	}
	toolCall := ToolCall{
		ID:        "call_789",
		Type:      "function",
		Name:      "test",
		Arguments: args,
	}
	if toolCall.Arguments["key1"] != "value1" {
		t.Errorf("expected key1='value1', got '%v'", toolCall.Arguments["key1"])
	}
	if toolCall.Arguments["key2"] != 42 {
		t.Errorf("expected key2=42, got '%v'", toolCall.Arguments["key2"])
	}
}

func TestLLMResponse(t *testing.T) {
	usage := &UsageInfo{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}
	response := &LLMResponse{
		Content:      "test response",
		FinishReason: "stop",
		Usage:        usage,
	}
	if response.Usage.TotalTokens != 150 {
		t.Errorf("expected total tokens 150, got %d", response.Usage.TotalTokens)
	}
}

func TestLLMResponseWithToolCalls(t *testing.T) {
	toolCall := ToolCall{
		ID:   "call_1",
		Type: "function",
		Name: "get_weather",
	}
	response := &LLMResponse{
		Content:      "Getting weather",
		ToolCalls:    []ToolCall{toolCall},
		FinishReason: "tool_calls",
	}
	if response.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason 'tool_calls', got '%s'", response.FinishReason)
	}
	if len(response.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(response.ToolCalls))
	}
}

func TestToolDefinition(t *testing.T) {
	funcDef := ToolFunctionDefinition{
		Name:        "my_tool",
		Description: "Does something",
		Parameters: map[string]any{
			"param1": "string",
		},
	}
	toolDef := ToolDefinition{
		Type:     "function",
		Function: funcDef,
	}
	if toolDef.Function.Name != "my_tool" {
		t.Errorf("expected name 'my_tool', got '%s'", toolDef.Function.Name)
	}
}

func TestToolFunctionDefinition(t *testing.T) {
	params := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	funcDef := ToolFunctionDefinition{
		Name:        "test_function",
		Description: "A test function",
		Parameters:  params,
	}
	if funcDef.Name != "test_function" {
		t.Errorf("expected name 'test_function', got '%s'", funcDef.Name)
	}
	if funcDef.Description != "A test function" {
		t.Errorf("expected description 'A test function', got '%s'", funcDef.Description)
	}
}

func TestUsageInfo(t *testing.T) {
	usage := UsageInfo{
		PromptTokens:     200,
		CompletionTokens: 75,
		TotalTokens:      275,
	}
	if usage.PromptTokens != 200 {
		t.Errorf("expected prompt tokens 200, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 75 {
		t.Errorf("expected completion tokens 75, got %d", usage.CompletionTokens)
	}
	if usage.TotalTokens != 275 {
		t.Errorf("expected total tokens 275, got %d", usage.TotalTokens)
	}
}
