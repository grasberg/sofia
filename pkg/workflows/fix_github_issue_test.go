package workflows

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// --- fakes ------------------------------------------------------------------

type fakeClassifier struct {
	class  IssueClass
	reason string
	err    error
}

func (f *fakeClassifier) Classify(_ context.Context, _ Issue) (IssueClass, string, error) {
	return f.class, f.reason, f.err
}

type fakeCloner struct {
	called atomic.Int32
	err    error
	repos  []string
}

func (f *fakeCloner) Clone(_ context.Context, repo, _, _ string) error {
	f.called.Add(1)
	f.repos = append(f.repos, repo)
	return f.err
}

type fakeTester struct {
	passed bool
	output string
	err    error
}

func (f *fakeTester) RunTests(_ context.Context, _ string) (TestResult, error) {
	return TestResult{Passed: f.passed, Output: f.output, Command: "fake"}, f.err
}

type fakeFixer struct {
	result FixResult
	err    error
	called atomic.Int32
}

func (f *fakeFixer) Fix(_ context.Context, _ FixRequest) (FixResult, error) {
	f.called.Add(1)
	return f.result, f.err
}

type fakePusher struct {
	called atomic.Int32
	err    error
}

func (f *fakePusher) Push(_ context.Context, _, _ string) error {
	f.called.Add(1)
	return f.err
}

type fakePRCreator struct {
	url    string
	err    error
	called atomic.Int32
}

func (f *fakePRCreator) CreatePR(_ context.Context, _, _, _, _ string) (string, error) {
	f.called.Add(1)
	return f.url, f.err
}

type fakeCommenter struct {
	mu       sync.Mutex
	comments []struct{ Repo, Body string; Number int }
	err      error
}

func (f *fakeCommenter) Comment(_ context.Context, repo string, number int, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.comments = append(f.comments, struct{ Repo, Body string; Number int }{repo, body, number})
	return f.err
}

// --- helpers ----------------------------------------------------------------

func baseDeps() (*fakeClassifier, *fakeCloner, *fakeTester, *fakeFixer, *fakePusher, *fakePRCreator, *fakeCommenter, FixGitHubIssueDeps) {
	cls := &fakeClassifier{class: IssueClassAutoFixable, reason: "typo keyword"}
	cl := &fakeCloner{}
	te := &fakeTester{passed: false, output: "FAIL: sample test"}
	fi := &fakeFixer{result: FixResult{CommitSHA: "abc123", DiffSummary: "fix typo", FilesChanged: 1, TestsPass: true}}
	pu := &fakePusher{}
	pr := &fakePRCreator{url: "https://github.com/owner/repo/pull/7"}
	co := &fakeCommenter{}
	return cls, cl, te, fi, pu, pr, co, FixGitHubIssueDeps{
		Classifier:     cls,
		Cloner:         cl,
		Tester:         te,
		Fixer:          fi,
		Pusher:         pu,
		PRCreator:      pr,
		IssueCommenter: co,
		WorkspaceRoot:  "/tmp/sofia-test",
		BranchPrefix:   "sofia-autofix/",
		Locale:         "en",
	}
}

func runFixWorkflow(t *testing.T, deps FixGitHubIssueDeps, gate ApprovalGateway, inputs map[string]any) (*RunResult, error) {
	t.Helper()
	reg := NewRegistry()
	if err := RegisterFixGitHubIssue(reg, deps); err != nil {
		t.Fatalf("register: %v", err)
	}
	r := NewRunner(reg, nil, gate)
	return r.Run(context.Background(), WorkflowFixGitHubIssue, "github-poller", "fix issue", inputs)
}

func sampleIssueInputs() map[string]any {
	return map[string]any{
		InputRepo:        "owner/repo",
		InputIssueNumber: 42,
		InputIssueTitle:  "typo in README",
		InputIssueBody:   "Found a typo: 'seperate' should be 'separate'.",
		InputIssueLabels: []string{"sofia-autofix"},
		InputIssueURL:    "https://github.com/owner/repo/issues/42",
	}
}

// approvalGateAutoApprove is a gate that always returns approved — lets tests
// focus on the workflow logic without pending state.
type approvalGateAutoApprove struct{ calls atomic.Int32 }

func (g *approvalGateAutoApprove) RequestApproval(_ context.Context, _, _, _, _, _, _, _ string, _ map[string]string) (bool, error) {
	g.calls.Add(1)
	return true, nil
}

// approvalGateDeny always rejects.
type approvalGateDeny struct{ calls atomic.Int32 }

func (g *approvalGateDeny) RequestApproval(_ context.Context, _, _, _, _, _, _, _ string, _ map[string]string) (bool, error) {
	g.calls.Add(1)
	return false, nil
}

// --- tests ------------------------------------------------------------------

