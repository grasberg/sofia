package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/caarlos0/env/v11"

	"github.com/grasberg/sofia/pkg/fileutil"
)

// rrCounter is a global counter for round-robin load balancing across models.
var rrCounter atomic.Uint64

// FlexibleStringSlice is a []string that also accepts JSON numbers,
// so allow_from can contain both "123" and 123.
type FlexibleStringSlice []string

func (f *FlexibleStringSlice) UnmarshalJSON(data []byte) error {
	// Try []string first
	var ss []string
	if err := json.Unmarshal(data, &ss); err == nil {
		*f = ss
		return nil
	}

	// Try []interface{} to handle mixed types
	var raw []any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	result := make([]string, 0, len(raw))
	for _, v := range raw {
		switch val := v.(type) {
		case string:
			result = append(result, val)
		case float64:
			result = append(result, fmt.Sprintf("%.0f", val))
		default:
			result = append(result, fmt.Sprintf("%v", val))
		}
	}
	*f = result
	return nil
}

type Config struct {
	Agents     AgentsConfig     `json:"agents"`
	Bindings   []AgentBinding   `json:"bindings,omitempty"`
	Session    SessionConfig    `json:"session,omitempty"`
	Channels   ChannelsConfig   `json:"channels"`
	Providers  ProvidersConfig  `json:"providers,omitempty"`
	ModelList  []ModelConfig    `json:"model_list"` // New model-centric provider configuration
	Gateway    GatewayConfig    `json:"gateway"`
	Tools      ToolsConfig      `json:"tools"`
	Triggers   TriggersConfig   `json:"triggers,omitempty"`
	Heartbeat  HeartbeatConfig  `json:"heartbeat"`
	Autonomy   AutonomyConfig   `json:"autonomy,omitempty"`
	Evolution  EvolutionConfig  `json:"evolution,omitempty"`
	Devices    DevicesConfig    `json:"devices"`
	WebUI        WebUIConfig        `json:"webui"`
	TTS          TTSConfig          `json:"tts"`
	RemoteAccess RemoteAccessConfig `json:"remote_access,omitempty"`
	Guardrails   GuardrailsConfig     `json:"guardrails,omitempty"`
	Webhooks     []WebhookNotifyConfig `json:"webhooks,omitempty"`
	Digests      []DigestConfig        `json:"digests,omitempty"`
	UserName   string           `json:"user_name"           env:"SOFIA_USER_NAME"`
	MemoryDB   string           `json:"memory_db"           env:"SOFIA_MEMORY_DB"` // Path to SQLite memory database (default: ~/.sofia/memory.db)
}

// WebhookNotifyConfig configures an outbound notification webhook.
type WebhookNotifyConfig struct {
	URL     string   `json:"url"`
	Secret  string   `json:"secret,omitempty"`  // HMAC-SHA256 signing secret
	Events  []string `json:"events"`            // event types to deliver
	Enabled bool     `json:"enabled"`
}

// DigestConfig configures a scheduled digest report.
type DigestConfig struct {
	Period        string `json:"period"`                    // "daily", "weekly"
	Channel       string `json:"channel"`                   // target channel for delivery
	ChatID        string `json:"chat_id"`                   // target chat
	AgentID       string `json:"agent_id"`                  // which agent generates
	IncludeMemory bool   `json:"include_memory,omitempty"`
	IncludeUsage  bool   `json:"include_usage,omitempty"`
}

// RemoteAccessConfig configures remote access via Tailscale or similar providers.
type RemoteAccessConfig struct {
	Enabled  bool   `json:"enabled"`            // Whether remote access is enabled
	Provider string `json:"provider,omitempty"` // Provider name: "tailscale" (default)
	Port     int    `json:"port,omitempty"`     // Local port to expose, default 3000
}

// GuardrailsConfig configures safety and trust features.
type GuardrailsConfig struct {
	InputValidation InputValidationConfig `json:"input_validation,omitempty"`
	OutputFiltering OutputFilteringConfig `json:"output_filtering,omitempty"`
	RateLimiting    RateLimitingConfig    `json:"rate_limiting,omitempty"`
	SandboxedExec   SandboxedExecConfig   `json:"sandboxed_exec,omitempty"`
	PromptInjection PromptInjectionConfig `json:"prompt_injection,omitempty"`
	PIIDetection    PIIDetectionConfig    `json:"pii_detection,omitempty"`
	Approval        ApprovalConfig        `json:"approval,omitempty"`
}

// ApprovalConfig defines which tool calls require human-in-the-loop approval.
type ApprovalConfig struct {
	Enabled       bool     `json:"enabled"`
	RequireFor    []string `json:"require_for"`    // tool names requiring approval
	PatternMatch  []string `json:"pattern_match"`  // regex patterns on tool args
	TimeoutSec    int      `json:"timeout_sec"`    // how long to wait (default 300)
	DefaultAction string   `json:"default_action"` // "deny" or "allow" on timeout
}

// PIIDetectionConfig configures automatic PII detection on inbound messages.
type PIIDetectionConfig struct {
	Enabled bool   `json:"enabled" env:"SOFIA_GUARDRAILS_PII_ENABLED"`
	Action  string `json:"action"  env:"SOFIA_GUARDRAILS_PII_ACTION"` // "warn" (default), "redact", or "block"
}

