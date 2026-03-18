package providers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// classifyForTest maps specific error messages to FailoverReasons for testing.
func classifyForTest(err error) FailoverReason {
	msg := err.Error()
	switch msg {
	case "rate limited":
		return FailoverRateLimit
	case "timed out":
		return FailoverTimeout
	case "overloaded":
		return FailoverOverloaded
	case "auth failure":
		return FailoverAuth
	case "billing error":
		return FailoverBilling
	case "bad format":
		return FailoverFormat
	default:
		return FailoverUnknown
	}
}

func fastRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		Multiplier:     2.0,
		JitterFactor:   0.0, // no jitter for deterministic tests
	}
}

func TestRetryWithBackoff_SuccessFirstTry(t *testing.T) {
	calls := 0
	err := RetryWithBackoff(context.Background(), fastRetryConfig(), classifyForTest, func() error {
		calls++
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, calls, "should only call fn once on immediate success")
}

func TestRetryWithBackoff_SuccessAfterRetry(t *testing.T) {
	calls := 0
	err := RetryWithBackoff(context.Background(), fastRetryConfig(), classifyForTest, func() error {
		calls++
		if calls < 3 {
			return errors.New("rate limited")
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, calls, "should succeed on the third attempt")
}

func TestRetryWithBackoff_NonRetriableError(t *testing.T) {
	tests := []struct {
		name   string
		errMsg string
		reason FailoverReason
	}{
		{"auth", "auth failure", FailoverAuth},
		{"billing", "billing error", FailoverBilling},
		{"format", "bad format", FailoverFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			expectedErr := errors.New(tt.errMsg)
			err := RetryWithBackoff(context.Background(), fastRetryConfig(), classifyForTest, func() error {
				calls++
				return expectedErr
			})

			require.Error(t, err)
			assert.Equal(t, 1, calls, "non-retriable errors should not be retried")
			assert.Equal(t, expectedErr, err)
		})
	}
}

func TestRetryWithBackoff_MaxRetriesExceeded(t *testing.T) {
	cfg := fastRetryConfig()
	cfg.MaxRetries = 2

	calls := 0
	expectedErr := errors.New("rate limited")
	err := RetryWithBackoff(context.Background(), cfg, classifyForTest, func() error {
		calls++
		return expectedErr
	})

	require.Error(t, err)
	assert.Equal(t, 3, calls, "should attempt 1 initial + 2 retries = 3 total")
	assert.Equal(t, expectedErr, err)
}

func TestRetryWithBackoff_ContextCancelled(t *testing.T) {
	t.Run("cancelled before first attempt", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately.

		calls := 0
		err := RetryWithBackoff(ctx, fastRetryConfig(), classifyForTest, func() error {
			calls++
			return nil
		})

		require.Error(t, err)
		assert.Equal(t, 0, calls, "should not call fn when context is already cancelled")
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("cancelled during backoff wait", func(t *testing.T) {
		cfg := RetryConfig{
			MaxRetries:     3,
			InitialBackoff: 5 * time.Second, // long backoff so cancel fires during wait
			MaxBackoff:     30 * time.Second,
			Multiplier:     2.0,
			JitterFactor:   0.0,
		}

		ctx, cancel := context.WithCancel(context.Background())

		calls := 0
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		err := RetryWithBackoff(ctx, cfg, classifyForTest, func() error {
			calls++
			return errors.New("rate limited")
		})

		require.Error(t, err)
		assert.Equal(t, 1, calls, "should stop retrying after context is cancelled during backoff")
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestRetryWithBackoff_BackoffIncreases(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:     4,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     500 * time.Millisecond,
		Multiplier:     2.0,
		JitterFactor:   0.0, // no jitter for deterministic timing
	}

	var timestamps []time.Time
	err := RetryWithBackoff(context.Background(), cfg, classifyForTest, func() error {
		timestamps = append(timestamps, time.Now())
		return errors.New("rate limited")
	})

	require.Error(t, err)
	require.Len(t, timestamps, 5, "should have 1 initial + 4 retries = 5 attempts")

	// Verify delays increase monotonically.
	// Expected delays: ~10ms, ~20ms, ~40ms, ~80ms
	for i := 2; i < len(timestamps); i++ {
		prevDelay := timestamps[i-1].Sub(timestamps[i-2])
		currDelay := timestamps[i].Sub(timestamps[i-1])
		assert.GreaterOrEqual(t, currDelay, prevDelay*8/10,
			"delay[%d]=%v should be >= ~80%% of delay[%d]=%v (allowing for scheduling variance)",
			i-1, currDelay, i-2, prevDelay)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 1*time.Second, cfg.InitialBackoff)
	assert.Equal(t, 30*time.Second, cfg.MaxBackoff)
	assert.Equal(t, 2.0, cfg.Multiplier)
	assert.Equal(t, 0.1, cfg.JitterFactor)
}
