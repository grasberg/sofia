package config

type ChannelsConfig struct {
	Telegram TelegramConfig `json:"telegram"`
	Discord  DiscordConfig  `json:"discord"`
	Email    EmailConfig    `json:"email"`
}

// EmailConfig holds email channel configuration.
type EmailConfig struct {
	Enabled      bool     `json:"enabled"              env:"SOFIA_CHANNELS_EMAIL_ENABLED"`
	IMAPServer   string   `json:"imap_server"          env:"SOFIA_CHANNELS_EMAIL_IMAP_SERVER"`
	SMTPServer   string   `json:"smtp_server"          env:"SOFIA_CHANNELS_EMAIL_SMTP_SERVER"`
	Username     string   `json:"username"             env:"SOFIA_CHANNELS_EMAIL_USERNAME"`
	Password     string   `json:"password"             env:"SOFIA_CHANNELS_EMAIL_PASSWORD"`
	PollInterval int      `json:"poll_interval_sec"    env:"SOFIA_CHANNELS_EMAIL_POLL_INTERVAL"`
	AllowFrom    []string `json:"allow_from,omitempty"`
}

type TelegramConfig struct {
	Enabled   bool                `json:"enabled"             env:"SOFIA_CHANNELS_TELEGRAM_ENABLED"`
	Token     string              `json:"token"               env:"SOFIA_CHANNELS_TELEGRAM_TOKEN"`
	Proxy     string              `json:"proxy"               env:"SOFIA_CHANNELS_TELEGRAM_PROXY"`
	AllowFrom FlexibleStringSlice `json:"allow_from"          env:"SOFIA_CHANNELS_TELEGRAM_ALLOW_FROM"`
	DMPolicy  string              `json:"dm_policy,omitempty" env:"SOFIA_CHANNELS_TELEGRAM_DM_POLICY"`
}

type DiscordConfig struct {
	Enabled     bool                `json:"enabled"             env:"SOFIA_CHANNELS_DISCORD_ENABLED"`
	Token       string              `json:"token"               env:"SOFIA_CHANNELS_DISCORD_TOKEN"`
	AllowFrom   FlexibleStringSlice `json:"allow_from"          env:"SOFIA_CHANNELS_DISCORD_ALLOW_FROM"`
	MentionOnly bool                `json:"mention_only"        env:"SOFIA_CHANNELS_DISCORD_MENTION_ONLY"`
	DMPolicy    string              `json:"dm_policy,omitempty" env:"SOFIA_CHANNELS_DISCORD_DM_POLICY"`
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
	Enabled         bool    `json:"enabled"          env:"SOFIA_AUTONOMY_ENABLED"`
	Suggestions     bool    `json:"suggestions"      env:"SOFIA_AUTONOMY_SUGGESTIONS"`
	Goals           bool    `json:"goals"            env:"SOFIA_AUTONOMY_GOALS"`
	Research        bool    `json:"research"         env:"SOFIA_AUTONOMY_RESEARCH"`
	ContextTriggers bool    `json:"context_triggers" env:"SOFIA_AUTONOMY_CONTEXT_TRIGGERS"`
	IntervalMinutes int     `json:"interval_minutes" env:"SOFIA_AUTONOMY_INTERVAL"`
	MaxCostPerDay   float64 `json:"max_cost_per_day" env:"SOFIA_AUTONOMY_MAX_COST"`
}

// EvolutionConfig configures the self-improving evolution engine.
type EvolutionConfig struct {
	Enabled                bool     `json:"enabled"                   env:"SOFIA_EVOLUTION_ENABLED"`
	IntervalMinutes        int      `json:"interval_minutes"          env:"SOFIA_EVOLUTION_INTERVAL"`
	MaxCostPerDay          float64  `json:"max_cost_per_day"          env:"SOFIA_EVOLUTION_MAX_COST"`
	DailySummary           bool     `json:"daily_summary"             env:"SOFIA_EVOLUTION_DAILY_SUMMARY"`
	DailySummaryTime       string   `json:"daily_summary_time"        env:"SOFIA_EVOLUTION_SUMMARY_TIME"`
	DailySummaryChannel    string   `json:"daily_summary_channel"     env:"SOFIA_EVOLUTION_SUMMARY_CHANNEL"`
	DailySummaryChatID     string   `json:"daily_summary_chat_id"     env:"SOFIA_EVOLUTION_SUMMARY_CHAT_ID"`
	RetirementThreshold    float64  `json:"retirement_threshold"`
	RetirementMinTasks     int      `json:"retirement_min_tasks"`
	RetirementInactiveDays int      `json:"retirement_inactive_days"`
	SelfModifyEnabled      bool     `json:"self_modify_enabled"`
	ImmutableFiles         []string `json:"immutable_files,omitempty"`
	MaxAgents              int      `json:"max_agents"`
	RequireApproval        bool     `json:"require_approval"`
	MemoryConsolidation    bool     `json:"memory_consolidation"      env:"SOFIA_EVOLUTION_MEMORY_CONSOLIDATION"`
	ConsolidationIntervalH int      `json:"consolidation_interval_h"  env:"SOFIA_EVOLUTION_CONSOLIDATION_INTERVAL"` // default 6
	SkillAutoImprove       bool     `json:"skill_auto_improve"        env:"SOFIA_EVOLUTION_SKILL_AUTO_IMPROVE"`
}

type DevicesConfig struct {
	Enabled    bool `json:"enabled"     env:"SOFIA_DEVICES_ENABLED"`
	MonitorUSB bool `json:"monitor_usb" env:"SOFIA_DEVICES_MONITOR_USB"`
}
