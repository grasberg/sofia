package providers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func makeCandidate(provider, model string) FallbackCandidate {
	return FallbackCandidate{Provider: provider, Model: model}
}

func successRun(content string) func(ctx context.Context, provider, model string) (*LLMResponse, error) {
	return func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		return &LLMResponse{Content: content, FinishReason: "stop"}, nil
	}
}

func TestFallback_SingleCandidate_Success(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{makeCandidate("openai", "gpt-4")}
	result, err := fc.Execute(context.Background(), candidates, successRun("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response.Content != "hello" {
		t.Errorf("content = %q, want hello", result.Response.Content)
	}
	if result.Provider != "openai" || result.Model != "gpt-4" {
		t.Errorf("provider/model = %s/%s, want openai/gpt-4", result.Provider, result.Model)
	}
}

func TestFallback_SecondCandidateSuccess(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{
		makeCandidate("openai", "gpt-4"),
		makeCandidate("anthropic", "claude-opus"),
	}

	attempt := 0
	run := func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		attempt++
		if attempt == 1 {
			return nil, errors.New("rate limit exceeded")
		}
		return &LLMResponse{Content: "from claude", FinishReason: "stop"}, nil
	}

	result, err := fc.Execute(context.Background(), candidates, run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Provider != "anthropic" {
		t.Errorf("provider = %q, want anthropic", result.Provider)
	}
	if result.Response.Content != "from claude" {
		t.Errorf("content = %q, want 'from claude'", result.Response.Content)
	}
	if len(result.Attempts) != 1 {
		t.Errorf("attempts = %d, want 1 (failed attempt recorded)", len(result.Attempts))
	}
}

func TestFallback_AllFail(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{
		makeCandidate("openai", "gpt-4"),
		makeCandidate("anthropic", "claude"),
		makeCandidate("groq", "llama"),
	}

	run := func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		return nil, errors.New("rate limit exceeded")
	}

	_, err := fc.Execute(context.Background(), candidates, run)
	if err == nil {
		t.Fatal("expected error when all candidates fail")
	}
	var exhausted *FallbackExhaustedError
	if !errors.As(err, &exhausted) {
		t.Errorf("expected FallbackExhaustedError, got %T: %v", err, err)
	}
	if len(exhausted.Attempts) != 3 {
		t.Errorf("attempts = %d, want 3", len(exhausted.Attempts))
	}
}

func TestFallback_ContextCanceled(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	ctx, cancel := context.WithCancel(context.Background())
	candidates := []FallbackCandidate{
		makeCandidate("openai", "gpt-4"),
		makeCandidate("anthropic", "claude"),
	}

	attempt := 0
	run := func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		attempt++
		if attempt == 1 {
			cancel() // cancel context
			return nil, context.Canceled
		}
		t.Error("should not reach second candidate after cancel")
		return nil, nil
	}

	_, err := fc.Execute(ctx, candidates, run)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestFallback_NonRetriableError(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{
		makeCandidate("openai", "gpt-4"),
		makeCandidate("anthropic", "claude"),
	}

	attempt := 0
	run := func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		attempt++
		return nil, errors.New("string should match pattern")
	}

	_, err := fc.Execute(context.Background(), candidates, run)
	if err == nil {
		t.Fatal("expected error for non-retriable")
	}
	var fe *FailoverError
	if !errors.As(err, &fe) {
		t.Fatalf("expected FailoverError, got %T", err)
	}
	if fe.Reason != FailoverFormat {
		t.Errorf("reason = %q, want format", fe.Reason)
	}
	if attempt != 1 {
		t.Errorf("attempt = %d, want 1 (non-retriable should not try next)", attempt)
	}
}

