// Package workflows provides deterministic, named multi-step flows that
// orchestrate Sofia's tool and subagent surface. A workflow is data (a slice
// of WorkflowStep functions) rather than LLM-improvised planning, so support
// and bug-fix flows behave consistently across runs.
//
// Workflows compose with the existing goals system for UI visibility but do
// NOT depend on the autonomy dispatcher: the runner executes steps directly
// and logs progress via GoalManager + goal_log entries.
package workflows

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// StepResult captures what a step produced and signals follow-up behavior to
// the runner.
type StepResult struct {
	// Output is an opaque map written to the shared StepCtx.Output so later
	// steps can consume it. Keys are step-defined (e.g. "draft", "risk_level").
	Output map[string]any

	// RequiresApproval, when true, instructs the runner to obtain a human
	// approval via ApprovalGate BEFORE this step's side-effects apply. The
	// runner re-invokes Run only after approval is granted when Idempotent
	// is false; when Idempotent is true the initial Run has already happened
	// and approval only gates downstream steps.
	RequiresApproval bool

	// ApprovalReason surfaces a short string in the approval UI so the user
	// understands *why* the step wants human eyes.
	ApprovalReason string

	// Halt ends the workflow successfully — remaining steps are skipped. Use
	// for early-exit paths (e.g. "could not reproduce, commented on issue").
	Halt bool

	// HaltReason appears in the final goal result when Halt is true.
	HaltReason string
}

// StepCtx is the shared mutable state threaded through a workflow run. Steps
// read prior outputs and write their own results here. The ctx.Output map is
// shallow-merged step-by-step to keep reads simple.
type StepCtx struct {
	// Inputs provides the immutable inputs the caller passed to Run.
	Inputs map[string]any

	// Output accumulates step outputs under keys chosen by each step.
	Output map[string]any

	// GoalID is the backing goal row this run is logged against. Zero if the
	// runner was built without a GoalSink.
	GoalID int64

	// Workflow carries the workflow name so downstream code (logs, events)
	// can label events without callers passing it separately.
	Workflow string

	// AgentID of the run (forwarded from Runner.Run).
	AgentID string

	// gate lets a step request approval mid-run. Populated by the runner;
	// nil when the runner was constructed without an ApprovalGateway.
	gate ApprovalGateway
}

// RequestApproval blocks until the request is approved, denied, or times
// out. When no ApprovalGateway is wired, returns (true, nil) — permissive —
// so test harnesses work without a gate. Errors propagate as workflow
// failures.
func (sc *StepCtx) RequestApproval(ctx context.Context, stepName, toolName, argumentsJSON string, hints map[string]string) (bool, error) {
	if sc.gate == nil {
		return true, nil
	}
	id := fmt.Sprintf("%s-%s-%d", sc.Workflow, stepName, sc.GoalID)
	return sc.gate.RequestApproval(ctx, id, toolName, argumentsJSON,
		sc.AgentID, "", "workflow", "", hints)
}

// StringInput fetches a string from Inputs with an empty-string default —
// saves every step from reimplementing the type assertion.
func (sc *StepCtx) StringInput(key string) string {
	if v, ok := sc.Inputs[key].(string); ok {
		return v
	}
	return ""
}

// StringOutput fetches a previously written string output; empty if missing.
func (sc *StepCtx) StringOutput(key string) string {
	if v, ok := sc.Output[key].(string); ok {
		return v
	}
	return ""
}

// StepRunFunc is the body of a step. It receives the shared StepCtx and
// returns a StepResult plus an optional error. A non-nil error aborts the
// workflow; to signal a recoverable early exit, set Result.Halt and return
// nil.
type StepRunFunc func(ctx context.Context, sc *StepCtx) (StepResult, error)

// WorkflowStep declares a single step in a workflow.
type WorkflowStep struct {
	Name string

	// Run executes the step. Required.
	Run StepRunFunc

	// Retries is the maximum retry count on non-approval errors. 0 disables
	// retry (single attempt). Runner uses simple constant backoff.
	Retries int

	// BackoffSec is the wait between retries. 0 defaults to 2 seconds.
	BackoffSec int
}

// Workflow binds a name to an ordered list of steps.
type Workflow struct {
	Name  string
	Steps []WorkflowStep
}

// Validate checks for obvious mistakes in a workflow definition. Run returns
// validation errors so registration bugs surface at boot rather than first
// invocation.
func (w *Workflow) Validate() error {
	if w == nil {
		return errors.New("workflows: nil workflow")
	}
	if w.Name == "" {
		return errors.New("workflows: workflow name is required")
	}
	if len(w.Steps) == 0 {
		return fmt.Errorf("workflows: %q has no steps", w.Name)
	}
	seen := make(map[string]bool, len(w.Steps))
	for i, s := range w.Steps {
		if s.Name == "" {
			return fmt.Errorf("workflows: %q step %d has empty name", w.Name, i)
		}
		if seen[s.Name] {
			return fmt.Errorf("workflows: %q has duplicate step name %q", w.Name, s.Name)
		}
		seen[s.Name] = true
		if s.Run == nil {
			return fmt.Errorf("workflows: %q step %q has nil Run", w.Name, s.Name)
		}
	}
	return nil
}

// backoff returns the configured step backoff or the default.
func (s WorkflowStep) backoff() time.Duration {
	if s.BackoffSec > 0 {
		return time.Duration(s.BackoffSec) * time.Second
	}
	return 2 * time.Second
}
