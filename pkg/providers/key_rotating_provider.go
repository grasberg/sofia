package providers

import (
	"context"
	"errors"
	"fmt"
)

// rotationTriggers defines which error reasons should trigger key rotation.
// Rate limit, auth, and billing failures mean the current key is exhausted.
// Timeout, overloaded, and format errors are not key-specific.
func isKeyRotationReason(reason FailoverReason) bool {
	return reason == FailoverRateLimit || reason == FailoverAuth || reason == FailoverBilling
}

// keyedProvider pairs an API key with the provider instance that uses it.
type keyedProvider struct {
	key      string
	provider LLMProvider
}

// KeyRotatingProvider wraps multiple LLMProvider instances (one per API key) and
// rotates between them when a key is exhausted. It transparently implements
// LLMProvider, StreamingProvider, and EmbeddingProvider so that callers need
// no awareness of the rotation.
//
// Rotation behavior:
//   - On FailoverRateLimit / FailoverAuth / FailoverBilling → mark key exhausted, try next key.
//   - On context cancellation → return immediately.
//   - On other errors → return immediately (format error, etc.).
//   - When all keys are exhausted → return a descriptive error.
type KeyRotatingProvider struct {
	pool      *KeyPool
	providers map[string]LLMProvider // key string → provider
}

// NewKeyRotatingProvider constructs a KeyRotatingProvider from a key pool and a
// slice of (key, provider) pairs. The order in kp must match the pool's key order.
func NewKeyRotatingProvider(pool *KeyPool, kp []keyedProvider) *KeyRotatingProvider {
	m := make(map[string]LLMProvider, len(kp))
	for _, kv := range kp {
		m[kv.key] = kv.provider
	}
	return &KeyRotatingProvider{pool: pool, providers: m}
}

// Chat implements LLMProvider. It selects a key, calls the underlying provider,
// and rotates to the next key on exhaustion errors.
func (p *KeyRotatingProvider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	maxAttempts := p.pool.Len()
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}

		key := p.pool.Select()
		if key == "" {
			return nil, fmt.Errorf("key_pool: all API keys exhausted (%s)", p.pool.Status())
		}

		resp, err := p.providers[key].Chat(ctx, messages, tools, model, options)
		if err == nil {
			p.pool.MarkSuccess(key)
			return resp, nil
		}

		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}

		if reason, status := classifyKeyError(err); isKeyRotationReason(reason) {
			p.pool.MarkExhausted(key, status)
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("key_pool: all %d API keys exhausted after rotation (%s)",
		p.pool.Len(), p.pool.Status())
}

// ChatStream implements StreamingProvider with the same key-rotation logic.
func (p *KeyRotatingProvider) ChatStream(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (<-chan StreamChunk, error) {
	maxAttempts := p.pool.Len()
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}

		key := p.pool.Select()
		if key == "" {
			return nil, fmt.Errorf("key_pool: all API keys exhausted (%s)", p.pool.Status())
		}

		sp, ok := p.providers[key].(StreamingProvider)
		if !ok {
			// Provider doesn't support streaming; fall back to Chat.
			return nil, fmt.Errorf("key_pool: provider for key does not support streaming")
		}

		ch, err := sp.ChatStream(ctx, messages, tools, model, options)
		if err == nil {
			p.pool.MarkSuccess(key)
			return ch, nil
		}

		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}

		if reason, status := classifyKeyError(err); isKeyRotationReason(reason) {
			p.pool.MarkExhausted(key, status)
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("key_pool: all %d API keys exhausted after rotation (%s)",
		p.pool.Len(), p.pool.Status())
}

// Embeddings implements EmbeddingProvider. Uses the first available key.
func (p *KeyRotatingProvider) Embeddings(
	ctx context.Context,
	texts []string,
	model string,
) ([]EmbeddingResult, error) {
	maxAttempts := p.pool.Len()
	for attempt := 0; attempt < maxAttempts; attempt++ {
		key := p.pool.Select()
		if key == "" {
			return nil, fmt.Errorf("key_pool: all API keys exhausted (%s)", p.pool.Status())
		}

		ep, ok := p.providers[key].(EmbeddingProvider)
		if !ok {
			return nil, fmt.Errorf("key_pool: provider does not support embeddings")
		}

		results, err := ep.Embeddings(ctx, texts, model)
		if err == nil {
			p.pool.MarkSuccess(key)
			return results, nil
		}

		if reason, status := classifyKeyError(err); isKeyRotationReason(reason) {
			p.pool.MarkExhausted(key, status)
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("key_pool: all %d API keys exhausted after rotation", p.pool.Len())
}

// GetDefaultModel delegates to the first configured provider.
func (p *KeyRotatingProvider) GetDefaultModel() string {
	key := p.pool.Select()
	if key == "" {
		return ""
	}
	type defaultModeler interface {
		GetDefaultModel() string
	}
	if dm, ok := p.providers[key].(defaultModeler); ok {
		return dm.GetDefaultModel()
	}
	return ""
}

// classifyKeyError extracts a FailoverReason and HTTP status from an error.
// Returns (FailoverUnknown, 0) when the error is not a recognized provider error.
func classifyKeyError(err error) (FailoverReason, int) {
	var fe *FailoverError
	if errors.As(err, &fe) {
		return fe.Reason, fe.Status
	}
	// Fallback: ask the error classifier to parse the message.
	classified := ClassifyError(err, "", "")
	if classified != nil {
		return classified.Reason, classified.Status
	}
	return FailoverUnknown, 0
}