func TestFallback_CooldownSkip(t *testing.T) {
	now := time.Now()
	ct, _ := newTestTracker(now)
	fc := NewFallbackChain(ct)

	// Put openai in cooldown
	ct.MarkFailure("openai", FailoverRateLimit)

	candidates := []FallbackCandidate{
		makeCandidate("openai", "gpt-4"),
		makeCandidate("anthropic", "claude"),
	}

	run := func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		if provider == "openai" {
			t.Error("should not call openai (in cooldown)")
		}
		return &LLMResponse{Content: "claude response", FinishReason: "stop"}, nil
	}

	result, err := fc.Execute(context.Background(), candidates, run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Provider != "anthropic" {
		t.Errorf("provider = %q, want anthropic", result.Provider)
	}
	// Should have 1 skipped attempt
	skipped := 0
	for _, a := range result.Attempts {
		if a.Skipped {
			skipped++
		}
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
}

func TestFallback_AllInCooldown_BypassAttempted(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	// Put all providers in cooldown. openai's rate-limit cooldown (1 min)
	// expires sooner than anthropic's billing cooldown (5 h), so the bypass
	// should pick openai.
	ct.MarkFailure("openai", FailoverRateLimit)
	ct.MarkFailure("anthropic", FailoverBilling)

	candidates := []FallbackCandidate{
		makeCandidate("openai", "gpt-4"),
		makeCandidate("anthropic", "claude"),
	}

	var callCount int
	var calledProvider string
	_, err := fc.Execute(context.Background(), candidates,
		func(ctx context.Context, provider, model string) (*LLMResponse, error) {
			callCount++
			calledProvider = provider
			return nil, errors.New("503 Service Unavailable")
		})

	if err == nil {
		t.Fatal("expected error when bypass attempt also fails")
	}
	if callCount != 1 {
		t.Errorf("expected 1 bypass call, got %d", callCount)
	}
	if calledProvider != "openai" {
		t.Errorf("expected bypass to pick openai (shortest cooldown), got %q", calledProvider)
	}
	var exhausted *FallbackExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("expected FallbackExhaustedError, got %T", err)
	}
	// Last recorded attempt should be the bypass (not a skip) and be flagged
	// with reason=cooldown_bypass for observability.
	last := exhausted.Attempts[len(exhausted.Attempts)-1]
	if last.Skipped {
		t.Error("expected bypass attempt to not be Skipped")
	}
	if last.Reason != FailoverCooldownBypass {
		t.Errorf("expected reason=cooldown_bypass, got %q", last.Reason)
	}
}

func TestFallback_AllInCooldown_BypassSucceeds(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	ct.MarkFailure("openai", FailoverRateLimit)
	ct.MarkFailure("anthropic", FailoverBilling)

	candidates := []FallbackCandidate{
		makeCandidate("openai", "gpt-4"),
		makeCandidate("anthropic", "claude"),
	}

	result, err := fc.Execute(context.Background(), candidates,
		func(ctx context.Context, provider, model string) (*LLMResponse, error) {
			return &LLMResponse{Content: "hello"}, nil
		})

	if err != nil {
		t.Fatalf("expected success from bypass, got %v", err)
	}
	if result.Response == nil || result.Response.Content != "hello" {
		t.Errorf("expected response 'hello', got %+v", result.Response)
	}
	if result.Provider != "openai" {
		t.Errorf("expected provider=openai, got %q", result.Provider)
	}
	// Success should reset the cooldown for the bypassed provider.
	if !ct.IsAvailable("openai") {
		t.Error("expected openai cooldown to be reset after successful bypass")
	}
	// Anthropic's cooldown is untouched by the bypass.
	if ct.IsAvailable("anthropic") {
		t.Error("expected anthropic to still be in cooldown")
	}
}

func TestFallback_NoCandidates(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	_, err := fc.Execute(context.Background(), nil, successRun("ok"))
	if err == nil {
		t.Error("expected error for empty candidates")
	}
}