type InputValidationConfig struct {
	Enabled          bool     `json:"enabled" env:"SOFIA_GUARDRAILS_INPUT_ENABLED"`
	MaxMessageLength int      `json:"max_message_length" env:"SOFIA_GUARDRAILS_INPUT_MAX_LENGTH"`
	DenyPatterns     []string `json:"deny_patterns" env:"SOFIA_GUARDRAILS_INPUT_DENY_PATTERNS"`
}

type OutputFilteringConfig struct {
	Enabled        bool     `json:"enabled" env:"SOFIA_GUARDRAILS_OUTPUT_ENABLED"`
	RedactPatterns []string `json:"redact_patterns" env:"SOFIA_GUARDRAILS_OUTPUT_REDACT_PATTERNS"`
	Action         string   `json:"action" env:"SOFIA_GUARDRAILS_OUTPUT_ACTION"` // "redact" or "block"
}

type RateLimitingConfig struct {
	Enabled          bool `json:"enabled" env:"SOFIA_GUARDRAILS_RATELIMIT_ENABLED"`
	MaxRPM           int  `json:"max_rpm" env:"SOFIA_GUARDRAILS_RATELIMIT_RPM"`
	MaxTokensPerHour int  `json:"max_tokens_per_hour" env:"SOFIA_GUARDRAILS_RATELIMIT_TOKENS"`
}

type SandboxedExecConfig struct {
	Enabled     bool   `json:"enabled" env:"SOFIA_GUARDRAILS_SANDBOX_ENABLED"`
	DockerImage string `json:"docker_image" env:"SOFIA_GUARDRAILS_SANDBOX_DOCKER_IMAGE"` // e.g., "alpine:latest"
}

type PromptInjectionConfig struct {
	Enabled      bool   `json:"enabled" env:"SOFIA_GUARDRAILS_INJECTION_ENABLED"`
	Action       string `json:"action" env:"SOFIA_GUARDRAILS_INJECTION_ACTION"` // "block" or "warn"
	SystemSuffix string `json:"system_suffix" env:"SOFIA_GUARDRAILS_INJECTION_SUFFIX"`
}

// TriggersConfig configures event-driven triggers.
type TriggersConfig struct {
	Webhooks  []WebhookTriggerConfig   `json:"webhooks,omitempty"`
	FileWatch []FileWatchTriggerConfig `json:"file_watch,omitempty"`
	Patterns  []PatternTriggerConfig   `json:"patterns,omitempty"`
}

// WebhookTriggerConfig configures an HTTP webhook trigger.
type WebhookTriggerConfig struct {
	Path    string `json:"path"`               // URL path (e.g., "/webhook/deploy")
	AgentID string `json:"agent_id,omitempty"` // Target agent (default: main)
	Secret  string `json:"secret,omitempty"`   // Optional HMAC secret for verification
}

// FileWatchTriggerConfig configures a filesystem watcher trigger.
type FileWatchTriggerConfig struct {
	Path      string `json:"path"`                // File or directory to watch
	Pattern   string `json:"pattern,omitempty"`   // Glob pattern filter (e.g., "*.log")
	AgentID   string `json:"agent_id,omitempty"`  // Target agent (default: main)
	Prompt    string `json:"prompt,omitempty"`    // Prompt template ({{.File}} {{.Event}} available)
	Recursive bool   `json:"recursive,omitempty"` // Watch subdirectories
}

// PatternTriggerConfig configures a regex pattern trigger on messages.
type PatternTriggerConfig struct {
	Regex   string `json:"regex"`              // Regex pattern to match on messages
	AgentID string `json:"agent_id,omitempty"` // Target agent (default: main)
	Prompt  string `json:"prompt,omitempty"`   // Prompt template ({{.Match}} available)
}

type WebUIConfig struct {
	Enabled bool   `json:"enabled" env:"SOFIA_WEBUI_ENABLED"`
	Host    string `json:"host"    env:"SOFIA_WEBUI_HOST"`
	Port    int    `json:"port"    env:"SOFIA_WEBUI_PORT"`
}

// TTSConfig configures text-to-speech synthesis.
type TTSConfig struct {
	Enabled  bool   `json:"enabled"           env:"SOFIA_TTS_ENABLED"`
	Provider string `json:"provider"          env:"SOFIA_TTS_PROVIDER"` // "elevenlabs" or "system"
	APIKey   string `json:"api_key,omitempty" env:"SOFIA_TTS_API_KEY"`
	Voice    string `json:"voice,omitempty"   env:"SOFIA_TTS_VOICE"`
}

