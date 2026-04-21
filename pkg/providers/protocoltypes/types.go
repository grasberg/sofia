package protocoltypes

type ToolCall struct {
	ID               string         `json:"id"`
	Type             string         `json:"type,omitempty"`
	Function         *FunctionCall  `json:"function,omitempty"`
	Name             string         `json:"-"`
	Arguments        map[string]any `json:"-"`
	ThoughtSignature string         `json:"-"` // Internal use only
	ExtraContent     *ExtraContent  `json:"extra_content,omitempty"`
}

type ExtraContent struct {
	Google *GoogleExtra `json:"google,omitempty"`
}

type GoogleExtra struct {
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

type FunctionCall struct {
	Name             string `json:"name"`
	Arguments        string `json:"arguments"`
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

type LLMResponse struct {
	Content            string     `json:"content"`
	ReasoningContent   string     `json:"reasoning_content,omitempty"`
	ReasoningSignature string     `json:"reasoning_signature,omitempty"`
	ToolCalls          []ToolCall `json:"tool_calls,omitempty"`
	FinishReason       string     `json:"finish_reason"`
	Usage              *UsageInfo `json:"usage,omitempty"`
}

type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CacheControl marks a content block for LLM-side prefix caching.
// Currently only "ephemeral" is supported (used by Anthropic).
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// ContentBlock represents a structured segment of a system message.
// Adapters that understand SystemParts can use these blocks to set
// per-block cache control (e.g. Anthropic's cache_control: ephemeral).
type ContentBlock struct {
	Type         string        `json:"type"` // "text"
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

type Message struct {
	Role               string         `json:"role"`
	Content            string         `json:"content"`
	Images             []string       `json:"images,omitempty"` // base64 data URLs for vision (e.g. "data:image/png;base64,...")
	ReasoningContent   string         `json:"reasoning_content,omitempty"`
	ReasoningSignature string         `json:"reasoning_signature,omitempty"`
	SystemParts        []ContentBlock `json:"system_parts,omitempty"` // structured system blocks for cache-aware adapters
	ToolCalls          []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID         string         `json:"tool_call_id,omitempty"`
	ToolName           string         `json:"tool_name,omitempty"` // function name for tool result messages (required by Gemini)
}

type ToolDefinition struct {
	Type     string                 `json:"type"`
	Function ToolFunctionDefinition `json:"function"`
}

type ToolFunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type EmbeddingResult struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

// StreamChunk represents a chunk of a streaming LLM response.
//
// For mid-stream chunks, Delta carries an incremental text fragment and
// ToolCalls carries accumulating tool-call deltas for observability.
// When Done is true, Final (if non-nil) holds the fully aggregated response
// — finish_reason, complete tool_calls with parsed arguments, and usage —
// which callers can hand back to the agent loop in place of a non-streaming
// Chat() result. Providers that only stream text can leave Final nil; the
// agent-side collector will assemble one from the accumulated deltas.
type StreamChunk struct {
	Delta     string       `json:"delta"`                // Text content delta
	ToolCalls []ToolCall   `json:"tool_calls,omitempty"` // Incremental tool calls
	Done      bool         `json:"done"`                 // True when streaming is complete
	Final     *LLMResponse `json:"final,omitempty"`      // Aggregated response on the terminal chunk
}
