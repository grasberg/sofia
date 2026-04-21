package providers

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// FallbackChain orchestrates model fallback across multiple candidates.
type FallbackChain struct {
	cooldown *CooldownTracker
}

// FallbackCandidate represents one model/provider to try.
type FallbackCandidate struct {
	Provider string
	Model    string
}

// FallbackResult contains the successful response and metadata about all attempts.
type FallbackResult struct {
	Response *LLMResponse
	Provider string
	Model    string
	Attempts []FallbackAttempt
}

// FallbackAttempt records one attempt in the fallback chain.
type FallbackAttempt struct {
	Provider string
	Model    string
	Error    error
	Reason   FailoverReason
	Duration time.Duration
	Skipped  bool // true if skipped due to cooldown
}

// NewFallbackChain creates a new fallback chain with the given cooldown tracker.
func NewFallbackChain(cooldown *CooldownTracker) *FallbackChain {
	return &FallbackChain{cooldown: cooldown}
}

// ResolveCandidates parses model config into a deduplicated candidate list.
func ResolveCandidates(cfg ModelConfig, defaultProvider string) []FallbackCandidate {
	seen := make(map[string]bool)
	var candidates []FallbackCandidate

	addCandidate := func(raw string) {
		ref := ParseModelRef(raw, defaultProvider)
		if ref == nil {
			return
		}
		key := ModelKey(ref.Provider, ref.Model)
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, FallbackCandidate{
			Provider: ref.Provider,
			Model:    ref.Model,
		})
	}

	// Primary first.
	addCandidate(cfg.Primary)

	// Then fallbacks.
	for _, fb := range cfg.Fallbacks {
		addCandidate(fb)
	}

	return candidates
}

// Execute runs the fallback chain for text/chat requests.
// It tries each candidate in order, respecting cooldowns and error classification.
//
// Behavior:
//   - Candidates in cooldown are skipped (logged as skipped attempt).
//   - context.Canceled aborts immediately (user abort, no fallback).
//   - Non-retriable errors (format) abort immediately.
//   - Retriable errors trigger fallback to next candidate.
//   - Success marks provider as good (resets cooldown).
//   - If all fail, returns aggregate error with all attempts.
func (fc *FallbackChain) Execute(
	ctx context.Context,
	candidates []FallbackCandidate,
	run func(ctx context.Context, provider, model string) (*LLMResponse, error),
) (*FallbackResult, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("fallback: no candidates configured")
	}

	result := &FallbackResult{
		Attempts: make([]FallbackAttempt, 0, len(candidates)),
	}

	for i, candidate := range candidates {
		// Check context before each attempt.
		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}

		// Check cooldown.
		if !fc.cooldown.IsAvailable(candidate.Provider) {
			remaining := fc.cooldown.CooldownRemaining(candidate.Provider)
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Skipped:  true,
				Reason:   FailoverRateLimit,
				Error: fmt.Errorf(
					"provider %s in cooldown (%s remaining)",
					candidate.Provider,
					remaining.Round(time.Second),
				),
			})
			continue
		}

		// Execute the run function.
		start := time.Now()
		resp, err := run(ctx, candidate.Provider, candidate.Model)
		elapsed := time.Since(start)

		if err == nil {
			// Success.
			fc.cooldown.MarkSuccess(candidate.Provider)
			result.Response = resp
			result.Provider = candidate.Provider
			result.Model = candidate.Model
			return result, nil
		}

		// Context cancellation: abort immediately, no fallback.
		if ctx.Err() == context.Canceled {
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Error:    err,
				Duration: elapsed,
			})
			return nil, context.Canceled
		}

		// Classify the error.
		failErr := ClassifyError(err, candidate.Provider, candidate.Model)

		if failErr == nil {
			// Unclassifiable error: do not fallback, return immediately.
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Error:    err,
				Duration: elapsed,
			})
			return nil, fmt.Errorf("fallback: unclassified error from %s/%s: %w",
				candidate.Provider, candidate.Model, err)
		}

		// Non-retriable error: abort immediately.
		if !failErr.IsRetriable() {
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Error:    failErr,
				Reason:   failErr.Reason,
				Duration: elapsed,
			})
			return nil, failErr
		}

		// Retriable error: mark failure and continue to next candidate.
		fc.cooldown.MarkFailure(candidate.Provider, failErr.Reason)
		result.Attempts = append(result.Attempts, FallbackAttempt{
			Provider: candidate.Provider,
			Model:    candidate.Model,
			Error:    failErr,
			Reason:   failErr.Reason,
			Duration: elapsed,
		})

		// If this was the last candidate, return aggregate error.
		if i == len(candidates)-1 {
			return nil, &FallbackExhaustedError{Attempts: result.Attempts}
		}
	}

	// All candidates were skipped (all in cooldown). Before hard-failing,
	// take one last-resort shot at the candidate whose cooldown expires
	// soonest — otherwise a transient "everything is cooling down" moment
	// produces a user-visible error even when a call might succeed right now.
	// The cooldown's exponential backoff still protects against runaway
	// retries: a failed bypass extends the cooldown further.
	if allAttemptsSkipped(result.Attempts) {
		pick := shortestCooldownCandidate(fc.cooldown, candidates)
		start := time.Now()
		resp, err := run(ctx, pick.Provider, pick.Model)
		elapsed := time.Since(start)

		if err == nil {
			fc.cooldown.MarkSuccess(pick.Provider)
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: pick.Provider,
				Model:    pick.Model,
				Reason:   FailoverCooldownBypass,
				Duration: elapsed,
			})
			result.Response = resp
			result.Provider = pick.Provider
			result.Model = pick.Model
			return result, nil
		}
		if failErr := ClassifyError(err, pick.Provider, pick.Model); failErr != nil && failErr.IsRetriable() {
			fc.cooldown.MarkFailure(pick.Provider, failErr.Reason)
		}
		result.Attempts = append(result.Attempts, FallbackAttempt{
			Provider: pick.Provider,
			Model:    pick.Model,
			Error:    err,
			Reason:   FailoverCooldownBypass,
			Duration: elapsed,
		})
	}

	return nil, &FallbackExhaustedError{Attempts: result.Attempts}
}

