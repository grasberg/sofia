package workflows

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// HeuristicIssueClassifier — the "cheap and safe" default
// ---------------------------------------------------------------------------

// HeuristicIssueClassifier classifies issues using simple keyword rules. By
// design it's conservative: it's much more likely to emit
// IssueClassAmbiguous (proceed with approval) than IssueClassAutoFixable,
// so automatic action is always the result of a deliberate signal.
type HeuristicIssueClassifier struct {
	autoFixable   *regexp.Regexp
	needsHuman    *regexp.Regexp
	complexityCap int // title+body char count above which we always ask
}

// NewHeuristicIssueClassifier returns a classifier with the default keyword
// lists. Customize by passing non-empty overrides.
func NewHeuristicIssueClassifier() *HeuristicIssueClassifier {
	autoFix := `(?i)\b(typo|spelling|misspelling|nil\s*pointer|null\s*pointer|panic|segfault|crash|off[- ]?by[- ]?one|flaky\s*test|broken\s*link|dead\s*link|missing\s*import)\b`
	needsHuman := `(?i)\b(proposal|rfc|discussion|design|architecture|refactor|feature\s*request|enhancement|question|help\s*wanted|good\s*first\s*issue|wontfix|epic|roadmap)\b`

	return &HeuristicIssueClassifier{
		autoFixable:   regexp.MustCompile(autoFix),
		needsHuman:    regexp.MustCompile(needsHuman),
		complexityCap: 4000,
	}
}

// Classify applies heuristics. Priority: needs_human wins, then autoFixable,
// otherwise ambiguous. Over-long issues always fall to ambiguous — no
// autoFixable shortcut when the text is sprawling.
func (c *HeuristicIssueClassifier) Classify(_ context.Context, issue Issue) (IssueClass, string, error) {
	combined := strings.ToLower(issue.Title + "\n" + issue.Body)
	if c.needsHuman != nil && c.needsHuman.MatchString(combined) {
		return IssueClassNeedsHuman, "matched discussion/design keywords", nil
	}
	if len(combined) > c.complexityCap {
		return IssueClassAmbiguous, "issue body exceeds complexity cap", nil
	}
	if c.autoFixable != nil && c.autoFixable.MatchString(combined) {
		return IssueClassAutoFixable, "matched typo/panic/crash keywords", nil
	}
	return IssueClassAmbiguous, "no strong signal; proceeding with approval gates", nil
}

// ---------------------------------------------------------------------------
// ShellCloner — `git clone` + checkout via os/exec
// ---------------------------------------------------------------------------

// shellRunner is a tiny abstraction over exec.Command so tests can inject a
// fake runner without starting real subprocesses.
type shellRunner interface {
	Run(ctx context.Context, dir, name string, args ...string) (string, error)
}

type execShellRunner struct {
	timeout time.Duration
}

