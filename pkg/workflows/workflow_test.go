package workflows

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// --- fakes ------------------------------------------------------------------

type fakeGoalSink struct {
	mu       sync.Mutex
	nextID   int64
	created  []createdGoal
	logs     []logEntry
	finals   []finalEntry
	createErr error
}

type createdGoal struct{ Name, Desc, Agent, Priority string }
type logEntry struct {
	GoalID   int64
	Step     string
	Result   string
	Success  bool
	Duration int64
}
type finalEntry struct {
	GoalID  int64
	Status  string // "complete" | "fail"
	Message string
}

func (f *fakeGoalSink) CreateGoal(agentID, name, desc, prio string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return 0, f.createErr
	}
	f.nextID++
	f.created = append(f.created, createdGoal{Name: name, Desc: desc, Agent: agentID, Priority: prio})
	return f.nextID, nil
}
func (f *fakeGoalSink) CompleteGoal(id int64, msg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finals = append(f.finals, finalEntry{GoalID: id, Status: "complete", Message: msg})
	return nil
}
func (f *fakeGoalSink) FailGoal(id int64, msg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finals = append(f.finals, finalEntry{GoalID: id, Status: "fail", Message: msg})
	return nil
}
func (f *fakeGoalSink) LogStep(id int64, agent, step, result string, ok bool, d int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, logEntry{GoalID: id, Step: step, Result: result, Success: ok, Duration: d})
	return nil
}

type fakeGate struct {
	mu       sync.Mutex
	calls    []string
	approved bool
	err      error
}

func (g *fakeGate) RequestApproval(_ context.Context, id, tool, args, agentID, session, channel, chatID string, _ map[string]string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, id)
	return g.approved, g.err
}

// --- registry tests ---------------------------------------------------------

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	wf := &Workflow{Name: "w", Steps: []WorkflowStep{{Name: "a", Run: noopStep}}}
	if err := r.Register(wf); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, err := r.Get("w")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "w" {
		t.Errorf("got name %q", got.Name)
	}
}

func TestRegistry_Validation_RejectsEmpty(t *testing.T) {
	r := NewRegistry()
	cases := []*Workflow{
		{Name: "", Steps: []WorkflowStep{{Name: "s", Run: noopStep}}},
		{Name: "w", Steps: nil},
		{Name: "w", Steps: []WorkflowStep{{Name: "", Run: noopStep}}},
		{Name: "w", Steps: []WorkflowStep{{Name: "s", Run: nil}}},
		{Name: "w", Steps: []WorkflowStep{{Name: "s", Run: noopStep}, {Name: "s", Run: noopStep}}},
	}
	for i, c := range cases {
		if err := r.Register(c); err == nil {
			t.Errorf("case %d: expected validation error, got nil", i)
		}
	}
}

func TestRegistry_Get_UnknownReturnsError(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("nope"); err == nil {
		t.Error("expected error for unknown workflow")
	}
}

func TestRegistry_Names_Sorted(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&Workflow{Name: "beta", Steps: []WorkflowStep{{Name: "s", Run: noopStep}}})
	r.MustRegister(&Workflow{Name: "alpha", Steps: []WorkflowStep{{Name: "s", Run: noopStep}}})
	got := r.Names()
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Errorf("want [alpha beta], got %v", got)
	}
}

// --- runner tests -----------------------------------------------------------

func noopStep(_ context.Context, _ *StepCtx) (StepResult, error) {
	return StepResult{Output: map[string]any{"noop": true}}, nil
}

func TestRunner_HappyPath_CompletesAllSteps(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(&Workflow{Name: "w", Steps: []WorkflowStep{
		{Name: "s1", Run: func(_ context.Context, sc *StepCtx) (StepResult, error) {
			return StepResult{Output: map[string]any{"s1": "ran"}}, nil
		}},
		{Name: "s2", Run: func(_ context.Context, sc *StepCtx) (StepResult, error) {
			if sc.Output["s1"] != "ran" {
				t.Error("s2 did not see s1's output")
			}
			return StepResult{Output: map[string]any{"s2": "ran"}}, nil
		}},
	}})
	sink := &fakeGoalSink{}

	r := NewRunner(reg, sink, nil)
	res, err := r.Run(context.Background(), "w", "agent", "desc", nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res.Completed) != 2 {
		t.Errorf("completed = %v", res.Completed)
	}
	if res.GoalID == 0 {
		t.Error("GoalID should have been set")
	}
	if len(sink.finals) != 1 || sink.finals[0].Status != "complete" {
		t.Errorf("unexpected finals: %+v", sink.finals)
	}
	if res.Output["s2"] != "ran" {
		t.Error("output map not propagated")
	}
}

