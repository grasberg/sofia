package providers

import (
	"encoding/json"
	"strings"
)

// extractToolCallsFromText parses tool call JSON from response text.
// Both ClaudeCliProvider and CodexCliProvider use this to extract
// tool calls that the model outputs in its response text.
func extractToolCallsFromText(text string) []ToolCall {
	start := strings.Index(text, `{"tool_calls"`)
	if start == -1 {
		return nil
	}

	end := findMatchingBrace(text, start)
	if end == start {
		return nil
	}

	jsonStr := text[start:end]

	var wrapper struct {
		ToolCalls []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
		return nil
	}

	var result []ToolCall
	for _, tc := range wrapper.ToolCalls {
		var args map[string]any
		json.Unmarshal([]byte(tc.Function.Arguments), &args)

		result = append(result, ToolCall{
			ID:        tc.ID,
			Type:      tc.Type,
			Name:      tc.Function.Name,
			Arguments: args,
			Function: &FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	return result
}

// extractXMLToolCalls parses tool calls from XML-style tags that local models
// (Qwen, Ollama, etc.) often emit instead of proper OpenAI function-calling JSON.
// Handles formats like:
//
//	<tool_call>{"name":"exec","arguments":{"command":"ls"}}</tool_call>
//	<tool_call>exec command="ls"</tool_call>
//	<function_call>{"name":"read_file","arguments":{"path":"/tmp/x"}}</function_call>
func extractXMLToolCalls(text string) []ToolCall {
	var result []ToolCall

	// Handle <minimax_tool_call><invoke name="..."><parameter name="...">...</parameter></invoke></minimax_tool_call>
	if calls := extractInvokeStyleToolCalls(text); len(calls) > 0 {
		result = append(result, calls...)
	}

	for _, tag := range []string{"tool_call", "function_call"} {
		openTag := "<" + tag + ">"
		closeTag := "</" + tag + ">"

		remaining := text
		for {
			start := strings.Index(remaining, openTag)
			if start == -1 {
				break
			}
			end := strings.Index(remaining[start:], closeTag)
			if end == -1 {
				break
			}
			inner := strings.TrimSpace(remaining[start+len(openTag) : start+end])
			remaining = remaining[start+end+len(closeTag):]

			tc := parseXMLToolCallBody(inner)
			if tc != nil {
				result = append(result, *tc)
			}
		}
	}

	return result
}

// parseXMLToolCallBody parses the inner body of an XML tool call tag.
// Supports multiple formats:
//
//	Format 1 (JSON):  {"name":"exec","arguments":{"command":"ls"}}
//	Format 2 (simple): exec command="ls"
//	Format 3 (param):  name="exec"\n<param name="command">ls</param>
func parseXMLToolCallBody(body string) *ToolCall {
	// Try JSON format first: {"name":"exec","arguments":{"command":"ls"}}
	var jsonCall struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(body), &jsonCall); err == nil && jsonCall.Name != "" {
		argsJSON, _ := json.Marshal(jsonCall.Arguments)
		return &ToolCall{
			ID:        "xmltool_" + jsonCall.Name,
			Name:      jsonCall.Name,
			Arguments: jsonCall.Arguments,
			Function: &FunctionCall{
				Name:      jsonCall.Name,
				Arguments: string(argsJSON),
			},
		}
	}

	body = strings.TrimSpace(body)

	// Try format 3: name="exec" with <param> tags
	// Detects: name="toolname"\n args="\n<param name="key">value</param>
	if tc := parseParamStyleToolCall(body); tc != nil {
		return tc
	}

	// Try simple format: tool_name key="value" key2="value2"
	parts := strings.SplitN(body, " ", 2)
	if len(parts) == 0 || parts[0] == "" {
		return nil
	}

	name := strings.TrimSpace(parts[0])
	// If the first "word" contains = it's not a tool name but a key=value
	if strings.Contains(name, "=") {
		return nil
	}
	args := parseKeyValuePairs(parts)

	if name == "" {
		return nil
	}

	argsJSON, _ := json.Marshal(args)
	return &ToolCall{
		ID:        "xmltool_" + name,
		Name:      name,
		Arguments: args,
		Function: &FunctionCall{
			Name:      name,
			Arguments: string(argsJSON),
		},
	}
}

// parseParamStyleToolCall handles the format:
//
//	name="exec"
//	args="
//	<param name="command">curl -s "..."</param>
//	<param name="label">check balance</param>
func parseParamStyleToolCall(body string) *ToolCall {
	// Extract name="..." from the body
	name := extractQuotedValue(body, "name")
	if name == "" {
		return nil
	}

	// Extract <param name="key">value</param> tags
	args := make(map[string]any)
	remaining := body
	for {
		paramStart := strings.Index(remaining, "<param ")
		if paramStart == -1 {
			break
		}
		// Find the name attribute of the param tag
		nameAttrStart := strings.Index(remaining[paramStart:], `name="`)
		if nameAttrStart == -1 {
			break
		}
		nameAttrStart += paramStart + len(`name="`)
		nameAttrEnd := strings.Index(remaining[nameAttrStart:], `"`)
		if nameAttrEnd == -1 {
			break
		}
		paramName := remaining[nameAttrStart : nameAttrStart+nameAttrEnd]

		// Find >value</param>
		tagClose := strings.Index(remaining[nameAttrStart:], ">")
		if tagClose == -1 {
			break
		}
		valueStart := nameAttrStart + tagClose + 1
		paramClose := strings.Index(remaining[valueStart:], "</param>")
		if paramClose == -1 {
			break
		}
		paramValue := remaining[valueStart : valueStart+paramClose]
		args[paramName] = paramValue
		remaining = remaining[valueStart+paramClose+len("</param>"):]
	}

	if len(args) == 0 {
		return nil
	}

	argsJSON, _ := json.Marshal(args)
	return &ToolCall{
		ID:        "xmltool_" + name,
		Name:      name,
		Arguments: args,
		Function: &FunctionCall{
			Name:      name,
			Arguments: string(argsJSON),
		},
	}
}

// extractQuotedValue finds key="value" in text and returns value.
func extractQuotedValue(text, key string) string {
	pattern := key + `="`
	idx := strings.Index(text, pattern)
	if idx == -1 {
		return ""
	}
	start := idx + len(pattern)
	end := strings.Index(text[start:], `"`)
	if end == -1 {
		// Take to end of line
		end = strings.Index(text[start:], "\n")
		if end == -1 {
			return text[start:]
		}
	}
	return text[start : start+end]
}

// parseKeyValuePairs extracts key="value" pairs from parts[1].
func parseKeyValuePairs(parts []string) map[string]any {
	args := make(map[string]any)
	if len(parts) < 2 {
		return args
	}

	remainder := parts[1]
	for remainder != "" {
		remainder = strings.TrimSpace(remainder)
		eqIdx := strings.Index(remainder, "=")
		if eqIdx == -1 {
			break
		}
		key := strings.TrimSpace(remainder[:eqIdx])
		remainder = remainder[eqIdx+1:]

		if len(remainder) > 0 && remainder[0] == '"' {
			closeIdx := 1
			for closeIdx < len(remainder) {
				if remainder[closeIdx] == '"' && (closeIdx == 0 || remainder[closeIdx-1] != '\\') {
					break
				}
				closeIdx++
			}
			if closeIdx < len(remainder) {
				args[key] = remainder[1:closeIdx]
				remainder = remainder[closeIdx+1:]
			} else {
				args[key] = remainder[1:]
				remainder = ""
			}
		} else {
			spaceIdx := strings.Index(remainder, " ")
			if spaceIdx == -1 {
				args[key] = remainder
				remainder = ""
			} else {
				args[key] = remainder[:spaceIdx]
				remainder = remainder[spaceIdx:]
			}
		}
	}

	return args
}

// stripThinkTags removes <think>...</think> blocks from model output.
// Many local models (Qwen, DeepSeek R1, etc.) emit these for chain-of-thought.
func stripThinkTags(text string) string {
	for {
		start := strings.Index(text, "<think>")
		if start == -1 {
			break
		}
		end := strings.Index(text[start:], "</think>")
		if end == -1 {
			// Unclosed think tag — strip from <think> to end
			text = strings.TrimSpace(text[:start])
			break
		}
		text = text[:start] + text[start+end+len("</think>"):]
	}
	return strings.TrimSpace(text)
}

// extractInvokeStyleToolCalls handles the MiniMax/invoke format:
//
//	<minimax_tool_call>
//	  <invoke name="read_file">
//	    <parameter name="path">/some/file.txt</parameter>
//	  </invoke>
//	</minimax_tool_call>
//
// Also handles bare <invoke> without a wrapper tag.
func extractInvokeStyleToolCalls(text string) []ToolCall {
	var result []ToolCall

	remaining := text
	for {
		invokeStart := strings.Index(remaining, "<invoke ")
		if invokeStart == -1 {
			break
		}
		invokeClose := strings.Index(remaining[invokeStart:], "</invoke>")
		if invokeClose == -1 {
			break
		}
		invokeBlock := remaining[invokeStart : invokeStart+invokeClose+len("</invoke>")]
		remaining = remaining[invokeStart+invokeClose+len("</invoke>"):]

		// Extract tool name from <invoke name="...">
		toolName := extractQuotedValue(invokeBlock, "name")
		if toolName == "" {
			continue
		}

		// Extract <parameter name="key">value</parameter> pairs
		args := make(map[string]any)
		paramRemaining := invokeBlock
		for {
			pStart := strings.Index(paramRemaining, "<parameter ")
			if pStart == -1 {
				break
			}
			pNameAttr := extractQuotedValue(paramRemaining[pStart:], "name")
			tagEnd := strings.Index(paramRemaining[pStart:], ">")
			if tagEnd == -1 {
				break
			}
			valueStart := pStart + tagEnd + 1
			pClose := strings.Index(paramRemaining[valueStart:], "</parameter")
			if pClose == -1 {
				break
			}
			pValue := strings.TrimSpace(paramRemaining[valueStart : valueStart+pClose])
			if pNameAttr != "" {
				args[pNameAttr] = pValue
			}
			paramRemaining = paramRemaining[valueStart+pClose+len("</parameter>"):]
		}

		argsJSON, _ := json.Marshal(args)
		result = append(result, ToolCall{
			ID:        "xmltool_" + toolName,
			Name:      toolName,
			Arguments: args,
			Function: &FunctionCall{
				Name:      toolName,
				Arguments: string(argsJSON),
			},
		})
	}

	return result
}

// stripXMLToolCalls removes <tool_call>...</tool_call>, <function_call>...</function_call>,
// and <minimax_tool_call>...</minimax_tool_call> tags from text after they have been parsed.
func stripXMLToolCalls(text string) string {
	for _, tag := range []string{"tool_call", "function_call", "minimax_tool_call"} {
		openTag := "<" + tag + ">"
		closeTag := "</" + tag + ">"
		for {
			start := strings.Index(text, openTag)
			if start == -1 {
				break
			}
			end := strings.Index(text[start:], closeTag)
			if end == -1 {
				break
			}
			text = text[:start] + text[start+end+len(closeTag):]
		}
	}
	return strings.TrimSpace(text)
}

// stripToolCallsFromText removes tool call JSON from response text.
func stripToolCallsFromText(text string) string {
	start := strings.Index(text, `{"tool_calls"`)
	if start == -1 {
		return text
	}

	end := findMatchingBrace(text, start)
	if end == start {
		return text
	}

	return strings.TrimSpace(text[:start] + text[end:])
}
