package workflows

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/memory"
)

// --- fakes ------------------------------------------------------------------

type fakeTriager struct {
	result TriageResult
	err    error
}

func (f *fakeTriager) Triage(_ context.Context, _, _ string) (TriageResult, error) {
	return f.result, f.err
}

type fakeDrafter struct {
	body string
	err  error
}

func (f *fakeDrafter) Draft(_ context.Context, _ DraftRequest) (string, error) {
	return f.body, f.err
}

type fakeKBSearcher struct {
	hits []memory.KBEntry
	err  error
}

func (f *fakeKBSearcher) Search(_, _ string, _ int) ([]memory.KBEntry, error) {
	return f.hits, f.err
}

type fakeKBUpserter struct {
	mu      sync.Mutex
	records []struct{ Q, A, Source string; Tags []string }
	err     error
}

func (f *fakeKBUpserter) Upsert(_, q, a, src string, tags []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, struct{ Q, A, Source string; Tags []string }{q, a, src, tags})
	return f.err
}

type fakeSender struct {
	mu     sync.Mutex
	calls  []struct{ To, Subject, Body string }
	err    error
}

func (f *fakeSender) Send(_ context.Context, to, subject, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, struct{ To, Subject, Body string }{to, subject, body})
	return f.err
}

type fakeArchiver struct {
	called atomic.Int32
	err    error
}

func (f *fakeArchiver) Archive(_ context.Context, _ string) error {
	f.called.Add(1)
	return f.err
}

type stubRisk struct{ level agent.RiskLevel }

func (s *stubRisk) Classify(_ context.Context, _ agent.ToolCallDescriptor) agent.RiskLevel {
	return s.level
}

// approvalGateFake records calls and returns a scripted answer.
type approvalGateFake struct {
	mu       sync.Mutex
	calls    int
	approved bool
	err      error
}

func (a *approvalGateFake) RequestApproval(_ context.Context, _, _, _, _, _, _, _ string, _ map[string]string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	return a.approved, a.err
}

// --- helpers ----------------------------------------------------------------

func newSupportDeps() (*fakeTriager, *fakeDrafter, *fakeKBSearcher, *fakeKBUpserter, *fakeSender, *fakeArchiver, SupportReplyDeps) {
	tri := &fakeTriager{result: TriageResult{Priority: PriorityP3, Sentiment: SentimentNeutral, Summary: "summary"}}
	drafter := &fakeDrafter{body: "Hej,\n\nTack för ditt mail.\n\nVänligen,\nSofia"}
	kbs := &fakeKBSearcher{hits: nil}
	kbu := &fakeKBUpserter{}
	sender := &fakeSender{}
	archiver := &fakeArchiver{}
	deps := SupportReplyDeps{
		Triager:       tri,
		Drafter:       drafter,
		KBSearcher:    kbs,
		KBUpserter:    kbu,
		Sender:        sender,
		Archiver:      archiver,
		DefaultLocale: "sv",
	}
	return tri, drafter, kbs, kbu, sender, archiver, deps
}

func runSupportReply(t *testing.T, deps SupportReplyDeps, gate ApprovalGateway, inputs map[string]any) (*RunResult, error) {
	t.Helper()
	reg := NewRegistry()
	if err := RegisterSupportReply(reg, deps); err != nil {
		t.Fatalf("register: %v", err)
	}
	r := NewRunner(reg, nil, gate)
	return r.Run(context.Background(), WorkflowSupportReply, "support-agent", "inbound reply", inputs)
}

func sampleInputs() map[string]any {
	return map[string]any{
		InputFrom:      "Alice <alice@example.com>",
		InputSubject:   "Reset my password please",
		InputBody:      "Hi team, I can't log in anymore.",
		InputMessageID: "mid-123",
		InputLocale:    "en",
		InputAgentID:   "support",
	}
}

// --- tests ------------------------------------------------------------------

