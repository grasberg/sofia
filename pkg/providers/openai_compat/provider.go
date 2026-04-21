package openai_compat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/providers/protocoltypes"
)

type (
	ToolCall               = protocoltypes.ToolCall
	FunctionCall           = protocoltypes.FunctionCall
	LLMResponse            = protocoltypes.LLMResponse
	UsageInfo              = protocoltypes.UsageInfo
	Message                = protocoltypes.Message
	ToolDefinition         = protocoltypes.ToolDefinition
	ToolFunctionDefinition = protocoltypes.ToolFunctionDefinition
	ExtraContent           = protocoltypes.ExtraContent
	GoogleExtra            = protocoltypes.GoogleExtra
	EmbeddingResult        = protocoltypes.EmbeddingResult
	StreamChunk            = protocoltypes.StreamChunk
)

type Provider struct {
	apiKey         string
	apiBase        string
	maxTokensField string // Field name for max tokens (e.g., "max_completion_tokens" for o1/glm models)
	httpClient     *http.Client
	requestDelay   time.Duration          // delay before each request (rate-limit friendly)
	tokenSource    func() (string, error) // optional: refresh token before each request
}

type Option func(*Provider)

const defaultRequestTimeout = 120 * time.Second

func WithMaxTokensField(maxTokensField string) Option {
	return func(p *Provider) {
		p.maxTokensField = maxTokensField
	}
}

func WithRequestTimeout(timeout time.Duration) Option {
	return func(p *Provider) {
		if timeout > 0 {
			p.httpClient.Timeout = timeout
		}
	}
}

func WithRequestDelay(delay time.Duration) Option {
	return func(p *Provider) {
		if delay > 0 {
			p.requestDelay = delay
		}
	}
}

// WithTokenSource sets a function that returns a fresh access token before each request.
// If set, the returned token overrides the initial apiKey.
func WithTokenSource(ts func() (string, error)) Option {
	return func(p *Provider) {
		p.tokenSource = ts
	}
}

// resolveAPIKey returns the current API key, checking the token source if set.
func (p *Provider) resolveAPIKey() string {
	if p.tokenSource != nil {
		if tok, err := p.tokenSource(); err == nil && tok != "" {
			return tok
		}
	}
	return p.apiKey
}

func NewProvider(apiKey, apiBase, proxy string, opts ...Option) *Provider {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     20,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	if proxy != "" {
		parsed, err := url.Parse(proxy)
		if err == nil {
			transport.Proxy = http.ProxyURL(parsed)
		} else {
			log.Printf("openai_compat: invalid proxy URL %q: %v", proxy, err)
		}
	}

	client := &http.Client{
		Timeout:   defaultRequestTimeout,
		Transport: transport,
	}

	// Local models (Ollama) need a longer timeout — the model may need to
	// load into memory and process large prompts on consumer hardware.
	if isOllamaEndpoint(apiBase) && client.Timeout == defaultRequestTimeout {
		client.Timeout = 10 * time.Minute
	}

	p := &Provider{
		apiKey:     apiKey,
		apiBase:    strings.TrimRight(apiBase, "/"),
		httpClient: client,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}

	return p
}

func NewProviderWithMaxTokensField(apiKey, apiBase, proxy, maxTokensField string) *Provider {
	return NewProvider(apiKey, apiBase, proxy, WithMaxTokensField(maxTokensField))
}

func NewProviderWithMaxTokensFieldAndTimeout(
	apiKey, apiBase, proxy, maxTokensField string,
	requestTimeoutSeconds int,
) *Provider {
	return NewProvider(
		apiKey,
		apiBase,
		proxy,
		WithMaxTokensField(maxTokensField),
		WithRequestTimeout(time.Duration(requestTimeoutSeconds)*time.Second),
	)
}