// allAttemptsSkipped reports whether every recorded attempt was a cooldown
// skip — i.e. no real network call was made during the chain.
func allAttemptsSkipped(attempts []FallbackAttempt) bool {
	if len(attempts) == 0 {
		return false
	}
	for _, a := range attempts {
		if !a.Skipped {
			return false
		}
	}
	return true
}

// shortestCooldownCandidate picks the candidate whose cooldown expires
// soonest. Ties resolve to the earliest candidate in the list (priority
// order). Falls back to the first candidate if no cooldown data is
// available (shouldn't happen when called after an all-skipped run).
func shortestCooldownCandidate(ct *CooldownTracker, candidates []FallbackCandidate) FallbackCandidate {
	best := candidates[0]
	bestRemaining := ct.CooldownRemaining(best.Provider)
	for _, c := range candidates[1:] {
		r := ct.CooldownRemaining(c.Provider)
		if r < bestRemaining {
			best = c
			bestRemaining = r
		}
	}
	return best
}

// ExecuteImage runs the fallback chain for image/vision requests.
// Simpler than Execute: no cooldown checks (image endpoints have different rate limits).
// Image dimension/size errors abort immediately (non-retriable).
func (fc *FallbackChain) ExecuteImage(
	ctx context.Context,
	candidates []FallbackCandidate,
	run func(ctx context.Context, provider, model string) (*LLMResponse, error),
) (*FallbackResult, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("image fallback: no candidates configured")
	}

	result := &FallbackResult{
		Attempts: make([]FallbackAttempt, 0, len(candidates)),
	}

	for i, candidate := range candidates {
		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}

		start := time.Now()
		resp, err := run(ctx, candidate.Provider, candidate.Model)
		elapsed := time.Since(start)

		if err == nil {
			result.Response = resp
			result.Provider = candidate.Provider
			result.Model = candidate.Model
			return result, nil
		}

		if ctx.Err() == context.Canceled {
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Error:    err,
				Duration: elapsed,
			})
			return nil, context.Canceled
		}

		// Image dimension/size errors are non-retriable.
		errMsg := strings.ToLower(err.Error())
		if IsImageDimensionError(errMsg) || IsImageSizeError(errMsg) {
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Error:    err,
				Reason:   FailoverFormat,
				Duration: elapsed,
			})
			return nil, &FailoverError{
				Reason:   FailoverFormat,
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Wrapped:  err,
			}
		}

		// Any other error: record and try next.
		result.Attempts = append(result.Attempts, FallbackAttempt{
			Provider: candidate.Provider,
			Model:    candidate.Model,
			Error:    err,
			Duration: elapsed,
		})

		if i == len(candidates)-1 {
			return nil, &FallbackExhaustedError{Attempts: result.Attempts}
		}
	}

	return nil, &FallbackExhaustedError{Attempts: result.Attempts}
}