func TestFixGHIssue_HappyPath_DispatchesAllSteps(t *testing.T) {
	cls, cl, te, fi, pu, pr, co, deps := baseDeps()
	_ = cls
	_ = co

	gate := &approvalGateAutoApprove{}
	res, err := runFixWorkflow(t, deps, gate, sampleIssueInputs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Completed) != 6 {
		t.Errorf("completed steps = %v", res.Completed)
	}

	if cl.called.Load() != 1 {
		t.Error("cloner must run")
	}
	if fi.called.Load() != 1 {
		t.Error("fixer must run")
	}
	if pu.called.Load() != 1 {
		t.Error("pusher must run")
	}
	if pr.called.Load() != 1 {
		t.Error("PR creator must run")
	}
	if gate.calls.Load() != 2 {
		t.Errorf("approval calls = %d, want 2 (push + pr)", gate.calls.Load())
	}
	if res.Output[OutputPRURL] != "https://github.com/owner/repo/pull/7" {
		t.Errorf("pr url = %v", res.Output[OutputPRURL])
	}
	// Reproduce step should have noticed tests fail (as expected for a bug).
	if !isTrue(res.Output[OutputReproFailed]) {
		t.Errorf("repro_failed expected true, got %v", res.Output[OutputReproFailed])
	}
	_ = te
}

