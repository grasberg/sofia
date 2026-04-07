package config

import (
	"fmt"
	"strings"
)

// Validate checks the configuration for common errors and returns the first issue found.
// It is called automatically after loading config from disk.
// Zero-value fields are given sensible defaults before validation runs.
func (c *Config) Validate() error {
	c.applyDefaults()

	if err := c.validateAgents(); err != nil {
		return fmt.Errorf("agents: %w", err)
	}

	if err := c.validateChannels(); err != nil {
		return fmt.Errorf("channels: %w", err)
	}

	if err := c.validateBindings(); err != nil {
		return fmt.Errorf("bindings: %w", err)
	}

	if err := c.validateGuardrails(); err != nil {
		return fmt.Errorf("guardrails: %w", err)
	}

	if err := c.validatePorts(); err != nil {
		return err
	}

	if err := c.validateIntervals(); err != nil {
		return err
	}

	return nil
}

// applyDefaults fills in zero-value config fields with sensible defaults.
func (c *Config) applyDefaults() {
	if c.Agents.Defaults.MaxTokens == 0 {
		c.Agents.Defaults.MaxTokens = 4096
	}
	if c.Agents.Defaults.MaxToolIterations == 0 {
		c.Agents.Defaults.MaxToolIterations = 25
	}
}

// validatePorts checks that configured ports are in the valid range 1-65535.
func (c *Config) validatePorts() error {
	if err := validateEnabledPort("webui.port", c.WebUI.Enabled, c.WebUI.Port); err != nil {
		return err
	}
	if err := validateEnabledPort("remote_access.port", c.RemoteAccess.Enabled, c.RemoteAccess.Port); err != nil {
		return err
	}
	return nil
}

// validateIntervals checks that enabled features have sane minimum intervals.
func (c *Config) validateIntervals() error {
	if err := validateEnabledMinimum(
		"heartbeat.interval",
		c.Heartbeat.Enabled,
		c.Heartbeat.Interval,
		5,
		"minutes",
	); err != nil {
		return err
	}
	if err := validateEnabledMinimum(
		"autonomy.interval_minutes",
		c.Autonomy.Enabled,
		c.Autonomy.IntervalMinutes,
		1,
		"",
	); err != nil {
		return err
	}
	if err := validateEnabledMinimum(
		"evolution.interval_minutes",
		c.Evolution.Enabled,
		c.Evolution.IntervalMinutes,
		5,
		"",
	); err != nil {
		return err
	}
	return nil
}

func validateEnabledPort(field string, enabled bool, port int) error {
	if !enabled || port == 0 {
		return nil
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535, got %d", field, port)
	}
	return nil
}

func validateEnabledMinimum(field string, enabled bool, value, min int, unit string) error {
	if !enabled || value == 0 || value >= min {
		return nil
	}

	requirement := fmt.Sprintf("%d", min)
	if unit != "" {
		requirement = fmt.Sprintf("%d %s", min, unit)
	}

	return fmt.Errorf("%s must be at least %s, got %d", field, requirement, value)
}

func (c *Config) validateAgents() error {
	defaults := c.Agents.Defaults

	if defaults.MaxTokens < 0 {
		return fmt.Errorf("defaults.max_tokens must be non-negative, got %d", defaults.MaxTokens)
	}

	if defaults.MaxToolIterations < 0 {
		return fmt.Errorf("defaults.max_tool_iterations must be non-negative, got %d", defaults.MaxToolIterations)
	}

	if defaults.MaxConcurrentSubagents < 0 {
		return fmt.Errorf("defaults.max_concurrent_subagents must be non-negative, got %d", defaults.MaxConcurrentSubagents)
	}

	if defaults.Temperature != nil {
		t := *defaults.Temperature
		if t < 0 || t > 2 {
			return fmt.Errorf("defaults.temperature must be between 0 and 2, got %f", t)
		}
	}

	if err := validateAllowedValues(
		"defaults.code_editor",
		defaults.CodeEditor,
		"opencode", "claudecode", "codex", "qwencode",
	); err != nil {
		return err
	}

	// Check for duplicate agent IDs
	seen := make(map[string]bool)
	for i, agent := range c.Agents.List {
		id := strings.TrimSpace(agent.ID)
		if id == "" {
			return fmt.Errorf("list[%d].id is empty", i)
		}
		if seen[id] {
			return fmt.Errorf("list[%d]: duplicate agent id %q", i, id)
		}
		seen[id] = true
	}

	return nil
}

func (c *Config) validateChannels() error {
	if err := validateChannelToken("telegram", c.Channels.Telegram.Enabled, c.Channels.Telegram.Token); err != nil {
		return err
	}

	if err := validateChannelToken("discord", c.Channels.Discord.Enabled, c.Channels.Discord.Token); err != nil {
		return err
	}

	if err := validateDMPolicy("telegram", c.Channels.Telegram.DMPolicy); err != nil {
		return err
	}

	if err := validateDMPolicy("discord", c.Channels.Discord.DMPolicy); err != nil {
		return err
	}

	return nil
}

func validateChannelToken(channel string, enabled bool, token string) error {
	if enabled && token == "" {
		return fmt.Errorf("%s is enabled but token is empty", channel)
	}

	return nil
}

// validateDMPolicy checks that a channel's dm_policy value is one of the
// allowed values: empty (use allowlist), "pairing", "open", or "deny".
func validateDMPolicy(channel, policy string) error {
	switch policy {
	case "", "pairing", "open", "deny":
		return nil
	default:
		return fmt.Errorf(
			"%s.dm_policy must be 'pairing', 'open', 'deny', or empty, got %q",
			channel, policy,
		)
	}
}

func (c *Config) validateBindings() error {
	for i, b := range c.Bindings {
		if b.AgentID == "" {
			return fmt.Errorf("[%d].agent_id is empty", i)
		}
		if b.Match.Channel == "" {
			return fmt.Errorf("[%d].match.channel is empty", i)
		}
	}
	return nil
}

func (c *Config) validateGuardrails() error {
	if c.Guardrails.OutputFiltering.Enabled {
		if err := validateAllowedValues(
			"output_filtering.action",
			c.Guardrails.OutputFiltering.Action,
			"redact",
			"block",
		); err != nil {
			return err
		}
	}

	if c.Guardrails.PromptInjection.Enabled {
		if err := validateAllowedValues(
			"prompt_injection.action",
			c.Guardrails.PromptInjection.Action,
			"block",
			"warn",
		); err != nil {
			return err
		}
	}

	if c.Guardrails.RateLimiting.Enabled {
		if c.Guardrails.RateLimiting.MaxRPM < 0 {
			return fmt.Errorf("rate_limiting.max_rpm must be non-negative")
		}
		if c.Guardrails.RateLimiting.MaxTokensPerHour < 0 {
			return fmt.Errorf("rate_limiting.max_tokens_per_hour must be non-negative")
		}
	}

	return nil
}

func validateAllowedValues(field, value string, allowed ...string) error {
	if value == "" {
		return nil
	}

	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}

	return fmt.Errorf("%s must be %s, got %q", field, joinQuotedValues(allowed), value)
}

func joinQuotedValues(values []string) string {
	if len(values) == 0 {
		return ""
	}

	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = fmt.Sprintf("'%s'", value)
	}

	if len(quoted) == 1 {
		return quoted[0]
	}

	if len(quoted) == 2 {
		return quoted[0] + " or " + quoted[1]
	}

	return strings.Join(quoted[:len(quoted)-1], ", ") + ", or " + quoted[len(quoted)-1]
}