// prepareRequest runs the shared pre-flight for Chat and ChatStream:
// it honours the request-delay knob, validates the API base, normalises the
// model id, and guards against missing keys on remote endpoints. Returns the
// (possibly rewritten) model id to use in the request body.
func (p *Provider) prepareRequest(ctx context.Context, model string) (string, error) {
	if p.requestDelay > 0 {
		select {
		case <-time.After(p.requestDelay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if p.apiBase == "" {
		return "", fmt.Errorf("API base not configured")
	}
	model = normalizeModel(model, p.apiBase)
	if err := p.preflightAPIKey(model); err != nil {
		return "", err
	}
	return model, nil
}

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

func (p *Provider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	model, err := p.prepareRequest(ctx, model)
	if err != nil {
		return nil, err
	}

	jsonData, err := json.Marshal(p.buildRequestBody(model, messages, tools, options, false))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if key := p.resolveAPIKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, p.formatHTTPError(resp.StatusCode, body, p.apiBase+"/chat/completions", model)
	}

	return parseResponse(body)
}

func (p *Provider) ChatStream(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (<-chan StreamChunk, error) {
	model, err := p.prepareRequest(ctx, model)
	if err != nil {
		return nil, err
	}

	jsonData, err := json.Marshal(p.buildRequestBody(model, messages, tools, options, true))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if key := p.resolveAPIKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
		resp.Body.Close()
		return nil, p.formatHTTPError(resp.StatusCode, body, p.apiBase+"/chat/completions", model)
	}

	ch := make(chan StreamChunk, 32)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		// Use a larger buffer: SSE frames for tool-call arguments can run
		// past the default 64KB scanner token size when the model returns
		// a long JSON argument payload.
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		acc := newStreamAccumulator()
		sendFinal := func() {
			select {
			case ch <- StreamChunk{Done: true, Final: acc.finalize()}:
			case <-ctx.Done():
			}
		}

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")

			if data == "[DONE]" {
				sendFinal()
				return
			}

			var chunk streamChunkPayload
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			// Usage is only emitted by providers that honour
			// stream_options.include_usage (OpenAI) and arrives on a
			// "choices: []" chunk right before [DONE]. Capture it for
			// the Final payload either way.
			if chunk.Usage != nil {
				acc.usage = chunk.Usage
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]

			if choice.Delta.Content != "" {
				acc.text.WriteString(choice.Delta.Content)
				select {
				case ch <- StreamChunk{Delta: choice.Delta.Content}:
				case <-ctx.Done():
					return
				}
			}
			if choice.Delta.ReasoningContent != "" {
				acc.reasoning.WriteString(choice.Delta.ReasoningContent)
			}
			for _, tcDelta := range choice.Delta.ToolCalls {
				acc.applyToolCallDelta(tcDelta)
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				acc.finishReason = *choice.FinishReason
			}
		}
		// Scanner ended without an explicit [DONE] (common with providers
		// that close the connection after the last "stop" chunk). Still
		// emit the terminal Final so callers see a consistent contract.
		sendFinal()
	}()

	return ch, nil
}

