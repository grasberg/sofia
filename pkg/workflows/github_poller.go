package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
)

const (
	// githubIssueNodeLabel is the semantic-node label used to remember
	// which issues we've already dispatched a workflow for. Reusing the
	// existing semantic_nodes storage avoids a new migration.
	githubIssueNodeLabel = "GitHubIssueProcessed"

	// githubPollerAgentID scopes the dedupe nodes. A constant keeps them
	// distinguishable from regular agent memory.
	githubPollerAgentID = "system:github-poller"
)

// IssueLister queries GitHub for issues matching a label+state. Production
// uses `gh issue list`; tests inject a fake.
type IssueLister interface {
	List(ctx context.Context, repo, label string) ([]Issue, error)
}

// ProcessedStore tracks which issues have already been dispatched to the
// workflow so restarts + poll jitter don't produce duplicate PRs.
type ProcessedStore interface {
	IsProcessed(repo string, number int) (bool, error)
	MarkProcessed(repo string, number int, title string) error
}

// WorkflowRunner is the narrow subset of *Runner the poller needs. Using an
// interface lets tests inject a capture-only fake.
type WorkflowRunner interface {
	Run(ctx context.Context, name, agentID, description string, inputs map[string]any) (*RunResult, error)
}

// GitHubPollerConfig bundles behavior knobs. Zero-value fields fall back to
// sensible defaults (Label=sofia-autofix, MaxConcurrent=2).
type GitHubPollerConfig struct {
	Repos         []string
	Label         string
	MaxConcurrent int
	BranchPrefix  string
	CloneRoot     string
	UseFork       bool
	Locale        string
}

// GitHubPoller scans configured repos for eligible issues and dispatches the
// fix-github-issue workflow for each unseen one. Safe to invoke concurrently
// (a mutex guards the in-flight counter).
type GitHubPoller struct {
	cfg       GitHubPollerConfig
	lister    IssueLister
	processed ProcessedStore
	runner    WorkflowRunner

	mu       sync.Mutex
	inflight int
}

// NewGitHubPoller wires the collaborators. Returns an error when required
// collaborators are missing.
func NewGitHubPoller(cfg GitHubPollerConfig, lister IssueLister, processed ProcessedStore, runner WorkflowRunner) (*GitHubPoller, error) {
	if lister == nil {
		return nil, fmt.Errorf("github poller: lister required")
	}
	if processed == nil {
		return nil, fmt.Errorf("github poller: processed store required")
	}
	if runner == nil {
		return nil, fmt.Errorf("github poller: runner required")
	}
	if cfg.Label == "" {
		cfg.Label = "sofia-autofix"
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 2
	}
	return &GitHubPoller{
		cfg:       cfg,
		lister:    lister,
		processed: processed,
		runner:    runner,
	}, nil
}

// Poll runs one iteration: for each configured repo, list eligible issues,
// mark newly-seen ones as processed, and dispatch the workflow. Marking
// happens BEFORE dispatch so a crash between steps still prevents reruns
// (the alternative — mark after success — would re-submit PRs on restart).
func (p *GitHubPoller) Poll(ctx context.Context) error {
	if len(p.cfg.Repos) == 0 {
		return nil
	}

	sem := make(chan struct{}, p.cfg.MaxConcurrent)
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for _, repo := range p.cfg.Repos {
		if err := ctx.Err(); err != nil {
			return err
		}
		issues, err := p.lister.List(ctx, repo, p.cfg.Label)
		if err != nil {
			logger.WarnCF("workflows", "issue list failed",
				map[string]any{"repo": repo, "error": err.Error()})
			continue
		}

		for _, issue := range issues {
			if err := ctx.Err(); err != nil {
				wg.Wait()
				return err
			}

			seen, err := p.processed.IsProcessed(issue.Repo, issue.Number)
			if err != nil {
				logger.WarnCF("workflows", "processed lookup failed",
					map[string]any{"repo": issue.Repo, "number": issue.Number, "error": err.Error()})
				continue
			}
			if seen {
				continue
			}
			if err := p.processed.MarkProcessed(issue.Repo, issue.Number, issue.Title); err != nil {
				logger.WarnCF("workflows", "mark processed failed",
					map[string]any{"repo": issue.Repo, "number": issue.Number, "error": err.Error()})
				continue
			}

			wg.Add(1)
			sem <- struct{}{}
			p.incInflight()
			go func(issue Issue) {
				defer func() {
					<-sem
					p.decInflight()
					wg.Done()
				}()
				if err := p.dispatch(ctx, issue); err != nil {
					logger.WarnCF("workflows", "dispatch failed",
						map[string]any{"repo": issue.Repo, "number": issue.Number, "error": err.Error()})
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
				}
			}(issue)
		}
	}

	wg.Wait()
	return firstErr
}

func (p *GitHubPoller) dispatch(ctx context.Context, issue Issue) error {
	inputs := map[string]any{
		InputRepo:        issue.Repo,
		InputIssueNumber: issue.Number,
		InputIssueTitle:  issue.Title,
		InputIssueBody:   issue.Body,
		InputIssueLabels: issue.Labels,
		InputIssueURL:    issue.URL,
		InputCloneRoot:   p.cfg.CloneRoot,
		InputUseFork:     p.cfg.UseFork,
	}

	descr := fmt.Sprintf("%s#%d: %s", issue.Repo, issue.Number, truncateTestOutput(issue.Title))
	_, err := p.runner.Run(ctx, WorkflowFixGitHubIssue, githubPollerAgentID, descr, inputs)
	return err
}

