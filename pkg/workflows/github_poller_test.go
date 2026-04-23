package workflows

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
)

// --- fakes for poller tests ------------------------------------------------

type fakeIssueLister struct {
	byRepo map[string][]Issue
	errs   map[string]error
	calls  atomic.Int32
}

func (f *fakeIssueLister) List(_ context.Context, repo, _ string) ([]Issue, error) {
	f.calls.Add(1)
	if err, ok := f.errs[repo]; ok {
		return nil, err
	}
	return f.byRepo[repo], nil
}

type fakeProcessedStore struct {
	mu     sync.Mutex
	seen   map[string]bool
	marked []string
}

func newFakeProcessedStore() *fakeProcessedStore {
	return &fakeProcessedStore{seen: map[string]bool{}}
}

func (f *fakeProcessedStore) IsProcessed(repo string, number int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seen[processedKey(repo, number)], nil
}

func (f *fakeProcessedStore) MarkProcessed(repo string, number int, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := processedKey(repo, number)
	f.seen[key] = true
	f.marked = append(f.marked, key)
	return nil
}

type fakeWorkflowRunner struct {
	mu     sync.Mutex
	runs   []map[string]any
	err    error
	delay  time.Duration
}

func (f *fakeWorkflowRunner) Run(ctx context.Context, _, _, _ string, inputs map[string]any) (*RunResult, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, inputs)
	return &RunResult{}, f.err
}

// --- tests -----------------------------------------------------------------

func TestPoller_DispatchesNewIssues(t *testing.T) {
	lister := &fakeIssueLister{byRepo: map[string][]Issue{
		"a/b": {
			{Repo: "a/b", Number: 1, Title: "bug one"},
			{Repo: "a/b", Number: 2, Title: "bug two"},
		},
	}}
	store := newFakeProcessedStore()
	runner := &fakeWorkflowRunner{}
	p, err := NewGitHubPoller(GitHubPollerConfig{Repos: []string{"a/b"}}, lister, store, runner)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if err := p.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(runner.runs) != 2 {
		t.Errorf("runs = %d, want 2", len(runner.runs))
	}
	if len(store.marked) != 2 {
		t.Errorf("marked = %d, want 2", len(store.marked))
	}
}

func TestPoller_SkipsAlreadyProcessed(t *testing.T) {
	lister := &fakeIssueLister{byRepo: map[string][]Issue{
		"a/b": {
			{Repo: "a/b", Number: 1, Title: "bug one"},
			{Repo: "a/b", Number: 2, Title: "bug two"},
		},
	}}
	store := newFakeProcessedStore()
	// Pre-mark #1 so only #2 should dispatch.
	_ = store.MarkProcessed("a/b", 1, "bug one")
	runner := &fakeWorkflowRunner{}
	p, _ := NewGitHubPoller(GitHubPollerConfig{Repos: []string{"a/b"}}, lister, store, runner)

	if err := p.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(runner.runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runner.runs))
	}
	if runner.runs[0][InputIssueNumber].(int) != 2 {
		t.Errorf("wrong issue dispatched: %v", runner.runs[0][InputIssueNumber])
	}
}

func TestPoller_ListError_ContinuesWithOtherRepos(t *testing.T) {
	lister := &fakeIssueLister{
		byRepo: map[string][]Issue{
			"ok/one": {{Repo: "ok/one", Number: 1}},
		},
		errs: map[string]error{
			"broken/repo": errors.New("403"),
		},
	}
	store := newFakeProcessedStore()
	runner := &fakeWorkflowRunner{}
	p, _ := NewGitHubPoller(GitHubPollerConfig{
		Repos: []string{"broken/repo", "ok/one"},
	}, lister, store, runner)

	if err := p.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(runner.runs) != 1 {
		t.Errorf("broken repo error should not block others; runs=%d", len(runner.runs))
	}
}

