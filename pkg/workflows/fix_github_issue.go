package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/grasberg/sofia/pkg/logger"
)

// RegisterFixGitHubIssue installs the fix-github-issue workflow in registry.
// Returns an error when required collaborators are missing — config bugs
// surface at boot rather than first invocation.
func RegisterFixGitHubIssue(r *Registry, deps FixGitHubIssueDeps) error {
	if r == nil {
		return fmt.Errorf("fix_github_issue: registry is nil")
	}
	if deps.Cloner == nil {
		return fmt.Errorf("fix_github_issue: Cloner is required")
	}
	if deps.Tester == nil {
		return fmt.Errorf("fix_github_issue: Tester is required")
	}
	if deps.Fixer == nil {
		return fmt.Errorf("fix_github_issue: Fixer is required")
	}
	if deps.Pusher == nil {
		return fmt.Errorf("fix_github_issue: Pusher is required")
	}
	if deps.PRCreator == nil {
		return fmt.Errorf("fix_github_issue: PRCreator is required")
	}
	if deps.Classifier == nil {
		deps.Classifier = NewHeuristicIssueClassifier()
	}
	if deps.BranchPrefix == "" {
		deps.BranchPrefix = "sofia-autofix/"
	}
	if deps.Locale == "" {
		deps.Locale = "sv"
	}

	wf := &Workflow{
		Name: WorkflowFixGitHubIssue,
		Steps: []WorkflowStep{
			buildClassifyStep(deps),
			buildCloneStep(deps),
			buildReproduceStep(deps),
			buildFixAndVerifyStep(deps),
			buildPushStep(deps),
			buildOpenPRStep(deps),
		},
	}
	return r.Register(wf)
}

// inputsToIssue assembles an Issue from StepCtx.Inputs. Used by each step
// that needs issue context.
func inputsToIssue(sc *StepCtx) Issue {
	labels, _ := sc.Inputs[InputIssueLabels].([]string)
	return Issue{
		Repo:   sc.StringInput(InputRepo),
		Number: intInput(sc.Inputs, InputIssueNumber),
		Title:  sc.StringInput(InputIssueTitle),
		Body:   sc.StringInput(InputIssueBody),
		Labels: labels,
		URL:    sc.StringInput(InputIssueURL),
	}
}

