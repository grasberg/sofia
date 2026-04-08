package config

import "encoding/json"

type AgentsConfig struct {
	Defaults AgentDefaults `json:"defaults"`
	List     []AgentConfig `json:"list,omitempty"`
}

// AgentModelConfig supports both string and structured model config.
// String format: "gpt-4" (just primary, no fallbacks)
// Object format: {"primary": "gpt-4", "fallbacks": ["claude-haiku"]}
type AgentModelConfig struct {
	Primary   string   `json:"primary,omitempty"`
	Fallbacks []string `json:"fallbacks,omitempty"`
}

func (m *AgentModelConfig) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		m.Primary = s
		m.Fallbacks = nil
		return nil
	}
	type raw struct {
		Primary   string   `json:"primary"`
		Fallbacks []string `json:"fallbacks"`
	}
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	m.Primary = r.Primary
	m.Fallbacks = r.Fallbacks
	return nil
}

func (m AgentModelConfig) MarshalJSON() ([]byte, error) {
	if len(m.Fallbacks) == 0 && m.Primary != "" {
		return json.Marshal(m.Primary)
	}
	type raw struct {
		Primary   string   `json:"primary,omitempty"`
		Fallbacks []string `json:"fallbacks,omitempty"`
	}
	return json.Marshal(raw(m))
}

// BudgetConfig defines spending limits for an agent over a recurring period.
type BudgetConfig struct {
	MaxCostUSD float64 `json:"max_cost_usd"`
	Period     string  `json:"period"` // "daily", "weekly", or "monthly"
}

// SummarizationConfig controls when and how conversation history is compressed.
type SummarizationConfig struct {
	ContextTriggerPct       int `json:"context_trigger_pct,omitempty"`
	ForceTriggerPct         int `json:"force_trigger_pct,omitempty"`
	ProtectHead             int `json:"protect_head,omitempty"`
	ProtectTailPct          int `json:"protect_tail_pct,omitempty"`
	MinTail                 int `json:"min_tail,omitempty"`
	ToolResultTruncateChars int `json:"tool_result_truncate_chars,omitempty"`
}

func (s SummarizationConfig) ContextTriggerPctOrDefault() int {
	if s.ContextTriggerPct > 0 {
		return s.ContextTriggerPct
	}
	return 75
}

func (s SummarizationConfig) ForceTriggerPctOrDefault() int {
	if s.ForceTriggerPct > 0 {
		return s.ForceTriggerPct
	}
	return 90
}

func (s SummarizationConfig) ProtectHeadOrDefault() int {
	if s.ProtectHead > 0 {
		return s.ProtectHead
	}
	return 2
}

func (s SummarizationConfig) ProtectTailPctOrDefault() int {
	if s.ProtectTailPct > 0 {
		return s.ProtectTailPct
	}
	return 30
}

func (s SummarizationConfig) MinTailOrDefault() int {
	if s.MinTail > 0 {
		return s.MinTail
	}
	return 4
}

func (s SummarizationConfig) ToolResultTruncateCharsOrDefault() int {
	if s.ToolResultTruncateChars > 0 {
		return s.ToolResultTruncateChars
	}
	return 200
}

type AgentConfig struct {
	ID                 string              `json:"id"`
	Default            bool                `json:"default,omitempty"`
	Name               string              `json:"name,omitempty"`
	Template           string              `json:"template,omitempty"`
	TemplateSkillsMode string              `json:"template_skills_mode,omitempty"`
	Workspace          string              `json:"workspace,omitempty"`
	Model              *AgentModelConfig   `json:"model,omitempty"`
	Skills             []string            `json:"skills,omitempty"`
	Subagents          *SubagentsConfig    `json:"subagents,omitempty"`
	Budget             *BudgetConfig       `json:"budget,omitempty"`
	Summarization      SummarizationConfig `json:"summarization,omitempty"`
	ThinkingBudget     int                 `json:"thinking_budget,omitempty"`
}

type SubagentsConfig struct {
	AllowAgents []string          `json:"allow_agents,omitempty"`
	Model       *AgentModelConfig `json:"model,omitempty"`
}

type PeerMatch struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type BindingMatch struct {
	Channel   string     `json:"channel"`
	AccountID string     `json:"account_id,omitempty"`
	Peer      *PeerMatch `json:"peer,omitempty"`
	GuildID   string     `json:"guild_id,omitempty"`
	TeamID    string     `json:"team_id,omitempty"`
}

type AgentBinding struct {
	AgentID string       `json:"agent_id"`
	Match   BindingMatch `json:"match"`
}

type SessionConfig struct {
	DMScope       string              `json:"dm_scope,omitempty"`
	IdentityLinks map[string][]string `json:"identity_links,omitempty"`
}

