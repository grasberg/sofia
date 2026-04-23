package workflows

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeShellRunner records calls & returns scripted responses.
type fakeShellRunner struct {
	mu        sync.Mutex
	calls     []shellCall
	responses map[string]shellResponse
	always    *shellResponse
}

type shellCall struct {
	Dir, Name string
	Args      []string
}

type shellResponse struct {
	out string
	err error
}

func (f *fakeShellRunner) Run(_ context.Context, dir, name string, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, shellCall{Dir: dir, Name: name, Args: append([]string(nil), args...)})
	key := name + " " + strings.Join(args, " ")
	if r, ok := f.responses[key]; ok {
		return r.out, r.err
	}
	for pattern, r := range f.responses {
		if strings.Contains(key, pattern) {
			return r.out, r.err
		}
	}
	if f.always != nil {
		return f.always.out, f.always.err
	}
	return "", nil
}

// --- ShellCloner -----------------------------------------------------------

func TestShellCloner_UsesGhFirst(t *testing.T) {
	runner := &fakeShellRunner{responses: map[string]shellResponse{
		"gh repo clone":    {out: "done"},
		"git checkout -b":  {out: "Switched"},
	}}
	c := newShellCloner(runner)

	dir := filepath.Join(t.TempDir(), "r")
	if err := c.Clone(context.Background(), "owner/repo", dir, "feature"); err != nil {
		t.Fatalf("clone: %v", err)
	}

	var sawGh, sawGitCheckout bool
	for _, call := range runner.calls {
		joined := call.Name + " " + strings.Join(call.Args, " ")
		if strings.Contains(joined, "gh repo clone owner/repo") {
			sawGh = true
		}
		if call.Name == "git" && len(call.Args) > 0 && call.Args[0] == "checkout" {
			sawGitCheckout = true
		}
	}
	if !sawGh {
		t.Error("gh clone should have run")
	}
	if !sawGitCheckout {
		t.Error("git checkout -b should have run")
	}
}

func TestShellCloner_FallsBackToGitOnGHFailure(t *testing.T) {
	runner := &fakeShellRunner{responses: map[string]shellResponse{
		"gh repo clone": {err: errors.New("gh not authed")},
		"git clone":     {out: "done"},
		"checkout -b":   {out: "Switched"},
	}}
	c := newShellCloner(runner)
	dir := filepath.Join(t.TempDir(), "r")
	if err := c.Clone(context.Background(), "owner/repo", dir, "feature"); err != nil {
		t.Fatalf("clone: %v", err)
	}
	// verify git clone was invoked
	found := false
	for _, call := range runner.calls {
		if call.Name == "git" && len(call.Args) > 0 && call.Args[0] == "clone" {
			found = true
		}
	}
	if !found {
		t.Error("git clone fallback should have run")
	}
}

func TestShellCloner_ValidatesInputs(t *testing.T) {
	c := newShellCloner(&fakeShellRunner{})
	if err := c.Clone(context.Background(), "", "/tmp/r", "b"); err == nil {
		t.Error("empty repo should error")
	}
	if err := c.Clone(context.Background(), "r", "", "b"); err == nil {
		t.Error("empty dir should error")
	}
	if err := c.Clone(context.Background(), "r", "/tmp/r", ""); err == nil {
		t.Error("empty branch should error")
	}
}

// --- ShellTester -----------------------------------------------------------

func TestShellTester_DetectsGoProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeShellRunner{always: &shellResponse{out: "PASS\nok"}}
	te := newShellTester(runner)

	res, err := te.RunTests(context.Background(), dir)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.Passed {
		t.Error("expected pass")
	}
	if !strings.Contains(res.Command, "go test") {
		t.Errorf("command = %q", res.Command)
	}
}

func TestShellTester_DetectsNodeProject(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644)
	runner := &fakeShellRunner{always: &shellResponse{out: "ok"}}
	te := newShellTester(runner)
	res, _ := te.RunTests(context.Background(), dir)
	if !strings.Contains(res.Command, "npm") {
		t.Errorf("expected npm command, got %q", res.Command)
	}
}

