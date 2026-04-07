package evolution

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/grasberg/sofia/pkg/logger"
	pt "github.com/grasberg/sofia/pkg/providers/protocoltypes"
	"github.com/grasberg/sofia/pkg/utils"
)

// observe gathers runtime metrics from all agents and tools.
func (e *EvolutionEngine) observe(_ context.Context) ObservationReport {
	report := ObservationReport{
		AgentStats:   make(map[string]*AgentPerfSnapshot),
		ToolFailures: make(map[string]int),
	}

	agentIDs := e.registrar.ListAgentIDs()
	for _, id := range agentIDs {
		perf, err := e.tracker.GetAgentPerformance(id)
		if err != nil {
			logger.DebugCF("evolution", "Failed to get performance for agent", map[string]any{
				"agent_id": id,
				"error":    err.Error(),
			})
			continue
		}
		report.AgentStats[id] = &AgentPerfSnapshot{
			AgentID:     id,
			SuccessRate: perf.SuccessRate24h,
			TaskCount:   perf.TaskCount24h,
			AvgScore:    perf.AvgScore24h,
			Trend:       perf.Trend,
		}
		report.TotalTasks += perf.TaskCount24h
	}

	// Gather tool failure stats.
	if e.toolStats != nil {
		stats := e.toolStats.GetStats()
		for tool, v := range stats {
			if count, ok := v.(int); ok {
				report.ToolFailures[tool] = count
			}
		}
	}

	// Compute overall error rate.
	if report.TotalTasks > 0 {
		totalFailures := 0
		for _, snap := range report.AgentStats {
			if snap.TaskCount > 0 {
				totalFailures += int(float64(snap.TaskCount) * (1 - snap.SuccessRate))
			}
		}
		report.ErrorRate = float64(totalFailures) / float64(report.TotalTasks)
	}

	logger.InfoCF("evolution", "Observation complete", map[string]any{
		"agents":      len(report.AgentStats),
		"total_tasks": report.TotalTasks,
		"error_rate":  report.ErrorRate,
	})

	return report
}

// diagnose sends the observation report to the LLM for analysis.
func (e *EvolutionEngine) diagnose(ctx context.Context, report ObservationReport) (Diagnosis, error) {
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return Diagnosis{}, fmt.Errorf("marshal observation report: %w", err)
	}

	messages := []pt.Message{
		{
			Role: "system",
			Content: "You are an AI system analyst. Analyze the provided metrics and " +
				"identify issues. Respond with valid JSON only, no markdown fences.",
		},
		{
			Role: "user",
			Content: fmt.Sprintf(
				"Analyze these agent system metrics and identify issues.\n\n"+
					"Metrics:\n%s\n\n"+
					"Respond in JSON: "+
					"{\"capability_gaps\": [\"...\"], "+
					"\"underperformers\": [\"agent_id\", ...], "+
					"\"success_patterns\": [\"...\"], "+
					"\"prompt_suggestions\": [\"...\"]}",
				string(reportJSON),
			),
		},
	}

	resp, err := e.provider.Chat(ctx, messages, nil, e.model, nil)
	if err != nil {
		return Diagnosis{}, fmt.Errorf("diagnosis LLM call: %w", err)
	}

	// Estimate cost from response tokens ($0.01 per 1K tokens as safe default).
	if resp.Usage != nil {
		estimatedCost := float64(resp.Usage.TotalTokens) / 1000.0 * 0.01
		e.budgetSpent += estimatedCost

		// Check budget after spending to prevent overshooting
		if e.cfg.MaxCostPerDay > 0 && e.budgetSpent >= e.cfg.MaxCostPerDay {
			logger.WarnCF("evolution", "Daily budget exceeded during diagnosis", map[string]any{
				"spent": e.budgetSpent,
				"limit": e.cfg.MaxCostPerDay,
				"phase": "diagnose",
			})
			return Diagnosis{}, fmt.Errorf("daily budget exceeded during diagnosis: $%.2f/$%.2f", e.budgetSpent, e.cfg.MaxCostPerDay)
		}
	}

	content := utils.CleanJSONFences(resp.Content)

	var diagnosis Diagnosis
	if err := json.Unmarshal([]byte(content), &diagnosis); err != nil {
		return Diagnosis{}, fmt.Errorf("parse diagnosis response: %w", err)
	}

	logger.InfoCF("evolution", "Diagnosis complete", map[string]any{
		"capability_gaps":  len(diagnosis.CapabilityGaps),
		"underperformers":  len(diagnosis.Underperformers),
		"success_patterns": len(diagnosis.SuccessPatterns),
	})

	return diagnosis, nil
}