// MarshalJSON implements custom JSON marshaling for Config
// to omit providers section when empty and session when empty
func (c Config) MarshalJSON() ([]byte, error) {
	type Alias Config
	aux := &struct {
		Providers *ProvidersConfig `json:"providers,omitempty"`
		Session   *SessionConfig   `json:"session,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(&c),
	}

	// Only include providers if not empty
	if !c.Providers.IsEmpty() {
		aux.Providers = &c.Providers
	}

	// Only include session if not empty
	if c.Session.DMScope != "" || len(c.Session.IdentityLinks) > 0 {
		aux.Session = &c.Session
	}

	return json.Marshal(aux)
}

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
	return json.Marshal(raw{Primary: m.Primary, Fallbacks: m.Fallbacks})
}

// BudgetConfig defines spending limits for an agent over a recurring period.
type BudgetConfig struct {
	MaxCostUSD float64 `json:"max_cost_usd"`
	Period     string  `json:"period"` // "daily", "weekly", or "monthly"
}

type AgentConfig struct {
	ID                 string            `json:"id"`
	Default            bool              `json:"default,omitempty"`
	Name               string            `json:"name,omitempty"`
	Template           string            `json:"template,omitempty"`
	TemplateSkillsMode string            `json:"template_skills_mode,omitempty"`
	Workspace          string            `json:"workspace,omitempty"`
	Model              *AgentModelConfig `json:"model,omitempty"`
	Skills             []string          `json:"skills,omitempty"`
	Subagents          *SubagentsConfig  `json:"subagents,omitempty"`
	Budget             *BudgetConfig     `json:"budget,omitempty"`
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
	Workspace           string                    `json:"workspace"                       env:"SOFIA_AGENTS_DEFAULTS_WORKSPACE"`
	RestrictToWorkspace bool                      `json:"restrict_to_workspace"           env:"SOFIA_AGENTS_DEFAULTS_RESTRICT_TO_WORKSPACE"`
	UseOpenCode         bool                      `json:"use_opencode"                    env:"SOFIA_AGENTS_DEFAULTS_USE_OPENCODE"`
	Provider            string                    `json:"provider"                        env:"SOFIA_AGENTS_DEFAULTS_PROVIDER"`
	ModelName           string                    `json:"model_name,omitempty"            env:"SOFIA_AGENTS_DEFAULTS_MODEL_NAME"`
	Model               string                    `json:"model,omitempty"                 env:"SOFIA_AGENTS_DEFAULTS_MODEL"` // Deprecated: use model_name instead
	ModelFallbacks      []string                  `json:"model_fallbacks,omitempty"`
	ImageModel          string                    `json:"image_model,omitempty"           env:"SOFIA_AGENTS_DEFAULTS_IMAGE_MODEL"`
	ImageModelFallbacks []string                  `json:"image_model_fallbacks,omitempty"`
	MaxTokens           int                       `json:"max_tokens"                      env:"SOFIA_AGENTS_DEFAULTS_MAX_TOKENS"`
	Temperature         *float64                  `json:"temperature,omitempty"           env:"SOFIA_AGENTS_DEFAULTS_TEMPERATURE"`
	MaxToolIterations   int                       `json:"max_tool_iterations"             env:"SOFIA_AGENTS_DEFAULTS_MAX_TOOL_ITERATIONS"`
	ReflectionInterval  int                       `json:"reflection_interval,omitempty"   env:"SOFIA_AGENTS_DEFAULTS_REFLECTION_INTERVAL"`
	LearnFromFeedback   bool                      `json:"learn_from_feedback,omitempty"   env:"SOFIA_AGENTS_DEFAULTS_LEARN_FROM_FEEDBACK"`
	ParallelToolCalls   bool                      `json:"parallel_tool_calls,omitempty"   env:"SOFIA_AGENTS_DEFAULTS_PARALLEL_TOOL_CALLS"`
	PostTaskReflection  bool                      `json:"post_task_reflection,omitempty"  env:"SOFIA_AGENTS_DEFAULTS_POST_TASK_REFLECTION"`
	PerformanceScoring  bool                      `json:"performance_scoring,omitempty"   env:"SOFIA_AGENTS_DEFAULTS_PERFORMANCE_SCORING"`
	Personas            map[string]PersonaConfig  `json:"personas,omitempty"`
	Budget              *BudgetConfig             `json:"budget,omitempty"`
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

type ChannelsConfig struct {
	Telegram TelegramConfig `json:"telegram"`
	Discord  DiscordConfig  `json:"discord"`
	Email    EmailConfig    `json:"email"`
}

// EmailConfig holds email channel configuration.
type EmailConfig struct {
	Enabled      bool     `json:"enabled"            env:"SOFIA_CHANNELS_EMAIL_ENABLED"`
	IMAPServer   string   `json:"imap_server"        env:"SOFIA_CHANNELS_EMAIL_IMAP_SERVER"`
	SMTPServer   string   `json:"smtp_server"        env:"SOFIA_CHANNELS_EMAIL_SMTP_SERVER"`
	Username     string   `json:"username"            env:"SOFIA_CHANNELS_EMAIL_USERNAME"`
	Password     string   `json:"password"            env:"SOFIA_CHANNELS_EMAIL_PASSWORD"`
	PollInterval int      `json:"poll_interval_sec"  env:"SOFIA_CHANNELS_EMAIL_POLL_INTERVAL"`
	AllowFrom    []string `json:"allow_from,omitempty"`
}

type TelegramConfig struct {
	Enabled   bool                `json:"enabled"              env:"SOFIA_CHANNELS_TELEGRAM_ENABLED"`
	Token     string              `json:"token"                env:"SOFIA_CHANNELS_TELEGRAM_TOKEN"`
	Proxy     string              `json:"proxy"                env:"SOFIA_CHANNELS_TELEGRAM_PROXY"`
	AllowFrom FlexibleStringSlice `json:"allow_from"           env:"SOFIA_CHANNELS_TELEGRAM_ALLOW_FROM"`
	DMPolicy  string              `json:"dm_policy,omitempty"  env:"SOFIA_CHANNELS_TELEGRAM_DM_POLICY"`
}

type DiscordConfig struct {
	Enabled     bool                `json:"enabled"              env:"SOFIA_CHANNELS_DISCORD_ENABLED"`
	Token       string              `json:"token"                env:"SOFIA_CHANNELS_DISCORD_TOKEN"`
	AllowFrom   FlexibleStringSlice `json:"allow_from"           env:"SOFIA_CHANNELS_DISCORD_ALLOW_FROM"`
	MentionOnly bool                `json:"mention_only"         env:"SOFIA_CHANNELS_DISCORD_MENTION_ONLY"`
	DMPolicy    string              `json:"dm_policy,omitempty"  env:"SOFIA_CHANNELS_DISCORD_DM_POLICY"`
}

type HeartbeatConfig struct {
	Enabled     bool     `json:"enabled"      env:"SOFIA_HEARTBEAT_ENABLED"`
	Interval    int      `json:"interval"     env:"SOFIA_HEARTBEAT_INTERVAL"`     // minutes, min 5
	Model       string   `json:"model"        env:"SOFIA_HEARTBEAT_MODEL"`        // optional: use a specific model (e.g. cheaper/faster) instead of default
	ActiveHours string   `json:"active_hours" env:"SOFIA_HEARTBEAT_ACTIVE_HOURS"` // e.g. "09:00-17:00"
	ActiveDays  []string `json:"active_days"  env:"SOFIA_HEARTBEAT_ACTIVE_DAYS"`  // e.g. ["Monday", "Tuesday"]
}

// AutonomyConfig configures proactive behaviors, goal persistence, and autonomous research.
type AutonomyConfig struct {
	Enabled         bool `json:"enabled"          env:"SOFIA_AUTONOMY_ENABLED"`
	Suggestions     bool `json:"suggestions"      env:"SOFIA_AUTONOMY_SUGGESTIONS"`
	Goals           bool `json:"goals"            env:"SOFIA_AUTONOMY_GOALS"`
	Research        bool `json:"research"         env:"SOFIA_AUTONOMY_RESEARCH"`
	ContextTriggers bool `json:"context_triggers" env:"SOFIA_AUTONOMY_CONTEXT_TRIGGERS"`
	IntervalMinutes int  `json:"interval_minutes" env:"SOFIA_AUTONOMY_INTERVAL"`
}

// EvolutionConfig configures the self-improving evolution engine.
type EvolutionConfig struct {
	Enabled                bool     `json:"enabled"                env:"SOFIA_EVOLUTION_ENABLED"`
	IntervalMinutes        int      `json:"interval_minutes"       env:"SOFIA_EVOLUTION_INTERVAL"`
	MaxCostPerDay          float64  `json:"max_cost_per_day"       env:"SOFIA_EVOLUTION_MAX_COST"`
	DailySummary           bool     `json:"daily_summary"          env:"SOFIA_EVOLUTION_DAILY_SUMMARY"`
	DailySummaryTime       string   `json:"daily_summary_time"     env:"SOFIA_EVOLUTION_SUMMARY_TIME"`
	DailySummaryChannel    string   `json:"daily_summary_channel"  env:"SOFIA_EVOLUTION_SUMMARY_CHANNEL"`
	DailySummaryChatID     string   `json:"daily_summary_chat_id"  env:"SOFIA_EVOLUTION_SUMMARY_CHAT_ID"`
	RetirementThreshold    float64  `json:"retirement_threshold"`
	RetirementMinTasks     int      `json:"retirement_min_tasks"`
	RetirementInactiveDays int      `json:"retirement_inactive_days"`
	SelfModifyEnabled      bool     `json:"self_modify_enabled"`
	ImmutableFiles         []string `json:"immutable_files,omitempty"`
	MaxAgents              int      `json:"max_agents"`
}

type DevicesConfig struct {
	Enabled    bool `json:"enabled"     env:"SOFIA_DEVICES_ENABLED"`
	MonitorUSB bool `json:"monitor_usb" env:"SOFIA_DEVICES_MONITOR_USB"`
}

type ProvidersConfig struct {
	Anthropic     ProviderConfig       `json:"anthropic"`
	OpenAI        OpenAIProviderConfig `json:"openai"`
	OpenRouter    ProviderConfig       `json:"openrouter"`
	Groq          ProviderConfig       `json:"groq"`
	Zhipu         ProviderConfig       `json:"zhipu"`
	VLLM          ProviderConfig       `json:"vllm"`
	Gemini        ProviderConfig       `json:"gemini"`
	Nvidia        ProviderConfig       `json:"nvidia"`
	Ollama        ProviderConfig       `json:"ollama"`
	Moonshot      ProviderConfig       `json:"moonshot"`
	ShengSuanYun  ProviderConfig       `json:"shengsuanyun"`
	DeepSeek      ProviderConfig       `json:"deepseek"`
	Cerebras      ProviderConfig       `json:"cerebras"`
	VolcEngine    ProviderConfig       `json:"volcengine"`
	GitHubCopilot ProviderConfig       `json:"github_copilot"`
	Antigravity   ProviderConfig       `json:"antigravity"`
	Qwen          ProviderConfig       `json:"qwen"`
	Mistral       ProviderConfig       `json:"mistral"`
	MiniMax       ProviderConfig       `json:"minimax"`
	Zai           ProviderConfig       `json:"zai"`
	Grok          ProviderConfig       `json:"grok"`
}

// IsEmpty checks if all provider configs are empty (no API keys or API bases set)
// Note: WebSearch is an optimization option and doesn't count as "non-empty"
func (p ProvidersConfig) IsEmpty() bool {
	return p.Anthropic.APIKey == "" && p.Anthropic.APIBase == "" &&
		p.OpenAI.APIKey == "" && p.OpenAI.APIBase == "" &&
		p.OpenRouter.APIKey == "" && p.OpenRouter.APIBase == "" &&
		p.Groq.APIKey == "" && p.Groq.APIBase == "" &&
		p.Zhipu.APIKey == "" && p.Zhipu.APIBase == "" &&
		p.VLLM.APIKey == "" && p.VLLM.APIBase == "" &&
		p.Gemini.APIKey == "" && p.Gemini.APIBase == "" &&
		p.Nvidia.APIKey == "" && p.Nvidia.APIBase == "" &&
		p.Ollama.APIKey == "" && p.Ollama.APIBase == "" &&
		p.Moonshot.APIKey == "" && p.Moonshot.APIBase == "" &&
		p.ShengSuanYun.APIKey == "" && p.ShengSuanYun.APIBase == "" &&
		p.DeepSeek.APIKey == "" && p.DeepSeek.APIBase == "" &&
		p.Cerebras.APIKey == "" && p.Cerebras.APIBase == "" &&
		p.VolcEngine.APIKey == "" && p.VolcEngine.APIBase == "" &&
		p.GitHubCopilot.APIKey == "" && p.GitHubCopilot.APIBase == "" &&
		p.Antigravity.APIKey == "" && p.Antigravity.APIBase == "" &&
		p.Qwen.APIKey == "" && p.Qwen.APIBase == "" &&
		p.Mistral.APIKey == "" && p.Mistral.APIBase == ""
}

// MarshalJSON implements custom JSON marshaling for ProvidersConfig
// to omit the entire section when empty
func (p ProvidersConfig) MarshalJSON() ([]byte, error) {
	if p.IsEmpty() {
		return []byte("null"), nil
	}
	type Alias ProvidersConfig
	return json.Marshal((*Alias)(&p))
}

type ProviderConfig struct {
	APIKey         string `json:"api_key"                   env:"SOFIA_PROVIDERS_{{.Name}}_API_KEY"`
	APIBase        string `json:"api_base"                  env:"SOFIA_PROVIDERS_{{.Name}}_API_BASE"`
	Proxy          string `json:"proxy,omitempty"           env:"SOFIA_PROVIDERS_{{.Name}}_PROXY"`
	RequestTimeout int    `json:"request_timeout,omitempty" env:"SOFIA_PROVIDERS_{{.Name}}_REQUEST_TIMEOUT"`
	AuthMethod     string `json:"auth_method,omitempty"     env:"SOFIA_PROVIDERS_{{.Name}}_AUTH_METHOD"`
	ConnectMode    string `json:"connect_mode,omitempty"    env:"SOFIA_PROVIDERS_{{.Name}}_CONNECT_MODE"` // only for Github Copilot, `stdio` or `grpc`
}

type OpenAIProviderConfig struct {
	ProviderConfig
	WebSearch bool `json:"web_search" env:"SOFIA_PROVIDERS_OPENAI_WEB_SEARCH"`
}

// ModelConfig represents a model-centric provider configuration.
// It allows adding new providers (especially OpenAI-compatible ones) via configuration only.
// The model field uses protocol prefix format: [protocol/]model-identifier
// Supported protocols: openai, anthropic, antigravity, claude-cli, codex-cli, github-copilot
// Default protocol is "openai" if no prefix is specified.
type ModelConfig struct {
	// Required fields
	ModelName string `json:"model_name"` // User-facing alias for the model
	Model     string `json:"model"`      // Protocol/model-identifier (e.g., "openai/gpt-4o", "anthropic/claude-sonnet-4.6")

	// HTTP-based providers
	APIBase string `json:"api_base,omitempty"` // API endpoint URL
	APIKey  string `json:"api_key"`            // API authentication key
	Proxy   string `json:"proxy,omitempty"`    // HTTP proxy URL

	// Special providers (CLI-based, OAuth, etc.)
	AuthMethod  string `json:"auth_method,omitempty"`  // Authentication method: oauth, token
	ConnectMode string `json:"connect_mode,omitempty"` // Connection mode: stdio, grpc
	Workspace   string `json:"workspace,omitempty"`    // Workspace path for CLI-based providers

	// Optional optimizations
	RPM            int    `json:"rpm,omitempty"`              // Requests per minute limit
	MaxTokens      int    `json:"max_tokens,omitempty"`       // Max tokens per request (overrides agent default)
	MaxTokensField string `json:"max_tokens_field,omitempty"` // Field name for max tokens (e.g., "max_completion_tokens")
	RequestTimeout int    `json:"request_timeout,omitempty"`
}

// Validate checks if the ModelConfig has all required fields.
func (c *ModelConfig) Validate() error {
	if c.ModelName == "" {
		return fmt.Errorf("model_name is required")
	}
	if c.Model == "" {
		return fmt.Errorf("model is required")
	}
	return nil
}

type GatewayConfig struct {
	Host string `json:"host" env:"SOFIA_GATEWAY_HOST"`
	Port int    `json:"port" env:"SOFIA_GATEWAY_PORT"`
}

type BraveConfig struct {
	Enabled    bool   `json:"enabled"     env:"SOFIA_TOOLS_WEB_BRAVE_ENABLED"`
	APIKey     string `json:"api_key"     env:"SOFIA_TOOLS_WEB_BRAVE_API_KEY"`
	MaxResults int    `json:"max_results" env:"SOFIA_TOOLS_WEB_BRAVE_MAX_RESULTS"`
}

type TavilyConfig struct {
	Enabled    bool   `json:"enabled"     env:"SOFIA_TOOLS_WEB_TAVILY_ENABLED"`
	APIKey     string `json:"api_key"     env:"SOFIA_TOOLS_WEB_TAVILY_API_KEY"`
	BaseURL    string `json:"base_url"    env:"SOFIA_TOOLS_WEB_TAVILY_BASE_URL"`
	MaxResults int    `json:"max_results" env:"SOFIA_TOOLS_WEB_TAVILY_MAX_RESULTS"`
}

type DuckDuckGoConfig struct {
	Enabled    bool `json:"enabled"     env:"SOFIA_TOOLS_WEB_DUCKDUCKGO_ENABLED"`
	MaxResults int  `json:"max_results" env:"SOFIA_TOOLS_WEB_DUCKDUCKGO_MAX_RESULTS"`
}

type PerplexityConfig struct {
	Enabled    bool   `json:"enabled"     env:"SOFIA_TOOLS_WEB_PERPLEXITY_ENABLED"`
	APIKey     string `json:"api_key"     env:"SOFIA_TOOLS_WEB_PERPLEXITY_API_KEY"`
	MaxResults int    `json:"max_results" env:"SOFIA_TOOLS_WEB_PERPLEXITY_MAX_RESULTS"`
}

type BrowserConfig struct {
	Headless       bool   `json:"headless"        env:"SOFIA_TOOLS_WEB_BROWSER_HEADLESS"`
	TimeoutSeconds int    `json:"timeout_seconds" env:"SOFIA_TOOLS_WEB_BROWSER_TIMEOUT_SECONDS"`
	BrowserType    string `json:"browser_type"    env:"SOFIA_TOOLS_WEB_BROWSER_TYPE"` // "chromium", "firefox", "webkit"
	ScreenshotDir  string `json:"screenshot_dir"  env:"SOFIA_TOOLS_WEB_BROWSER_SCREENSHOT_DIR"`
}

type WebToolsConfig struct {
	Brave      BraveConfig      `json:"brave"`
	Tavily     TavilyConfig     `json:"tavily"`
	DuckDuckGo DuckDuckGoConfig `json:"duckduckgo"`
	Perplexity PerplexityConfig `json:"perplexity"`
	Browser    BrowserConfig    `json:"browser"`
	// Proxy is an optional proxy URL for web tools (http/https/socks5/socks5h).
	// For authenticated proxies, prefer HTTP_PROXY/HTTPS_PROXY env vars instead of embedding credentials in config.
	Proxy string `json:"proxy,omitempty" env:"SOFIA_TOOLS_WEB_PROXY"`
}

type CronToolsConfig struct {
	ExecTimeoutMinutes int `json:"exec_timeout_minutes" env:"SOFIA_TOOLS_CRON_EXEC_TIMEOUT_MINUTES"` // 0 means no timeout
}

type ExecConfig struct {
	EnableDenyPatterns bool     `json:"enable_deny_patterns" env:"SOFIA_TOOLS_EXEC_ENABLE_DENY_PATTERNS"`
	CustomDenyPatterns []string `json:"custom_deny_patterns" env:"SOFIA_TOOLS_EXEC_CUSTOM_DENY_PATTERNS"`
	ConfirmPatterns    []string `json:"confirm_patterns"     env:"SOFIA_TOOLS_EXEC_CONFIRM_PATTERNS"`
}

type GoogleToolsConfig struct {
	Enabled         bool     `json:"enabled"          env:"SOFIA_TOOLS_GOOGLE_ENABLED"`
	BinaryPath      string   `json:"binary_path"      env:"SOFIA_TOOLS_GOOGLE_BINARY_PATH"`
	TimeoutSeconds  int      `json:"timeout_seconds"  env:"SOFIA_TOOLS_GOOGLE_TIMEOUT_SECONDS"`
	AllowedCommands []string `json:"allowed_commands" env:"SOFIA_TOOLS_GOOGLE_ALLOWED_COMMANDS"`
}

type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// BraveSearchConfig configures the Brave Search web search tool.
type BraveSearchConfig struct {
	Enabled bool   `json:"enabled" env:"SOFIA_TOOLS_BRAVE_SEARCH_ENABLED"`
	APIKey  string `json:"api_key" env:"SOFIA_TOOLS_BRAVE_SEARCH_API_KEY"`
}

// GitHubCLIConfig configures the GitHub CLI (gh) tool.
type GitHubCLIConfig struct {
	Enabled         bool     `json:"enabled"          env:"SOFIA_TOOLS_GITHUB_ENABLED"`
	BinaryPath      string   `json:"binary_path"      env:"SOFIA_TOOLS_GITHUB_BINARY_PATH"`
	TimeoutSeconds  int      `json:"timeout_seconds"  env:"SOFIA_TOOLS_GITHUB_TIMEOUT_SECONDS"`
	AllowedCommands []string `json:"allowed_commands" env:"SOFIA_TOOLS_GITHUB_ALLOWED_COMMANDS"`
}

// CpanelConfig configures the cPanel hosting management tool.
type CpanelConfig struct {
	Enabled  bool   `json:"enabled"   env:"SOFIA_TOOLS_CPANEL_ENABLED"`
	Host     string `json:"host"      env:"SOFIA_TOOLS_CPANEL_HOST"`
	Port     int    `json:"port"      env:"SOFIA_TOOLS_CPANEL_PORT"`
	Username string `json:"username"  env:"SOFIA_TOOLS_CPANEL_USERNAME"`
	APIToken string `json:"api_token" env:"SOFIA_TOOLS_CPANEL_API_TOKEN"`
}

// BitcoinConfig configures the Bitcoin wallet and blockchain tool.
type BitcoinConfig struct {
	Enabled    bool   `json:"enabled"      env:"SOFIA_TOOLS_BITCOIN_ENABLED"`
	Network    string `json:"network"      env:"SOFIA_TOOLS_BITCOIN_NETWORK"`    // mainnet, testnet, signet
	WalletPath string `json:"wallet_path"  env:"SOFIA_TOOLS_BITCOIN_WALLET_PATH"` // path to encrypted wallet file
	Passphrase string `json:"passphrase"   env:"SOFIA_TOOLS_BITCOIN_PASSPHRASE"` // wallet encryption passphrase
}

// PorkbunConfig configures the Porkbun domain management tool.
type PorkbunConfig struct {
	Enabled      bool   `json:"enabled"        env:"SOFIA_TOOLS_PORKBUN_ENABLED"`
	APIKey       string `json:"api_key"        env:"SOFIA_TOOLS_PORKBUN_API_KEY"`
	SecretAPIKey string `json:"secret_api_key" env:"SOFIA_TOOLS_PORKBUN_SECRET_API_KEY"`
}

// VercelConfig configures the Vercel CLI deployment tool.
type VercelConfig struct {
	Enabled         bool     `json:"enabled"          env:"SOFIA_TOOLS_VERCEL_ENABLED"`
	BinaryPath      string   `json:"binary_path"      env:"SOFIA_TOOLS_VERCEL_BINARY_PATH"`
	TimeoutSeconds  int      `json:"timeout_seconds"  env:"SOFIA_TOOLS_VERCEL_TIMEOUT_SECONDS"`
	AllowedCommands []string `json:"allowed_commands"  env:"SOFIA_TOOLS_VERCEL_ALLOWED_COMMANDS"`
}

type ToolsConfig struct {
	Web         WebToolsConfig             `json:"web"`
	Cron        CronToolsConfig            `json:"cron"`
	Exec        ExecConfig                 `json:"exec"`
	Google      GoogleToolsConfig          `json:"google"`
	GitHub      GitHubCLIConfig            `json:"github"`
	BraveSearch BraveSearchConfig          `json:"brave_search"`
	Porkbun     PorkbunConfig              `json:"porkbun"`
	Cpanel      CpanelConfig               `json:"cpanel"`
	Bitcoin     BitcoinConfig              `json:"bitcoin"`
	Vercel      VercelConfig               `json:"vercel"`
	Skills      SkillsToolsConfig          `json:"skills"`
	MCP         map[string]MCPServerConfig `json:"mcp,omitempty"`
}

type SkillsToolsConfig struct {
	Registries            SkillsRegistriesConfig `json:"registries"`
	MaxConcurrentSearches int                    `json:"max_concurrent_searches" env:"SOFIA_SKILLS_MAX_CONCURRENT_SEARCHES"`
	SearchCache           SearchCacheConfig      `json:"search_cache"`
}

type SearchCacheConfig struct {
	MaxSize    int `json:"max_size"    env:"SOFIA_SKILLS_SEARCH_CACHE_MAX_SIZE"`
	TTLSeconds int `json:"ttl_seconds" env:"SOFIA_SKILLS_SEARCH_CACHE_TTL_SECONDS"`
}

type SkillsRegistriesConfig struct {
	ClawHub ClawHubRegistryConfig `json:"clawhub"`
}

type ClawHubRegistryConfig struct {
	Enabled         bool   `json:"enabled"           env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_ENABLED"`
	BaseURL         string `json:"base_url"          env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_BASE_URL"`
	AuthToken       string `json:"auth_token"        env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_AUTH_TOKEN"`
	SearchPath      string `json:"search_path"       env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_SEARCH_PATH"`
	SkillsPath      string `json:"skills_path"       env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_SKILLS_PATH"`
	DownloadPath    string `json:"download_path"     env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_DOWNLOAD_PATH"`
	Timeout         int    `json:"timeout"           env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_TIMEOUT"`
	MaxZipSize      int    `json:"max_zip_size"      env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_MAX_ZIP_SIZE"`
	MaxResponseSize int    `json:"max_response_size" env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_MAX_RESPONSE_SIZE"`
}

func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	// Pre-scan the JSON to check how many model_list entries the user provided.
	// Go's JSON decoder reuses existing slice backing-array elements rather than
	// zero-initializing them, so fields absent from the user's JSON (e.g. api_base)
	// would silently inherit values from the DefaultConfig template at the same
	// index position. We only reset cfg.ModelList when the user actually provides
	// entries; when count is 0 we keep DefaultConfig's built-in list as fallback.
	// The same logic applies to agents.list.
	var tmp Config
	if err := json.Unmarshal(data, &tmp); err != nil {
		return nil, err
	}
	if len(tmp.ModelList) > 0 {
		cfg.ModelList = nil
	}
	if len(tmp.Agents.List) > 0 {
		cfg.Agents.List = nil
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	// Auto-migrate: if only legacy providers config exists, convert to model_list
	if len(cfg.ModelList) == 0 && cfg.HasProvidersConfig() {
		cfg.ModelList = ConvertProvidersToModelList(cfg)
	}

	// Ensure the main/default agent is always present in agents.list.
	// This handles existing configs written before the main agent was added to
	// DefaultConfig, so existing users get the correct behavior on upgrade.
	ensureMainAgent(cfg)

	// Validate model_list for uniqueness and required fields
	if err := cfg.ValidateModelList(); err != nil {
		return nil, err
	}

	// Validate overall config structure
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	// Use unified atomic write utility with explicit sync for flash storage reliability.
	return fileutil.WriteFileAtomic(path, data, 0o600)
}

func (c *Config) WorkspacePath() string {
	return expandHome(c.Agents.Defaults.Workspace)
}

func (c *Config) GetAPIKey() string {
	if c.Providers.OpenRouter.APIKey != "" {
		return c.Providers.OpenRouter.APIKey
	}
	if c.Providers.Anthropic.APIKey != "" {
		return c.Providers.Anthropic.APIKey
	}
	if c.Providers.OpenAI.APIKey != "" {
		return c.Providers.OpenAI.APIKey
	}
	if c.Providers.Gemini.APIKey != "" {
		return c.Providers.Gemini.APIKey
	}
	if c.Providers.Zhipu.APIKey != "" {
		return c.Providers.Zhipu.APIKey
	}
	if c.Providers.Groq.APIKey != "" {
		return c.Providers.Groq.APIKey
	}
	if c.Providers.VLLM.APIKey != "" {
		return c.Providers.VLLM.APIKey
	}
	if c.Providers.ShengSuanYun.APIKey != "" {
		return c.Providers.ShengSuanYun.APIKey
	}
	if c.Providers.Cerebras.APIKey != "" {
		return c.Providers.Cerebras.APIKey
	}
	return ""
}

func (c *Config) GetAPIBase() string {
	if c.Providers.OpenRouter.APIKey != "" {
		if c.Providers.OpenRouter.APIBase != "" {
			return c.Providers.OpenRouter.APIBase
		}
		return "https://openrouter.ai/api/v1"
	}
	if c.Providers.Zhipu.APIKey != "" {
		return c.Providers.Zhipu.APIBase
	}
	if c.Providers.VLLM.APIKey != "" && c.Providers.VLLM.APIBase != "" {
		return c.Providers.VLLM.APIBase
	}
	return ""
}

func expandHome(path string) string {
	if path == "" {
		return path
	}
	if path[0] == '~' {
		home, _ := os.UserHomeDir()
		if len(path) > 1 && path[1] == '/' {
			return home + path[1:]
		}
		return home
	}
	return path
}

// GetModelConfig returns the ModelConfig for the given model name.
// If multiple configs exist with the same model_name, it uses round-robin
// selection for load balancing. Returns an error if the model is not found.
func (c *Config) GetModelConfig(modelName string) (*ModelConfig, error) {
	matches := c.findMatches(modelName)
	if len(matches) == 0 {
		return nil, fmt.Errorf("model %q not found in model_list or providers", modelName)
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}

	// Multiple configs - use round-robin for load balancing
	idx := rrCounter.Add(1) % uint64(len(matches))
	return &matches[idx], nil
}

// findMatches finds all ModelConfig entries with the given model_name.
func (c *Config) findMatches(modelName string) []ModelConfig {
	var matches []ModelConfig
	for i := range c.ModelList {
		if c.ModelList[i].ModelName == modelName {
			matches = append(matches, c.ModelList[i])
		}
	}
	return matches
}

// HasProvidersConfig checks if any provider in the old providers config has configuration.
func (c *Config) HasProvidersConfig() bool {
	v := c.Providers
	return v.Anthropic.APIKey != "" || v.Anthropic.APIBase != "" ||
		v.OpenAI.APIKey != "" || v.OpenAI.APIBase != "" ||
		v.OpenRouter.APIKey != "" || v.OpenRouter.APIBase != "" ||
		v.Groq.APIKey != "" || v.Groq.APIBase != "" ||
		v.Zhipu.APIKey != "" || v.Zhipu.APIBase != "" ||
		v.VLLM.APIKey != "" || v.VLLM.APIBase != "" ||
		v.Gemini.APIKey != "" || v.Gemini.APIBase != "" ||
		v.Nvidia.APIKey != "" || v.Nvidia.APIBase != "" ||
		v.Ollama.APIKey != "" || v.Ollama.APIBase != "" ||
		v.Moonshot.APIKey != "" || v.Moonshot.APIBase != "" ||
		v.ShengSuanYun.APIKey != "" || v.ShengSuanYun.APIBase != "" ||
		v.DeepSeek.APIKey != "" || v.DeepSeek.APIBase != "" ||
		v.Cerebras.APIKey != "" || v.Cerebras.APIBase != "" ||
		v.VolcEngine.APIKey != "" || v.VolcEngine.APIBase != "" ||
		v.GitHubCopilot.APIKey != "" || v.GitHubCopilot.APIBase != "" ||
		v.Antigravity.APIKey != "" || v.Antigravity.APIBase != "" ||
		v.Qwen.APIKey != "" || v.Qwen.APIBase != "" ||
		v.Mistral.APIKey != "" || v.Mistral.APIBase != ""
}

// ValidateModelList validates all ModelConfig entries in the model_list.
// It checks that each model config is valid.
// Note: Multiple entries with the same model_name are allowed for load balancing.
func (c *Config) ValidateModelList() error {
	for i := range c.ModelList {
		if err := c.ModelList[i].Validate(); err != nil {
			return fmt.Errorf("model_list[%d]: %w", i, err)
		}
	}
	return nil
}
