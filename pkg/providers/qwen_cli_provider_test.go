package providers

import (
	"testing"
)

func TestQwenCli_ParseJSONRPCEvents_MessageChunks(t *testing.T) {
	p := &QwenCliProvider{}
	events := `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{}}
{"jsonrpc":"2.0","id":0,"result":{"protocolVersion":1,"agentInfo":{"name":"qwen-code"}}}
{"jsonrpc":"2.0","id":1,"method":"session/new","params":{}}
{"jsonrpc":"2.0","id":1,"result":{"sessionId":"test-session"}}
{"jsonrpc":"2.0","id":2,"method":"session/prompt","params":{"prompt":[{"type":"text","text":"hello"}]}}
{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"test-session","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Hello"}}}}
{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"test-session","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":" world!"}}}}
{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"test-session","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":""},"_meta":{"usage":{"inputTokens":100,"outputTokens":10,"totalTokens":110,"thoughtTokens":5,"cachedReadTokens":80}}}}}
{"jsonrpc":"2.0","id":2,"result":{"stopReason":"end_turn"}}`

	resp, err := p.parseJSONRPCEvents(events)
	if err != nil {
		t.Fatalf("parseJSONRPCEvents() error: %v", err)
	}
	if resp.Content != "Hello world!" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hello world!")
	}
	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "stop")
	}
	if resp.Usage == nil {
		t.Fatal("Usage should not be nil")
	}
	if resp.Usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 10 {
		t.Errorf("CompletionTokens = %d, want 10", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 110 {
		t.Errorf("TotalTokens = %d, want 110", resp.Usage.TotalTokens)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("ToolCalls should be empty, got %d", len(resp.ToolCalls))
	}
}

func TestQwenCli_ParseJSONRPCEvents_NoContent(t *testing.T) {
	p := &QwenCliProvider{}
	events := `{"jsonrpc":"2.0","id":0,"result":{}}
{"jsonrpc":"2.0","id":2,"result":{"stopReason":"end_turn"}}`

	_, err := p.parseJSONRPCEvents(events)
	if err == nil {
		t.Fatal("expected error for no content, got nil")
	}
}

func TestQwenCli_GetDefaultModel(t *testing.T) {
	p := NewQwenCliProvider("")
	if got := p.GetDefaultModel(); got != "qwen-code" {
		t.Errorf("GetDefaultModel() = %q, want %q", got, "qwen-code")
	}
}

func TestQwenCli_BuildPrompt_SimpleMessage(t *testing.T) {
	p := &QwenCliProvider{}
	messages := []Message{
		{Role: "user", Content: "hello"},
	}
	got := p.buildPrompt(messages, nil)
	if got != "hello" {
		t.Errorf("buildPrompt() = %q, want %q", got, "hello")
	}
}

func TestQwenCli_BuildPrompt_WithSystem(t *testing.T) {
	p := &QwenCliProvider{}
	messages := []Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "hello"},
	}
	got := p.buildPrompt(messages, nil)
	if got == "hello" {
		t.Error("buildPrompt() should include system message")
	}
	if !contains(got, "System Instructions") {
		t.Error("buildPrompt() should contain 'System Instructions' header")
	}
	if !contains(got, "You are helpful") {
		t.Error("buildPrompt() should contain system content")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