type AgentDefaults struct {
	Workspace              string                   `json:"workspace"                          env:"SOFIA_AGENTS_DEFAULTS_WORKSPACE"`
	RestrictToWorkspace    bool                     `json:"restrict_to_workspace"              env:"SOFIA_AGENTS_DEFAULTS_RESTRICT_TO_WORKSPACE"`
	CodeEditor             string                   `json:"code_editor,omitempty"              env:"SOFIA_AGENTS_DEFAULTS_CODE_EDITOR"`
	Provider               string                   `json:"provider"                           env:"SOFIA_AGENTS_DEFAULTS_PROVIDER"`
	ModelName              string                   `json:"model_name,omitempty"               env:"SOFIA_AGENTS_DEFAULTS_MODEL_NAME"`
	Model                  string                   `json:"model,omitempty"                    env:"SOFIA_AGENTS_DEFAULTS_MODEL"` // Deprecated: use model_name instead
	ModelFallbacks         []string                 `json:"model_fallbacks,omitempty"`
	ImageModel             string                   `json:"image_model,omitempty"              env:"SOFIA_AGENTS_DEFAULTS_IMAGE_MODEL"`
	ImageModelFallbacks    []string                 `json:"image_model_fallbacks,omitempty"`
	MaxTokens              int                      `json:"max_tokens"                         env:"SOFIA_AGENTS_DEFAULTS_MAX_TOKENS"`
	Temperature            *float64                 `json:"temperature,omitempty"              env:"SOFIA_AGENTS_DEFAULTS_TEMPERATURE"`
	MaxToolIterations      int                      `json:"max_tool_iterations"                env:"SOFIA_AGENTS_DEFAULTS_MAX_TOOL_ITERATIONS"`
	MaxConcurrentSubagents int                      `json:"max_concurrent_subagents,omitempty" env:"SOFIA_AGENTS_DEFAULTS_MAX_CONCURRENT_SUBAGENTS"`
	AutoRollbackThreshold  int                      `json:"auto_rollback_threshold,omitempty"  env:"SOFIA_AGENTS_DEFAULTS_AUTO_ROLLBACK_THRESHOLD"` // errors before rollback (default 3)
	ReflectionInterval     int                      `json:"reflection_interval,omitempty"      env:"SOFIA_AGENTS_DEFAULTS_REFLECTION_INTERVAL"`
	LearnFromFeedback      bool                     `json:"learn_from_feedback,omitempty"      env:"SOFIA_AGENTS_DEFAULTS_LEARN_FROM_FEEDBACK"`
	ParallelToolCalls      bool                     `json:"parallel_tool_calls,omitempty"      env:"SOFIA_AGENTS_DEFAULTS_PARALLEL_TOOL_CALLS"`
	PostTaskReflection     bool                     `json:"post_task_reflection,omitempty"     env:"SOFIA_AGENTS_DEFAULTS_POST_TASK_REFLECTION"`
	PerformanceScoring     bool                     `json:"performance_scoring,omitempty"      env:"SOFIA_AGENTS_DEFAULTS_PERFORMANCE_SCORING"`
	EvaluationLoop         EvaluationLoopConfig     `json:"evaluation_loop,omitempty"`
	DoomLoopDetection      DoomLoopConfig           `json:"doom_loop_detection,omitempty"`
	AutoEscalation         AutoEscalationConfig     `json:"auto_escalation,omitempty"`
	Personas               map[string]PersonaConfig `json:"personas,omitempty"`
	Budget                 *BudgetConfig            `json:"budget,omitempty"`
	PromptOptimization     PromptOptimizationConfig `json:"prompt_optimization,omitempty"`
	Summarization          SummarizationConfig      `json:"summarization,omitempty"`
	ThinkingBudget         int                      `json:"thinking_budget,omitempty"`
}

// PromptOptimizationConfig configures automatic prompt refinement via A/B testing.
type PromptOptimizationConfig struct {
	Enabled          bool    `json:"enabled"            env:"SOFIA_PROMPT_OPT_ENABLED"`
	ScoreThreshold   float64 `json:"score_threshold"    env:"SOFIA_PROMPT_OPT_THRESHOLD"`    // trigger below this (default 0.6)
	MinTraces        int     `json:"min_traces"         env:"SOFIA_PROMPT_OPT_MIN_TRACES"`   // minimum traces before evaluating (default 20)
	MaxVariants      int     `json:"max_variants"       env:"SOFIA_PROMPT_OPT_MAX_VARIANTS"` // variants to generate (default 2)
	TrialsPerVariant int     `json:"trials_per_variant" env:"SOFIA_PROMPT_OPT_TRIALS"`       // interactions per variant (default 10)
}

// EvaluationLoopConfig enables iterative response improvement.
// After each LLM response, it is scored; if below threshold the LLM is
// re-run with feedback until the threshold is met or max retries exhausted.
type EvaluationLoopConfig struct {
	Enabled    bool    `json:"enabled"     env:"SOFIA_EVAL_LOOP_ENABLED"`
	Threshold  float64 `json:"threshold"   env:"SOFIA_EVAL_LOOP_THRESHOLD"`   // default 0.7
	MaxRetries int     `json:"max_retries" env:"SOFIA_EVAL_LOOP_MAX_RETRIES"` // default 3
}

// DoomLoopConfig enables detection of stuck agent loops with graduated recovery.
type DoomLoopConfig struct {
	Enabled             bool `json:"enabled"              env:"SOFIA_DOOM_LOOP_ENABLED"`
	RepetitionThreshold int  `json:"repetition_threshold" env:"SOFIA_DOOM_LOOP_REP_THRESHOLD"` // default 3
}

// AutoEscalationConfig enables automatic adjustment of iteration limits
// and model selection based on detected message complexity.
type AutoEscalationConfig struct {
	Enabled           bool `json:"enabled"             env:"SOFIA_AUTO_ESCALATION_ENABLED"`
	SmartModelRouting bool `json:"smart_model_routing" env:"SOFIA_SMART_MODEL_ROUTING"` // Route simple messages to fallback model
}

// PersonaConfig defines a switchable persona in the config file.
type PersonaConfig struct {
	SystemPrompt string   `json:"system_prompt"`
	Model        string   `json:"model,omitempty"`
	AllowedTools []string `json:"allowed_tools,omitempty"`
	Description  string   `json:"description,omitempty"`
}

// GetModelName returns the effective model name for the agent defaults.
// It prefers the new "model_name" field but falls back to "model" for backward compatibility.
func (d *AgentDefaults) GetModelName() string {
	if d.ModelName != "" {
		return d.ModelName
	}
	return d.Model
}
