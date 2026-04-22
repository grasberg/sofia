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
