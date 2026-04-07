// Sofia - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Sofia contributors

package providers

import (
	"context"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/providers/openai_compat"
)

type HTTPProvider struct {
	delegate *openai_compat.Provider
}

func NewHTTPProvider(apiKey, apiBase, proxy string) *HTTPProvider {
	return &HTTPProvider{
		delegate: openai_compat.NewProvider(apiKey, apiBase, proxy),
	}
}

func NewHTTPProviderWithMaxTokensField(apiKey, apiBase, proxy, maxTokensField string) *HTTPProvider {
	return NewHTTPProviderWithMaxTokensFieldAndRequestTimeout(apiKey, apiBase, proxy, maxTokensField, 0)
}

func NewHTTPProviderWithMaxTokensFieldAndRequestTimeout(
	apiKey, apiBase, proxy, maxTokensField string,
	requestTimeoutSeconds int,
	opts ...openai_compat.Option,
) *HTTPProvider {
	allOpts := []openai_compat.Option{
		openai_compat.WithMaxTokensField(maxTokensField),
		openai_compat.WithRequestTimeout(time.Duration(requestTimeoutSeconds) * time.Second),
	}
	allOpts = append(allOpts, opts...)
	return &HTTPProvider{
		delegate: openai_compat.NewProvider(apiKey, apiBase, proxy, allOpts...),
	}
}

func (p *HTTPProvider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	resp, err := p.delegate.Chat(ctx, messages, tools, model, options)
	if err != nil {
		return resp, err
	}

	// Post-process: strip <think> tags and extract XML-style tool calls
	// that local models (Qwen, Ollama, DeepSeek R1) emit instead of
	// proper OpenAI function-calling JSON.
	if resp != nil && len(resp.ToolCalls) == 0 && resp.Content != "" {
		resp.Content = stripThinkTags(resp.Content)

		if xmlCalls := extractXMLToolCalls(resp.Content); len(xmlCalls) > 0 {
			resp.ToolCalls = xmlCalls
			resp.Content = stripXMLToolCalls(resp.Content)
		}
	}

	return resp, nil
}

func (p *HTTPProvider) ChatStream(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (<-chan StreamChunk, error) {
	ch, err := p.delegate.ChatStream(ctx, messages, tools, model, options)
	if err != nil {
		return ch, err
	}

	// Wrap the stream to strip <think> tags from streamed output.
	// We buffer chunks and suppress content inside <think>...</think>.
	out := make(chan StreamChunk, 64)
	go func() {
		defer close(out)
		inThink := false
		for chunk := range ch {
			if chunk.Delta == "" {
				out <- chunk
				continue
			}

			delta := chunk.Delta

			// Handle think tags that may span multiple chunks.
			for delta != "" {
				if inThink {
					end := strings.Index(delta, "</think>")
					if end == -1 {
						// Still inside <think>, suppress entire delta.
						delta = ""
					} else {
						// End of think block — skip past it.
						delta = delta[end+len("</think>"):]
						inThink = false
					}
				} else {
					start := strings.Index(delta, "<think>")
					if start == -1 {
						// No think tags — pass through.
						out <- StreamChunk{Delta: delta}
						delta = ""
					} else {
						// Emit content before <think>, enter think mode.
						if start > 0 {
							out <- StreamChunk{Delta: delta[:start]}
						}
						delta = delta[start+len("<think>"):]
						inThink = true
					}
				}
			}
		}
	}()

	return out, nil
}

func (p *HTTPProvider) GetDefaultModel() string {
	return ""
}

func (p *HTTPProvider) Embeddings(
	ctx context.Context,
	texts []string,
	model string,
) ([]EmbeddingResult, error) {
	return p.delegate.Embeddings(ctx, texts, model)
}
