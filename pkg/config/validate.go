package config

import (
	"fmt"
	"strings"
)

// Validate checks the configuration for common errors and returns the first issue found.
// It is called automatically after loading config from disk.
func (c *Config) Validate() error {
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

	return nil
}

func (c *Config) validateAgents() error {
	defaults := c.Agents.Defaults

	if defaults.MaxTokens < 0 {
		return fmt.Errorf("defaults.max_tokens must be non-negative, got %d", defaults.MaxTokens)
	}

	if defaults.MaxToolIterations < 0 {
		return fmt.Errorf("defaults.max_tool_iterations must be non-negative, got %d", defaults.MaxToolIterations)
	}

	if defaults.Temperature != nil {
		t := *defaults.Temperature
		if t < 0 || t > 2 {
			return fmt.Errorf("defaults.temperature must be between 0 and 2, got %f", t)
		}
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
	if c.Channels.Telegram.Enabled && c.Channels.Telegram.Token == "" {
		return fmt.Errorf("telegram is enabled but token is empty")
	}

	if c.Channels.Discord.Enabled && c.Channels.Discord.Token == "" {
		return fmt.Errorf("discord is enabled but token is empty")
	}

	if err := validateDMPolicy("telegram", c.Channels.Telegram.DMPolicy); err != nil {
		return err
	}

	if err := validateDMPolicy("discord", c.Channels.Discord.DMPolicy); err != nil {
		return err
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
		action := c.Guardrails.OutputFiltering.Action
		if action != "" && action != "redact" && action != "block" {
			return fmt.Errorf("output_filtering.action must be 'redact' or 'block', got %q", action)
		}
	}

	if c.Guardrails.PromptInjection.Enabled {
		action := c.Guardrails.PromptInjection.Action
		if action != "" && action != "block" && action != "warn" {
			return fmt.Errorf("prompt_injection.action must be 'block' or 'warn', got %q", action)
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
