package doctor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/config"
)

func TestNewDoctorCommand(t *testing.T) {
	cmd := NewDoctorCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "doctor", cmd.Use)
	assert.Equal(t, "Check Sofia's configuration and environment", cmd.Short)
	assert.False(t, cmd.HasSubCommands())
	assert.NotNil(t, cmd.RunE)
}

func TestCheckProviderAPIKeys_AllPresent(t *testing.T) {
	cfg := &config.Config{
		ModelList: []config.ModelConfig{
			{ModelName: "gpt-4o", Model: "openai/gpt-4o", APIKey: "sk-test"},
			{ModelName: "claude", Model: "anthropic/claude-sonnet-4.6", APIKey: "sk-ant-test"},
		},
	}

	passed, warned, failed := checkProviderAPIKeys(cfg)
	assert.Equal(t, 1, passed)
	assert.Equal(t, 0, warned)
	assert.Equal(t, 0, failed)
}

func TestCheckProviderAPIKeys_SomeMissing(t *testing.T) {
	cfg := &config.Config{
		ModelList: []config.ModelConfig{
			{ModelName: "gpt-4o", Model: "openai/gpt-4o", APIKey: "sk-test"},
			{ModelName: "claude", Model: "anthropic/claude-sonnet-4.6", APIKey: ""},
		},
	}

	passed, warned, failed := checkProviderAPIKeys(cfg)
	assert.Equal(t, 0, passed)
	assert.Equal(t, 1, warned)
	assert.Equal(t, 0, failed)
}

func TestCheckProviderAPIKeys_OllamaNoKey(t *testing.T) {
	cfg := &config.Config{
		ModelList: []config.ModelConfig{
			{ModelName: "local-llama", Model: "ollama/llama3", APIKey: ""},
		},
	}

	// Ollama does not need an API key, so this should pass.
	passed, warned, failed := checkProviderAPIKeys(cfg)
	assert.Equal(t, 1, passed)
	assert.Equal(t, 0, warned)
	assert.Equal(t, 0, failed)
}

func TestCheckProviderAPIKeys_Empty(t *testing.T) {
	cfg := &config.Config{
		ModelList: []config.ModelConfig{},
	}

	passed, warned, failed := checkProviderAPIKeys(cfg)
	assert.Equal(t, 0, passed)
	assert.Equal(t, 1, warned)
	assert.Equal(t, 0, failed)
}

func TestCheckChannelTokens_NoneEnabled(t *testing.T) {
	cfg := &config.Config{
		Channels: config.ChannelsConfig{
			Telegram: config.TelegramConfig{Enabled: false},
			Discord:  config.DiscordConfig{Enabled: false},
		},
	}

	passed, warned, failed := checkChannelTokens(cfg)
	assert.Equal(t, 0, passed)
	assert.Equal(t, 1, warned)
	assert.Equal(t, 0, failed)
}

func TestCheckChannelTokens_TelegramConfigured(t *testing.T) {
	cfg := &config.Config{
		Channels: config.ChannelsConfig{
			Telegram: config.TelegramConfig{Enabled: true, Token: "123:ABC"},
			Discord:  config.DiscordConfig{Enabled: false},
		},
	}

	passed, warned, failed := checkChannelTokens(cfg)
	assert.Equal(t, 1, passed)
	assert.Equal(t, 0, warned)
	assert.Equal(t, 0, failed)
}

func TestCheckChannelTokens_EnabledButEmpty(t *testing.T) {
	cfg := &config.Config{
		Channels: config.ChannelsConfig{
			Telegram: config.TelegramConfig{Enabled: true, Token: ""},
			Discord:  config.DiscordConfig{Enabled: false},
		},
	}

	passed, warned, failed := checkChannelTokens(cfg)
	assert.Equal(t, 0, passed)
	assert.Equal(t, 0, warned)
	assert.Equal(t, 1, failed)
}

func TestCheckSecurity_OpenAccess(t *testing.T) {
	cfg := &config.Config{
		Channels: config.ChannelsConfig{
			Telegram: config.TelegramConfig{
				Enabled:   true,
				Token:     "123:ABC",
				AllowFrom: config.FlexibleStringSlice{},
			},
			Discord: config.DiscordConfig{Enabled: false},
		},
	}

	passed, warned, failed := checkSecurity(cfg)
	assert.Equal(t, 0, passed)
	assert.Equal(t, 1, warned)
	assert.Equal(t, 0, failed)
}

func TestCheckSecurity_Restricted(t *testing.T) {
	cfg := &config.Config{
		Channels: config.ChannelsConfig{
			Telegram: config.TelegramConfig{
				Enabled:   true,
				Token:     "123:ABC",
				AllowFrom: config.FlexibleStringSlice{"12345"},
			},
			Discord: config.DiscordConfig{Enabled: false},
		},
	}

	passed, warned, failed := checkSecurity(cfg)
	assert.Equal(t, 1, passed)
	assert.Equal(t, 0, warned)
	assert.Equal(t, 0, failed)
}

func TestCheckSecurity_NoChannelsEnabled(t *testing.T) {
	cfg := &config.Config{
		Channels: config.ChannelsConfig{
			Telegram: config.TelegramConfig{Enabled: false},
			Discord:  config.DiscordConfig{Enabled: false},
		},
	}

	passed, warned, failed := checkSecurity(cfg)
	assert.Equal(t, 0, passed)
	assert.Equal(t, 0, warned)
	assert.Equal(t, 0, failed)
}

func TestCheckDatabase_InMemory(t *testing.T) {
	// Use the MemoryDB config field to point at an in-memory database.
	cfg := &config.Config{
		MemoryDB: ":memory:",
	}

	passed, warned, failed := checkDatabase(cfg)
	assert.Equal(t, 1, passed)
	assert.Equal(t, 0, warned)
	assert.Equal(t, 0, failed)
}

func TestDefaultAPIBase(t *testing.T) {
	assert.Equal(t, "https://api.openai.com/v1", defaultAPIBase("openai"))
	assert.Equal(t, "https://api.anthropic.com/v1", defaultAPIBase("anthropic"))
	assert.Equal(t, "", defaultAPIBase("unknown-protocol"))
}

func TestPrintHelpers(t *testing.T) {
	// Smoke test: these should not panic.
	printPass("test", "detail")
	printWarn("test", "detail")
	printFail("test", "detail")
}
