package agent

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// safeBuffer is a bytes.Buffer with a mutex so the spinner goroutine and the
// test can access it without a data race.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestLiveStatus_WritesSpinnerAndStatus verifies the indicator actually emits
// animation frames and the current status text before cancellation.
func TestLiveStatus_WritesSpinnerAndStatus(t *testing.T) {
	var buf safeBuffer
	ctx, cancel := context.WithCancel(context.Background())

	getStatus := func() string { return "Executing tool: exec" }

	done := make(chan struct{})
	go func() {
		defer close(done)
		liveStatus(ctx, getStatus, &buf)
	}()

	// Give the ticker room to fire at least twice (ticker is 120ms).
	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("liveStatus did not return after ctx cancel")
	}

	out := buf.String()
	if !strings.Contains(out, "Executing tool: exec") {
		t.Errorf("output missing status text; got %q", out)
	}
	sawFrame := false
	for _, frame := range spinnerFrames {
		if strings.Contains(out, frame) {
			sawFrame = true
			break
		}
	}
	if !sawFrame {
		t.Errorf("output missing any spinner frame; got %q", out)
	}
	if !strings.HasSuffix(out, ansiClearLine) {
		t.Errorf("output should end with the clear-line sequence so the caller "+
			"can print on a fresh line; got tail=%q", out[max(0, len(out)-20):])
	}
}

// TestLiveStatus_DefaultsStatusWhenEmpty verifies the spinner falls back to a
// generic "Thinking..." label when the agent hasn't populated activeStatus
// yet (early in a request, before the first LLM call).
func TestLiveStatus_DefaultsStatusWhenEmpty(t *testing.T) {
	var buf safeBuffer
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		liveStatus(ctx, func() string { return "" }, &buf)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	if !strings.Contains(buf.String(), "Thinking...") {
		t.Errorf("expected fallback 'Thinking...' text; got %q", buf.String())
	}
}

// TestLiveStatus_TreatsIdleAsEmpty covers the idle path where the agent has
// reset activeStatus to "Idle" but the request is still in flight (e.g. between
// iterations). The spinner should still show a meaningful label, not "Idle".
func TestLiveStatus_TreatsIdleAsEmpty(t *testing.T) {
	var buf safeBuffer
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		liveStatus(ctx, func() string { return "Idle" }, &buf)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	if strings.Contains(buf.String(), "Idle") {
		t.Errorf("'Idle' leaked into spinner output; got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "Thinking...") {
		t.Errorf("expected fallback 'Thinking...' text; got %q", buf.String())
	}
}
