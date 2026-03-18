package tools

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_NormalOperation(t *testing.T) {
	cb := NewCircuitBreaker(5, 2*time.Minute)

	// Unknown tools default to closed.
	assert.Equal(t, CircuitClosed, cb.GetState("my_tool"))

	// Should allow execution.
	assert.True(t, cb.AllowExecution("my_tool"))

	// Record a few successes — circuit stays closed.
	for i := 0; i < 10; i++ {
		cb.RecordSuccess("my_tool")
	}
	assert.Equal(t, CircuitClosed, cb.GetState("my_tool"))
	assert.True(t, cb.AllowExecution("my_tool"))

	// Stats reflect the calls.
	stats := cb.GetStats()
	require.Contains(t, stats, "my_tool")
	assert.Equal(t, 10, stats["my_tool"].TotalCalls)
	assert.Equal(t, 0, stats["my_tool"].TotalFailures)
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, 2*time.Minute)

	// Two failures: still closed.
	cb.RecordFailure("flaky")
	cb.RecordFailure("flaky")
	assert.Equal(t, CircuitClosed, cb.GetState("flaky"))
	assert.True(t, cb.AllowExecution("flaky"))

	// Third failure hits threshold — circuit opens.
	cb.RecordFailure("flaky")
	assert.Equal(t, CircuitOpen, cb.GetState("flaky"))

	// Execution is now blocked.
	assert.False(t, cb.AllowExecution("flaky"))

	// Stats are correct.
	stats := cb.GetStats()
	require.Contains(t, stats, "flaky")
	assert.Equal(t, 3, stats["flaky"].TotalCalls)
	assert.Equal(t, 3, stats["flaky"].TotalFailures)
	assert.Equal(t, 3, stats["flaky"].Failures)
}

func TestCircuitBreaker_CooldownToHalfOpen(t *testing.T) {
	now := time.Now()
	cb := NewCircuitBreaker(2, 1*time.Minute)
	cb.nowFunc = func() time.Time { return now }

	// Trip the circuit.
	cb.RecordFailure("slow_api")
	cb.RecordFailure("slow_api")
	assert.Equal(t, CircuitOpen, cb.GetState("slow_api"))

	// Before cooldown: still blocked.
	cb.nowFunc = func() time.Time { return now.Add(30 * time.Second) }
	assert.False(t, cb.AllowExecution("slow_api"))
	assert.Equal(t, CircuitOpen, cb.GetState("slow_api"))

	// After cooldown: transitions to half-open on next AllowExecution.
	cb.nowFunc = func() time.Time { return now.Add(61 * time.Second) }
	assert.True(t, cb.AllowExecution("slow_api"))
	assert.Equal(t, CircuitHalfOpen, cb.GetState("slow_api"))
}

func TestCircuitBreaker_HalfOpenRecovery(t *testing.T) {
	now := time.Now()
	cb := NewCircuitBreaker(2, 1*time.Minute)
	cb.nowFunc = func() time.Time { return now }

	// Trip and wait for cooldown.
	cb.RecordFailure("recoverable")
	cb.RecordFailure("recoverable")
	assert.Equal(t, CircuitOpen, cb.GetState("recoverable"))

	cb.nowFunc = func() time.Time { return now.Add(2 * time.Minute) }
	assert.True(t, cb.AllowExecution("recoverable")) // → half-open
	assert.Equal(t, CircuitHalfOpen, cb.GetState("recoverable"))

	// Probe succeeds → circuit closes.
	cb.RecordSuccess("recoverable")
	assert.Equal(t, CircuitClosed, cb.GetState("recoverable"))
	assert.True(t, cb.AllowExecution("recoverable"))

	// Consecutive failure count was reset.
	stats := cb.GetStats()
	assert.Equal(t, 0, stats["recoverable"].Failures)
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	now := time.Now()
	cb := NewCircuitBreaker(2, 1*time.Minute)
	cb.nowFunc = func() time.Time { return now }

	// Trip and wait for cooldown.
	cb.RecordFailure("fragile")
	cb.RecordFailure("fragile")
	assert.Equal(t, CircuitOpen, cb.GetState("fragile"))

	cb.nowFunc = func() time.Time { return now.Add(2 * time.Minute) }
	assert.True(t, cb.AllowExecution("fragile")) // → half-open

	// Probe fails → circuit reopens.
	cb.RecordFailure("fragile")
	assert.Equal(t, CircuitOpen, cb.GetState("fragile"))
	assert.False(t, cb.AllowExecution("fragile"))

	// Total failures incremented.
	stats := cb.GetStats()
	assert.Equal(t, 3, stats["fragile"].TotalFailures)
}

