package evolution

// AgentRegistrar manages agent registration (satisfied by agent.AgentRegistry).
type AgentRegistrar interface {
	RemoveAgent(agentID string) error
	ListAgentIDs() []string
}

// A2ARegistrar handles inter-agent routing (satisfied by agent.A2ARouter).
type A2ARegistrar interface {
	Register(agentID string)
}

// ToolStatsProvider gives tool execution statistics (satisfied by tools.ToolTracker).
type ToolStatsProvider interface {
	GetStats() map[string]any
}