// FallbackExhaustedError indicates all fallback candidates were tried and failed.
type FallbackExhaustedError struct {
	Attempts []FallbackAttempt
}

func (e *FallbackExhaustedError) Error() string {
	var attempted, skipped int
	reasonCounts := map[FailoverReason]int{}
	for _, a := range e.Attempts {
		if a.Skipped {
			skipped++
			continue
		}
		attempted++
		if a.Reason != "" {
			reasonCounts[a.Reason]++
		}
	}

	var sb strings.Builder
	sb.WriteString(fallbackHeaderFor(attempted, skipped, reasonCounts))

	for i, a := range e.Attempts {
		if a.Skipped {
			fmt.Fprintf(&sb, "\n  [%d] %s/%s: skipped (cooldown)", i+1, a.Provider, a.Model)
		} else {
			fmt.Fprintf(&sb, "\n  [%d] %s/%s: %v (reason=%s, %s)",
				i+1, a.Provider, a.Model, a.Error, a.Reason, a.Duration.Round(time.Millisecond))
		}
	}
	return sb.String()
}

// fallbackHeaderFor picks an actionable one-line summary for the aggregate
// error. When every *attempted* candidate failed with the same reason, the
// header points the user at the concrete fix (update keys, check billing,
// wait for rate limit). Otherwise it falls back to a neutral summary.
//
// Called from FallbackExhaustedError.Error; keep the output single-line so
// the per-attempt detail can be appended after a newline.
func fallbackHeaderFor(attempted, skipped int, reasonCounts map[FailoverReason]int) string {
	if attempted == 0 {
		return fmt.Sprintf("fallback: %d attempted, %d skipped (cooldown):", attempted, skipped)
	}
	// Check single-reason dominance in a deterministic priority order so the
	// most-actionable headers win when counts tie.
	priority := []FailoverReason{
		FailoverAuth,
		FailoverBilling,
		FailoverRateLimit,
		FailoverTimeout,
		FailoverOverloaded,
	}
	for _, r := range priority {
		if reasonCounts[r] == attempted {
			switch r {
			case FailoverAuth:
				return fmt.Sprintf(
					"fallback: all %d configured provider(s) rejected the API key (HTTP 401). "+
						"Update your keys in Settings → AI Models and retry. Details:",
					attempted)
			case FailoverBilling:
				return fmt.Sprintf(
					"fallback: all %d configured provider(s) reported a billing problem. "+
						"Check your account/subscription and the keys in Settings → AI Models. Details:",
					attempted)
			case FailoverRateLimit:
				return fmt.Sprintf(
					"fallback: all %d configured provider(s) are rate-limited right now. "+
						"Wait a bit, or add more fallback models in Settings → AI Models. Details:",
					attempted)
			case FailoverTimeout:
				return fmt.Sprintf(
					"fallback: all %d configured provider(s) timed out. "+
						"Check your network/VPN, or pick providers in a closer region. Details:",
					attempted)
			case FailoverOverloaded:
				return fmt.Sprintf(
					"fallback: all %d configured provider(s) reported they are overloaded. "+
						"Retry in a minute, or add more fallbacks in Settings → AI Models. Details:",
					attempted)
			}
		}
	}
	return fmt.Sprintf("fallback: %d attempted, %d skipped (cooldown):", attempted, skipped)
}
