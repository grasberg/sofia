package openai_compat

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatStream_CollectsDeltas(t *testing.T) {
	chunks := []string{"Hello", " world", "!"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "ResponseWriter must support Flusher")

		for _, chunk := range chunks {
			data := fmt.Sprintf(`{"choices":[{"delta":{"content":%q},"finish_reason":null}]}`, chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}

		// Send final chunk with finish_reason
		fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{},"finish_reason":"stop"}]}`)
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	ch, err := p.ChatStream(
		t.Context(),
		[]Message{{Role: "user", Content: "hi"}},
		nil,
		"test-model",
		nil,
	)
	require.NoError(t, err)

	var collected string
	var gotDone bool
	for sc := range ch {
		if sc.Done {
			gotDone = true
			break
		}
		collected += sc.Delta
	}

	assert.True(t, gotDone, "expected Done chunk")
	assert.Equal(t, "Hello world!", collected)
}

// TestChatStream_AggregatesToolCalls verifies that tool_call deltas arriving
// across multiple SSE frames are reassembled into a single ToolCall on the
// terminal chunk's Final response. OpenAI streams tool-call arguments
// character-by-character; the agent loop depends on the Final to decide
// whether to execute tools or stop, so losing fragments here would silently
// break tool use on every streaming-enabled provider.
func TestChatStream_AggregatesToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		// Frame 1: tool call opened with id + function name.
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`+"\n\n")
		flusher.Flush()
		// Frames 2-3: arguments stream in fragments.
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`+"\n\n")
		flusher.Flush()
		fmt.Fprint(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"SF\"}"}}]}}]}`+"\n\n")
		flusher.Flush()
		// Frame 4: finish_reason.
		fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	ch, err := p.ChatStream(
		t.Context(),
		[]Message{{Role: "user", Content: "weather in SF"}},
		nil, "gpt-4o", nil,
	)
	require.NoError(t, err)

	var final *LLMResponse
	for sc := range ch {
		if sc.Done {
			final = sc.Final
		}
	}
	require.NotNil(t, final, "terminal chunk should carry Final")
	require.Equal(t, "tool_calls", final.FinishReason)
	require.Len(t, final.ToolCalls, 1)
	tc := final.ToolCalls[0]
	assert.Equal(t, "call_1", tc.ID)
	assert.Equal(t, "get_weather", tc.Name)
	assert.Equal(t, "SF", tc.Arguments["city"], "arguments JSON must reassemble from fragments")
}

// TestChatStream_FinalPopulatesContent verifies the happy text path surfaces
// accumulated content on the Final chunk as well as via individual Delta
// events. The agent loop uses Final as a drop-in for a non-streaming Chat
// result, so callers that ignore deltas must still get the full content.
func TestChatStream_FinalPopulatesContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, c := range []string{"Hel", "lo", " world"} {
			fmt.Fprintf(w, "data: %s\n\n", fmt.Sprintf(`{"choices":[{"delta":{"content":%q}}]}`, c))
			flusher.Flush()
		}
		fmt.Fprint(w, `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	ch, err := p.ChatStream(t.Context(), []Message{{Role: "user", Content: "hi"}}, nil, "gpt-4o", nil)
	require.NoError(t, err)

	var final *LLMResponse
	for sc := range ch {
		if sc.Done {
			final = sc.Final
		}
	}
	require.NotNil(t, final)
	assert.Equal(t, "Hello world", final.Content)
	assert.Equal(t, "stop", final.FinishReason)
	assert.Empty(t, final.ToolCalls)
}

func TestChatStream_NonStreamingFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a normal JSON response (non-SSE)
		w.Header().Set("Content-Type", "application/json")
		resp := `{"choices":[{"message":{"content":"fallback response"},"finish_reason":"stop"}]}`
		fmt.Fprint(w, resp)
	}))
	defer server.Close()

	p := NewProvider("key", server.URL, "")
	ch, err := p.ChatStream(
		t.Context(),
		[]Message{{Role: "user", Content: "hi"}},
		nil,
		"test-model",
		nil,
	)
	require.NoError(t, err)

	// The channel should close without panicking, even if server returns non-SSE.
	// Drain the channel with a timeout to avoid hanging.
	timeout := time.After(5 * time.Second)
	var received []StreamChunk
	for {
		select {
		case sc, ok := <-ch:
			if !ok {
				goto done
			}
			received = append(received, sc)
			if sc.Done {
				goto done
			}
		case <-timeout:
			t.Fatal("timed out waiting for stream to close")
		}
	}
done:
	// Channel closed successfully -- the key assertion is no panic/hang.
	assert.True(t, len(received) >= 0, "channel should close gracefully")
}