func TestShellTester_NoManifestReportsSkip(t *testing.T) {
	dir := t.TempDir()
	te := newShellTester(&fakeShellRunner{})
	res, _ := te.RunTests(context.Background(), dir)
	if !res.Passed {
		t.Error("unknown project → Passed=true (no repro possible)")
	}
	if !strings.Contains(res.Output, "no recognized test manifest") {
		t.Errorf("unexpected output: %q", res.Output)
	}
}

func TestShellTester_NonZeroExitMapsToFailNotError(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte(""), 0o644)
	runner := &fakeShellRunner{always: &shellResponse{out: "FAIL\n", err: errors.New("exit 1")}}
	te := newShellTester(runner)
	res, err := te.RunTests(context.Background(), dir)
	if err != nil {
		t.Errorf("non-zero exit should not be an error, got %v", err)
	}
	if res.Passed {
		t.Error("expected Passed=false")
	}
}

// --- ShellPusher -----------------------------------------------------------

func TestShellPusher_RunsGitPush(t *testing.T) {
	runner := &fakeShellRunner{always: &shellResponse{}}
	p := newShellPusher(runner)
	if err := p.Push(context.Background(), "/tmp/r", "feat"); err != nil {
		t.Fatalf("push: %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0].Name != "git" {
		t.Errorf("unexpected calls: %+v", runner.calls)
	}
	joined := strings.Join(runner.calls[0].Args, " ")
	if !strings.Contains(joined, "push -u origin feat") {
		t.Errorf("args = %q", joined)
	}
}

func TestShellPusher_RejectsEmpty(t *testing.T) {
	p := newShellPusher(&fakeShellRunner{})
	if err := p.Push(context.Background(), "", ""); err == nil {
		t.Error("empty args should error")
	}
}

// --- ShellPRCreator --------------------------------------------------------

func TestShellPRCreator_ParsesURL(t *testing.T) {
	runner := &fakeShellRunner{always: &shellResponse{
		out: "https://github.com/owner/repo/pull/42\n",
	}}
	p := newShellPRCreator(runner)
	url, err := p.CreatePR(context.Background(), "/tmp/r", "Fix #1", "body", "feat")
	if err != nil {
		t.Fatalf("pr: %v", err)
	}
	if url != "https://github.com/owner/repo/pull/42" {
		t.Errorf("url = %q", url)
	}
}

func TestShellPRCreator_PropagatesError(t *testing.T) {
	runner := &fakeShellRunner{always: &shellResponse{err: errors.New("gh auth required")}}
	p := newShellPRCreator(runner)
	if _, err := p.CreatePR(context.Background(), "/tmp/r", "t", "b", "h"); err == nil {
		t.Error("expected error")
	}
}

// --- ShellIssueCommenter ---------------------------------------------------

func TestShellIssueCommenter_CallsGh(t *testing.T) {
	runner := &fakeShellRunner{always: &shellResponse{}}
	c := newShellIssueCommenter(runner)
	if err := c.Comment(context.Background(), "a/b", 5, "hello"); err != nil {
		t.Fatalf("comment: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatal("expected one call")
	}
	joined := strings.Join(runner.calls[0].Args, " ")
	if !strings.Contains(joined, "issue comment 5") || !strings.Contains(joined, "--repo a/b") {
		t.Errorf("args = %q", joined)
	}
}

func TestShellIssueCommenter_ValidatesInputs(t *testing.T) {
	c := newShellIssueCommenter(&fakeShellRunner{})
	if err := c.Comment(context.Background(), "", 1, "x"); err == nil {
		t.Error("empty repo should error")
	}
	if err := c.Comment(context.Background(), "a/b", 0, "x"); err == nil {
		t.Error("zero number should error")
	}
	if err := c.Comment(context.Background(), "a/b", 1, ""); err == nil {
		t.Error("empty body should error")
	}
}

// --- NoopFixer --------------------------------------------------------------

func TestNoopFixer_Errors(t *testing.T) {
	_, err := NewNoopFixer().Fix(context.Background(), FixRequest{})
	if err == nil {
		t.Error("noop fixer must error so the workflow fails fast")
	}
}
