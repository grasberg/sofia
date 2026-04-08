package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/grasberg/sofia/pkg/fileutil"
)

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
	Agents       AgentsConfig          `json:"agents"`
	Bindings     []AgentBinding        `json:"bindings,omitempty"`
	Session      SessionConfig         `json:"session,omitempty"`
	Channels     ChannelsConfig        `json:"channels"`
	Providers    ProvidersConfig       `json:"providers,omitempty"`
	ModelList    []ModelConfig         `json:"model_list"` // New model-centric provider configuration
	Gateway      GatewayConfig         `json:"gateway"`
	Tools        ToolsConfig           `json:"tools"`
	Triggers     TriggersConfig        `json:"triggers,omitempty"`
	Heartbeat    HeartbeatConfig       `json:"heartbeat"`
	Autonomy     AutonomyConfig        `json:"autonomy,omitempty"`
	Evolution    EvolutionConfig       `json:"evolution,omitempty"`
	Devices      DevicesConfig         `json:"devices"`
	WebUI        WebUIConfig           `json:"webui"`
	TTS          TTSConfig             `json:"tts"`
	RemoteAccess RemoteAccessConfig    `json:"remote_access,omitempty"`
	Guardrails   GuardrailsConfig      `json:"guardrails,omitempty"`
	Webhooks     []WebhookNotifyConfig `json:"webhooks,omitempty"`
	Digests      []DigestConfig        `json:"digests,omitempty"`
	UserName     string                `json:"user_name"               env:"SOFIA_USER_NAME"`
	MemoryDB     string                `json:"memory_db"               env:"SOFIA_MEMORY_DB"` // Path to SQLite memory database (default: ~/.sofia/memory.db)
}

// WebhookNotifyConfig configures an outbound notification webhook.
type WebhookNotifyConfig struct {
	URL     string   `json:"url"`
	Secret  string   `json:"secret,omitempty"` // HMAC-SHA256 signing secret
	Events  []string `json:"events"`           // event types to deliver
	Enabled bool     `json:"enabled"`
}

// DigestConfig configures a scheduled digest report.
type DigestConfig struct {
	Period        string `json:"period"`   // "daily", "weekly"
	Channel       string `json:"channel"`  // target channel for delivery
	ChatID        string `json:"chat_id"`  // target chat
	AgentID       string `json:"agent_id"` // which agent generates
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
	CircuitBreaker  CircuitBreakerConfig  `json:"circuit_breaker,omitempty"`
}

// CircuitBreakerConfig configures the circuit breaker for tool calls.
type CircuitBreakerConfig struct {
	Enabled          bool          `json:"enabled"           env:"SOFIA_GUARDRAILS_CB_ENABLED"`
	FailureThreshold int           `json:"failure_threshold" env:"SOFIA_GUARDRAILS_CB_THRESHOLD"`
	CooldownPeriod   time.Duration `json:"cooldown_period"   env:"SOFIA_GUARDRAILS_CB_COOLDOWN"`
}

// ApprovalConfig defines which tool calls require human-in-the-loop approval.
type ApprovalConfig struct {
	Enabled       bool     `json:"enabled"`
	RequireFor    []string `json:"require_for"`    // tool names requiring approval
	PatternMatch  []string `json:"pattern_match"`  // regex patterns on tool args
	TimeoutSec    int      `json:"timeout_sec"`    // how long to wait (default 300)
	DefaultAction string   `json:"default_action"` // "deny" or "allow" on timeout
	GooseMode     string   `json:"goose_mode"`     // "auto", "approve", "smart_approve", "chat"
}

// PIIDetectionConfig configures automatic PII detection on inbound messages.
type PIIDetectionConfig struct {
	Enabled bool   `json:"enabled" env:"SOFIA_GUARDRAILS_PII_ENABLED"`
	Action  string `json:"action"  env:"SOFIA_GUARDRAILS_PII_ACTION"` // "warn" (default), "redact", or "block"
}

type InputValidationConfig struct {
	Enabled          bool     `json:"enabled"            env:"SOFIA_GUARDRAILS_INPUT_ENABLED"`
	MaxMessageLength int      `json:"max_message_length" env:"SOFIA_GUARDRAILS_INPUT_MAX_LENGTH"`
	DenyPatterns     []string `json:"deny_patterns"      env:"SOFIA_GUARDRAILS_INPUT_DENY_PATTERNS"`
}

type OutputFilteringConfig struct {
	Enabled        bool     `json:"enabled"         env:"SOFIA_GUARDRAILS_OUTPUT_ENABLED"`
	RedactPatterns []string `json:"redact_patterns" env:"SOFIA_GUARDRAILS_OUTPUT_REDACT_PATTERNS"`
	Action         string   `json:"action"          env:"SOFIA_GUARDRAILS_OUTPUT_ACTION"` // "redact" or "block"
}

type RateLimitingConfig struct {
	Enabled          bool `json:"enabled"             env:"SOFIA_GUARDRAILS_RATELIMIT_ENABLED"`
	MaxRPM           int  `json:"max_rpm"             env:"SOFIA_GUARDRAILS_RATELIMIT_RPM"`
	MaxTokensPerHour int  `json:"max_tokens_per_hour" env:"SOFIA_GUARDRAILS_RATELIMIT_TOKENS"`
}

type SandboxedExecConfig struct {
	Enabled     bool   `json:"enabled"      env:"SOFIA_GUARDRAILS_SANDBOX_ENABLED"`
	DockerImage string `json:"docker_image" env:"SOFIA_GUARDRAILS_SANDBOX_DOCKER_IMAGE"` // e.g., "alpine:latest"
}

type PromptInjectionConfig struct {
	Enabled      bool   `json:"enabled"       env:"SOFIA_GUARDRAILS_INJECTION_ENABLED"`
	Action       string `json:"action"        env:"SOFIA_GUARDRAILS_INJECTION_ACTION"` // "block" or "warn"
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
	Enabled   bool   `json:"enabled"              env:"SOFIA_WEBUI_ENABLED"`
	Host      string `json:"host"                 env:"SOFIA_WEBUI_HOST"`
	Port      int    `json:"port"                 env:"SOFIA_WEBUI_PORT"`
	AuthToken string `json:"auth_token,omitempty" env:"SOFIA_WEBUI_AUTH_TOKEN"`
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

	// Merge catalog: add any default catalog entries not already in the user's list.
	// This ensures new models added to the built-in catalog in software updates are
	// immediately available after restart, without overriding user customizations.
	mergeCatalogEntries(cfg)

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

// MemoryDBPath returns the resolved path to the SQLite memory database.
// Falls back to ~/.sofia/memory.db when not explicitly configured.
func (c *Config) MemoryDBPath() string {
	if c.MemoryDB != "" {
		return expandHome(c.MemoryDB)
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}
	return filepath.Join(home, ".sofia", "memory.db")
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
