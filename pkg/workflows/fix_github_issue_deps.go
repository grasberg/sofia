package workflows

import "context"

// Constants naming the fix-github-issue workflow and its steps. Exported so
// adapter code, tests, and observability dashboards can reference them
// without stringly-typed drift.
const (
	WorkflowFixGitHubIssue = "fix-github-issue"

	StepClassify      = "classify"
	StepClone         = "clone"
	StepReproduce     = "reproduce"
	StepFixAndVerify  = "fix_and_verify"
	StepPushBranch    = "push_branch"
	StepOpenPR        = "open_pr"

	// Input/output keys for the workflow's StepCtx.
	InputRepo        = "repo"         // "owner/name"
	InputIssueNumber = "issue_number" // int
	InputIssueTitle  = "issue_title"
	InputIssueBody   = "issue_body"
	InputIssueLabels = "issue_labels" // []string
	InputIssueURL    = "issue_url"
	InputCloneRoot   = "clone_root"   // base dir; step appends goal/issue folder
	InputBranchName  = "branch_name"  // pre-computed branch
	InputUseFork     = "use_fork"     // bool

	OutputIssueClass   = "issue_class"
	OutputClassReason  = "class_reason"
	OutputCloneDir     = "clone_dir"
	OutputReproFailed  = "repro_failed"  // bool — true means bug was reproduced (expected)
	OutputReproOutput  = "repro_output"
	OutputTestsPass    = "tests_pass"
	OutputCommitSHA    = "commit_sha"
	OutputDiffSummary  = "diff_summary"
	OutputPushed       = "pushed"
	OutputPRURL        = "pr_url"
)

// IssueClass is the classifier verdict: can Sofia attempt this bug herself,
// or should she hand back to a human?
type IssueClass string

const (
	// IssueClassAutoFixable means heuristics think the bug is small / clear
	// / well-bounded. Workflow proceeds and — when risk is low — auto-pushes
	// a PR. Approval still gates side-effects by policy.
	IssueClassAutoFixable IssueClass = "auto_fixable"

	// IssueClassAmbiguous means Sofia will attempt a fix but every
	// side-effect step requires human approval. Safe default.
	IssueClassAmbiguous IssueClass = "ambiguous"

	// IssueClassNeedsHuman means Sofia should not attempt a fix (e.g.
	// design discussions, architectural proposals, features). Workflow
	// halts early without touching the repo.
	IssueClassNeedsHuman IssueClass = "needs_human"
)

// Issue is the flat view of a GitHub issue the workflow consumes. The poller
// produces one from `gh issue view` JSON; tests construct them directly.
type Issue struct {
	Repo   string   // "owner/name"
	Number int
	Title  string
	Body   string
	Labels []string
	URL    string
}

// TestResult captures what a test-runner invocation produced.
type TestResult struct {
	// Passed is true when the command exited with status 0.
	Passed bool

	// Output is the combined stdout+stderr, truncated so log consumers don't
	// choke on ginormous test logs.
	Output string

	// Command is the textual command that was run, for logs/metrics.
	Command string
}

// FixRequest bundles everything the Fixer needs to propose a patch.
type FixRequest struct {
	Issue        Issue
	CloneDir     string
	BranchName   string
	ReproOutput  string

	// Locale is the agent's preferred language for PR/commit messaging.
	Locale string
}

// FixResult describes the change the Fixer produced. When the Fixer cannot
// fix the issue it returns a non-nil error instead; the workflow then halts
// (and the caller may post an issue comment explaining why).
type FixResult struct {
	// CommitSHA of the fix commit. Empty when Fixer made no commit.
	CommitSHA string

	// DiffSummary is a short human-readable summary of what changed. Used
	// for the PR body and approval preview.
	DiffSummary string

	// FilesChanged counts the files touched — feeds the risk classifier.
	FilesChanged int

	// TestsPass is true when Fixer verified the fix before returning.
	TestsPass bool
}

// IssueClassifier decides whether to attempt a fix at all. Heuristic and
// LLM-backed implementations share this interface.
type IssueClassifier interface {
	Classify(ctx context.Context, issue Issue) (IssueClass, string, error)
}

// Cloner clones a remote repo and checks out (or creates) a branch.
type Cloner interface {
	Clone(ctx context.Context, repo, targetDir, branch string) error
}

// Tester runs the project's test suite in a directory and reports pass/fail.
type Tester interface {
	RunTests(ctx context.Context, dir string) (TestResult, error)
}

// Fixer proposes and applies a patch. Implementations typically loop
// fix→verify internally and return only when tests pass or the attempt is
// abandoned.
type Fixer interface {
	Fix(ctx context.Context, req FixRequest) (FixResult, error)
}

// Pusher pushes a branch to the configured remote.
type Pusher interface {
	Push(ctx context.Context, dir, branch string) error
}

// PRCreator opens a pull request via `gh pr create`. Returns the PR URL.
type PRCreator interface {
	CreatePR(ctx context.Context, dir, title, body, head string) (string, error)
}

// IssueCommenter posts a comment on an issue — used when Sofia can't
// reproduce or can't fix so the human gets context.
type IssueCommenter interface {
	Comment(ctx context.Context, repo string, number int, body string) error
}

// FixGitHubIssueDeps aggregates collaborators. Classifier and Commenter are
// optional (safe no-op behavior when nil); the rest are required.
type FixGitHubIssueDeps struct {
	Classifier     IssueClassifier
	Cloner         Cloner
	Tester         Tester
	Fixer          Fixer
	Pusher         Pusher
	PRCreator      PRCreator
	IssueCommenter IssueCommenter

	// BranchPrefix defines the branch namespace; default "sofia-autofix/".
	BranchPrefix string

	// WorkspaceRoot is the base directory for checkouts. Per-issue
	// subdirs are appended by the workflow.
	WorkspaceRoot string

	// UseFork tells the Pusher/PRCreator to operate on a user fork. The
	// workflow forwards the value via StepCtx so adapters can honor it.
	UseFork bool

	// Locale is forwarded to the Fixer for PR/commit language.
	Locale string
}
