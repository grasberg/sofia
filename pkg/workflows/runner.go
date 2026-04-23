package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
)

// GoalSink writes workflow progress into the goal/log system so the Sofia
// monitor tab shows each run. Implementations are expected to be thread-safe.
//
// This is the minimum surface required from autonomy.GoalManager + MemoryDB;
// callers inject concrete impls to avoid pulling the autonomy package into
// the workflows module (which would create an import cycle since future
// workflows may want to invoke goals-related tools).
type GoalSink interface {
	CreateGoal(agentID, name, description, priority string) (int64, error)
	CompleteGoal(goalID int64, summary string) error
	FailGoal(goalID int64, reason string) error
	LogStep(goalID int64, agentID, step, result string, success bool, durationMs int64) error
}

// ApprovalGateway is the subset of agent.ApprovalGate needed to request human
// approval. Held as an interface so tests can inject a stub.
type ApprovalGateway interface {
	RequestApproval(ctx context.Context, id, toolName, arguments, agentID, sessionKey, channel, chatID string, hints map[string]string) (bool, error)
}

// Runner executes registered workflows. It is goroutine-safe; callers may
// Run multiple workflows concurrently.
type Runner struct {
	registry *Registry
	goals    GoalSink
	gate     ApprovalGateway

	activeMu sync.Mutex
	active   map[string]int // workflow name -> active run count
}

// NewRunner builds a runner over the given registry. goals and gate may be
// nil; when nil, the corresponding features (goal tracking, approval gating)
// degrade gracefully to no-ops.
func NewRunner(registry *Registry, goals GoalSink, gate ApprovalGateway) *Runner {
	if registry == nil {
		registry = Default
	}
	return &Runner{
		registry: registry,
		goals:    goals,
		gate:     gate,
		active:   make(map[string]int),
	}
}

// RunResult summarizes the outcome of a Run call.
type RunResult struct {
	GoalID      int64          `json:"goal_id,omitempty"`
	Workflow    string         `json:"workflow"`
	Completed   []string       `json:"completed_steps"`
	Halted      bool           `json:"halted,omitempty"`
	HaltReason  string         `json:"halt_reason,omitempty"`
	FailedStep  string         `json:"failed_step,omitempty"`
	FailedError string         `json:"failed_error,omitempty"`
	Output      map[string]any `json:"output"`
}

// Run executes the named workflow to completion or first error. The
// agentID/description string are stored on the backing goal so it shows up
// in the monitor tab; pass empty strings if you don't have them.
func (r *Runner) Run(ctx context.Context, workflowName string, agentID, description string, inputs map[string]any) (*RunResult, error) {
	wf, err := r.registry.Get(workflowName)
	if err != nil {
		return nil, err
	}

	r.trackStart(workflowName)
	defer r.trackEnd(workflowName)

	res := &RunResult{
		Workflow:  workflowName,
		Completed: make([]string, 0, len(wf.Steps)),
		Output:    make(map[string]any),
	}

	sc := &StepCtx{
		Inputs:   cloneMap(inputs),
		Output:   res.Output,
		Workflow: workflowName,
		AgentID:  agentID,
		gate:     r.gate,
	}

	if r.goals != nil {
		goalName := fmt.Sprintf("workflow:%s", workflowName)
		if description == "" {
			description = "automated workflow run"
		}
		if id, err := r.goals.CreateGoal(agentID, goalName, description, "medium"); err == nil {
			sc.GoalID = id
			res.GoalID = id
		} else {
			logger.WarnCF("workflows", "Failed to create backing goal; continuing without tracking",
				map[string]any{"workflow": workflowName, "error": err.Error()})
		}
	}

	for _, step := range wf.Steps {
		if err := ctx.Err(); err != nil {
			r.failGoal(res, step.Name, err.Error())
			return res, err
		}

		stepRes, err := r.runStep(ctx, step, sc, agentID)
		if err != nil {
			r.failGoal(res, step.Name, err.Error())
			res.FailedStep = step.Name
			res.FailedError = err.Error()
			return res, err
		}

		// Shallow-merge step outputs.
		for k, v := range stepRes.Output {
			sc.Output[k] = v
		}
		res.Completed = append(res.Completed, step.Name)

		if stepRes.Halt {
			res.Halted = true
			res.HaltReason = stepRes.HaltReason
			if r.goals != nil && sc.GoalID != 0 {
				_ = r.goals.CompleteGoal(sc.GoalID, "halted: "+stepRes.HaltReason)
			}
			return res, nil
		}
	}

	if r.goals != nil && sc.GoalID != 0 {
		_ = r.goals.CompleteGoal(sc.GoalID, "all steps succeeded")
	}
	return res, nil
}

