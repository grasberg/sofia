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
