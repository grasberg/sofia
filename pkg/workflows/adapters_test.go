package workflows

import (
	"context"
	"testing"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/memory"
)

func openAdaptersDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// --- GoalSinkAdapter --------------------------------------------------------

func TestGoalSinkAdapter_CreateAndComplete(t *testing.T) {
	db := openAdaptersDB(t)
	gm := autonomy.NewGoalManager(db)
	a := NewGoalSinkAdapter(gm, db)

	id, err := a.CreateGoal("agent-1", "workflow:support-reply", "desc", "medium")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero goal ID")
	}

	if err := a.CompleteGoal(id, "all steps done"); err != nil {
		t.Fatalf("complete: %v", err)
	}

	g, err := gm.GetGoalByID(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if g.Status != autonomy.GoalStatusCompleted {
		t.Errorf("status = %q, want completed", g.Status)
	}
}

func TestGoalSinkAdapter_FailPath(t *testing.T) {
	db := openAdaptersDB(t)
	gm := autonomy.NewGoalManager(db)
	a := NewGoalSinkAdapter(gm, db)

	id, _ := a.CreateGoal("agent-1", "x", "d", "low")
	if err := a.FailGoal(id, "sender broke"); err != nil {
		t.Fatalf("fail: %v", err)
	}
	g, _ := gm.GetGoalByID(id)
	if g.Status != autonomy.GoalStatusFailed {
		t.Errorf("status = %q, want failed", g.Status)
	}
}

func TestGoalSinkAdapter_LogStepWritesRow(t *testing.T) {
	db := openAdaptersDB(t)
	gm := autonomy.NewGoalManager(db)
	a := NewGoalSinkAdapter(gm, db)

	id, _ := a.CreateGoal("agent-1", "x", "d", "low")
	if err := a.LogStep(id, "agent-1", "triage", "ok", true, 42); err != nil {
		t.Fatalf("log: %v", err)
	}

	entries, err := db.GetGoalLog(id)
	if err != nil {
		t.Fatalf("get log: %v", err)
	}
	if len(entries) != 1 || entries[0].Step != "triage" {
		t.Errorf("log entries: %+v", entries)
	}
}

func TestGoalSinkAdapter_NilManagerErrs(t *testing.T) {
	a := NewGoalSinkAdapter(nil, nil)
	if _, err := a.CreateGoal("", "n", "d", "low"); err == nil {
		t.Error("nil GoalManager should error on CreateGoal")
	}
	// Complete/Fail/LogStep are tolerant of nil (best-effort).
	if err := a.CompleteGoal(1, "s"); err != nil {
		t.Errorf("complete should tolerate nil mgr, got %v", err)
	}
}

// --- ApprovalGateAdapter ----------------------------------------------------

func TestApprovalGateAdapter_NilIsPermissive(t *testing.T) {
	a := NewApprovalGateAdapter(nil)
	ok, err := a.RequestApproval(context.Background(), "id", "tool", "{}", "a", "", "", "", nil)
	if err != nil || !ok {
		t.Errorf("nil gate must return (true, nil), got (%v, %v)", ok, err)
	}
}

func TestApprovalGateAdapter_ForwardsRiskLevelHint(t *testing.T) {
	gate := agent.NewApprovalGate(config.ApprovalConfig{
		Enabled:       true,
		TimeoutSec:    1,
		DefaultAction: "allow",
	})
	a := NewApprovalGateAdapter(gate)

	// With DefaultAction=allow, the 1-sec timeout lets the request time out
	// and the adapter returns true. That's enough to exercise the forwarding
	// code without a live approver.
	ok, err := a.RequestApproval(context.Background(), "req-1", "email_send", `{}`,
		"agent-1", "", "workflow", "", map[string]string{"risk_level": "medium"})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if !ok {
		t.Error("default-allow timeout should yield true")
	}
}

// --- KBAdapter --------------------------------------------------------------

func TestKBAdapter_UpsertAndSearchRoundTrip(t *testing.T) {
	db := openAdaptersDB(t)
	kb := NewKBAdapter(db)

	if err := kb.Upsert("agent-1", "How to reset password",
		"Go to Settings → Security → Reset.", "email:m-1", []string{"account"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	hits, err := kb.Search("agent-1", "password reset", 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	if hits[0].Answer == "" {
		t.Error("answer should round-trip")
	}
}

func TestKBAdapter_NilSafe(t *testing.T) {
	a := NewKBAdapter(nil)
	hits, err := a.Search("a", "q", 3)
	if err != nil || len(hits) != 0 {
		t.Errorf("nil adapter should return empty, got (%v, %v)", hits, err)
	}
	if err := a.Upsert("a", "q", "ans", "", nil); err != nil {
		t.Errorf("nil adapter upsert should no-op, got %v", err)
	}
}
