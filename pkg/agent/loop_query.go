package agent

import (
	"fmt"

	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/budget"
	"github.com/grasberg/sofia/pkg/dashboard"
	mcpPkg "github.com/grasberg/sofia/pkg/mcp"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/routing"
	"github.com/grasberg/sofia/pkg/session"
	"github.com/grasberg/sofia/pkg/tools"
)

// GetDefaultSessionManager returns the session manager for the default agent.
// This is used by the web server to expose session history endpoints.
func (al *AgentLoop) GetDefaultSessionManager() *session.SessionManager {
	agent := al.getRegistry().GetDefaultAgent()
	if agent == nil {
		return nil
	}
	return agent.Sessions
}

// DashboardHub returns the dashboard hub for websocket connections.
func (al *AgentLoop) DashboardHub() *dashboard.Hub {
	return al.dashboardHub
}

// GetApprovalGate returns the approval gate, or nil if not configured.
func (al *AgentLoop) GetApprovalGate() *ApprovalGate {
	return al.approvalGate
}

// ListGoals returns all goals across all agents (or for a specific agent).
func (al *AgentLoop) ListGoals(agentID string) ([]*autonomy.Goal, error) {
	if al.memDB == nil {
		return nil, nil
	}
	gm := autonomy.NewGoalManager(al.memDB)
	if agentID != "" {
		return gm.ListAllGoals(agentID)
	}
	// Collect goals from all agents
	var allGoals []*autonomy.Goal
	for _, id := range al.getRegistry().ListAgentIDs() {
		goals, err := gm.ListAllGoals(id)
		if err != nil {
			continue
		}
		allGoals = append(allGoals, goals...)
	}
	return allGoals, nil
}

func (al *AgentLoop) GetStartupInfo() map[string]any {
	info := make(map[string]any)

	agent := al.getRegistry().GetDefaultAgent()
	if agent == nil {
		return info
	}

	// Tools info
	toolsList := agent.Tools.List()
	detailedTools := make([]map[string]string, 0, len(toolsList))
	for _, name := range toolsList {
		if t, ok := agent.Tools.Get(name); ok {
			detailedTools = append(detailedTools, map[string]string{
				"name":        t.Name(),
				"description": t.Description(),
			})
		}
	}

	info["tools"] = map[string]any{
		"count": len(toolsList),
		"names": toolsList,
		"list":  detailedTools,
	}

	// Skills info
	info["skills"] = agent.ContextBuilder.GetSkillsInfo()

	// Agents info — per-agent metadata for Agent Monitor
	allAgents := al.getRegistry().ListAgents()
	agentMeta := make([]map[string]any, 0, len(allAgents))
	for _, a := range allAgents {
		role := "subagent"
		if a.ID == routing.DefaultAgentID {
			role = "sofia"
		}
		// Look up capability description if this agent was created from a template
		capDescription := ""
		if a.Template != "" {
			for _, c := range builtinCapabilities {
				if c.ID == a.Template {
					capDescription = c.Description
					break
				}
			}
		}
		// Fall back to purpose prompt summary
		if capDescription == "" && a.PurposePrompt != "" {
			capDescription = a.PurposePrompt
			if len(capDescription) > 150 {
				capDescription = capDescription[:147] + "..."
			}
		}

		agentMeta = append(agentMeta, map[string]any{
			"id":          a.ID,
			"name":        a.Name,
			"role":        role,
			"model":       a.Model,
			"model_id":    a.ModelID,
			"template":    a.Template,
			"skills":      a.SkillsFilter,
			"description": capDescription,
		})
	}
	info["agents"] = map[string]any{
		"count": len(allAgents),
		"ids":   al.getRegistry().ListAgentIDs(),
		"list":  agentMeta,
		"active": map[string]any{
			"id":     al.activeAgentID.Load(),
			"status": al.activeStatus.Load(),
		},
	}

	return info
}

// GetActivePlan returns the active plan's formatted status, or empty string if no plan.
func (al *AgentLoop) GetActivePlan() map[string]any {
	if al.planManager == nil {
		return nil
	}
	plan := al.planManager.GetActivePlan()
	if plan == nil {
		return nil
	}
	steps := make([]map[string]any, len(plan.Steps))
	for i, s := range plan.Steps {
		steps[i] = map[string]any{
			"index":       s.Index,
			"description": s.Description,
			"status":      string(s.Status),
			"result":      s.Result,
			"assigned_to": s.AssignedTo,
		}
	}
	return map[string]any{
		"id":     plan.ID,
		"goal":   plan.Goal,
		"status": string(plan.Status),
		"steps":  steps,
	}
}

