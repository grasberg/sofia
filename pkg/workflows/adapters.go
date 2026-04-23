package workflows

import (
	"context"
	"fmt"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/memory"
)

// ---------------------------------------------------------------------------
// GoalSink — autonomy.GoalManager + MemoryDB adapter
// ---------------------------------------------------------------------------

// GoalSinkAdapter implements workflows.GoalSink using the existing goals
// system. It upserts goal nodes via GoalManager and writes per-step rows to
// the goal_log table directly via MemoryDB, which lets the monitor UI show
// workflow progress alongside regular goals.
type GoalSinkAdapter struct {
	goals *autonomy.GoalManager
	memDB *memory.MemoryDB
}

// NewGoalSinkAdapter builds an adapter wired to concrete dependencies. Both
// are required; pass nil explicitly to disable the adapter — callers should
// skip wiring in that case instead.
func NewGoalSinkAdapter(goals *autonomy.GoalManager, memDB *memory.MemoryDB) *GoalSinkAdapter {
	return &GoalSinkAdapter{goals: goals, memDB: memDB}
}

// CreateGoal adds an "active" goal and returns its semantic-node ID.
func (a *GoalSinkAdapter) CreateGoal(agentID, name, description, priority string) (int64, error) {
	if a == nil || a.goals == nil {
		return 0, fmt.Errorf("goal sink: GoalManager not configured")
	}
	raw, err := a.goals.AddGoal(agentID, name, description, priority)
	if err != nil {
		return 0, err
	}
	g, ok := raw.(*autonomy.Goal)
	if !ok || g == nil {
		return 0, fmt.Errorf("goal sink: AddGoal returned %T", raw)
	}
	return g.ID, nil
}

// CompleteGoal transitions the goal to "completed" and stores the summary.
func (a *GoalSinkAdapter) CompleteGoal(goalID int64, summary string) error {
	if a == nil || a.goals == nil {
		return nil
	}
	if _, err := a.goals.UpdateGoalStatus(goalID, autonomy.GoalStatusCompleted); err != nil {
		return err
	}
	if summary != "" {
		_ = a.goals.UpdateGoalResult(goalID, summary)
	}
	return nil
}

// FailGoal transitions the goal to "failed" and records the reason.
func (a *GoalSinkAdapter) FailGoal(goalID int64, reason string) error {
	if a == nil || a.goals == nil {
		return nil
	}
	if _, err := a.goals.UpdateGoalStatus(goalID, autonomy.GoalStatusFailed); err != nil {
		return err
	}
	if reason != "" {
		_ = a.goals.UpdateGoalResult(goalID, reason)
	}
	return nil
}

// LogStep appends a row to goal_log. Failures here are swallowed — a logging
// miss must not abort a live workflow.
func (a *GoalSinkAdapter) LogStep(goalID int64, agentID, step, result string, success bool, durationMs int64) error {
	if a == nil || a.memDB == nil {
		return nil
	}
	return a.memDB.InsertGoalLog(goalID, agentID, step, result, success, durationMs)
}

// ---------------------------------------------------------------------------
// ApprovalGateway — agent.ApprovalGate adapter
// ---------------------------------------------------------------------------

// ApprovalGateAdapter bridges an *agent.ApprovalGate to the workflows
// ApprovalGateway interface, mapping the flat-string call signature onto the
// gate's richer ApprovalRequest struct.
type ApprovalGateAdapter struct {
	gate *agent.ApprovalGate
}

// NewApprovalGateAdapter returns an adapter; nil gate returns a permissive
// no-op stub (never blocks) so workflows still function without approvals.
func NewApprovalGateAdapter(gate *agent.ApprovalGate) *ApprovalGateAdapter {
	return &ApprovalGateAdapter{gate: gate}
}

// RequestApproval forwards to the underlying gate. A nil gate returns
// (true, nil) — permissive — matching StepCtx.RequestApproval semantics.
func (a *ApprovalGateAdapter) RequestApproval(
	ctx context.Context,
	id, toolName, arguments, agentID, sessionKey, channel, chatID string,
	hints map[string]string,
) (bool, error) {
	if a == nil || a.gate == nil {
		return true, nil
	}
	level := agent.RiskLevel(hints["risk_level"])
	return a.gate.RequestApproval(ctx, agent.ApprovalRequest{
		ID:         id,
		ToolName:   toolName,
		Arguments:  arguments,
		AgentID:    agentID,
		SessionKey: sessionKey,
		Channel:    channel,
		ChatID:     chatID,
		RiskLevel:  level,
	})
}

// ---------------------------------------------------------------------------
// KBSearcher / KBUpserter — memory.MemoryDB adapter
// ---------------------------------------------------------------------------

// KBAdapter wraps MemoryDB to satisfy KBSearcher + KBUpserter. One adapter
// covers both interfaces because they share the same backing store.
type KBAdapter struct {
	memDB *memory.MemoryDB
}

// NewKBAdapter builds an adapter. Pass nil to get a permissive no-op
// adapter (Search returns empty, Upsert succeeds silently) — useful in test
// harnesses that don't need real persistence.
func NewKBAdapter(memDB *memory.MemoryDB) *KBAdapter {
	return &KBAdapter{memDB: memDB}
}

// Search forwards to MemoryDB.SearchKBEntries.
func (a *KBAdapter) Search(agentID, query string, topK int) ([]memory.KBEntry, error) {
	if a == nil || a.memDB == nil {
		return nil, nil
	}
	return a.memDB.SearchKBEntries(agentID, query, topK)
}

// Upsert forwards to MemoryDB.UpsertKBEntry.
func (a *KBAdapter) Upsert(agentID, question, answer, source string, tags []string) error {
	if a == nil || a.memDB == nil {
		return nil
	}
	_, err := a.memDB.UpsertKBEntry(agentID, question, answer, source, tags)
	return err
}
