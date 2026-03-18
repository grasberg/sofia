package tools

import (
	"fmt"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
)

// CircuitState represents the current state of a tool's circuit breaker.
type CircuitState int

const (
	// CircuitClosed is normal operation: all calls are allowed.
	CircuitClosed CircuitState = iota
	// CircuitOpen means the tool is disabled due to repeated failures.
	CircuitOpen
	// CircuitHalfOpen means the circuit is testing whether the tool has recovered.
	CircuitHalfOpen
)

// String returns a human-readable representation of the circuit state.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// toolCircuit holds the per-tool circuit breaker state and lifetime statistics.
type toolCircuit struct {
	State         CircuitState `json:"state"`
	Failures      int          `json:"failures"`
	LastFailure   time.Time    `json:"last_failure"`
	OpenedAt      time.Time    `json:"opened_at"`
	TotalCalls    int          `json:"total_calls"`
	TotalFailures int          `json:"total_failures"`
	ProbeInFlight bool         `json:"probe_in_flight"`
}

// CircuitBreaker tracks failure counts per tool and disables tools that exceed a
// configurable failure threshold. After a cooldown period, the circuit transitions
// to half-open and allows a single probe call to test recovery.
type CircuitBreaker struct {
	mu               sync.RWMutex
	toolStates       map[string]*toolCircuit
	failureThreshold int
	cooldownPeriod   time.Duration
	nowFunc          func() time.Time // injectable clock for testing
}

// NewCircuitBreaker creates a CircuitBreaker with the given failure threshold and
// cooldown period. If failureThreshold <= 0 it defaults to 5. If cooldownPeriod <= 0
// it defaults to 2 minutes.
func NewCircuitBreaker(failureThreshold int, cooldownPeriod time.Duration) *CircuitBreaker {
	if failureThreshold <= 0 {
		failureThreshold = 5
	}
	if cooldownPeriod <= 0 {
		cooldownPeriod = 2 * time.Minute
	}
	return &CircuitBreaker{
		toolStates:       make(map[string]*toolCircuit),
		failureThreshold: failureThreshold,
		cooldownPeriod:   cooldownPeriod,
		nowFunc:          time.Now,
	}
}

// now returns the current time, using the injected clock if available.
func (cb *CircuitBreaker) now() time.Time {
	if cb.nowFunc != nil {
		return cb.nowFunc()
	}
	return time.Now()
}

// getOrCreate returns the toolCircuit for the given tool, creating one if needed.
// Caller must hold cb.mu.
func (cb *CircuitBreaker) getOrCreate(toolName string) *toolCircuit {
	tc, ok := cb.toolStates[toolName]
	if !ok {
		tc = &toolCircuit{State: CircuitClosed}
		cb.toolStates[toolName] = tc
	}
	return tc
}

// AllowExecution checks whether the named tool is permitted to execute.
//
//   - Closed: always allowed.
//   - Open: rejected unless the cooldown period has elapsed, in which case the
//     circuit transitions to HalfOpen and a single probe call is allowed.
//   - HalfOpen: allowed (exactly one call to test recovery).
func (cb *CircuitBreaker) AllowExecution(toolName string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	tc := cb.getOrCreate(toolName)

	switch tc.State {
	case CircuitClosed:
		return true

	case CircuitOpen:
		if cb.now().Sub(tc.OpenedAt) >= cb.cooldownPeriod {
			tc.State = CircuitHalfOpen
			logger.InfoCF("circuit_breaker", "Circuit transitioned to half-open",
				map[string]any{"tool": toolName})
			return true
		}
		logger.WarnCF("circuit_breaker", "Execution blocked by open circuit",
			map[string]any{"tool": toolName})
		return false

	case CircuitHalfOpen:
		if tc.ProbeInFlight {
			return false // Only one probe at a time
		}
		tc.ProbeInFlight = true
		return true

	default:
		return true
	}
}

// RecordSuccess records a successful execution for the named tool. If the circuit
// was half-open, it transitions back to closed.
func (cb *CircuitBreaker) RecordSuccess(toolName string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	tc := cb.getOrCreate(toolName)
	tc.TotalCalls++

	tc.ProbeInFlight = false
	if tc.State == CircuitHalfOpen {
		tc.State = CircuitClosed
		tc.Failures = 0
		logger.InfoCF("circuit_breaker", "Circuit recovered, closed",
			map[string]any{"tool": toolName})
	} else {
		// Reset consecutive failure count on any success in closed state.
		tc.Failures = 0
	}
}

// RecordFailure records a failed execution for the named tool. If the consecutive
// failure count reaches the threshold, the circuit opens. In half-open state a
// single failure reopens the circuit immediately.
func (cb *CircuitBreaker) RecordFailure(toolName string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	tc := cb.getOrCreate(toolName)
	tc.TotalCalls++
	tc.TotalFailures++
	tc.Failures++
	tc.LastFailure = cb.now()
	tc.ProbeInFlight = false

	switch tc.State {
	case CircuitClosed:
		if tc.Failures >= cb.failureThreshold {
			tc.State = CircuitOpen
			tc.OpenedAt = cb.now()
			logger.WarnCF("circuit_breaker", "Circuit opened after reaching failure threshold",
				map[string]any{
					"tool":      toolName,
					"failures":  tc.Failures,
					"threshold": cb.failureThreshold,
				})
		}

	case CircuitHalfOpen:
		tc.State = CircuitOpen
		tc.OpenedAt = cb.now()
		logger.WarnCF("circuit_breaker", "Half-open probe failed, circuit reopened",
			map[string]any{"tool": toolName})
	}
}

// GetState returns the current CircuitState for the named tool. Tools that have
// never been seen are considered Closed.
func (cb *CircuitBreaker) GetState(toolName string) CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	tc, ok := cb.toolStates[toolName]
	if !ok {
		return CircuitClosed
	}
	return tc.State
}

// Reset manually resets the named tool's circuit to Closed and clears its
// consecutive failure count.
func (cb *CircuitBreaker) Reset(toolName string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	tc, ok := cb.toolStates[toolName]
	if !ok {
		return
	}
	tc.State = CircuitClosed
	tc.Failures = 0
	logger.InfoCF("circuit_breaker", "Circuit manually reset",
		map[string]any{"tool": toolName})
}

// GetStats returns a snapshot of every tool's circuit state. The returned map
// contains copies so callers cannot mutate internal state.
func (cb *CircuitBreaker) GetStats() map[string]*toolCircuit {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	result := make(map[string]*toolCircuit, len(cb.toolStates))
	for name, tc := range cb.toolStates {
		cp := *tc
		result[name] = &cp
	}
	return result
}

// circuitBreakerError returns a formatted error message when a tool is blocked.
func circuitBreakerError(toolName string) string {
	return fmt.Sprintf(
		"tool %q is temporarily disabled (circuit breaker open due to repeated failures). "+
			"It will be retried automatically after the cooldown period.",
		toolName,
	)
}