func (r *execShellRunner) Run(ctx context.Context, dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.String() + stderr.String()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w — %s",
			name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// newExecShellRunner constructs the production runner.
func newExecShellRunner(timeoutSec int) *execShellRunner {
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	return &execShellRunner{timeout: time.Duration(timeoutSec) * time.Second}
}

// ShellCloner shells out to `gh repo clone` (falling back to `git clone`) and
// checks out the feature branch.
type ShellCloner struct {
	runner shellRunner
}

// NewShellCloner returns a cloner wired to real subprocesses.
func NewShellCloner() *ShellCloner {
	return newShellCloner(newExecShellRunner(300))
}

func newShellCloner(runner shellRunner) *ShellCloner {
	return &ShellCloner{runner: runner}
}

// Clone removes any stale target dir, clones the repo, and creates the
// branch. Parent directories are created if needed.
func (c *ShellCloner) Clone(ctx context.Context, repo, targetDir, branch string) error {
	if repo == "" {
		return errors.New("clone: repo empty")
	}
	if targetDir == "" {
		return errors.New("clone: targetDir empty")
	}
	if branch == "" {
		return errors.New("clone: branch empty")
	}

	// Fresh start — stale leftovers from prior failed runs would confuse git.
	_ = os.RemoveAll(targetDir)
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return fmt.Errorf("clone: mkdir parent: %w", err)
	}

	if _, err := c.runner.Run(ctx, "", "gh", "repo", "clone", repo, targetDir); err != nil {
		// Fall back to plain git in case gh isn't authed for this repo.
		url := fmt.Sprintf("https://github.com/%s.git", repo)
		if _, err2 := c.runner.Run(ctx, "", "git", "clone", url, targetDir); err2 != nil {
			return fmt.Errorf("clone (gh+git both failed): %w / %w", err, err2)
		}
	}
	if _, err := c.runner.Run(ctx, targetDir, "git", "checkout", "-b", branch); err != nil {
		return fmt.Errorf("clone: create branch %s: %w", branch, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// ShellTester — detects the project language and runs the canonical test
// command. Zero-config for Go, Node, Rust, Python projects.
// ---------------------------------------------------------------------------

type ShellTester struct {
	runner shellRunner
}

func NewShellTester() *ShellTester {
	return newShellTester(newExecShellRunner(600))
}

func newShellTester(runner shellRunner) *ShellTester {
	return &ShellTester{runner: runner}
}

// RunTests picks a command based on files present in the repo root.
// When no recognized manifest exists RunTests returns a passing result with
// output explaining the skip — callers interpret "tests passed" as cannot-
// reproduce.
func (t *ShellTester) RunTests(ctx context.Context, dir string) (TestResult, error) {
	name, args, ok := detectTestCommand(dir)
	if !ok {
		return TestResult{
			Passed:  true, // "no test suite" looks like pass to reproduce step → halts with cannot_reproduce
			Output:  "no recognized test manifest in " + dir,
			Command: "(none)",
		}, nil
	}
	out, err := t.runner.Run(ctx, dir, name, args...)
	cmdStr := name + " " + strings.Join(args, " ")
	if err != nil {
		// Non-zero exit is NOT an error for our purposes — it just means
		// tests failed. We pass that back as TestResult{Passed:false}.
		return TestResult{Passed: false, Output: out, Command: cmdStr}, nil
	}
	return TestResult{Passed: true, Output: out, Command: cmdStr}, nil
}

// detectTestCommand looks for familiar manifests and returns the test cmd.
func detectTestCommand(dir string) (string, []string, bool) {
	has := func(p string) bool {
		_, err := os.Stat(filepath.Join(dir, p))
		return err == nil
	}
	switch {
	case has("go.mod"):
		return "go", []string{"test", "./..."}, true
	case has("package.json"):
		return "npm", []string{"test", "--silent"}, true
	case has("Cargo.toml"):
		return "cargo", []string{"test"}, true
	case has("pytest.ini"), has("pyproject.toml"), has("setup.py"):
		return "pytest", []string{"-q"}, true
	}
	return "", nil, false
}

// ---------------------------------------------------------------------------
// ShellPusher — `git push -u origin <branch>`
// ---------------------------------------------------------------------------

type ShellPusher struct {
	runner shellRunner
}

func NewShellPusher() *ShellPusher {
	return newShellPusher(newExecShellRunner(120))
}

func newShellPusher(runner shellRunner) *ShellPusher {
	return &ShellPusher{runner: runner}
}

// Push publishes the branch to the remote named "origin".
func (p *ShellPusher) Push(ctx context.Context, dir, branch string) error {
	if dir == "" || branch == "" {
		return errors.New("push: dir/branch empty")
	}
	if _, err := p.runner.Run(ctx, dir, "git", "push", "-u", "origin", branch); err != nil {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// ShellPRCreator — `gh pr create`
// ---------------------------------------------------------------------------

type ShellPRCreator struct {
	runner shellRunner
}

func NewShellPRCreator() *ShellPRCreator {
	return newShellPRCreator(newExecShellRunner(60))
}

func newShellPRCreator(runner shellRunner) *ShellPRCreator {
	return &ShellPRCreator{runner: runner}
}

// CreatePR opens a PR from `head` into the repo's default branch and returns
// the PR URL parsed from `gh`'s stdout.
func (p *ShellPRCreator) CreatePR(ctx context.Context, dir, title, body, head string) (string, error) {
	if dir == "" || title == "" || head == "" {
		return "", errors.New("pr: dir/title/head empty")
	}
	args := []string{"pr", "create",
		"--head", head,
		"--title", title,
		"--body", body,
	}
	out, err := p.runner.Run(ctx, dir, "gh", args...)
	if err != nil {
		return "", err
	}
	return extractPRURL(out), nil
}

// extractPRURL pulls the first github.com/.../pull/NNN URL from gh's output.
var prURLRegex = regexp.MustCompile(`https://github\.com/[^\s]+/pull/\d+`)

func extractPRURL(s string) string {
	if m := prURLRegex.FindString(s); m != "" {
		return m
	}
	return strings.TrimSpace(s)
}

// ---------------------------------------------------------------------------
// ShellIssueCommenter — `gh issue comment`
// ---------------------------------------------------------------------------

type ShellIssueCommenter struct {
	runner shellRunner
}

func NewShellIssueCommenter() *ShellIssueCommenter {
	return newShellIssueCommenter(newExecShellRunner(30))
}

func newShellIssueCommenter(runner shellRunner) *ShellIssueCommenter {
	return &ShellIssueCommenter{runner: runner}
}

// Comment posts a comment on the issue.
func (c *ShellIssueCommenter) Comment(ctx context.Context, repo string, number int, body string) error {
	if repo == "" || number <= 0 || body == "" {
		return errors.New("comment: missing fields")
	}
	_, err := c.runner.Run(ctx, "",
		"gh", "issue", "comment", fmt.Sprintf("%d", number),
		"--repo", repo, "--body", body)
	return err
}

// ---------------------------------------------------------------------------
// NoopFixer — placeholder for the LLM-backed Fixer
// ---------------------------------------------------------------------------

// NoopFixer returns a consistent error so the workflow fails cleanly when no
// real Fixer has been wired. Replace with an LLM-backed implementation
// before enabling the workflow in production.
type NoopFixer struct{}

// NewNoopFixer returns the zero-value placeholder.
func NewNoopFixer() *NoopFixer { return &NoopFixer{} }

func (NoopFixer) Fix(_ context.Context, _ FixRequest) (FixResult, error) {
	return FixResult{}, errors.New("no Fixer configured — wire an LLM-backed Fixer before enabling fix-github-issue")
}
