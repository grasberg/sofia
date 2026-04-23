package agent

import (
	"context"
	"testing"
	"time"

	"github.com/grasberg/sofia/pkg/config"
)

func TestHeuristicClassifier_LowByDefault(t *testing.T) {
	c := NewHeuristicClassifier(0, nil)
	got := c.Classify(context.Background(), ToolCallDescriptor{
		ToolName:  "email_send",
		Arguments: `{"body":"Tack för din fråga, vi hör av oss nästa vecka."}`,
	})
	if got != RiskLow {
		t.Errorf("want RiskLow, got %q", got)
	}
}

func TestHeuristicClassifier_LargeAmountTriggersMedium(t *testing.T) {
	c := NewHeuristicClassifier(100, nil)
	cases := []string{
		`refund of $500`,
		`We owe you SEK 2500 back`,
		`€1,200 invoice`,
		`500 USD please`,
	}
	for _, txt := range cases {
		got := c.Classify(context.Background(), ToolCallDescriptor{Arguments: txt})
		if got != RiskMedium {
			t.Errorf("%q: want RiskMedium, got %q", txt, got)
		}
	}
}

func TestHeuristicClassifier_SmallAmountsStayLow(t *testing.T) {
	c := NewHeuristicClassifier(100, nil)
	got := c.Classify(context.Background(), ToolCallDescriptor{
		Arguments: `coupon for $5 discount`,
	})
	if got != RiskLow {
		t.Errorf("want RiskLow for small amount, got %q", got)
	}
}

func TestHeuristicClassifier_AngryHintsTriggerMedium(t *testing.T) {
	c := NewHeuristicClassifier(0, nil)
	cases := []string{
		`I'm going to file a LAWSUIT over this`,
		`Kunden är förbannad`,
		`please cancel my subscription immediately`,
		`this is a scam`,
	}
	for _, txt := range cases {
		got := c.Classify(context.Background(), ToolCallDescriptor{
			Hints: map[string]string{"content": txt},
		})
		if got != RiskMedium {
			t.Errorf("%q: want RiskMedium, got %q", txt, got)
		}
	}
}

func TestHeuristicClassifier_ExtraAngryKeywordsMerge(t *testing.T) {
	c := NewHeuristicClassifier(0, []string{"DPA violation", "GDPR breach"})
	got := c.Classify(context.Background(), ToolCallDescriptor{
		Hints: map[string]string{"content": "this is a GDPR breach, please respond"},
	})
	if got != RiskMedium {
		t.Errorf("extra keyword should match, got %q", got)
	}
}

func TestHeuristicClassifier_ManyFilesChangedHint(t *testing.T) {
	c := NewHeuristicClassifier(0, nil)
	got := c.Classify(context.Background(), ToolCallDescriptor{
		ToolName: "edit_file",
		Hints:    map[string]string{"files_changed": "12"},
	})
	if got != RiskMedium {
		t.Errorf("want RiskMedium when many files changed, got %q", got)
	}

	got = c.Classify(context.Background(), ToolCallDescriptor{
		ToolName: "edit_file",
		Hints:    map[string]string{"files_changed": "2"},
	})
	if got != RiskLow {
		t.Errorf("want RiskLow for small patch, got %q", got)
	}
}

func TestHeuristicClassifier_NegativeSentimentHint(t *testing.T) {
	c := NewHeuristicClassifier(0, nil)
	got := c.Classify(context.Background(), ToolCallDescriptor{
		Hints: map[string]string{"sentiment": "negative"},
	})
	if got != RiskMedium {
		t.Errorf("negative sentiment should flag, got %q", got)
	}
}

// stubClassifier lets tests force a specific risk level without building a
// full heuristic + regex setup.
type stubClassifier struct{ level RiskLevel }

func (s *stubClassifier) Classify(_ context.Context, _ ToolCallDescriptor) RiskLevel {
	return s.level
}

func TestApprovalGate_ClassifierEscalatesUnlistedTool(t *testing.T) {
	cfg := config.ApprovalConfig{Enabled: true, TimeoutSec: 1}
	gate := NewApprovalGate(cfg)

	// No classifier → auto-pass for unlisted tools.
	if gate.RequiresApproval("", "email_send", `{"to":"a@b"}`) {
		t.Fatal("baseline: gate should not require approval without classifier")
	}

	gate.SetClassifier(&stubClassifier{level: RiskMedium})
	if !gate.RequiresApproval("", "email_send", `{"to":"a@b"}`) {
		t.Error("classifier returning Medium must trigger approval")
	}

	gate.SetClassifier(&stubClassifier{level: RiskLow})
	if gate.RequiresApproval("", "email_send", `{"to":"a@b"}`) {
		t.Error("classifier returning Low must not trigger approval")
	}
}

