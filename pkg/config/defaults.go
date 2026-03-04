// Sofia - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Sofia contributors

package config

import "strings"

// ensureMainAgent guarantees that agents.list always contains exactly one entry
// with id "main" and default=true. It is called from LoadConfig to handle
// configs written before this requirement was introduced.
func ensureMainAgent(cfg *Config) {
	for _, a := range cfg.Agents.List {
		if a.Default || strings.EqualFold(strings.TrimSpace(a.ID), "main") {
			return // already present
		}
	}
	// Prepend so it appears first in the list.
	cfg.Agents.List = append([]AgentConfig{{ID: "main", Name: "Sofia", Default: true}}, cfg.Agents.List...)
}

// DefaultConfig returns the default configuration for Sofia.
func DefaultConfig() *Config {
	return &Config{
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Workspace:           "~/.sofia/workspace",
				RestrictToWorkspace: true,
				Provider:            "",
				Model:               "",
				MaxTokens:           32768,
				Temperature:         nil, // nil means use provider default
				MaxToolIterations:   50,
			},
			List: []AgentConfig{
				{
					ID:      "main",
					Name:    "Sofia",
					Default: true,
				},
			},
		},
		Bindings: []AgentBinding{},
		Session: SessionConfig{
			DMScope: "per-channel-peer",
		},
		Channels: ChannelsConfig{
			Telegram: TelegramConfig{
				Enabled:   false,
				Token:     "",
				AllowFrom: FlexibleStringSlice{},
			},
			Discord: DiscordConfig{
				Enabled:     false,
				Token:       "",
				AllowFrom:   FlexibleStringSlice{},
				MentionOnly: false,
			},
		},
		Providers: ProvidersConfig{
			OpenAI: OpenAIProviderConfig{WebSearch: true},
		},
		ModelList: []ModelConfig{},
		Gateway: GatewayConfig{
			Host: "127.0.0.1",
			Port: 18790,
		},
		Tools: ToolsConfig{
			Web: WebToolsConfig{
				Proxy: "",
				Brave: BraveConfig{
					Enabled:    false,
					APIKey:     "",
					MaxResults: 5,
				},
				DuckDuckGo: DuckDuckGoConfig{
					Enabled:    true,
					MaxResults: 5,
				},
				Perplexity: PerplexityConfig{
					Enabled:    false,
					APIKey:     "",
					MaxResults: 5,
				},
			},
			Cron: CronToolsConfig{
				ExecTimeoutMinutes: 5,
			},
			Exec: ExecConfig{
				EnableDenyPatterns: true,
			},
			Google: GoogleToolsConfig{
				Enabled:         false,
				BinaryPath:      "gog",
				TimeoutSeconds:  90,
				AllowedCommands: []string{"gmail", "drive", "calendar"},
			},
			Skills: SkillsToolsConfig{
				Registries: SkillsRegistriesConfig{
					ClawHub: ClawHubRegistryConfig{
						Enabled: true,
						BaseURL: "https://clawhub.ai",
					},
				},
				MaxConcurrentSearches: 2,
				SearchCache: SearchCacheConfig{
					MaxSize:    50,
					TTLSeconds: 300,
				},
			},
		},
		Heartbeat: HeartbeatConfig{
			Enabled:     true,
			Interval:    30,
			ActiveHours: "",
			ActiveDays:  []string{},
		},
		Devices: DevicesConfig{
			Enabled:    false,
			MonitorUSB: true,
		},
		WebUI: WebUIConfig{
			Enabled: true,
			Host:    "0.0.0.0",
			Port:    18795,
		},
		UserName: "User",
	}
}