func intInput(in map[string]any, key string) int {
	switch v := in[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// buildClassifyStep decides whether to proceed. NeedsHuman → halt early with
// a best-effort comment back to the issue author.
func buildClassifyStep(deps FixGitHubIssueDeps) WorkflowStep {
	return WorkflowStep{
		Name:    StepClassify,
		Retries: 1,
		Run: func(ctx context.Context, sc *StepCtx) (StepResult, error) {
			issue := inputsToIssue(sc)
			class, reason, err := deps.Classifier.Classify(ctx, issue)
			if err != nil {
				return StepResult{}, fmt.Errorf("classify: %w", err)
			}
			out := map[string]any{
				OutputIssueClass:  string(class),
				OutputClassReason: reason,
			}

			if class == IssueClassNeedsHuman {
				if deps.IssueCommenter != nil {
					msg := fmt.Sprintf(
						"Sofia skipped this issue because it looks like it needs human judgement: %s",
						reason,
					)
					_ = deps.IssueCommenter.Comment(ctx, issue.Repo, issue.Number, msg)
				}
				return StepResult{
					Output:     out,
					Halt:       true,
					HaltReason: fmt.Sprintf("needs_human: %s", reason),
				}, nil
			}
			return StepResult{Output: out}, nil
		},
	}
}

// buildCloneStep creates the per-issue workspace, clones the repo, and
// checks out the feature branch.
func buildCloneStep(deps FixGitHubIssueDeps) WorkflowStep {
	return WorkflowStep{
		Name:    StepClone,
		Retries: 2,
		Run: func(ctx context.Context, sc *StepCtx) (StepResult, error) {
			issue := inputsToIssue(sc)
			cloneRoot := sc.StringInput(InputCloneRoot)
			if cloneRoot == "" {
				cloneRoot = deps.WorkspaceRoot
			}
			if cloneRoot == "" {
				return StepResult{}, fmt.Errorf("clone: no workspace root configured")
			}

			dir := filepath.Join(cloneRoot,
				fmt.Sprintf("%s-%d", slugRepo(issue.Repo), issue.Number))
			branch := sc.StringInput(InputBranchName)
			if branch == "" {
				branch = fmt.Sprintf("%s%d", deps.BranchPrefix, issue.Number)
			}

			if err := deps.Cloner.Clone(ctx, issue.Repo, dir, branch); err != nil {
				return StepResult{}, fmt.Errorf("clone: %w", err)
			}
			return StepResult{Output: map[string]any{
				OutputCloneDir:  dir,
				InputBranchName: branch,
			}}, nil
		},
	}
}

// buildReproduceStep runs the tests expecting them to FAIL (confirming the
// bug exists). Tests passing before the fix is a strong signal the repro
// isn't in the test suite — we halt with an issue comment asking for clarity.
func buildReproduceStep(deps FixGitHubIssueDeps) WorkflowStep {
	return WorkflowStep{
		Name: StepReproduce,
		Run: func(ctx context.Context, sc *StepCtx) (StepResult, error) {
			dir := sc.StringOutput(OutputCloneDir)
			if dir == "" {
				return StepResult{}, fmt.Errorf("reproduce: clone_dir missing")
			}
			tr, err := deps.Tester.RunTests(ctx, dir)
			if err != nil {
				return StepResult{}, fmt.Errorf("reproduce: %w", err)
			}
			out := map[string]any{
				OutputReproOutput: truncateTestOutput(tr.Output),
				OutputReproFailed: !tr.Passed,
			}

			if tr.Passed {
				// Tests pass → cannot reproduce. Comment & halt.
				if deps.IssueCommenter != nil {
					issue := inputsToIssue(sc)
					msg := "Sofia attempted to reproduce this bug but the test suite passed on a fresh checkout. " +
						"Could you share a failing test case or repro steps?"
					_ = deps.IssueCommenter.Comment(ctx, issue.Repo, issue.Number, msg)
				}
				return StepResult{
					Output:     out,
					Halt:       true,
					HaltReason: "cannot_reproduce",
				}, nil
			}
			return StepResult{Output: out}, nil
		},
	}
}

// buildFixAndVerifyStep delegates to the Fixer which handles the fix/verify
// inner loop itself (it knows best how to iterate). When the fixer cannot
// converge it returns an error and the workflow fails.
func buildFixAndVerifyStep(deps FixGitHubIssueDeps) WorkflowStep {
	return WorkflowStep{
		Name: StepFixAndVerify,
		Run: func(ctx context.Context, sc *StepCtx) (StepResult, error) {
			issue := inputsToIssue(sc)
			req := FixRequest{
				Issue:       issue,
				CloneDir:    sc.StringOutput(OutputCloneDir),
				BranchName:  sc.StringOutput(InputBranchName),
				ReproOutput: sc.StringOutput(OutputReproOutput),
				Locale:      deps.Locale,
			}
			fr, err := deps.Fixer.Fix(ctx, req)
			if err != nil {
				if deps.IssueCommenter != nil {
					msg := fmt.Sprintf(
						"Sofia attempted an automated fix but couldn't converge: %s",
						err.Error(),
					)
					_ = deps.IssueCommenter.Comment(ctx, issue.Repo, issue.Number, msg)
				}
				return StepResult{}, fmt.Errorf("fix: %w", err)
			}
			if !fr.TestsPass {
				return StepResult{}, fmt.Errorf("fix: tests still failing after fix attempt")
			}
			return StepResult{Output: map[string]any{
				OutputCommitSHA:   fr.CommitSHA,
				OutputDiffSummary: fr.DiffSummary,
				OutputTestsPass:   true,
				"files_changed":   fr.FilesChanged, // feeds classifier for push/pr approval
			}}, nil
		},
	}
}

// buildPushStep pushes the branch. Always approval-gated: pushing is a
// side-effect on an external repository.
func buildPushStep(deps FixGitHubIssueDeps) WorkflowStep {
	return WorkflowStep{
		Name: StepPushBranch,
		Run: func(ctx context.Context, sc *StepCtx) (StepResult, error) {
			dir := sc.StringOutput(OutputCloneDir)
			branch := sc.StringOutput(InputBranchName)

			hints := pushHints(sc)
			args := pushApprovalArgs(sc)
			ok, err := sc.RequestApproval(ctx, StepPushBranch, "git_push", args, hints)
			if err != nil {
				return StepResult{}, fmt.Errorf("push approval: %w", err)
			}
			if !ok {
				return StepResult{}, fmt.Errorf("push: denied by approver")
			}

			if err := deps.Pusher.Push(ctx, dir, branch); err != nil {
				return StepResult{}, fmt.Errorf("push: %w", err)
			}
			return StepResult{Output: map[string]any{
				OutputPushed: true,
			}}, nil
		},
	}
}

// buildOpenPRStep opens a PR via `gh pr create` after a separate approval.
// Kept separate from push so a reviewer can approve the push-for-review and
// stop before the PR is filed.
func buildOpenPRStep(deps FixGitHubIssueDeps) WorkflowStep {
	return WorkflowStep{
		Name: StepOpenPR,
		Run: func(ctx context.Context, sc *StepCtx) (StepResult, error) {
			issue := inputsToIssue(sc)
			dir := sc.StringOutput(OutputCloneDir)
			branch := sc.StringOutput(InputBranchName)

			hints := map[string]string{
				"repo":        issue.Repo,
				"issue":       fmt.Sprintf("#%d", issue.Number),
				"branch":      branch,
				"risk_level":  sc.StringOutput(OutputIssueClass),
				"diff_summary": sc.StringOutput(OutputDiffSummary),
			}

			argsBytes, _ := json.Marshal(map[string]string{
				"repo":   issue.Repo,
				"branch": branch,
				"title":  prTitle(issue),
			})
			ok, err := sc.RequestApproval(ctx, StepOpenPR, "github_pr_create",
				string(argsBytes), hints)
			if err != nil {
				return StepResult{}, fmt.Errorf("pr approval: %w", err)
			}
			if !ok {
				return StepResult{}, fmt.Errorf("pr: denied by approver")
			}

			title := prTitle(issue)
			body := prBody(issue, sc.StringOutput(OutputDiffSummary), deps.Locale)
			url, err := deps.PRCreator.CreatePR(ctx, dir, title, body, branch)
			if err != nil {
				return StepResult{}, fmt.Errorf("pr create: %w", err)
			}
			return StepResult{Output: map[string]any{
				OutputPRURL: url,
			}}, nil
		},
	}
}

func pushHints(sc *StepCtx) map[string]string {
	return map[string]string{
		"repo":         sc.StringInput(InputRepo),
		"branch":       sc.StringOutput(InputBranchName),
		"files_changed": fmt.Sprintf("%v", sc.Output["files_changed"]),
		"risk_level":   sc.StringOutput(OutputIssueClass),
	}
}

func pushApprovalArgs(sc *StepCtx) string {
	payload := map[string]any{
		"repo":          sc.StringInput(InputRepo),
		"branch":        sc.StringOutput(InputBranchName),
		"commit_sha":    sc.StringOutput(OutputCommitSHA),
		"diff_summary":  sc.StringOutput(OutputDiffSummary),
		"files_changed": sc.Output["files_changed"],
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(b)
}

// slugRepo turns "owner/repo" into a filesystem-friendly slug.
func slugRepo(repo string) string {
	return strings.ReplaceAll(repo, "/", "-")
}

// truncateTestOutput keeps log payloads manageable when tests produce gigantic
// output (e.g. integration suites).
func truncateTestOutput(s string) string {
	const maxLen = 8 * 1024
	if len(s) <= maxLen {
		return s
	}
	head := s[:maxLen/2]
	tail := s[len(s)-maxLen/2:]
	return head + "\n… [truncated " + fmt.Sprintf("%d", len(s)-maxLen) + " bytes] …\n" + tail
}

func prTitle(issue Issue) string {
	title := strings.TrimSpace(issue.Title)
	if title == "" {
		title = fmt.Sprintf("Fix #%d", issue.Number)
	} else {
		title = fmt.Sprintf("Fix #%d: %s", issue.Number, title)
	}
	if len(title) > 72 {
		title = title[:72] + "…"
	}
	return title
}

func prBody(issue Issue, diffSummary, locale string) string {
	var b strings.Builder
	if strings.ToLower(locale) == "sv" {
		fmt.Fprintf(&b, "Fixar #%d.\n\n", issue.Number)
		if diffSummary != "" {
			b.WriteString("**Sammanfattning:**\n")
			b.WriteString(diffSummary)
			b.WriteString("\n\n")
		}
		b.WriteString("Genererad av Sofia (autonom agent).")
	} else {
		fmt.Fprintf(&b, "Fixes #%d.\n\n", issue.Number)
		if diffSummary != "" {
			b.WriteString("**Summary:**\n")
			b.WriteString(diffSummary)
			b.WriteString("\n\n")
		}
		b.WriteString("Generated by Sofia (autonomous agent).")
	}
	logger.DebugCF("workflows", "PR body composed", map[string]any{
		"issue":  issue.Number,
		"length": b.Len(),
	})
	return b.String()
}