func TestApprovalGate_ClassifierIgnoredWhenBypassed(t *testing.T) {
	cfg := config.ApprovalConfig{Enabled: true, TimeoutSec: 1}
	gate := NewApprovalGate(cfg)
	gate.SetClassifier(&stubClassifier{level: RiskHigh})
	gate.SetBypass("session-1", true)

	if gate.RequiresApproval("session-1", "email_send", "{}") {
		t.Error("bypass must override classifier")
	}
}

func TestApprovalGate_ClassifierIgnoredWhenGateDisabled(t *testing.T) {
	cfg := config.ApprovalConfig{Enabled: false}
	gate := NewApprovalGate(cfg)
	gate.SetClassifier(&stubClassifier{level: RiskHigh})

	if gate.RequiresApproval("", "email_send", "{}") {
		t.Error("disabled gate must short-circuit before classifier")
	}
}

func TestApprovalGate_HeuristicFromConfig(t *testing.T) {
	cfg := config.ApprovalConfig{
		Enabled:             true,
		TimeoutSec:          1,
		RiskClassifier:      "heuristic",
		RiskAmountThreshold: 50,
	}
	gate := NewApprovalGate(cfg)

	if !gate.RequiresApprovalWithHints("", "email_send", `refund $200`, nil) {
		t.Error("heuristic classifier from config should flag money amount")
	}
	if gate.RequiresApprovalWithHints("", "email_send", `thanks`, nil) {
		t.Error("benign content must not require approval")
	}
}

func TestApprovalGate_ClassifyExposesUnknownWhenUnset(t *testing.T) {
	cfg := config.ApprovalConfig{Enabled: true, TimeoutSec: 1}
	gate := NewApprovalGate(cfg)

	if got := gate.Classify(context.Background(), ToolCallDescriptor{}); got != RiskUnknown {
		t.Errorf("unset classifier should yield RiskUnknown, got %q", got)
	}
}

func TestApprovalGate_BroadcastsOnApprove(t *testing.T) {
	cfg := config.ApprovalConfig{Enabled: true, TimeoutSec: 5, DefaultAction: "deny"}
	gate := NewApprovalGate(cfg)

	events := make(chan map[string]any, 8)
	gate.SetBroadcaster(func(ev map[string]any) {
		events <- ev
	})

	done := make(chan bool, 1)
	go func() {
		approved, _ := gate.RequestApproval(context.Background(), ApprovalRequest{
			ID: "abc", ToolName: "email_send",
		})
		done <- approved
	}()

	// First event is the "created" notification.
	select {
	case ev := <-events:
		if ev["type"] != "approval_created" {
			t.Errorf("first event type = %v, want approval_created", ev["type"])
		}
		if ev["id"] != "gate-abc" {
			t.Errorf("first event id = %v, want gate-abc", ev["id"])
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive approval_created event")
	}

	if err := gate.Approve("abc"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	select {
	case ev := <-events:
		if ev["type"] != "approval_resolved" {
			t.Errorf("resolved event type = %v", ev["type"])
		}
		if ev["status"] != "approved" {
			t.Errorf("resolved event status = %v", ev["status"])
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive approval_resolved event")
	}

	select {
	case ok := <-done:
		if !ok {
			t.Error("RequestApproval should have returned true after Approve")
		}
	case <-time.After(time.Second):
		t.Fatal("RequestApproval did not unblock after Approve")
	}
}

func TestApprovalGate_BroadcasterPanicDoesNotCrashGate(t *testing.T) {
	cfg := config.ApprovalConfig{Enabled: true, TimeoutSec: 1}
	gate := NewApprovalGate(cfg)
	gate.SetBroadcaster(func(_ map[string]any) { panic("sink failure") })

	// Request with a short timeout so test doesn't hang; default-deny kicks in.
	approved, err := gate.RequestApproval(context.Background(), ApprovalRequest{
		ID: "p", ToolName: "email_send",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if approved {
		t.Error("default-deny should return false")
	}
}
