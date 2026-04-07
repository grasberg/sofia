package evolution

import (
	"context"
	"fmt"

	"github.com/grasberg/sofia/pkg/logger"
)

// verify checks recent unverified changelog entries and evaluates outcomes.
func (e *EvolutionEngine) verify(_ context.Context) {
	entries, err := e.changelog.QueryUnverified(10)
	if err != nil {
		logger.WarnCF("evolution", "Failed to query unverified entries", map[string]any{
			"error": err.Error(),
		})
		return
	}

	for _, entry := range entries {
		outcome := e.evaluateOutcome(entry)
		if err := e.changelog.UpdateOutcome(entry.ID, outcome); err != nil {
			logger.WarnCF("evolution", "Failed to update outcome", map[string]any{
				"entry_id": entry.ID,
				"error":    err.Error(),
			})
		}

		if outcome.Result == "degraded" {
			logger.WarnCF("evolution", "Action resulted in degradation, consider revert", map[string]any{
				"entry_id": entry.ID,
				"action":   entry.Action,
				"summary":  entry.Summary,
			})
		}
	}
}

// evaluateOutcome compares current metrics vs baseline for a changelog entry.
func (e *EvolutionEngine) evaluateOutcome(entry ChangelogEntry) ActionOutcome {
	// Extract agent_id from entry details for metric comparison.
	agentID, _ := entry.Details["agent_id"].(string)
	if agentID == "" {
		return ActionOutcome{Result: "no_change"}
	}

	perf, err := e.tracker.GetAgentPerformance(agentID)
	if err != nil {
		return ActionOutcome{Result: "no_change"}
	}

	metricAfter := perf.SuccessRate24h
	metricBefore := entry.MetricBefore

	outcome := ActionOutcome{
		MetricBefore: metricBefore,
		MetricAfter:  metricAfter,
	}

	delta := metricAfter - metricBefore
	switch {
	case delta > 0.05:
		outcome.Result = "improved"
	case delta < -0.05:
		outcome.Result = "degraded"
	default:
		outcome.Result = "no_change"
	}

	return outcome
}

// Revert reverts a specific changelog entry by ID.
func (e *EvolutionEngine) Revert(id string) error {
	entry, err := e.changelog.Get(id)
	if err != nil {
		return fmt.Errorf("get changelog entry: %w", err)
	}
	if entry == nil {
		return fmt.Errorf("changelog entry %s not found", id)
	}

	switch entry.Action {
	case string(ActionRetireAgent):
		agentID, _ := entry.Details["agent_id"].(string)
		if agentID != "" {
			// Re-activate in store.
			existing, _, err := e.store.Get(agentID)
			if err == nil && existing != nil {
				_ = e.store.Save(agentID, *existing)
				e.a2a.Register(agentID)
			}
		}
	case string(ActionModifyWorkspace):
		// Revert would need the backup path. Log a warning for now.
		logger.WarnCF("evolution", "Workspace modification revert requires manual backup restore",
			map[string]any{"entry_id": id})
	default:
		logger.InfoCF("evolution", "Revert not supported for action type", map[string]any{
			"action": entry.Action,
		})
	}

	// Mark the entry as reverted.
	return e.changelog.UpdateOutcome(id, ActionOutcome{Result: "reverted"})
}
