package evolution

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/grasberg/sofia/pkg/logger"
)

// act executes each planned action and logs results to the changelog.
// Destructive actions are queued as proposals when RequireApproval is enabled.
func (e *EvolutionEngine) act(ctx context.Context, actions []EvolutionAction) {
	for _, action := range actions {
		// Queue destructive actions as proposals when approval is required.
		if e.cfg.RequireApproval && isDestructiveAction(action.Type) {
			proposal := Proposal{
				ID:        uuid.NewString(),
				Action:    action,
				CreatedAt: time.Now().UTC(),
				Status:    "pending",
			}
			e.pendingProposals = append(e.pendingProposals, proposal)
			logger.InfoCF("evolution", "Action queued as pending proposal", map[string]any{
				"proposal_id": proposal.ID,
				"type":        string(action.Type),
				"reason":      action.Reason,
			})
			continue
		}

		e.executeAction(ctx, action)
	}
}

// executeAction runs a single evolution action immediately.
func (e *EvolutionEngine) executeAction(ctx context.Context, action EvolutionAction) {
	switch action.Type {
	case ActionCreateAgent:
		e.actCreateAgent(ctx, action)
	case ActionRetireAgent:
		e.actRetireAgent(action)
	case ActionTuneAgent:
		e.actTuneAgent(action)
	case ActionCreateSkill:
		e.actCreateSkill(ctx, action)
	case ActionModifyWorkspace:
		e.actModifyWorkspace(ctx, action)
	case ActionNoAction:
		logger.DebugCF("evolution", "No action required", map[string]any{
			"reason": action.Reason,
		})
	default:
		logger.WarnCF("evolution", "Unknown action type", map[string]any{
			"type": string(action.Type),
		})
	}
}

func (e *EvolutionEngine) actCreateAgent(ctx context.Context, action EvolutionAction) {
	gap, _ := action.Params["gap"].(string)
	if gap == "" {
		gap = action.Reason
	}

	cfg, err := e.architect.DesignAgent(ctx, gap)
	if err != nil {
		logger.WarnCF("evolution", "Failed to design agent", map[string]any{
			"error": err.Error(),
		})
		return
	}

	if err := e.architect.CreateAgent(ctx, *cfg); err != nil {
		logger.WarnCF("evolution", "Failed to create agent", map[string]any{
			"agent_id": cfg.ID,
			"error":    err.Error(),
		})
		return
	}

	e.logAction(action, fmt.Sprintf("Created agent %s (%s)", cfg.ID, cfg.Name))
}

func (e *EvolutionEngine) actRetireAgent(action EvolutionAction) {
	agentID := action.AgentID
	if agentID == "" {
		logger.WarnCF("evolution", "retire_agent action missing agent_id", nil)
		return
	}

	if err := e.registrar.RemoveAgent(agentID); err != nil {
		logger.WarnCF("evolution", "Failed to remove agent from registry", map[string]any{
			"agent_id": agentID,
			"error":    err.Error(),
		})
	}

	reason := action.Reason
	if reason == "" {
		reason = "retired by evolution engine"
	}
	if err := e.store.MarkRetired(agentID, reason); err != nil {
		logger.WarnCF("evolution", "Failed to mark agent retired in store", map[string]any{
			"agent_id": agentID,
			"error":    err.Error(),
		})
	}

	e.logAction(action, fmt.Sprintf("Retired agent %s: %s", agentID, reason))
}

