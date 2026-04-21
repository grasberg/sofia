package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/mcp"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/routing"
)

// AgentRegistry manages multiple agent instances and routes messages to them.
type AgentRegistry struct {
	agents     map[string]*AgentInstance
	resolver   *routing.RouteResolver
	mu         sync.RWMutex
	mcpManager *mcp.GlobalManager
}

// NewAgentRegistry creates a registry from config, instantiating all agents.
func NewAgentRegistry(
	cfg *config.Config,
	provider providers.LLMProvider,
	memDB *memory.MemoryDB,
) *AgentRegistry {
	mcpMgr := mcp.NewGlobalManager()
	if cfg.Tools.MCP != nil && len(cfg.Tools.MCP) > 0 {
		_ = mcpMgr.EnsureServers(context.Background(), cfg.Tools.MCP)
	}

	registry := &AgentRegistry{
		agents:     make(map[string]*AgentInstance),
		resolver:   routing.NewRouteResolver(cfg),
		mcpManager: mcpMgr,
	}

	// If no user-added subagents exist (only the implicit "main"), seed the
	// agent list with entries from all available templates. This gives users a
	// ready-to-use set of specialist agents out of the box.
	agentConfigs := cfg.Agents.List
	if !hasUserSubagents(agentConfigs) {
		agentConfigs = seedAgentsFromTemplates(agentConfigs)
		cfg.Agents.List = agentConfigs
	}

	if len(agentConfigs) == 0 {
		implicitAgent := &config.AgentConfig{
			ID:      "main",
			Default: true,
		}
		instance := NewAgentInstance(implicitAgent, &cfg.Agents.Defaults, cfg, provider, memDB, registry.mcpManager)
		registry.agents["main"] = instance
		logger.InfoCF("agent", "Created implicit main agent (no agents.list configured)", nil)
	} else {
		hasMainOrDefault := false
		for i := range agentConfigs {
			ac := &agentConfigs[i]
			id := routing.NormalizeAgentID(ac.ID)
			if id == "main" || ac.Default {
				hasMainOrDefault = true
			}
			instance := NewAgentInstance(ac, &cfg.Agents.Defaults, cfg, provider, memDB, registry.mcpManager)
			registry.agents[id] = instance
			logger.InfoCF("agent", "Registered agent",
				map[string]any{
					"agent_id":  id,
					"name":      ac.Name,
					"workspace": instance.Workspace,
					"model":     instance.Model,
				})
		}

		if !hasMainOrDefault {
			implicitAgent := &config.AgentConfig{
				ID:      "main",
				Default: true,
			}
			instance := NewAgentInstance(implicitAgent, &cfg.Agents.Defaults, cfg, provider, memDB, registry.mcpManager)
			registry.agents["main"] = instance
			logger.InfoCF("agent", "Created implicit main agent (no default in agents.list)", nil)
		}
	}

	return registry
}

// GetAgent returns the agent instance for a given ID.
func (r *AgentRegistry) GetAgent(agentID string) (*AgentInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id := routing.NormalizeAgentID(agentID)
	agent, ok := r.agents[id]
	return agent, ok
}

// ResolveRoute determines which agent handles the message.
func (r *AgentRegistry) ResolveRoute(input routing.RouteInput) routing.ResolvedRoute {
	return r.resolver.ResolveRoute(input)
}

// ListAgentIDs returns all registered agent IDs.
func (r *AgentRegistry) ListAgentIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	return ids
}

// CanSpawnSubagent checks if parentAgentID is allowed to spawn targetAgentID.
func (r *AgentRegistry) CanSpawnSubagent(parentAgentID, targetAgentID string) bool {
	parent, ok := r.GetAgent(parentAgentID)
	if !ok {
		return false
	}
	if parent.Subagents == nil || parent.Subagents.AllowAgents == nil {
		return false
	}
	targetNorm := routing.NormalizeAgentID(targetAgentID)
	for _, allowed := range parent.Subagents.AllowAgents {
		if allowed == "*" {
			return true
		}
		if routing.NormalizeAgentID(allowed) == targetNorm {
			return true
		}
	}
	return false
}

// ListAgents returns all registered agent instances (a snapshot copy).
func (r *AgentRegistry) ListAgents() []*AgentInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*AgentInstance, 0, len(r.agents))
	for _, a := range r.agents {
		list = append(list, a)
	}
	return list
}

// RegisterAgent adds a new agent instance to the registry at runtime.
// Returns an error if an agent with the same ID already exists.
func (r *AgentRegistry) RegisterAgent(instance *AgentInstance) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := routing.NormalizeAgentID(instance.ID)
	if _, exists := r.agents[id]; exists {
		return fmt.Errorf("agent %q already registered", id)
	}
	r.agents[id] = instance
	return nil
}

// RemoveAgent removes an agent from the registry by ID.
// Returns an error if the agent is not found.
func (r *AgentRegistry) RemoveAgent(agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := routing.NormalizeAgentID(agentID)
	if _, exists := r.agents[id]; !exists {
		return fmt.Errorf("agent %q not found", id)
	}
	delete(r.agents, id)
	return nil
}

// GetDefaultAgent returns the default agent instance.
func (r *AgentRegistry) GetDefaultAgent() *AgentInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if agent, ok := r.agents["main"]; ok {
		return agent
	}
	for _, agent := range r.agents {
		return agent
	}
	return nil
}

// hasUserSubagents returns true if the agent list contains any non-main agents
// (i.e. the user explicitly configured subagents).
func hasUserSubagents(agents []config.AgentConfig) bool {
	for _, a := range agents {
		id := routing.NormalizeAgentID(a.ID)
		if id != "main" && !a.Default {
			return true
		}
	}
	return false
}

// seedAgentsFromTemplates appends an AgentConfig for each available template
// that isn't already in the list. The main agent's subagents.allow_agents is
// set to ["*"] so it can delegate to all of them.
func seedAgentsFromTemplates(existing []config.AgentConfig) []config.AgentConfig {
	templates, err := ListPurposeTemplates()
	if err != nil || len(templates) == 0 {
		return existing
	}

	existingIDs := make(map[string]bool)
	for _, a := range existing {
		existingIDs[routing.NormalizeAgentID(a.ID)] = true
	}

	result := make([]config.AgentConfig, len(existing))
	copy(result, existing)

	var added []string
	for _, t := range templates {
		id := routing.NormalizeAgentID(t.Name)
		if existingIDs[id] {
			continue
		}
		result = append(result, config.AgentConfig{
			ID:       t.Name,
			Name:     templateDisplayName(t.Name),
			Template: t.Name,
		})
		added = append(added, t.Name)
	}

	// Allow the main agent to spawn all subagents.
	for i := range result {
		if result[i].Default || routing.NormalizeAgentID(result[i].ID) == "main" {
			if result[i].Subagents == nil {
				result[i].Subagents = &config.SubagentsConfig{}
			}
			result[i].Subagents.AllowAgents = []string{"*"}
			break
		}
	}

	if len(added) > 0 {
		logger.InfoCF("agent", "Auto-seeded agents from templates", map[string]any{
			"count": len(added),
		})
	}

	return result
}

// templateDisplayName converts "backend-specialist" to "Backend Specialist".
func templateDisplayName(name string) string {
	parts := strings.Split(name, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
