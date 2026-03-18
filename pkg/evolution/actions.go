package evolution

// ActionType defines the category of evolutionary change.
type ActionType string

const (
	ActionCreateAgent      ActionType = "create_agent"
	ActionRetireAgent      ActionType = "retire_agent"
	ActionTuneAgent        ActionType = "tune_agent"
	ActionCreateSkill      ActionType = "create_skill"
	ActionModifyWorkspace  ActionType = "modify_workspace"
	ActionAdjustGuardrails ActionType = "adjust_guardrails"
	ActionNoAction         ActionType = "no_action"
)

// EvolutionAction describes a single change the evolution engine wants to make.
type EvolutionAction struct {
	Type    ActionType     `json:"type"`
	AgentID string         `json:"agent_id,omitempty"`
	Params  map[string]any `json:"params"`
	Reason  string         `json:"reason"`
}

// ObservationReport aggregates runtime metrics for the evolution engine to analyze.
type ObservationReport struct {
	AgentStats       map[string]*AgentPerfSnapshot `json:"agent_stats"`
	ToolFailures     map[string]int                `json:"tool_failures"`
	DelegationMisses int                           `json:"delegation_misses"`
	TotalTasks       int                           `json:"total_tasks"`
	ErrorRate        float64                       `json:"error_rate"`
}

// AgentPerfSnapshot captures a point-in-time performance view for a single agent.
type AgentPerfSnapshot struct {
	AgentID     string  `json:"agent_id"`
	SuccessRate float64 `json:"success_rate"`
	TaskCount   int     `json:"task_count"`
	AvgScore    float64 `json:"avg_score"`
	Trend       string  `json:"trend"`
}

// Diagnosis is the output of analyzing an ObservationReport.
type Diagnosis struct {
	CapabilityGaps    []string `json:"capability_gaps"`
	Underperformers   []string `json:"underperformers"`
	SuccessPatterns   []string `json:"success_patterns"`
	PromptSuggestions []string `json:"prompt_suggestions"`
}
