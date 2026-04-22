package openai_compat

import (
	"strings"
)

// buildRequestBody assembles the /chat/completions payload shared by Chat and
// ChatStream. `stream` toggles the SSE flag; everything else (tool choice,
// max-tokens field routing, Kimi temperature pin, prompt caching, Ollama
// num_ctx) is identical across both call paths.
//
// `model` is assumed to already be post-normalizeModel. `lowerModel` is
// computed once here and shared across the downstream checks so we don't
// re-allocate a lowered copy for each string.Contains.
func (p *Provider) buildRequestBody(
	model string,
	messages []Message,
	tools []ToolDefinition,
	options map[string]any,
	stream bool,
) map[string]any {
	requestBody := map[string]any{
		"model":    model,
		"messages": stripSystemParts(messages),
	}
	if stream {
		requestBody["stream"] = true
	}
	if len(tools) > 0 {
		requestBody["tools"] = tools
		requestBody["tool_choice"] = "auto"
	}

	lowerModel := strings.ToLower(model)
	lowerBase := strings.ToLower(p.apiBase)

	if maxTokens, ok := asInt(options["max_tokens"]); ok {
		// DeepSeek strictly rejects max_tokens > 8192.
		if strings.Contains(lowerBase, "api.deepseek.com") && maxTokens > 8192 {
			maxTokens = 8192
		}
		fieldName := p.maxTokensField
		if fieldName == "" {
			fieldName = inferMaxTokensField(lowerModel)
		}
		requestBody[fieldName] = maxTokens
	}

	if temperature, ok := asFloat(options["temperature"]); ok {
		// Kimi k2 models only support temperature=1.
		if strings.Contains(lowerModel, "kimi") && strings.Contains(lowerModel, "k2") {
			requestBody["temperature"] = 1.0
		} else {
			requestBody["temperature"] = temperature
		}
	}

	// Prompt caching: skip for endpoints that don't honour the key (Gemini's
	// OpenAI-compat surface and Ollama). Applied to streaming too — servers
	// that don't support it simply ignore the field.
	if cacheKey, ok := options["prompt_cache_key"].(string); ok && cacheKey != "" {
		if !strings.Contains(p.apiBase, "generativelanguage.googleapis.com") &&
			!isOllamaEndpoint(p.apiBase) {
			requestBody["prompt_cache_key"] = cacheKey
		}
	}

	// Ollama: raise num_ctx from the 2048-token default so the local model
	// can hold the system prompt + tools + history.
	if isOllamaEndpoint(p.apiBase) {
		numCtx := 8192
		if maxTokens, ok := asInt(options["max_tokens"]); ok && maxTokens > 0 {
			numCtx = maxTokens
		}
		requestBody["options"] = map[string]any{"num_ctx": numCtx}
	}

	return requestBody
}

// openaiMessage is the wire-format message for OpenAI-compatible APIs.
// Content can be a plain string OR an array of content parts for vision (image_url).
// We use json.RawMessage so we can emit either format depending on context.
type openaiMessage struct {
	Role             string     `json:"role"`
	Content          any        `json:"content"`                     // string OR []openaiContentPart for vision
	ReasoningContent string     `json:"reasoning_content,omitempty"` // DeepSeek thinking mode
	Name             string     `json:"name,omitempty"`              // function name for tool result messages (required by Gemini)
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

type openaiContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *openaiImageURL `json:"image_url,omitempty"`
}

type openaiImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "auto" | "low" | "high"
}

// stripSystemParts converts []Message to []openaiMessage.
// For user messages with images, it builds the vision content array format.
// SystemParts is dropped so it doesn't leak into third-party endpoints.
func stripSystemParts(messages []Message) []openaiMessage {
	out := make([]openaiMessage, len(messages))
	for i, m := range messages {
		if m.Role == "user" && len(m.Images) > 0 {
			// Build vision content array
			var parts []openaiContentPart
			if m.Content != "" {
				parts = append(parts, openaiContentPart{Type: "text", Text: m.Content})
			}
			for _, dataURL := range m.Images {
				parts = append(parts, openaiContentPart{
					Type:     "image_url",
					ImageURL: &openaiImageURL{URL: dataURL, Detail: "auto"},
				})
			}
			out[i] = openaiMessage{
				Role:             m.Role,
				Content:          parts,
				ReasoningContent: m.ReasoningContent,
				Name:             m.ToolName,
				ToolCalls:        m.ToolCalls,
				ToolCallID:       m.ToolCallID,
			}
		} else {
			out[i] = openaiMessage{
				Role:             m.Role,
				Content:          m.Content,
				ReasoningContent: m.ReasoningContent,
				Name:             m.ToolName,
				ToolCalls:        m.ToolCalls,
				ToolCallID:       m.ToolCallID,
			}
		}
	}
	return out
}
