package providers

import (
	"context"
	"math"
	"math/rand"
	"time"
)

// RetryConfig configures retry behavior for provider calls.
type RetryConfig struct {
	MaxRetries     int           // max retry attempts (default 3)
	InitialBackoff time.Duration // initial wait (default 1s)
	MaxBackoff     time.Duration // maximum wait (default 30s)
	Multiplier     float64       // backoff multiplier (default 2.0)
	JitterFactor   float64       // random jitter 0-1 (default 0.1)
}

// DefaultRetryConfig returns a RetryConfig with sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		Multiplier:     2.0,
		JitterFactor:   0.1,
	}
}

// isRetriableReason returns true if the given FailoverReason should be retried.
// Rate limits, timeouts, and overloaded errors are retriable.
// Auth, billing, and format errors are not (retrying won't help).
func isRetriableReason(reason FailoverReason) bool {
	switch reason {
	case FailoverRateLimit, FailoverTimeout, FailoverOverloaded:
		return true
	default:
		return false
	}
}

// RetryWithBackoff executes fn with exponential backoff and jitter.
// Only retries on retriable errors (rate limits, timeouts, overloaded).
// Non-retriable errors (auth, billing, format) fail immediately.
//
// Backoff formula: min(initialBackoff * multiplier^attempt + random_jitter, maxBackoff)
func RetryWithBackoff(
	ctx context.Context,
	cfg RetryConfig,
	classify func(error) FailoverReason,
	fn func() error,
) error {
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		// Check context before each attempt.
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Classify the error.
		reason := classify(lastErr)
		if !isRetriableReason(reason) {
			return lastErr
		}

		// Don't sleep after the last attempt.
		if attempt == cfg.MaxRetries {
			break
		}

		// Calculate backoff: min(initialBackoff * multiplier^attempt + jitter, maxBackoff)
		backoff := float64(cfg.InitialBackoff) * math.Pow(cfg.Multiplier, float64(attempt))
		jitter := backoff * cfg.JitterFactor * rand.Float64() //nolint:gosec // jitter doesn't need crypto rand
		delay := time.Duration(math.Min(backoff+jitter, float64(cfg.MaxBackoff)))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt.
		}
	}

	return lastErr
}
