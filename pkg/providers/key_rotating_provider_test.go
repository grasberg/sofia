package providers

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/config"
)

// stubProvider is a minimal LLMProvider for testing that returns a fixed
// error or response based on the API key it was constructed with.
type stubProvider struct {
	key     string
	respErr error
}

func (s *stubProvider) Chat(
	_ context.Context,
	_ []Message,
	_ []ToolDefinition,
	_ string,
	_ map[string]any,
) (*LLMResponse, error) {
	if s.respErr != nil {
		return nil, s.respErr
	}
	return &LLMResponse{Content: "ok:" + s.key}, nil
}

func (s *stubProvider) GetDefaultModel() string { return "" }

// rateLimitErr creates a *FailoverError that looks like an HTTP 429.
func rateLimitErr() error {
	return &FailoverError{Reason: FailoverRateLimit, Status: 429}
}

// authErr creates a *FailoverError that looks like an HTTP 401.
func authErr() error {
	return &FailoverError{Reason: FailoverAuth, Status: 401}
}

func TestKeyRotatingProvider_SuccessOnFirstKey(t *testing.T) {
	pool := NewKeyPool([]string{"k1", "k2"}, PoolStrategyFillFirst)
	kp := []keyedProvider{
		{key: "k1", provider: &stubProvider{key: "k1"}},
		{key: "k2", provider: &stubProvider{key: "k2"}},
	}
	p := NewKeyRotatingProvider(pool, kp)

	resp, err := p.Chat(context.Background(), nil, nil, "model", nil)
	require.NoError(t, err)
	assert.Equal(t, "ok:k1", resp.Content)
}

func TestKeyRotatingProvider_RotatesOnRateLimit(t *testing.T) {
	pool := NewKeyPool([]string{"k1", "k2"}, PoolStrategyFillFirst)
	kp := []keyedProvider{
		{key: "k1", provider: &stubProvider{key: "k1", respErr: rateLimitErr()}},
		{key: "k2", provider: &stubProvider{key: "k2"}},
	}
	p := NewKeyRotatingProvider(pool, kp)

	resp, err := p.Chat(context.Background(), nil, nil, "model", nil)
	require.NoError(t, err)
	assert.Equal(t, "ok:k2", resp.Content)
}

func TestKeyRotatingProvider_RotatesOnAuthError(t *testing.T) {
	pool := NewKeyPool([]string{"k1", "k2"}, PoolStrategyFillFirst)
	kp := []keyedProvider{
		{key: "k1", provider: &stubProvider{key: "k1", respErr: authErr()}},
		{key: "k2", provider: &stubProvider{key: "k2"}},
	}
	p := NewKeyRotatingProvider(pool, kp)

	resp, err := p.Chat(context.Background(), nil, nil, "model", nil)
	require.NoError(t, err)
	assert.Equal(t, "ok:k2", resp.Content)
}

func TestKeyRotatingProvider_AllExhausted(t *testing.T) {
	pool := NewKeyPool([]string{"k1", "k2"}, PoolStrategyFillFirst)
	kp := []keyedProvider{
		{key: "k1", provider: &stubProvider{key: "k1", respErr: rateLimitErr()}},
		{key: "k2", provider: &stubProvider{key: "k2", respErr: rateLimitErr()}},
	}
	p := NewKeyRotatingProvider(pool, kp)

	_, err := p.Chat(context.Background(), nil, nil, "model", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exhausted")
}

func TestKeyRotatingProvider_NonRotatableErrorReturnsImmediately(t *testing.T) {
	pool := NewKeyPool([]string{"k1", "k2"}, PoolStrategyFillFirst)
	formatErr := &FailoverError{Reason: FailoverFormat, Status: 400}
	kp := []keyedProvider{
		{key: "k1", provider: &stubProvider{key: "k1", respErr: formatErr}},
		{key: "k2", provider: &stubProvider{key: "k2"}},
	}
	p := NewKeyRotatingProvider(pool, kp)

	_, err := p.Chat(context.Background(), nil, nil, "model", nil)
	require.Error(t, err)
	// Should NOT have used k2 — format error is not a key problem.
	var fe *FailoverError
	require.True(t, errors.As(err, &fe))
	assert.Equal(t, FailoverFormat, fe.Reason)
}

func TestKeyRotatingProvider_ContextCancellation(t *testing.T) {
	pool := NewKeyPool([]string{"k1"}, PoolStrategyFillFirst)
	kp := []keyedProvider{
		{key: "k1", provider: &stubProvider{key: "k1", respErr: fmt.Errorf("some error")}},
	}
	p := NewKeyRotatingProvider(pool, kp)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := p.Chat(ctx, nil, nil, "model", nil)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCreateProviderFromConfig_MultipleKeys(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-multi-key",
		Model:     "openai/gpt-4o",
		APIKey:    "key1",
		APIKeys:   []string{"key2", "key3"},
		APIBase:   "https://api.example.com/v1",
	}

	provider, modelID, err := CreateProviderFromConfig(cfg)
	require.NoError(t, err)
	assert.Equal(t, "gpt-4o", modelID)
	assert.IsType(t, &KeyRotatingProvider{}, provider)
}

func TestCreateProviderFromConfig_SingleKeyNoRotation(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-single-key",
		Model:     "openai/gpt-4o",
		APIKey:    "key1",
		APIBase:   "https://api.example.com/v1",
	}

	provider, _, err := CreateProviderFromConfig(cfg)
	require.NoError(t, err)
	assert.IsType(t, &HTTPProvider{}, provider)
}

func TestCreateProviderFromConfig_PoolStrategyRoundRobin(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName:    "test-rr",
		Model:        "openai/gpt-4o",
		APIKey:       "key1",
		APIKeys:      []string{"key2"},
		PoolStrategy: "round_robin",
		APIBase:      "https://api.example.com/v1",
	}

	provider, _, err := CreateProviderFromConfig(cfg)
	require.NoError(t, err)
	krp, ok := provider.(*KeyRotatingProvider)
	require.True(t, ok)
	assert.Equal(t, PoolStrategyRoundRobin, krp.pool.strategy)
}