// streamChunkPayload is the SSE frame shape for /chat/completions streaming.
// Broken out so both the main loop and the accumulator type can refer to the
// same struct without duplicating the nested literal.
type streamChunkPayload struct {
	Choices []struct {
		Delta struct {
			Content          string               `json:"content"`
			ReasoningContent string               `json:"reasoning_content"`
			ToolCalls        []streamToolCallPart `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *UsageInfo `json:"usage"`
}

// streamToolCallPart is a single tool_call entry inside a delta. OpenAI
// streams tool calls by index: the first chunk for an index carries the id
// and function name, subsequent chunks extend function.arguments one token
// at a time until the call is complete.
type streamToolCallPart struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function *struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// streamAccumulator assembles a streaming response into a final LLMResponse.
// Tool calls arrive keyed by "index" and need arguments concatenated across
// chunks before the JSON can be parsed, so we can't build ToolCall values
// eagerly — we stash raw fragments and materialise them in finalize().
type streamAccumulator struct {
	text         strings.Builder
	reasoning    strings.Builder
	finishReason string
	usage        *UsageInfo
	toolCalls    map[int]*streamToolCallState
	toolOrder    []int
}

type streamToolCallState struct {
	id       string
	name     string
	argsText strings.Builder
}

func newStreamAccumulator() *streamAccumulator {
	return &streamAccumulator{
		toolCalls: make(map[int]*streamToolCallState),
	}
}

func (a *streamAccumulator) applyToolCallDelta(part streamToolCallPart) {
	state, ok := a.toolCalls[part.Index]
	if !ok {
		state = &streamToolCallState{}
		a.toolCalls[part.Index] = state
		a.toolOrder = append(a.toolOrder, part.Index)
	}
	if part.ID != "" {
		state.id = part.ID
	}
	if part.Function != nil {
		if part.Function.Name != "" {
			state.name = part.Function.Name
		}
		if part.Function.Arguments != "" {
			state.argsText.WriteString(part.Function.Arguments)
		}
	}
}

func (a *streamAccumulator) finalize() *LLMResponse {
	resp := &LLMResponse{
		Content:          a.text.String(),
		ReasoningContent: a.reasoning.String(),
		FinishReason:     a.finishReason,
		Usage:            a.usage,
	}
	if resp.FinishReason == "" {
		resp.FinishReason = "stop"
	}
	if len(a.toolOrder) == 0 {
		return resp
	}
	calls := make([]ToolCall, 0, len(a.toolOrder))
	for _, idx := range a.toolOrder {
		state := a.toolCalls[idx]
		args := make(map[string]any)
		if raw := strings.TrimSpace(state.argsText.String()); raw != "" {
			if err := json.Unmarshal([]byte(raw), &args); err != nil {
				// Preserve the raw string when the model produced
				// malformed JSON — the agent loop surfaces a clearer
				// error upstream than a silent drop would.
				args["raw"] = raw
			}
		}
		calls = append(calls, ToolCall{
			ID:        state.id,
			Name:      state.name,
			Arguments: args,
		})
	}
	resp.ToolCalls = calls
	return resp
}

func parseResponse(body []byte) (*LLMResponse, error) {
	var apiResponse struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function *struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
					ExtraContent *struct {
						Google *struct {
							ThoughtSignature string `json:"thought_signature"`
						} `json:"google"`
					} `json:"extra_content"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *UsageInfo `json:"usage"`
	}

	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(apiResponse.Choices) == 0 {
		return &LLMResponse{
			Content:      "",
			FinishReason: "stop",
		}, nil
	}

	choice := apiResponse.Choices[0]
	toolCalls := make([]ToolCall, 0, len(choice.Message.ToolCalls))
	for _, tc := range choice.Message.ToolCalls {
		arguments := make(map[string]any)
		name := ""

		// Extract thought_signature from Gemini/Google-specific extra content
		thoughtSignature := ""
		if tc.ExtraContent != nil && tc.ExtraContent.Google != nil {
			thoughtSignature = tc.ExtraContent.Google.ThoughtSignature
		}

		if tc.Function != nil {
			name = tc.Function.Name
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &arguments); err != nil {
					log.Printf("openai_compat: failed to decode tool call arguments for %q: %v", name, err)
					arguments["raw"] = tc.Function.Arguments
				}
			}
		}

		// Build ToolCall with ExtraContent for Gemini 3 thought_signature persistence
		toolCall := ToolCall{
			ID:               tc.ID,
			Name:             name,
			Arguments:        arguments,
			ThoughtSignature: thoughtSignature,
		}

		if thoughtSignature != "" {
			toolCall.ExtraContent = &ExtraContent{
				Google: &GoogleExtra{
					ThoughtSignature: thoughtSignature,
				},
			}
		}

		toolCalls = append(toolCalls, toolCall)
	}

	return &LLMResponse{
		Content:          choice.Message.Content,
		ReasoningContent: choice.Message.ReasoningContent,
		ToolCalls:        toolCalls,
		FinishReason:     choice.FinishReason,
		Usage:            apiResponse.Usage,
	}, nil
}

