package agent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamingMockProvider implements both LLMProvider and StreamingProvider.
// ChatStream fans out the configured tokens as individual deltas and closes
// with a terminal Final chunk, mirroring how the openai_compat provider
// talks to CollectStream in production.
type streamingMockProvider struct {
	tokens       []string
	finishReason string
	toolCalls    []providers.ToolCall
	chatCalls    int
	streamCalls  int
}

func (m *streamingMockProvider) GetDefaultModel() string { return "mock-stream" }

func (m *streamingMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	m.chatCalls++
	return &providers.LLMResponse{
		Content:      strings.Join(m.tokens, ""),
		FinishReason: m.finishReason,
		ToolCalls:    m.toolCalls,
	}, nil
}

func (m *streamingMockProvider) ChatStream(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (<-chan providers.StreamChunk, error) {
	m.streamCalls++
	ch := make(chan providers.StreamChunk, len(m.tokens)+1)
	go func() {
		defer close(ch)
		for _, tok := range m.tokens {
			select {
			case ch <- providers.StreamChunk{Delta: tok}:
			case <-ctx.Done():
				return
			}
		}
		ch <- providers.StreamChunk{
			Done: true,
			Final: &providers.LLMResponse{
				Content:      strings.Join(m.tokens, ""),
				FinishReason: m.finishReason,
				ToolCalls:    m.toolCalls,
			},
		}
	}()
	return ch, nil
}

// TestProcessDirectStream_DeliversDeltasInOrder verifies the real streaming
// path: ChatStream is picked over Chat when the callback is set, each token
// is forwarded to the caller, and the terminal Done marker fires exactly
// once after all deltas.
func TestProcessDirectStream_DeliversDeltasInOrder(t *testing.T) {
	cfg := testAgentCfg(t)
	mock := &streamingMockProvider{
		tokens:       []string{"Hel", "lo", " ", "world"},
		finishReason: "stop",
	}
	al := NewAgentLoop(cfg, bus.NewMessageBus(), mock)

	var deltas []string
	doneCount := 0
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := al.ProcessDirectStream(ctx, "hi", "stream-session", func(text string, done bool) {
		if done {
			doneCount++
			return
		}
		deltas = append(deltas, text)
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"Hel", "lo", " ", "world"}, deltas,
		"every token from ChatStream should arrive via the callback in order")
	assert.Equal(t, 1, doneCount, "done signal fires exactly once")
	assert.Equal(t, 1, mock.streamCalls, "agent loop used the streaming path")
	assert.Equal(t, 0, mock.chatCalls, "non-streaming Chat was not called")
}

// TestProcessDirectStream_FallsBackWhenNoDeltas covers providers that don't
// emit any text (for example a turn that ends immediately with tool_calls
// only, or a non-streaming provider). The caller must still receive the
// agent-loop's final response via a single chunk so the terminal UX doesn't
// need to special-case "empty stream".
func TestProcessDirectStream_FallsBackWhenNoDeltas(t *testing.T) {
	cfg := testAgentCfg(t)
	// Use a non-streaming provider so the agent loop takes the plain
	// Chat path. ProcessDirectStream must still deliver the full reply
	// text — this covers the "legacy provider" case where streaming
	// isn't available.
	al := NewAgentLoop(cfg, bus.NewMessageBus(), &fallbackMockProvider{response: "done."})

	var deltas []string
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := al.ProcessDirectStream(ctx, "hi", "fallback-session", func(text string, done bool) {
		if done {
			return
		}
		deltas = append(deltas, text)
	})
	require.NoError(t, err)
	require.Len(t, deltas, 1, "non-streaming provider yields one final chunk")
	assert.Equal(t, "done.", deltas[0])
}

// fallbackMockProvider is a minimal non-streaming provider — it only
// implements Chat, so the agent loop cannot pick the ChatStream path.
type fallbackMockProvider struct {
	response string
}

func (m *fallbackMockProvider) GetDefaultModel() string { return "mock-nonstream" }

func (m *fallbackMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content:      m.response,
		FinishReason: "stop",
	}, nil
}

// testAgentCfg builds a minimal AgentLoop config in an isolated workspace.
func testAgentCfg(t *testing.T) *config.Config {
	t.Helper()
	tmp, err := os.MkdirTemp("", "agent-stream-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })
	return &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmp,
				Model:             "mock-model",
				MaxTokens:         4096,
				MaxToolIterations: 5,
			},
		},
	}
}