func TestCircuitBreaker_ManualReset(t *testing.T) {
	cb := NewCircuitBreaker(2, 5*time.Minute)

	// Trip the circuit.
	cb.RecordFailure("stuck")
	cb.RecordFailure("stuck")
	assert.Equal(t, CircuitOpen, cb.GetState("stuck"))
	assert.False(t, cb.AllowExecution("stuck"))

	// Manual reset brings it back to closed immediately.
	cb.Reset("stuck")
	assert.Equal(t, CircuitClosed, cb.GetState("stuck"))
	assert.True(t, cb.AllowExecution("stuck"))

	// Failure count is cleared; need threshold failures to trip again.
	cb.RecordFailure("stuck")
	assert.Equal(t, CircuitClosed, cb.GetState("stuck"))

	// Resetting an unknown tool is a no-op.
	cb.Reset("unknown_tool")
	assert.Equal(t, CircuitClosed, cb.GetState("unknown_tool"))
}

func TestCircuitBreaker_SuccessResetsConsecutiveFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Minute)

	// Two failures, then a success.
	cb.RecordFailure("intermittent")
	cb.RecordFailure("intermittent")
	cb.RecordSuccess("intermittent")

	// Consecutive failures should be reset; two more failures won't trip.
	cb.RecordFailure("intermittent")
	cb.RecordFailure("intermittent")
	assert.Equal(t, CircuitClosed, cb.GetState("intermittent"))

	// Third consecutive failure will now trip.
	cb.RecordFailure("intermittent")
	assert.Equal(t, CircuitOpen, cb.GetState("intermittent"))
}

func TestCircuitBreaker_DefaultValues(t *testing.T) {
	cb := NewCircuitBreaker(0, 0)
	assert.Equal(t, 5, cb.failureThreshold)
	assert.Equal(t, 2*time.Minute, cb.cooldownPeriod)
}

func TestCircuitBreaker_IndependentTools(t *testing.T) {
	cb := NewCircuitBreaker(2, time.Minute)

	// Trip one tool; the other should be unaffected.
	cb.RecordFailure("tool_a")
	cb.RecordFailure("tool_a")
	assert.Equal(t, CircuitOpen, cb.GetState("tool_a"))

	assert.Equal(t, CircuitClosed, cb.GetState("tool_b"))
	assert.True(t, cb.AllowExecution("tool_b"))
}

func TestCircuitState_String(t *testing.T) {
	assert.Equal(t, "closed", CircuitClosed.String())
	assert.Equal(t, "open", CircuitOpen.String())
	assert.Equal(t, "half-open", CircuitHalfOpen.String())
	assert.Equal(t, "unknown", CircuitState(99).String())
}

func TestCircuitBreaker_GetStatsReturnsCopies(t *testing.T) {
	cb := NewCircuitBreaker(5, time.Minute)
	cb.RecordFailure("tool_x")

	stats := cb.GetStats()
	// Mutating the returned copy should not affect internal state.
	stats["tool_x"].Failures = 999

	internal := cb.GetStats()
	assert.Equal(t, 1, internal["tool_x"].Failures)
}