func TestFallback_EmptyFallbacks(t *testing.T) {
	// Single primary, no fallbacks: should work like direct call
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{makeCandidate("openai", "gpt-4")}
	result, err := fc.Execute(context.Background(), candidates, successRun("ok"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response.Content != "ok" {
		t.Error("expected success with single candidate")
	}
}

func TestFallback_UnclassifiedError(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{
		makeCandidate("openai", "gpt-4"),
		makeCandidate("anthropic", "claude"),
	}

	attempt := 0
	run := func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		attempt++
		return nil, errors.New("completely unknown internal error")
	}

	_, err := fc.Execute(context.Background(), candidates, run)
	if err == nil {
		t.Fatal("expected error for unclassified error")
	}
	if attempt != 1 {
		t.Errorf("attempt = %d, want 1 (should not fallback on unclassified)", attempt)
	}
}

func TestFallback_SuccessResetsCooldown(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{makeCandidate("openai", "gpt-4")}

	attempt := 0
	run := func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		attempt++
		if attempt == 1 {
			ct.MarkFailure("openai", FailoverRateLimit) // simulate failure tracked elsewhere
		}
		return &LLMResponse{Content: "ok", FinishReason: "stop"}, nil
	}

	_, err := fc.Execute(context.Background(), candidates, run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ct.IsAvailable("openai") {
		t.Error("success should reset cooldown")
	}
}

// --- Image Fallback Tests ---

func TestImageFallback_Success(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{makeCandidate("openai", "gpt-4o")}
	result, err := fc.ExecuteImage(context.Background(), candidates, successRun("image result"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response.Content != "image result" {
		t.Error("expected image result")
	}
}

func TestImageFallback_DimensionError(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{
		makeCandidate("openai", "gpt-4o"),
		makeCandidate("anthropic", "claude"),
	}

	attempt := 0
	run := func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		attempt++
		return nil, errors.New("image dimensions exceed max 4096x4096")
	}

	_, err := fc.ExecuteImage(context.Background(), candidates, run)
	if err == nil {
		t.Fatal("expected error for image dimension error")
	}
	if attempt != 1 {
		t.Errorf("attempt = %d, want 1 (image dimension error should not retry)", attempt)
	}
}

func TestImageFallback_SizeError(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{
		makeCandidate("openai", "gpt-4o"),
		makeCandidate("anthropic", "claude"),
	}

	attempt := 0
	run := func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		attempt++
		return nil, errors.New("image exceeds 20 mb")
	}

	_, err := fc.ExecuteImage(context.Background(), candidates, run)
	if err == nil {
		t.Fatal("expected error for image size error")
	}
	if attempt != 1 {
		t.Errorf("attempt = %d, want 1 (image size error should not retry)", attempt)
	}
}

func TestImageFallback_RetryOnOtherErrors(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	candidates := []FallbackCandidate{
		makeCandidate("openai", "gpt-4o"),
		makeCandidate("anthropic", "claude-sonnet"),
	}

	attempt := 0
	run := func(ctx context.Context, provider, model string) (*LLMResponse, error) {
		attempt++
		if attempt == 1 {
			return nil, errors.New("rate limit exceeded")
		}
		return &LLMResponse{Content: "image ok", FinishReason: "stop"}, nil
	}

	result, err := fc.ExecuteImage(context.Background(), candidates, run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Provider != "anthropic" {
		t.Errorf("provider = %q, want anthropic", result.Provider)
	}
}

func TestImageFallback_NoCandidates(t *testing.T) {
	ct := NewCooldownTracker()
	fc := NewFallbackChain(ct)

	_, err := fc.ExecuteImage(context.Background(), nil, successRun("ok"))
	if err == nil {
		t.Error("expected error for empty candidates")
	}
}

// --- ResolveCandidates Tests ---

func TestResolveCandidates_Simple(t *testing.T) {
	cfg := ModelConfig{
		Primary:   "gpt-4",
		Fallbacks: []string{"anthropic/claude-opus", "groq/llama-3"},
	}

	candidates := ResolveCandidates(cfg, "openai")
	if len(candidates) != 3 {
		t.Fatalf("candidates = %d, want 3", len(candidates))
	}

	if candidates[0].Provider != "openai" || candidates[0].Model != "gpt-4" {
		t.Errorf("candidate[0] = %s/%s, want openai/gpt-4", candidates[0].Provider, candidates[0].Model)
	}
	if candidates[1].Provider != "anthropic" || candidates[1].Model != "claude-opus" {
		t.Errorf("candidate[1] = %s/%s, want anthropic/claude-opus", candidates[1].Provider, candidates[1].Model)
	}
	if candidates[2].Provider != "groq" || candidates[2].Model != "llama-3" {
		t.Errorf("candidate[2] = %s/%s, want groq/llama-3", candidates[2].Provider, candidates[2].Model)
	}
}

func TestResolveCandidates_Deduplication(t *testing.T) {
	cfg := ModelConfig{
		Primary:   "openai/gpt-4",
		Fallbacks: []string{"openai/gpt-4", "anthropic/claude"},
	}

	candidates := ResolveCandidates(cfg, "default")
	if len(candidates) != 2 {
		t.Errorf("candidates = %d, want 2 (duplicate removed)", len(candidates))
	}
}

func TestResolveCandidates_EmptyFallbacks(t *testing.T) {
	cfg := ModelConfig{
		Primary:   "gpt-4",
		Fallbacks: nil,
	}

	candidates := ResolveCandidates(cfg, "openai")
	if len(candidates) != 1 {
		t.Errorf("candidates = %d, want 1", len(candidates))
	}
}

func TestResolveCandidates_EmptyPrimary(t *testing.T) {
	cfg := ModelConfig{
		Primary:   "",
		Fallbacks: []string{"anthropic/claude"},
	}

	candidates := ResolveCandidates(cfg, "openai")
	if len(candidates) != 1 {
		t.Errorf("candidates = %d, want 1", len(candidates))
	}
}

func TestFallbackExhaustedError_Message(t *testing.T) {
	e := &FallbackExhaustedError{
		Attempts: []FallbackAttempt{
			{
				Provider: "openai",
				Model:    "gpt-4",
				Error:    errors.New("rate limited"),
				Reason:   FailoverRateLimit,
				Duration: 500 * time.Millisecond,
			},
			{Provider: "anthropic", Model: "claude", Skipped: true},
		},
	}
	msg := e.Error()
	if msg == "" {
		t.Error("expected non-empty error message")
	}
}

// When every attempted provider returns 401, the header must tell the user
// to update their API keys — otherwise they see a wall of status-401 noise
// with no hint about what to fix.
func TestFallbackExhaustedError_AllAuthHeader(t *testing.T) {
	e := &FallbackExhaustedError{
		Attempts: []FallbackAttempt{
			{Provider: "ollama", Model: "x", Error: errors.New("401 unauthorized"),
				Reason: FailoverAuth, Duration: 100 * time.Millisecond},
			{Provider: "minimax", Model: "y", Error: errors.New("401 unauthorized"),
				Reason: FailoverAuth, Duration: 200 * time.Millisecond},
		},
	}
	msg := e.Error()
	if !strings.Contains(msg, "rejected the API key") {
		t.Errorf("expected all-auth header, got: %s", msg)
	}
	if !strings.Contains(msg, "Settings") {
		t.Errorf("expected pointer to Settings, got: %s", msg)
	}
	if !strings.Contains(msg, "all 2") {
		t.Errorf("expected count 'all 2', got: %s", msg)
	}
}

func TestFallbackExhaustedError_AllBillingHeader(t *testing.T) {
	e := &FallbackExhaustedError{
		Attempts: []FallbackAttempt{
			{Provider: "openai", Model: "x", Error: errors.New("billing"), Reason: FailoverBilling},
			{Provider: "anthropic", Model: "y", Error: errors.New("billing"), Reason: FailoverBilling},
		},
	}
	msg := e.Error()
	if !strings.Contains(msg, "billing problem") {
		t.Errorf("expected billing header, got: %s", msg)
	}
}

func TestFallbackExhaustedError_AllRateLimitHeader(t *testing.T) {
	e := &FallbackExhaustedError{
		Attempts: []FallbackAttempt{
			{Provider: "openai", Model: "x", Error: errors.New("429"), Reason: FailoverRateLimit},
			{Provider: "anthropic", Model: "y", Error: errors.New("429"), Reason: FailoverRateLimit},
		},
	}
	msg := e.Error()
	if !strings.Contains(msg, "rate-limited") {
		t.Errorf("expected rate-limit header, got: %s", msg)
	}
}

// Mixed reasons should fall through to the neutral summary — we can't point
// at a single fix, so don't pretend to.
func TestFallbackExhaustedError_MixedReasonsNoSpecificHeader(t *testing.T) {
	e := &FallbackExhaustedError{
		Attempts: []FallbackAttempt{
			{Provider: "openai", Model: "x", Error: errors.New("auth"), Reason: FailoverAuth},
			{Provider: "anthropic", Model: "y", Error: errors.New("429"), Reason: FailoverRateLimit},
		},
	}
	msg := e.Error()
	if strings.Contains(msg, "rejected the API key") {
		t.Errorf("expected neutral header for mixed reasons, got: %s", msg)
	}
	if !strings.Contains(msg, "2 attempted") {
		t.Errorf("expected neutral '2 attempted' count, got: %s", msg)
	}
}

// Skipped-only (all in cooldown, no real attempts) keeps the neutral header
// so the cooldown-bypass path still looks right.
func TestFallbackExhaustedError_AllSkippedHeader(t *testing.T) {
	e := &FallbackExhaustedError{
		Attempts: []FallbackAttempt{
			{Provider: "openai", Model: "x", Skipped: true},
			{Provider: "anthropic", Model: "y", Skipped: true},
		},
	}
	msg := e.Error()
	if !strings.Contains(msg, "0 attempted, 2 skipped") {
		t.Errorf("expected skipped-only neutral header, got: %s", msg)
	}
}