// runStep handles a single step: optional approval, retries, logging.
func (r *Runner) runStep(ctx context.Context, step WorkflowStep, sc *StepCtx, agentID string) (StepResult, error) {
	// Peek approval: run the step's body once in "dry check" mode is not
	// feasible without complicating the StepRunFunc contract, so we require
	// approval up front for steps that are always gated. Steps whose need
	// depends on dynamic inputs must return RequiresApproval and the runner
	// re-requests approval before moving to the NEXT step.
	//
	// In practice the pattern is: a "risk_check" step writes RiskLevel to
	// output, then the next step's Run consults sc.Output and short-circuits
	// if approval was denied (by returning an explanatory error).

	var attempts = step.Retries + 1
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		start := time.Now()
		result, err := step.Run(ctx, sc)
		dur := time.Since(start).Milliseconds()

		if err == nil {
			r.logStep(sc, step.Name, agentID, "ok", true, dur)

			if result.RequiresApproval && r.gate != nil {
				ok, approvalErr := r.gate.RequestApproval(
					ctx,
					fmt.Sprintf("%s-%s-%d", sc.Workflow, step.Name, sc.GoalID),
					"workflow:"+sc.Workflow+":"+step.Name,
					shortJSON(result.Output),
					agentID,
					"", "workflow", "",
					approvalHints(sc, result),
				)
				if approvalErr != nil {
					return result, fmt.Errorf("step %q approval: %w", step.Name, approvalErr)
				}
				if !ok {
					return result, fmt.Errorf("step %q denied by approver (reason: %s)",
						step.Name, result.ApprovalReason)
				}
			}
			return result, nil
		}

		lastErr = err
		r.logStep(sc, step.Name, agentID, err.Error(), false, dur)

		if attempt < attempts {
			logger.WarnCF("workflows", "Step failed, retrying", map[string]any{
				"workflow": sc.Workflow, "step": step.Name,
				"attempt": attempt, "error": err.Error(),
			})
			select {
			case <-ctx.Done():
				return StepResult{}, ctx.Err()
			case <-time.After(step.backoff()):
			}
		}
	}

	return StepResult{}, fmt.Errorf("step %q failed after %d attempts: %w",
		step.Name, attempts, lastErr)
}

// approvalHints extracts classifier-friendly hints from accumulated state.
// Steps may seed sc.Output with "sentiment", "files_changed", "subject", or
// "content" to drive the risk classifier without the runner peeking at
// step-internal details.
func approvalHints(sc *StepCtx, r StepResult) map[string]string {
	out := make(map[string]string, 4)
	copyString := func(k string) {
		if v, ok := sc.Output[k].(string); ok && v != "" {
			out[k] = v
		}
	}
	copyString("sentiment")
	copyString("subject")
	copyString("content")
	copyString("files_changed")
	if r.ApprovalReason != "" {
		out["reason"] = r.ApprovalReason
	}
	return out
}

// shortJSON marshals a map compactly and truncates — approval UI only needs
// a preview, not the full payload.
func shortJSON(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	if len(b) > 512 {
		return string(b[:512]) + "…"
	}
	return string(b)
}

func (r *Runner) logStep(sc *StepCtx, step, agentID, result string, success bool, durMs int64) {
	if r.goals == nil || sc.GoalID == 0 {
		return
	}
	if err := r.goals.LogStep(sc.GoalID, agentID, step, result, success, durMs); err != nil {
		logger.DebugCF("workflows", "Failed to log step",
			map[string]any{"error": err.Error()})
	}
}

func (r *Runner) failGoal(res *RunResult, step, reason string) {
	if r.goals == nil || res.GoalID == 0 {
		return
	}
	_ = r.goals.FailGoal(res.GoalID, fmt.Sprintf("step %q failed: %s", step, reason))
}

func (r *Runner) trackStart(name string) {
	r.activeMu.Lock()
	r.active[name]++
	r.activeMu.Unlock()
}

func (r *Runner) trackEnd(name string) {
	r.activeMu.Lock()
	if r.active[name] > 0 {
		r.active[name]--
	}
	r.activeMu.Unlock()
}

// ActiveCount returns the number of in-flight runs for a workflow.
func (r *Runner) ActiveCount(name string) int {
	r.activeMu.Lock()
	defer r.activeMu.Unlock()
	return r.active[name]
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