func TestPoller_RespectsMaxConcurrent(t *testing.T) {
	lister := &fakeIssueLister{byRepo: map[string][]Issue{
		"a/b": {
			{Repo: "a/b", Number: 1},
			{Repo: "a/b", Number: 2},
			{Repo: "a/b", Number: 3},
			{Repo: "a/b", Number: 4},
		},
	}}
	store := newFakeProcessedStore()
	runner := &fakeWorkflowRunner{delay: 50 * time.Millisecond}
	p, _ := NewGitHubPoller(GitHubPollerConfig{
		Repos: []string{"a/b"}, MaxConcurrent: 2,
	}, lister, store, runner)

	// Kick poll in background and sample the inflight count while it's busy.
	done := make(chan struct{})
	go func() {
		_ = p.Poll(context.Background())
		close(done)
	}()

	peak := int32(0)
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if v := int32(p.Inflight()); v > peak {
			peak = v
		}
		select {
		case <-done:
			break
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	<-done

	if peak > 2 {
		t.Errorf("in-flight peaked at %d, want ≤ 2", peak)
	}
	if len(runner.runs) != 4 {
		t.Errorf("eventually all 4 should run; got %d", len(runner.runs))
	}
}

func TestPoller_DedupeBeforeDispatch_NoDoublePR(t *testing.T) {
	// Same issue returned twice simulates a second poll race.
	lister := &fakeIssueLister{byRepo: map[string][]Issue{
		"a/b": {
			{Repo: "a/b", Number: 7, Title: "dup"},
			{Repo: "a/b", Number: 7, Title: "dup"},
		},
	}}
	store := newFakeProcessedStore()
	runner := &fakeWorkflowRunner{}
	p, _ := NewGitHubPoller(GitHubPollerConfig{Repos: []string{"a/b"}}, lister, store, runner)

	if err := p.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(runner.runs) != 1 {
		t.Errorf("duplicate in listing should still dispatch once; runs=%d", len(runner.runs))
	}
}

func TestPoller_EmptyReposNoOp(t *testing.T) {
	p, _ := NewGitHubPoller(GitHubPollerConfig{}, &fakeIssueLister{}, newFakeProcessedStore(), &fakeWorkflowRunner{})
	if err := p.Poll(context.Background()); err != nil {
		t.Errorf("empty repos should be no-op, got %v", err)
	}
}

// --- SemanticGraphProcessedStore --------------------------------------------

type fakeProcessedBackend struct {
	mu    sync.Mutex
	nodes map[string]memory.SemanticNode // key = agent|label|name
}

func newFakeProcessedBackend() *fakeProcessedBackend {
	return &fakeProcessedBackend{nodes: map[string]memory.SemanticNode{}}
}

func (f *fakeProcessedBackend) FindNodes(agent, label, name string, _ int) ([]memory.SemanticNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := agent + "|" + label + "|" + name
	if n, ok := f.nodes[key]; ok {
		return []memory.SemanticNode{n}, nil
	}
	return nil, nil
}

func (f *fakeProcessedBackend) UpsertNode(agent, label, name, props string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := agent + "|" + label + "|" + name
	f.nodes[key] = memory.SemanticNode{
		AgentID: agent, Label: label, Name: name, Properties: props,
	}
	return int64(len(f.nodes)), nil
}

func TestSemanticGraphProcessedStore_RoundTrip(t *testing.T) {
	backend := newFakeProcessedBackend()
	s := NewSemanticGraphProcessedStore(backend)

	seen, err := s.IsProcessed("a/b", 1)
	if err != nil || seen {
		t.Fatalf("fresh store should be unseen, got (%v, %v)", seen, err)
	}

	if err := s.MarkProcessed("a/b", 1, "title"); err != nil {
		t.Fatalf("mark: %v", err)
	}
	seen, _ = s.IsProcessed("a/b", 1)
	if !seen {
		t.Error("expected seen after mark")
	}

	// Different number → not seen.
	seen, _ = s.IsProcessed("a/b", 2)
	if seen {
		t.Error("different issue number must be independent")
	}
}

func TestSemanticGraphProcessedStore_NilSafe(t *testing.T) {
	s := NewSemanticGraphProcessedStore(nil)
	seen, err := s.IsProcessed("x", 1)
	if err != nil || seen {
		t.Errorf("nil backend should report unseen; got (%v, %v)", seen, err)
	}
	if err := s.MarkProcessed("x", 1, "t"); err != nil {
		t.Errorf("nil backend mark should no-op; got %v", err)
	}
}

// --- ParseIssueList ---------------------------------------------------------

func TestParseIssueList_HappyPath(t *testing.T) {
	out := `[
		{"number":1, "title":"bug one", "body":"detail", "url":"https://gh.com/owner/repo/issues/1",
		 "labels":[{"name":"sofia-autofix"},{"name":"bug"}]}
	]`
	issues, err := parseIssueList(out, "owner/repo")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].Repo != "owner/repo" {
		t.Errorf("repo = %q", issues[0].Repo)
	}
	if len(issues[0].Labels) != 2 {
		t.Errorf("labels: %v", issues[0].Labels)
	}
}

func TestParseIssueList_EmptyReturnsNil(t *testing.T) {
	issues, err := parseIssueList("", "a/b")
	if err != nil || issues != nil {
		t.Errorf("empty input should yield (nil, nil), got (%v, %v)", issues, err)
	}
}

func TestParseIssueList_MalformedReturnsError(t *testing.T) {
	_, err := parseIssueList(`{not json`, "a/b")
	if err == nil {
		t.Error("malformed input should error")
	}
}