func TestSupportReply_HappyPath_AutoSends(t *testing.T) {
	_, _, _, kbu, sender, archiver, deps := newSupportDeps()
	deps.Classifier = &stubRisk{level: agent.RiskLow}

	res, err := runSupportReply(t, deps, nil, sampleInputs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Completed) != 6 {
		t.Errorf("want 6 completed steps, got %v", res.Completed)
	}
	if res.Output[OutputApproval] != "auto" {
		t.Errorf("approval status = %v, want auto", res.Output[OutputApproval])
	}
	if len(sender.calls) != 1 {
		t.Fatalf("sender calls = %d", len(sender.calls))
	}
	if !strings.HasPrefix(sender.calls[0].Subject, "Re:") {
		t.Errorf("subject = %q", sender.calls[0].Subject)
	}
	if sender.calls[0].To != "Alice <alice@example.com>" {
		t.Errorf("to = %q", sender.calls[0].To)
	}
	if len(kbu.records) != 1 {
		t.Errorf("kb upserts = %d", len(kbu.records))
	}
	if kbu.records[0].Source != "email:mid-123" {
		t.Errorf("kb source = %q", kbu.records[0].Source)
	}
	if archiver.called.Load() != 1 {
		t.Errorf("archive called %d times", archiver.called.Load())
	}
}

