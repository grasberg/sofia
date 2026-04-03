package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.Validate()
	require.NoError(t, err)
}

func TestValidate_DuplicateAgentIDs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents.List = append(cfg.Agents.List, AgentConfig{ID: "main", Name: "Duplicate"})
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate agent id")
}

func TestValidate_EmptyAgentID(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents.List = append(cfg.Agents.List, AgentConfig{ID: "", Name: "NoID"})
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id is empty")
}

func TestValidate_NegativeMaxTokens(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents.Defaults.MaxTokens = -1
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_tokens")
}

func TestValidate_InvalidTemperature(t *testing.T) {
	cfg := DefaultConfig()
	temp := 3.0
	cfg.Agents.Defaults.Temperature = &temp
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "temperature")
}

func TestValidate_TelegramEnabledNoToken(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.Token = ""
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "telegram")
}

func TestValidate_DiscordEnabledNoToken(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Channels.Discord.Enabled = true
	cfg.Channels.Discord.Token = ""
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "discord")
}

func TestValidate_InvalidOutputFilterAction(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Guardrails.OutputFiltering.Enabled = true
	cfg.Guardrails.OutputFiltering.Action = "invalid"
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output_filtering.action")
}

func TestValidate_InvalidPromptInjectionAction(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Guardrails.PromptInjection.Enabled = true
	cfg.Guardrails.PromptInjection.Action = "invalid"
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt_injection.action")
}

func TestValidate_EmptyBindingAgentID(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Bindings = append(cfg.Bindings, AgentBinding{
		AgentID: "",
		Match:   BindingMatch{Channel: "telegram"},
	})
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_id is empty")
}

func TestValidate_EmptyBindingChannel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Bindings = append(cfg.Bindings, AgentBinding{
		AgentID: "main",
		Match:   BindingMatch{Channel: ""},
	})
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "match.channel is empty")
}

func TestValidate_ValidDMPolicies(t *testing.T) {
	for _, policy := range []string{"", "pairing", "open", "deny"} {
		cfg := DefaultConfig()
		cfg.Channels.Telegram.DMPolicy = policy
		cfg.Channels.Discord.DMPolicy = policy
		err := cfg.Validate()
		require.NoError(t, err, "policy %q should be valid", policy)
	}
}

func TestValidate_InvalidTelegramDMPolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Channels.Telegram.DMPolicy = "invalid"
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "telegram.dm_policy")
}

func TestValidate_InvalidDiscordDMPolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Channels.Discord.DMPolicy = "whatever"
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "discord.dm_policy")
}

func TestValidate_ValidFullConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.Token = "test-token"
	cfg.Guardrails.OutputFiltering.Enabled = true
	cfg.Guardrails.OutputFiltering.Action = "redact"
	cfg.Guardrails.PromptInjection.Enabled = true
	cfg.Guardrails.PromptInjection.Action = "warn"
	cfg.Bindings = append(cfg.Bindings, AgentBinding{
		AgentID: "main",
		Match:   BindingMatch{Channel: "telegram"},
	})
	err := cfg.Validate()
	require.NoError(t, err)
}

func TestValidate_InvalidWebUIPort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WebUI.Enabled = true
	cfg.WebUI.Port = 70000

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "webui.port")
}

func TestValidate_InvalidRemoteAccessPort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RemoteAccess.Enabled = true
	cfg.RemoteAccess.Port = -1

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote_access.port")
}

func TestValidate_InvalidHeartbeatInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Heartbeat.Enabled = true
	cfg.Heartbeat.Interval = 4

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "heartbeat.interval")
}

func TestValidate_InvalidAutonomyInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Autonomy.Enabled = true
	cfg.Autonomy.IntervalMinutes = -1

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "autonomy.interval_minutes")
}

func TestValidate_InvalidEvolutionInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Evolution.Enabled = true
	cfg.Evolution.IntervalMinutes = 4

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "evolution.interval_minutes")
}
