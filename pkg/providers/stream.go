package providers

import (
	"context"
	"strings"
)

// CollectStream consumes a ChatStream channel and returns the aggregated
// LLMResponse. Text deltas are forwarded to onDelta (may be nil) as they
// arrive so UI layers can render tokens live. If the channel closes without
// a terminal chunk that carries Final, the response is synthesised from the
// accumulated deltas — this keeps the agent loop working against providers
// whose streaming impl doesn't populate Final yet.
//
// Context cancellation is honoured by ChatStream on the producer side, so
// this function only needs to drain the channel; when the producer sees
// ctx.Done() it closes the channel and the range loop terminates.
func CollectStream(ctx context.Context, ch <-chan StreamChunk, onDelta func(string)) (*LLMResponse, error) {
	var textFallback strings.Builder
	for sc := range ch {
		if sc.Delta != "" {
			textFallback.WriteString(sc.Delta)
			if onDelta != nil {
				onDelta(sc.Delta)
			}
		}
		if sc.Done {
			if sc.Final != nil {
				return sc.Final, nil
			}
			return &LLMResponse{
				Content:      textFallback.String(),
				FinishReason: "stop",
			}, nil
		}
	}
	// Channel closed without a Done marker — surface any context error so
	// callers can distinguish a clean cancel from a silent stream drop.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &LLMResponse{
		Content:      textFallback.String(),
		FinishReason: "stop",
	}, nil
}