// Start runs Poll immediately, then every `interval` until ctx is cancelled.
// Returns when ctx is done. Safe to run in a goroutine; intermediate errors
// are logged but don't terminate the loop.
func (p *GitHubPoller) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}

	tick := func() {
		pollCtx, cancel := context.WithTimeout(ctx, interval-time.Second)
		defer cancel()
		if err := p.Poll(pollCtx); err != nil {
			logger.WarnCF("workflows", "github poll cycle ended with error",
				map[string]any{"error": err.Error()})
		}
	}
	tick()

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick()
		}
	}
}

// Inflight reports how many dispatches are currently running — useful for
// metrics and kill-switch logic.
func (p *GitHubPoller) Inflight() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inflight
}

func (p *GitHubPoller) incInflight() {
	p.mu.Lock()
	p.inflight++
	p.mu.Unlock()
}

func (p *GitHubPoller) decInflight() {
	p.mu.Lock()
	if p.inflight > 0 {
		p.inflight--
	}
	p.mu.Unlock()
}

// ---------------------------------------------------------------------------
// ShellIssueLister — `gh issue list --json ... --label ...`
// ---------------------------------------------------------------------------

// ShellIssueLister invokes `gh issue list` and parses its JSON output.
type ShellIssueLister struct {
	runner shellRunner
}

// NewShellIssueLister returns the production lister.
func NewShellIssueLister() *ShellIssueLister {
	return newShellIssueLister(newExecShellRunner(60))
}

func newShellIssueLister(runner shellRunner) *ShellIssueLister {
	return &ShellIssueLister{runner: runner}
}

// List shells out to gh and decodes the JSON array it returns.
func (l *ShellIssueLister) List(ctx context.Context, repo, label string) ([]Issue, error) {
	if repo == "" {
		return nil, fmt.Errorf("lister: repo empty")
	}
	args := []string{
		"issue", "list",
		"--repo", repo,
		"--state", "open",
		"--json", "number,title,body,labels,url",
	}
	if label != "" {
		args = append(args, "--label", label)
	}
	out, err := l.runner.Run(ctx, "", "gh", args...)
	if err != nil {
		return nil, err
	}
	return parseIssueList(out, repo)
}

// rawGHIssue matches gh's JSON shape.
type rawGHIssue struct {
	Number int             `json:"number"`
	Title  string          `json:"title"`
	Body   string          `json:"body"`
	URL    string          `json:"url"`
	Labels []rawGHIssueLab `json:"labels"`
}

type rawGHIssueLab struct {
	Name string `json:"name"`
}

// parseIssueList decodes gh's JSON output into []Issue. Exported only via
// tests — keeps the regex around if gh's format ever drifts.
func parseIssueList(out, repo string) ([]Issue, error) {
	out = strings.TrimSpace(out)
	if out == "" || out == "null" {
		return nil, nil
	}
	var raw []rawGHIssue
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse gh output: %w", err)
	}
	issues := make([]Issue, 0, len(raw))
	for _, r := range raw {
		labels := make([]string, 0, len(r.Labels))
		for _, l := range r.Labels {
			if l.Name != "" {
				labels = append(labels, l.Name)
			}
		}
		issues = append(issues, Issue{
			Repo:   repo,
			Number: r.Number,
			Title:  r.Title,
			Body:   r.Body,
			Labels: labels,
			URL:    r.URL,
		})
	}
	return issues, nil
}

// ---------------------------------------------------------------------------
// SemanticGraphProcessedStore — piggybacks on memDB.UpsertNode for dedupe
// ---------------------------------------------------------------------------

// processedBackend is the narrow subset of memory.MemoryDB the store uses.
// An interface keeps the workflows package testable without the real DB.
type processedBackend interface {
	FindNodes(agentID, label, namePattern string, limit int) ([]memory.SemanticNode, error)
	UpsertNode(agentID, label, name, properties string) (int64, error)
}

// SemanticGraphProcessedStore tracks which "owner/repo#N" keys have been
// dispatched. Backed by the semantic_nodes table (label=GitHubIssueProcessed).
type SemanticGraphProcessedStore struct {
	db processedBackend
}

// NewSemanticGraphProcessedStore builds a store on top of the given backend.
// Pass nil to get a no-op store (every issue looks new — useful in tests
// that handle dedupe out of band).
func NewSemanticGraphProcessedStore(db processedBackend) *SemanticGraphProcessedStore {
	return &SemanticGraphProcessedStore{db: db}
}

// IsProcessed reports whether the key has been marked.
func (s *SemanticGraphProcessedStore) IsProcessed(repo string, number int) (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}
	key := processedKey(repo, number)
	nodes, err := s.db.FindNodes(githubPollerAgentID, githubIssueNodeLabel, key, 1)
	if err != nil {
		return false, err
	}
	for _, n := range nodes {
		if n.Name == key {
			return true, nil
		}
	}
	return false, nil
}

// MarkProcessed writes a node so the next poll skips the issue. Safe to call
// repeatedly; UpsertNode is idempotent.
func (s *SemanticGraphProcessedStore) MarkProcessed(repo string, number int, title string) error {
	if s == nil || s.db == nil {
		return nil
	}
	key := processedKey(repo, number)
	props := map[string]any{"title": title}
	b, err := json.Marshal(props)
	if err != nil {
		return err
	}
	_, err = s.db.UpsertNode(githubPollerAgentID, githubIssueNodeLabel, key, string(b))
	return err
}

func processedKey(repo string, number int) string {
	return fmt.Sprintf("%s#%d", repo, number)
}
