package providers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCollectStream_UsesFinalWhenProvided verifies the happy path: a stream
// that emits text deltas plus a terminal Final chunk returns the Final as-is
// and forwards every delta to the callback in order.
func TestCollectStream_UsesFinalWhenProvided(t *testing.T) {
	ch := make(chan StreamChunk, 4)
	ch <- StreamChunk{Delta: "Hello"}
	ch <- StreamChunk{Delta: " world"}
	ch <- StreamChunk{Done: true, Final: &LLMResponse{
		Content:      "Hello world",
		FinishReason: "stop",
		ToolCalls: []ToolCall{{
			ID: "call_1", Name: "noop", Arguments: map[string]any{},
		}},
	}}
	close(ch)

	var received []string
	resp, err := CollectStream(context.Background(), ch, func(d string) {
		received = append(received, d)
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, []string{"Hello", " world"}, received, "deltas forwarded in order")
	assert.Equal(t, "Hello world", resp.Content)
	assert.Equal(t, "stop", resp.FinishReason)
	assert.Len(t, resp.ToolCalls, 1, "Final's tool_calls must survive the collector")
}

// TestCollectStream_SynthesizesFromDeltasWhenFinalMissing covers providers
// that stream text but don't populate Final (older or simpler implementations).
// The collector must still return a coherent LLMResponse so the agent loop's
// tool-execution path doesn't see a nil pointer.
func TestCollectStream_SynthesizesFromDeltasWhenFinalMissing(t *testing.T) {
	ch := make(chan StreamChunk, 3)
	ch <- StreamChunk{Delta: "abc"}
	ch <- StreamChunk{Delta: "def"}
	ch <- StreamChunk{Done: true} // no Final
	close(ch)

	resp, err := CollectStream(context.Background(), ch, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "abcdef", resp.Content, "fallback content rebuilt from deltas")
	assert.Equal(t, "stop", resp.FinishReason)
	assert.Empty(t, resp.ToolCalls)
}

// TestCollectStream_NilCallbackIsSafe verifies that callers that want the
// final response without live rendering (e.g. programmatic consumers) can
// pass nil and still get a valid result.
func TestCollectStream_NilCallbackIsSafe(t *testing.T) {
	ch := make(chan StreamChunk, 2)
	ch <- StreamChunk{Delta: "x"}
	ch <- StreamChunk{Done: true, Final: &LLMResponse{Content: "x", FinishReason: "stop"}}
	close(ch)

	resp, err := CollectStream(context.Background(), ch, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "x", resp.Content)
}

// TestCollectStream_ContextCanceledBeforeDone covers the case where the
// producer is canceled and closes the channel without emitting a Done chunk.
// The collector should surface the context error rather than returning a
// synthesised "success" response that masks the cancel.
func TestCollectStream_ContextCanceledBeforeDone(t *testing.T) {
	ch := make(chan StreamChunk)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
		close(ch)
	}()

	resp, err := CollectStream(ctx, ch, nil)
	assert.Nil(t, resp, "no response when canceled before Done")
	assert.ErrorIs(t, err, context.Canceled)
}