// GetAllPlans returns all plans (active + completed + failed) for the UI.
func (al *AgentLoop) GetAllPlans() []map[string]any {
	if al.planManager == nil {
		return nil
	}
	plans := al.planManager.ListAllPlans()
	result := make([]map[string]any, 0, len(plans))
	for _, plan := range plans {
		steps := make([]map[string]any, len(plan.Steps))
		for i, s := range plan.Steps {
			steps[i] = map[string]any{
				"index":       s.Index,
				"description": s.Description,
				"status":      string(s.Status),
				"result":      s.Result,
				"assigned_to": s.AssignedTo,
				"sub_plan_id": s.SubPlanID,
			}
		}
		result = append(result, map[string]any{
			"id":      plan.ID,
			"goal":    plan.Goal,
			"goal_id": plan.GoalID,
			"status":  string(plan.Status),
			"steps":   steps,
		})
	}
	return result
}

// UpdateGoalStatus updates a goal's status from the web UI.
func (al *AgentLoop) UpdateGoalStatus(goalID int64, status string) error {
	if al.memDB == nil {
		return fmt.Errorf("memory database not available")
	}
	gm := autonomy.NewGoalManager(al.memDB)
	_, err := gm.UpdateGoalStatus(goalID, status)
	return err
}

// DeleteGoal removes a goal and its log from the web UI.
func (al *AgentLoop) DeleteGoal(goalID int64) error {
	if al.memDB == nil {
		return fmt.Errorf("memory database not available")
	}
	gm := autonomy.NewGoalManager(al.memDB)
	return gm.DeleteGoal(goalID)
}

// ListAgentIDs returns all registered agent IDs.
func (al *AgentLoop) ListAgentIDs() []string {
	return al.getRegistry().ListAgentIDs()
}

// ListAgentTools returns tool names for a given agent. If agentID is empty, uses default agent.
func (al *AgentLoop) ListAgentTools(agentID string) []string {
	reg := al.getRegistry()
	if agentID == "" {
		agent := reg.GetDefaultAgent()
		if agent == nil {
			return nil
		}
		return agent.Tools.List()
	}
	agent, ok := reg.GetAgent(agentID)
	if !ok {
		return nil
	}
	return agent.Tools.List()
}

// ListSessionMetas returns lightweight metadata for all sessions.
func (al *AgentLoop) ListSessionMetas() []mcpPkg.SessionMeta {
	sm := al.GetDefaultSessionManager()
	if sm == nil {
		return nil
	}
	sessions := sm.ListSessions()
	result := make([]mcpPkg.SessionMeta, len(sessions))
	for i, s := range sessions {
		result[i] = mcpPkg.SessionMeta{
			Key:          s.Key,
			Channel:      s.Channel,
			Preview:      s.Preview,
			MessageCount: s.MessageCount,
		}
	}
	return result
}

// GetSessionHistory returns messages for a session key.
func (al *AgentLoop) GetSessionHistory(sessionKey string) []mcpPkg.MessageInfo {
	sm := al.GetDefaultSessionManager()
	if sm == nil {
		return nil
	}
	history := sm.GetHistory(sessionKey)
	result := make([]mcpPkg.MessageInfo, len(history))
	for i, m := range history {
		result[i] = mcpPkg.MessageInfo{
			Role:    m.Role,
			Content: m.Content,
		}
	}
	return result
}

// GetMemoryDB returns the shared MemoryDB instance. Used by the web server
// for cross-session search queries.
func (al *AgentLoop) GetMemoryDB() *memory.MemoryDB {
	return al.memDB
}

// GetToolTracker returns the tool performance tracker (may be nil).
func (al *AgentLoop) GetToolTracker() *tools.ToolTracker {
	return al.toolTracker
}

// GetBudgetManager returns the budget manager (may be nil).
func (al *AgentLoop) GetBudgetManager() *budget.BudgetManager {
	return al.budgetManager
}

// GetPlanManager returns the plan manager. Used by web handlers for activity/completed data.
func (al *AgentLoop) GetPlanManager() *tools.PlanManager {
	return al.planManager
}

// GetActiveSubagentTasks returns running subagent tasks across all autonomy services.
func (al *AgentLoop) GetActiveSubagentTasks() []map[string]any {
	al.autonomyMu.Lock()
	defer al.autonomyMu.Unlock()

	var tasks []map[string]any
	for agentID, svc := range al.autonomyServices {
		if svc == nil {
			continue
		}
		subMgr := svc.GetSubagentManager()
		if subMgr == nil {
			continue
		}
		for _, task := range subMgr.ListTasks() {
			tasks = append(tasks, map[string]any{
				"agent_id":    agentID,
				"subagent_id": task.ID,
				"task":        task.Task,
				"label":       task.Label,
				"status":      task.Status,
				"created":     task.Created,
			})
		}
	}
	return tasks
}