func TestRunner_StepError_AbortsWorkflow(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(&Workflow{Name: "w", Steps: []WorkflowStep{
		{Name: "s1", Run: noopStep},
		{Name: "boom", Run: func(_ context.Context, _ *StepCtx) (StepResult, error) {
			return StepResult{}, errors.New("kaboom")
		}},
		{Name: "s3", Run: noopStep},
	}})
	sink := &fakeGoalSink{}

	r := NewRunner(reg, sink, nil)
	res, err := r.Run(context.Background(), "w", "a", "d", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if res.FailedStep != "boom" {
		t.Errorf("FailedStep = %q, want boom", res.FailedStep)
	}
	if len(res.Completed) != 1 || res.Completed[0] != "s1" {
		t.Errorf("completed = %v, want [s1]", res.Completed)
	}
	if len(sink.finals) != 1 || sink.finals[0].Status != "fail" {
		t.Errorf("finals: %+v", sink.finals)
	}
}

func TestRunner_Retries_EventuallySucceeds(t *testing.T) {
	var attempts atomic.Int32
	reg := NewRegistry()
	reg.MustRegister(&Workflow{Name: "w", Steps: []WorkflowStep{
		{Name: "flaky", Retries: 3, BackoffSec: 0, Run: func(_ context.Context, _ *StepCtx) (StepResult, error) {
			if attempts.Add(1) < 3 {
				return StepResult{}, errors.New("transient")
			}
			return StepResult{}, nil
		}},
	}})
	r := NewRunner(reg, nil, nil)
	if _, err := r.Run(context.Background(), "w", "", "", nil); err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if attempts.Load() != 3 {
		t.Errorf("want 3 attempts, got %d", attempts.Load())
	}
}

func TestRunner_Retries_ExhaustAndFail(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(&Workflow{Name: "w", Steps: []WorkflowStep{
		{Name: "always", Retries: 2, BackoffSec: 0, Run: func(_ context.Context, _ *StepCtx) (StepResult, error) {
			return StepResult{}, errors.New("nope")
		}},
	}})
	r := NewRunner(reg, nil, nil)
	if _, err := r.Run(context.Background(), "w", "", "", nil); err == nil {
		t.Fatal("expected failure after exhausted retries")
	}
}

func TestRunner_Halt_EndsEarlySuccessfully(t *testing.T) {
	var s2Ran bool
	reg := NewRegistry()
	reg.MustRegister(&Workflow{Name: "w", Steps: []WorkflowStep{
		{Name: "s1", Run: func(_ context.Context, _ *StepCtx) (StepResult, error) {
			return StepResult{Halt: true, HaltReason: "cant repro"}, nil
		}},
		{Name: "s2", Run: func(_ context.Context, _ *StepCtx) (StepResult, error) {
			s2Ran = true
			return StepResult{}, nil
		}},
	}})
	sink := &fakeGoalSink{}
	r := NewRunner(reg, sink, nil)
	res, err := r.Run(context.Background(), "w", "", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Halted || res.HaltReason != "cant repro" {
		t.Errorf("halt state wrong: %+v", res)
	}
	if s2Ran {
		t.Error("subsequent step should not have run")
	}
	if len(sink.finals) != 1 || sink.finals[0].Status != "complete" {
		t.Errorf("finals: %+v", sink.finals)
	}
}

func TestRunner_Approval_Granted(t *testing.T) {
	gate := &fakeGate{approved: true}
	reg := NewRegistry()
	reg.MustRegister(&Workflow{Name: "w", Steps: []WorkflowStep{
		{Name: "send", Run: func(_ context.Context, _ *StepCtx) (StepResult, error) {
			return StepResult{RequiresApproval: true, ApprovalReason: "unfamiliar sender"}, nil
		}},
	}})
	r := NewRunner(reg, nil, gate)
	if _, err := r.Run(context.Background(), "w", "", "", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gate.calls) != 1 {
		t.Errorf("gate called %d times", len(gate.calls))
	}
}

func TestRunner_Approval_Denied(t *testing.T) {
	gate := &fakeGate{approved: false}
	reg := NewRegistry()
	reg.MustRegister(&Workflow{Name: "w", Steps: []WorkflowStep{
		{Name: "send", Run: func(_ context.Context, _ *StepCtx) (StepResult, error) {
			return StepResult{RequiresApproval: true, ApprovalReason: "big refund"}, nil
		}},
	}})
	r := NewRunner(reg, nil, gate)
	if _, err := r.Run(context.Background(), "w", "", "", nil); err == nil {
		t.Fatal("denial must propagate as error")
	}
}

func TestRunner_ActiveCountReflectsRun(t *testing.T) {
	started := make(chan struct{})
	block := make(chan struct{})
	reg := NewRegistry()
	reg.MustRegister(&Workflow{Name: "w", Steps: []WorkflowStep{
		{Name: "s", Run: func(ctx context.Context, _ *StepCtx) (StepResult, error) {
			close(started)
			<-block
			return StepResult{}, nil
		}},
	}})
	r := NewRunner(reg, nil, nil)

	done := make(chan struct{})
	go func() {
		_, _ = r.Run(context.Background(), "w", "", "", nil)
		close(done)
	}()

	<-started
	if got := r.ActiveCount("w"); got != 1 {
		t.Errorf("active = %d, want 1", got)
	}
	close(block)
	<-done
	if got := r.ActiveCount("w"); got != 0 {
		t.Errorf("after run active = %d, want 0", got)
	}
}