func isTrue(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func TestFixGHIssue_NeedsHuman_HaltsEarly(t *testing.T) {
	cls, cl, _, fi, _, _, co, deps := baseDeps()
	cls.class = IssueClassNeedsHuman
	cls.reason = "design discussion"

	res, err := runFixWorkflow(t, deps, nil, sampleIssueInputs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Halted {
		t.Error("workflow should have halted")
	}
	if cl.called.Load() != 0 {
		t.Error("cloner must not run when NeedsHuman")
	}
	if fi.called.Load() != 0 {
		t.Error("fixer must not run when NeedsHuman")
	}
	if len(co.comments) != 1 {
		t.Errorf("expected 1 issue comment, got %d", len(co.comments))
	}
	if !strings.Contains(co.comments[0].Body, "human judgement") {
		t.Errorf("comment body: %q", co.comments[0].Body)
	}
}

func TestFixGHIssue_CannotReproduce_HaltsWithComment(t *testing.T) {
	_, cl, te, fi, _, _, co, deps := baseDeps()
	te.passed = true // bug "fixed" before we touched it → can't reproduce
	te.output = "all tests pass"

	res, err := runFixWorkflow(t, deps, nil, sampleIssueInputs())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Halted || res.HaltReason != "cannot_reproduce" {
		t.Errorf("expected cannot_reproduce halt, got %+v", res)
	}
	if cl.called.Load() != 1 {
		t.Error("clone should still have run")
	}
	if fi.called.Load() != 0 {
		t.Error("fixer must not run when repro fails")
	}
	if len(co.comments) != 1 {
		t.Errorf("expected repro-failed comment, got %d", len(co.comments))
	}
}

func TestFixGHIssue_FixerError_CommentsIssueAndFails(t *testing.T) {
	_, _, _, fi, pu, pr, co, deps := baseDeps()
	fi.err = errors.New("cannot converge")

	_, err := runFixWorkflow(t, deps, nil, sampleIssueInputs())
	if err == nil {
		t.Fatal("expected fix error to propagate")
	}
	if pu.called.Load() != 0 {
		t.Error("pusher must not run after fix error")
	}
	if pr.called.Load() != 0 {
		t.Error("PR creator must not run after fix error")
	}
	if len(co.comments) != 1 {
		t.Errorf("expected failure comment, got %d", len(co.comments))
	}
	if !strings.Contains(co.comments[0].Body, "couldn't converge") {
		t.Errorf("comment body: %q", co.comments[0].Body)
	}
}

func TestFixGHIssue_FixerReportsTestsStillFail_Errors(t *testing.T) {
	_, _, _, fi, pu, _, _, deps := baseDeps()
	fi.result = FixResult{CommitSHA: "abc", TestsPass: false}

	_, err := runFixWorkflow(t, deps, nil, sampleIssueInputs())
	if err == nil {
		t.Fatal("expected error when fixer reports tests still failing")
	}
	if pu.called.Load() != 0 {
		t.Error("pusher must not run when tests fail post-fix")
	}
}

func TestFixGHIssue_PushApprovalDenied_NoPR(t *testing.T) {
	_, _, _, _, pu, pr, _, deps := baseDeps()
	gate := &approvalGateDeny{}

	_, err := runFixWorkflow(t, deps, gate, sampleIssueInputs())
	if err == nil {
		t.Fatal("denied push should error")
	}
	if pu.called.Load() != 0 {
		t.Error("pusher must not run when push denied")
	}
	if pr.called.Load() != 0 {
		t.Error("PR must not run when push denied")
	}
}

func TestFixGHIssue_PRApprovalDenied_ButPushHappened(t *testing.T) {
	calls := atomic.Int32{}
	denyPRGate := &selectiveDenyGate{
		denyStep: "open_pr",
		calls:    &calls,
	}
	_, _, _, _, pu, pr, _, deps := baseDeps()

	_, err := runFixWorkflow(t, deps, denyPRGate, sampleIssueInputs())
	if err == nil {
		t.Fatal("denied PR should error")
	}
	if pu.called.Load() != 1 {
		t.Error("push should have completed before PR denial")
	}
	if pr.called.Load() != 0 {
		t.Error("PR must not run when PR denied")
	}
}

type selectiveDenyGate struct {
	denyStep string
	calls    *atomic.Int32
}

func (g *selectiveDenyGate) RequestApproval(_ context.Context, id, _, _, _, _, _, _ string, _ map[string]string) (bool, error) {
	g.calls.Add(1)
	if strings.Contains(id, g.denyStep) {
		return false, nil
	}
	return true, nil
}

func TestRegisterFixGHIssue_MissingDepsErrors(t *testing.T) {
	_, _, _, _, _, _, _, deps := baseDeps()
	reg := NewRegistry()
	// Missing Cloner
	d := deps
	d.Cloner = nil
	if err := RegisterFixGitHubIssue(reg, d); err == nil {
		t.Error("missing Cloner should error")
	}
	d = deps
	d.Fixer = nil
	if err := RegisterFixGitHubIssue(reg, d); err == nil {
		t.Error("missing Fixer should error")
	}
	d = deps
	d.Pusher = nil
	if err := RegisterFixGitHubIssue(reg, d); err == nil {
		t.Error("missing Pusher should error")
	}
	d = deps
	d.PRCreator = nil
	if err := RegisterFixGitHubIssue(reg, d); err == nil {
		t.Error("missing PRCreator should error")
	}
}

// --- heuristic classifier tests --------------------------------------------

func TestHeuristicIssueClassifier_NeedsHumanOnProposal(t *testing.T) {
	c := NewHeuristicIssueClassifier()
	class, _, err := c.Classify(context.Background(), Issue{
		Title: "RFC: new authentication architecture",
		Body:  "Let's discuss how we should design the new auth layer.",
	})
	if err != nil || class != IssueClassNeedsHuman {
		t.Errorf("got (%s, %v)", class, err)
	}
}

func TestHeuristicIssueClassifier_AutoFixableOnTypo(t *testing.T) {
	c := NewHeuristicIssueClassifier()
	class, _, _ := c.Classify(context.Background(), Issue{
		Title: "Typo in README.md",
		Body:  "'seperate' should be 'separate'",
	})
	if class != IssueClassAutoFixable {
		t.Errorf("typo should be auto-fixable, got %s", class)
	}
}

func TestHeuristicIssueClassifier_AmbiguousByDefault(t *testing.T) {
	c := NewHeuristicIssueClassifier()
	class, reason, _ := c.Classify(context.Background(), Issue{
		Title: "Something is weird",
		Body:  "I noticed the output looks slightly off when I do X.",
	})
	if class != IssueClassAmbiguous {
		t.Errorf("generic issue should be ambiguous, got %s (reason=%s)", class, reason)
	}
}

func TestHeuristicIssueClassifier_LongBodyFallsAmbiguous(t *testing.T) {
	c := NewHeuristicIssueClassifier()
	long := strings.Repeat("This is a long body with typo keyword. ", 200)
	class, _, _ := c.Classify(context.Background(), Issue{Title: "x", Body: long})
	if class != IssueClassAmbiguous {
		t.Errorf("over-long body should demote to ambiguous, got %s", class)
	}
}

// --- helper tests ----------------------------------------------------------

func TestPRTitle_TrimsLong(t *testing.T) {
	long := strings.Repeat("x", 150)
	got := prTitle(Issue{Number: 1, Title: long})
	if len(got) > 80 {
		t.Errorf("PR title longer than expected: %d", len(got))
	}
}

func TestPRBody_LocaleAware(t *testing.T) {
	sv := prBody(Issue{Number: 5}, "ändrad en rad", "sv")
	if !strings.Contains(sv, "Fixar #5") || !strings.Contains(sv, "Sammanfattning") {
		t.Errorf("swedish body: %q", sv)
	}
	en := prBody(Issue{Number: 5}, "one line", "en")
	if !strings.Contains(en, "Fixes #5") || !strings.Contains(en, "Summary") {
		t.Errorf("english body: %q", en)
	}
}

func TestExtractPRURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/owner/repo/pull/7\nsome trailing":       "https://github.com/owner/repo/pull/7",
		"Creating pull request: https://github.com/a/b/pull/123 ok": "https://github.com/a/b/pull/123",
		"no url here":                                              "no url here",
	}
	for in, want := range cases {
		if got := extractPRURL(in); got != want {
			t.Errorf("extractPRURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlugRepo(t *testing.T) {
	if got := slugRepo("owner/repo"); got != "owner-repo" {
		t.Errorf("got %q", got)
	}
}
