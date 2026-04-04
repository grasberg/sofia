package providers

import (
	"context"
	"fmt"

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
	ContentBlock           = protocoltypes.ContentBlock
	CacheControl           = protocoltypes.CacheControl
	EmbeddingResult        = protocoltypes.EmbeddingResult
	StreamChunk            = protocoltypes.StreamChunk
)

type LLMProvider interface {
	Chat(
		ctx context.Context,
		messages []Message,
		tools []ToolDefinition,
		model string,
		options map[string]any,
	) (*LLMResponse, error)
	GetDefaultModel() string
}

type StatefulProvider interface {
	LLMProvider
	Close()
}

// FailoverReason classifies why an LLM request failed for fallback decisions.
type FailoverReason string

const (
	FailoverAuth       FailoverReason = "auth"
	FailoverRateLimit  FailoverReason = "rate_limit"
	FailoverBilling    FailoverReason = "billing"
	FailoverTimeout    FailoverReason = "timeout"
	FailoverFormat     FailoverReason = "format"
	FailoverOverloaded FailoverReason = "overloaded"
	FailoverUnknown    FailoverReason = "unknown"
)

// FailoverError wraps an LLM provider error with classification metadata.
type FailoverError struct {
	Reason   FailoverReason
	Provider string
	Model    string
	Status   int
	Wrapped  error
}

func (e *FailoverError) Error() string {
	return fmt.Sprintf("failover(%s): provider=%s model=%s status=%d: %v",
		e.Reason, e.Provider, e.Model, e.Status, e.Wrapped)
}

func (e *FailoverError) Unwrap() error {
	return e.Wrapped
}

// IsRetriable returns true if this error should trigger fallback to next candidate.
// Non-retriable: Format errors (bad request structure, image dimension/size).
func (e *FailoverError) IsRetriable() bool {
	return e.Reason != FailoverFormat
}

// StreamingProvider is an optional interface that providers can implement
// to support streaming responses. Use ChatStream for the final iteration
// when no tool calls are expected, to allow real-time output to users.
type StreamingProvider interface {
	LLMProvider
	ChatStream(
		ctx context.Context,
		messages []Message,
		tools []ToolDefinition,
		model string,
		options map[string]any,
	) (<-chan StreamChunk, error)
}

// EmbeddingProvider is an optional interface that providers can implement
// to support generating vector embeddings for text.
type EmbeddingProvider interface {
	LLMProvider
	Embeddings(
		ctx context.Context,
		texts []string,
		model string,
	) ([]EmbeddingResult, error)
}

// ModelConfig holds primary model and fallback list.
type ModelConfig struct {
	Primary   string
	Fallbacks []string
}
