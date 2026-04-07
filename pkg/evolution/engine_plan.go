package evolution

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/grasberg/sofia/pkg/logger"
	pt "github.com/grasberg/sofia/pkg/providers/protocoltypes"
	"github.com/grasberg/sofia/pkg/utils"
)

// plan asks the LLM to propose evolution actions based on the diagnosis.
func (e *EvolutionEngine) plan(ctx context.Context, diagnosis Diagnosis) ([]EvolutionAction, error) {
	diagJSON, err := json.Marshal(diagnosis)
	if err != nil {
		return nil, fmt.Errorf("marshal diagnosis: %w", err)
	}

	messages := []pt.Message{
		{
			Role: "system",
			Content: "You are an AI system architect. Propose evolution actions. " +
				"Available types: create_agent, retire_agent, tune_agent, " +
				"create_skill, modify_workspace, no_action. " +
				"Be conservative — prefer no_action when metrics are acceptable. " +
				"Respond with valid JSON only, no markdown fences.",
		},
		{
			Role: "user",
			Content: fmt.Sprintf(
				"Based on this diagnosis, propose evolution actions.\n\n"+
					"Diagnosis:\n%s\n\n"+
					"Respond as a JSON array: "+
					"[{\"type\": \"...\", \"agent_id\": \"...\", "+
					"\"params\": {...}, \"reason\": \"...\"}]",
				string(diagJSON),
			),
		},
	}

	resp, err := e.provider.Chat(ctx, messages, nil, e.model, nil)
	if err != nil {
		return nil, fmt.Errorf("planning LLM call: %w", err)
	}

	// Estimate cost from response tokens ($0.01 per 1K tokens as safe default).
	if resp.Usage != nil {
		estimatedCost := float64(resp.Usage.TotalTokens) / 1000.0 * 0.01
		e.budgetSpent += estimatedCost

		// Check budget after spending to prevent overshooting
		if e.cfg.MaxCostPerDay > 0 && e.budgetSpent >= e.cfg.MaxCostPerDay {
			logger.WarnCF("evolution", "Daily budget exceeded during planning", map[string]any{
				"spent": e.budgetSpent,
				"limit": e.cfg.MaxCostPerDay,
				"phase": "plan",
			})
			return nil, fmt.Errorf("daily budget exceeded during planning: $%.2f/$%.2f", e.budgetSpent, e.cfg.MaxCostPerDay)
		}
	}

	content := utils.CleanJSONFences(resp.Content)

	var actions []EvolutionAction
	if err := json.Unmarshal([]byte(content), &actions); err != nil {
		return nil, fmt.Errorf("parse planning response: %w", err)
	}

	logger.InfoCF("evolution", "Planning complete", map[string]any{
		"actions": len(actions),
	})

	return actions, nil
}

// isDestructiveAction returns true for action types that require human approval.
func isDestructiveAction(t ActionType) bool {
	switch t {
	case ActionCreateAgent, ActionRetireAgent, ActionModifyWorkspace:
		return true
	default:
		return false
	}
}

// GetPendingProposals returns all proposals awaiting human approval.
func (e *EvolutionEngine) GetPendingProposals() []Proposal {
	e.mu.Lock()
	defer e.mu.Unlock()
	var pending []Proposal
	for _, p := range e.pendingProposals {
		if p.Status == "pending" {
			pending = append(pending, p)
		}
	}
	return pending
}

// ApproveProposal approves and executes a pending proposal.
func (e *EvolutionEngine) ApproveProposal(ctx context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.pendingProposals {
		if e.pendingProposals[i].ID == id && e.pendingProposals[i].Status == "pending" {
			e.pendingProposals[i].Status = "approved"
			e.executeAction(ctx, e.pendingProposals[i].Action)
			return nil
		}
	}
	return fmt.Errorf("proposal %s not found or not pending", id)
}

// RejectProposal rejects a pending proposal without executing it.
func (e *EvolutionEngine) RejectProposal(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.pendingProposals {
		if e.pendingProposals[i].ID == id && e.pendingProposals[i].Status == "pending" {
			e.pendingProposals[i].Status = "rejected"
			logger.InfoCF("evolution", "Proposal rejected", map[string]any{
				"proposal_id": id,
				"type":        string(e.pendingProposals[i].Action.Type),
			})
			return nil
		}
	}
	return fmt.Errorf("proposal %s not found or not pending", id)
}
