package web

import (
	"testing"
	"time"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/evolution"
)

func TestToolCallToUnified_PrefixesIDAndCopiesRisk(t *testing.T) {
	req := agent.ApprovalRequest{
		ID:        "abc",
		ToolName:  "email_send",
		Arguments: `{"to":"a@b"}`,
		AgentID:   "coder-1",
		Channel:   "web",
		ChatID:    "room",
		RiskLevel: agent.RiskHigh,
		Status:    "pending",
		CreatedAt: time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC),
	}
	u := toolCallToUnified(req)

	if u.ID != "gate-abc" {
		t.Errorf("ID = %q, want gate-abc", u.ID)
	}
	if u.Kind != "tool_call" {
		t.Errorf("Kind = %q, want tool_call", u.Kind)
	}
	if u.RiskLevel != "high" {
		t.Errorf("RiskLevel = %q, want high", u.RiskLevel)
	}
	if u.Title != "email_send" {
		t.Errorf("Title = %q, want email_send", u.Title)
	}
	if u.Arguments == "" {
		t.Error("Arguments should round-trip")
	}
}

func TestProposalToUnified_PrefixesIDAndCopiesReason(t *testing.T) {
	p := evolution.Proposal{
		ID:        "xyz",
		Status:    "pending",
		CreatedAt: time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC),
		Action: evolution.EvolutionAction{
			Type:    evolution.ActionCreateAgent,
			AgentID: "new-specialist",
			Reason:  "fill capability gap",
			Params:  map[string]any{"purpose": "refactoring"},
		},
	}
	u := proposalToUnified(p)

	if u.ID != "evo-xyz" {
		t.Errorf("ID = %q, want evo-xyz", u.ID)
	}
	if u.Kind != "evolution" {
		t.Errorf("Kind = %q, want evolution", u.Kind)
	}
	if u.Summary != "fill capability gap" {
		t.Errorf("Summary = %q", u.Summary)
	}
	if u.Action["type"] != string(evolution.ActionCreateAgent) {
		t.Errorf("Action type = %v", u.Action["type"])
	}
	if u.Action["params"] == nil {
		t.Error("Action params should round-trip")
	}
}
