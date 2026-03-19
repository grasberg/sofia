package agent

import (
	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/dashboard"
	mcpPkg "github.com/grasberg/sofia/pkg/mcp"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/routing"
	"github.com/grasberg/sofia/pkg/session"
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