func TestSupportReply_P1_AlwaysRequiresApproval(t *testing.T) {
	tri, _, _, _, sender, _, deps := newSupportDeps()
	tri.result = TriageResult{Priority: PriorityP1, Sentiment: SentimentNegative}
	deps.Classifier = &stubRisk{level: agent.RiskLow} // low risk, but P1 wins

	gate := &approvalGateFake{approved: true}
	res, err := runSupportReply(t, deps, gate, sampleInputs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gate.calls != 1 {
		t.Errorf("gate calls = %d, want 1", gate.calls)
	}
	if res.Output[OutputApproval] != "approved" {
		t.Errorf("approval status = %v", res.Output[OutputApproval])
	}
	if len(sender.calls) != 1 {
		t.Error("approved P1 should still get sent")
	}
}

func TestSupportReply_MediumRisk_RequiresApproval(t *testing.T) {
	_, _, _, _, sender, _, deps := newSupportDeps()
	deps.Classifier = &stubRisk{level: agent.RiskMedium}

	gate := &approvalGateFake{approved: true}
	_, err := runSupportReply(t, deps, gate, sampleInputs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gate.calls != 1 {
		t.Errorf("gate calls = %d, want 1", gate.calls)
	}
	if len(sender.calls) != 1 {
		t.Error("approved medium-risk should send")
	}
}

func TestSupportReply_NoClassifier_DefaultsToApproval(t *testing.T) {
	// Without a classifier, risk level is RiskUnknown — which the send step
	// treats as "needs approval" to fail safe.
	_, _, _, _, sender, _, deps := newSupportDeps()
	deps.Classifier = nil

	gate := &approvalGateFake{approved: true}
	_, err := runSupportReply(t, deps, gate, sampleInputs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gate.calls != 1 {
		t.Errorf("gate calls = %d, want 1", gate.calls)
	}
	if len(sender.calls) != 1 {
		t.Error("expected send after approval")
	}
}

func TestSupportReply_ApprovalDenied_DoesNotSend(t *testing.T) {
	_, _, _, _, sender, _, deps := newSupportDeps()
	deps.Classifier = &stubRisk{level: agent.RiskMedium}

	gate := &approvalGateFake{approved: false}
	_, err := runSupportReply(t, deps, gate, sampleInputs())
	if err == nil {
		t.Fatal("denial must propagate as error")
	}
	if len(sender.calls) != 0 {
		t.Error("denied reply should not have been sent")
	}
}

func TestSupportReply_TriageError_AbortsBeforeSearch(t *testing.T) {
	tri, _, kbs, _, sender, _, deps := newSupportDeps()
	tri.err = errors.New("triage blew up")

	kbs.hits = []memory.KBEntry{{Question: "q", Answer: "a"}} // should never be consulted

	_, err := runSupportReply(t, deps, nil, sampleInputs())
	if err == nil {
		t.Fatal("expected error")
	}
	if len(sender.calls) != 0 {
		t.Error("nothing should have been sent")
	}
}

func TestSupportReply_EmptyDraft_Errors(t *testing.T) {
	_, drafter, _, _, sender, _, deps := newSupportDeps()
	drafter.body = "   "

	_, err := runSupportReply(t, deps, nil, sampleInputs())
	if err == nil {
		t.Fatal("expected error for empty draft")
	}
	if len(sender.calls) != 0 {
		t.Error("empty draft should block send")
	}
}

func TestSupportReply_SenderError_PropagatesAfterApproval(t *testing.T) {
	_, _, _, kbu, sender, _, deps := newSupportDeps()
	sender.err = errors.New("smtp down")
	deps.Classifier = &stubRisk{level: agent.RiskLow}

	_, err := runSupportReply(t, deps, nil, sampleInputs())
	if err == nil {
		t.Fatal("expected send error")
	}
	if len(kbu.records) != 0 {
		t.Error("KB upsert must not happen when send failed")
	}
}

func TestSupportReply_ArchiveFailure_NonFatal(t *testing.T) {
	_, _, _, _, sender, archiver, deps := newSupportDeps()
	archiver.err = errors.New("label api rejected")
	deps.Classifier = &stubRisk{level: agent.RiskLow}

	res, err := runSupportReply(t, deps, nil, sampleInputs())
	if err != nil {
		t.Fatalf("archive error must not fail workflow: %v", err)
	}
	if len(sender.calls) != 1 {
		t.Error("send should still have happened")
	}
	if res.Output[OutputApproval] != "auto" {
		t.Errorf("status = %v", res.Output[OutputApproval])
	}
}

func TestSupportReply_KBHitsReachDrafter(t *testing.T) {
	_, drafter, kbs, _, _, _, deps := newSupportDeps()
	kbs.hits = []memory.KBEntry{
		{Question: "How to reset password", Answer: "Settings → Security → Reset", Tags: []string{"account"}},
	}
	// Capture what the drafter sees.
	var captured DraftRequest
	capturing := &capturingDrafter{inner: drafter, captured: &captured}
	deps.Drafter = capturing
	deps.Classifier = &stubRisk{level: agent.RiskLow}

	if _, err := runSupportReply(t, deps, nil, sampleInputs()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(captured.KBHits) != 1 || captured.KBHits[0].Question != "How to reset password" {
		t.Errorf("drafter did not receive KB hits: %+v", captured.KBHits)
	}
	if captured.Locale != "en" {
		t.Errorf("locale = %q", captured.Locale)
	}
}

type capturingDrafter struct {
	inner    Drafter
	captured *DraftRequest
}

func (c *capturingDrafter) Draft(ctx context.Context, req DraftRequest) (string, error) {
	*c.captured = req
	return c.inner.Draft(ctx, req)
}

// --- unit tests for small helpers ------------------------------------------

func TestPriorityValue(t *testing.T) {
	cases := map[string]int{
		"P1": 1, "P2": 2, "P3": 3, "P99": 99,
		"":   99, "p3": 3, "urgent": 99,
	}
	for in, want := range cases {
		if got := priorityValue(in); got != want {
			t.Errorf("priorityValue(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestNeedsApproval(t *testing.T) {
	cases := []struct {
		name     string
		risk     agent.RiskLevel
		priority string
		floor    string
		want     bool
	}{
		{"low risk, P3, floor P3", agent.RiskLow, "P3", "P3", false},
		{"low risk, P4, floor P3", agent.RiskLow, "P4", "P3", false},
		{"low risk, P2, floor P3", agent.RiskLow, "P2", "P3", true},
		{"low risk, P1, floor P3", agent.RiskLow, "P1", "P3", true},
		{"medium, P4, floor P3", agent.RiskMedium, "P4", "P3", true},
		{"high, P4, floor P3", agent.RiskHigh, "P4", "P3", true},
		{"unknown, P4, floor P3", agent.RiskUnknown, "P4", "P3", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := needsApproval(c.risk, c.priority, c.floor); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestReplySubject(t *testing.T) {
	cases := map[string]string{
		"":              "Re: Support",
		"Help":          "Re: Help",
		"Re: Help":      "Re: Help",
		"RE: Help":      "RE: Help",
		"sv: Hjälp":     "sv: Hjälp",
		"  Trim me  ":   "Re: Trim me",
	}
	for in, want := range cases {
		if got := replySubject(in); got != want {
			t.Errorf("replySubject(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHeuristicTriager_P1OnUrgentAndNegative(t *testing.T) {
	tri := NewHeuristicTriager(nil, nil)
	r, err := tri.Triage(context.Background(), "URGENT: outage",
		"This is unacceptable, we're losing money every minute.")
	if err != nil {
		t.Fatal(err)
	}
	if r.Priority != PriorityP1 {
		t.Errorf("priority = %q, want P1", r.Priority)
	}
	if r.Sentiment != SentimentNegative {
		t.Errorf("sentiment = %q, want negative", r.Sentiment)
	}
}

func TestHeuristicTriager_DefaultsToP3Neutral(t *testing.T) {
	tri := NewHeuristicTriager(nil, nil)
	r, _ := tri.Triage(context.Background(), "question about billing", "Hi, when is my next invoice due?")
	if r.Priority != PriorityP3 {
		t.Errorf("priority = %q, want P3", r.Priority)
	}
	if r.Sentiment != SentimentNeutral {
		t.Errorf("sentiment = %q, want neutral", r.Sentiment)
	}
}

func TestHeuristicTriager_ThankYouBoostsSentiment(t *testing.T) {
	tri := NewHeuristicTriager(nil, nil)
	r, _ := tri.Triage(context.Background(), "thanks", "Tack för all hjälp!")
	if r.Sentiment != SentimentPositive {
		t.Errorf("sentiment = %q, want positive", r.Sentiment)
	}
}

func TestTemplateDrafter_UsesKBHitsInSwedish(t *testing.T) {
	d := NewTemplateDrafter()
	out, err := d.Draft(context.Background(), DraftRequest{
		From:   "Alice <alice@example.com>",
		Locale: "sv",
		KBHits: []memory.KBEntry{
			{Answer: "Gå till Inställningar → Säkerhet → Återställ."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "Hej Alice") {
		t.Errorf("expected Swedish greeting, got %q", out)
	}
	if !strings.Contains(out, "Säkerhet") {
		t.Error("KB answer should be embedded")
	}
	if !strings.Contains(out, "Sofia") {
		t.Error("signature missing")
	}
}

func TestTemplateDrafter_FallsBackInEnglishWithoutHits(t *testing.T) {
	d := NewTemplateDrafter()
	out, err := d.Draft(context.Background(), DraftRequest{
		From:   "bob@example.com",
		Locale: "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "Hi bob") {
		t.Errorf("expected English greeting with fallback name, got %q", out)
	}
	if !strings.Contains(out, "look into this") {
		t.Error("no-hit fallback missing")
	}
}

func TestExtractFirstName(t *testing.T) {
	cases := map[string]string{
		"Alice Smith <alice@example.com>": "Alice",
		`"Alice" <alice@example.com>`:     "Alice",
		"alice@example.com":                "alice",
		"":                                 "",
	}
	for in, want := range cases {
		if got := extractFirstName(in); got != want {
			t.Errorf("extractFirstName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRegisterSupportReply_MissingKBSearcher(t *testing.T) {
	_, _, _, _, _, _, deps := newSupportDeps()
	deps.KBSearcher = nil
	reg := NewRegistry()
	if err := RegisterSupportReply(reg, deps); err == nil {
		t.Error("expected error for missing KBSearcher")
	}
}

func TestRegisterSupportReply_MissingSender(t *testing.T) {
	_, _, _, _, _, _, deps := newSupportDeps()
	deps.Sender = nil
	reg := NewRegistry()
	if err := RegisterSupportReply(reg, deps); err == nil {
		t.Error("expected error for missing Sender")
	}
}

func TestRegisterSupportReply_DefaultsFillIn(t *testing.T) {
	_, _, _, _, _, _, deps := newSupportDeps()
	deps.Triager = nil // should get the heuristic default
	deps.Drafter = nil // should get the template default
	reg := NewRegistry()
	if err := RegisterSupportReply(reg, deps); err != nil {
		t.Fatalf("register: %v", err)
	}
	// Verify it runs end-to-end with defaults.
	r := NewRunner(reg, nil, nil)
	deps.Classifier = &stubRisk{level: agent.RiskLow} // nb: deps is a copy, but runner holds the original closures
	_, err := r.Run(context.Background(), WorkflowSupportReply, "a", "d", sampleInputs())
	if err != nil {
		t.Errorf("run with defaults failed: %v", err)
	}
}
