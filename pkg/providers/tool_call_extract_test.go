package providers

import "testing"

func TestExtractInvokeStyleToolCalls_MiniMax(t *testing.T) {
	text := `<minimax_tool_call>
<invoke name="read_file">
<parameter name="path">/Users/magnusgrasberg/.sofia/config.json</parameter>
</invoke>
</minimax_tool_call>`

	calls := extractXMLToolCalls(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if calls[0].Name != "read_file" {
		t.Errorf("Name = %q, want %q", calls[0].Name, "read_file")
	}
	path, ok := calls[0].Arguments["path"].(string)
	if !ok || path != "/Users/magnusgrasberg/.sofia/config.json" {
		t.Errorf("path arg = %v, want config.json path", calls[0].Arguments["path"])
	}
}

func TestExtractInvokeStyleToolCalls_MultipleParams(t *testing.T) {
	text := `<minimax_tool_call>
<invoke name="bitcoin">
<parameter name="action">send</parameter>
<parameter name="to_address">bc1qabc123</parameter>
<parameter name="amount_btc">0.001</parameter>
</invoke>
</minimax_tool_call>`

	calls := extractXMLToolCalls(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if calls[0].Name != "bitcoin" {
		t.Errorf("Name = %q, want %q", calls[0].Name, "bitcoin")
	}
	if calls[0].Arguments["action"] != "send" {
		t.Errorf("action = %v, want send", calls[0].Arguments["action"])
	}
	if calls[0].Arguments["to_address"] != "bc1qabc123" {
		t.Errorf("to_address = %v, want bc1qabc123", calls[0].Arguments["to_address"])
	}
}

func TestStripXMLToolCalls_MiniMax(t *testing.T) {
	text := `Here is my response.
<minimax_tool_call>
<invoke name="read_file">
<parameter name="path">/tmp/x</parameter>
</invoke>
</minimax_tool_call>
More text.`

	stripped := stripXMLToolCalls(text)
	if stripped != "Here is my response.\n\nMore text." {
		t.Errorf("stripXMLToolCalls = %q", stripped)
	}
}

func TestExtractInvokeStyleToolCalls_BareInvoke(t *testing.T) {
	text := `<invoke name="exec">
<parameter name="command">ls -la</parameter>
</invoke>`

	calls := extractXMLToolCalls(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if calls[0].Name != "exec" {
		t.Errorf("Name = %q, want exec", calls[0].Name)
	}
}