func (e *EvolutionEngine) actTuneAgent(action EvolutionAction) {
	agentID := action.AgentID
	if agentID == "" {
		logger.WarnCF("evolution", "tune_agent action missing agent_id", nil)
		return
	}

	existing, _, err := e.store.Get(agentID)
	if err != nil || existing == nil {
		logger.WarnCF("evolution", "Cannot tune agent: not found in store", map[string]any{
			"agent_id": agentID,
		})
		return
	}

	// Apply tuning parameters from the action.
	if newPrompt, ok := action.Params["purpose_prompt"].(string); ok && newPrompt != "" {
		existing.PurposePrompt = newPrompt
	}
	if newModel, ok := action.Params["model"].(string); ok && newModel != "" {
		existing.ModelID = newModel
	}

	if err := e.store.Save(agentID, *existing); err != nil {
		logger.WarnCF("evolution", "Failed to save tuned agent config", map[string]any{
			"agent_id": agentID,
			"error":    err.Error(),
		})
		return
	}

	e.logAction(action, fmt.Sprintf("Tuned agent %s", agentID))
}

func (e *EvolutionEngine) actCreateSkill(ctx context.Context, action EvolutionAction) {
	skillID, _ := action.Params["skill_id"].(string)
	skillName, _ := action.Params["name"].(string)
	skillContent, _ := action.Params["content"].(string)

	if skillID == "" || skillName == "" {
		logger.WarnCF("evolution", "create_skill action missing required params", nil)
		return
	}

	// Validate skill ID is a safe slug (no path traversal)
	if strings.Contains(skillID, "/") || strings.Contains(skillID, "\\") || strings.Contains(skillID, "..") {
		logger.WarnCF("evolution", "Invalid skill ID blocked", map[string]any{
			"skill_id": skillID,
		})
		return
	}

	if skillContent == "" {
		skillContent = action.Reason
	}

	content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n",
		skillName, skillName, skillContent)

	skillDir := filepath.Join(e.architect.workspace, "skills", skillID)
	skillPath := filepath.Join(skillDir, "SKILL.md")

	if err := e.modifier.ModifyFile(ctx, skillPath, content); err != nil {
		logger.WarnCF("evolution", "Failed to create skill file", map[string]any{
			"skill_id": skillID,
			"error":    err.Error(),
		})
		return
	}

	e.logAction(action, fmt.Sprintf("Created skill %s (%s)", skillID, skillName))
}

func (e *EvolutionEngine) actModifyWorkspace(ctx context.Context, action EvolutionAction) {
	filePath, _ := action.Params["path"].(string)
	newContent, _ := action.Params["content"].(string)

	if filePath == "" || newContent == "" {
		logger.WarnCF("evolution", "modify_workspace action missing path or content", nil)
		return
	}

	// Validate path is within workspace
	absPath, _ := filepath.Abs(filePath)
	absWorkspace, _ := filepath.Abs(e.architect.workspace)
	if !strings.HasPrefix(absPath, absWorkspace) {
		logger.WarnCF("evolution", "Path traversal blocked", map[string]any{
			"path":      filePath,
			"workspace": e.architect.workspace,
		})
		return
	}

	if err := e.modifier.ModifyFile(ctx, filePath, newContent); err != nil {
		logger.WarnCF("evolution", "Failed to modify workspace file", map[string]any{
			"path":  filePath,
			"error": err.Error(),
		})
		return
	}

	e.logAction(action, fmt.Sprintf("Modified workspace file %s", filePath))
}

// logAction writes a changelog entry for the given action.
func (e *EvolutionEngine) logAction(action EvolutionAction, summary string) {
	var metricBefore float64
	if action.AgentID != "" {
		perf, err := e.tracker.GetAgentPerformance(action.AgentID)
		if err == nil {
			metricBefore = perf.SuccessRate24h
		}
	}

	entry := &ChangelogEntry{
		Timestamp:    time.Now().UTC(),
		Action:       string(action.Type),
		Summary:      summary,
		MetricBefore: metricBefore,
		Details: map[string]any{
			"agent_id": action.AgentID,
			"params":   action.Params,
			"reason":   action.Reason,
		},
	}
	if err := e.changelog.Write(entry); err != nil {
		logger.WarnCF("evolution", "Failed to write changelog entry", map[string]any{
			"error": err.Error(),
		})
	}
}