func (p *Provider) Embeddings(
	ctx context.Context,
	texts []string,
	model string,
) ([]EmbeddingResult, error) {
	if p.apiBase == "" {
		return nil, fmt.Errorf("API base not configured")
	}

	model = normalizeModel(model, p.apiBase)

	if err := p.preflightAPIKey(model); err != nil {
		return nil, err
	}

	requestBody := map[string]any{
		"model": model,
		"input": texts,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/embeddings", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if key := p.resolveAPIKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, p.formatHTTPError(resp.StatusCode, body, p.apiBase+"/embeddings", model)
	}

	var apiResponse struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	results := make([]EmbeddingResult, len(apiResponse.Data))
	for i, d := range apiResponse.Data {
		results[i] = EmbeddingResult{
			Embedding: d.Embedding,
			Index:     d.Index,
		}
	}

	return results, nil
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

func normalizeModel(model, apiBase string) string {
	idx := strings.Index(model, "/")
	if idx == -1 {
		return model
	}

	if strings.Contains(strings.ToLower(apiBase), "openrouter.ai") ||
		strings.HasSuffix(model, ":free") || strings.HasSuffix(model, ":extended") {
		return model
	}

	prefix := strings.ToLower(model[:idx])
	switch prefix {
	case "moonshot", "nvidia", "groq", "ollama", "deepseek", "google", "openrouter", "zhipu", "mistral":
		return model[idx+1:]
	default:
		return model
	}
}

// inferMaxTokensField picks the request body field name for the output-token
// cap when the provider config doesn't set one explicitly. OpenAI's reasoning
// models (o1/o3/o4/…) and all GPT-5 variants reject the classic "max_tokens"
// key and require "max_completion_tokens"; Z.ai's GLM family behaves the same.
// Everything else uses the conventional "max_tokens".
//
// Matching is prefix-plus-separator so embedding a family name in an
// unrelated id (e.g. "kimi-o1-preview", "foo-glm-tuned") does not trigger the
// reasoning-model branch. The caller is responsible for passing a lowercased
// id — buildRequestBody lowers once and reuses the same value for the Kimi
// temperature check, so re-lowering here would be wasted work on every call.
func inferMaxTokensField(lowerModel string) string {
	for _, prefix := range maxCompletionTokensPrefixes {
		if lowerModel == prefix ||
			strings.HasPrefix(lowerModel, prefix+"-") ||
			strings.HasPrefix(lowerModel, prefix+".") ||
			strings.HasPrefix(lowerModel, prefix+":") {
			return "max_completion_tokens"
		}
	}
	return "max_tokens"
}

// maxCompletionTokensPrefixes is the set of model-id prefixes whose APIs
// reject the classic "max_tokens" request key. Additions should be API
// contracts, not marketing names — if a provider adds a new reasoning family
// that accepts "max_tokens", do not list it here.
var maxCompletionTokensPrefixes = []string{"o1", "o3", "o4", "o5", "gpt-5", "glm"}

func isOllamaEndpoint(apiBase string) bool {
	return strings.Contains(apiBase, "localhost:11434") ||
		strings.Contains(apiBase, "127.0.0.1:11434") ||
		strings.Contains(apiBase, "ollama.com")
}

// formatHTTPError turns a non-2xx HTTP response into a helpful error. The
// default format is terse ("Status: X / Body: ..."), but for 404 — which
// almost always means the URL or model id is wrong rather than a real
// "not found" — we add the requested URL, the model, and (when possible)
// a list of models actually hosted at the endpoint, pulled live from the
// /models API. That turns "404 page not found" from a dead end into a
// fix-it-in-30-seconds error.
func (p *Provider) formatHTTPError(statusCode int, body []byte, requestURL, model string) error {
	if statusCode == http.StatusNotFound {
		hint := "either the API base is wrong (should normally end in \"/v1\" for " +
			"OpenAI-compatible providers) or the model id is not hosted at this endpoint. " +
			"Check Settings → AI Models."
		// Probe /models on the same base with a tight timeout. If it
		// succeeds, surfacing even a handful of valid ids is usually enough
		// for the user to spot the typo (case mismatch, wrong org, etc.).
		if ids := p.sampleAvailableModels(); len(ids) > 0 {
			hint += "\n  Available at this endpoint: " + strings.Join(ids, ", ")
		}
		return fmt.Errorf(
			"API request failed:\n"+
				"  Status: 404\n"+
				"  URL:    %s\n"+
				"  Model:  %q\n"+
				"  Body:   %s\n"+
				"  Hint:   %s",
			requestURL, model, strings.TrimSpace(string(body)), hint)
	}
	return fmt.Errorf("API request failed:\n  Status: %d\n  Body:   %s",
		statusCode, string(body))
}

// sampleAvailableModels performs a best-effort GET /v1/models against the
// provider's base URL and returns up to 10 model IDs. Any error (timeout,
// non-200, parse failure) returns an empty slice — this is a diagnostic
// enhancement, never a blocker.
func (p *Provider) sampleAvailableModels() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", p.apiBase+"/models", nil)
	if err != nil {
		return nil
	}
	if key := p.resolveAPIKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil
	}

	var decoded struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	out := make([]string, 0, 10)
	for _, m := range decoded.Data {
		if m.ID == "" {
			continue
		}
		out = append(out, m.ID)
		if len(out) >= 10 {
			break
		}
	}
	return out
}

// hostRequiresAPIKey reports whether the apiBase is a remote endpoint that
// normally needs an Authorization header. Local development servers
// (localhost, 127.0.0.1, 0.0.0.0) are assumed keyless — this matches how
// Ollama and similar self-hosted runners behave.
func hostRequiresAPIKey(apiBase string) bool {
	lower := strings.ToLower(apiBase)
	if strings.Contains(lower, "://localhost") ||
		strings.Contains(lower, "://127.0.0.1") ||
		strings.Contains(lower, "://0.0.0.0") {
		return false
	}
	return true
}

// preflightAPIKey fails the request before it hits the network when the
// provider has no API key but the endpoint clearly needs one. The error
// body deliberately mimics an HTTP 401 so the fallback chain's error
// classifier treats it as an auth failure (same as a real 401), which
// triggers the user-friendly "update your keys in Settings" header.
func (p *Provider) preflightAPIKey(model string) error {
	if p.resolveAPIKey() != "" {
		return nil
	}
	if !hostRequiresAPIKey(p.apiBase) {
		return nil
	}
	return fmt.Errorf("API request failed:\n  Status: 401\n  Body:   "+
		"no api key found for %s (model %q) — set it in Settings → AI Models",
		p.apiBase, model)
}

func asInt(v any) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	case float32:
		return int(val), true
	default:
		return 0, false
	}
}

func asFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	default:
		return 0, false
	}
}
