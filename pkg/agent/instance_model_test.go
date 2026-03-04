package agent

import (
	"os"
	"testing"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
)

// testCfgWithModelList builds a Config with a model_list for model alias tests.
func testCfgWithModelList(
	tmpDir string,
	modelList []config.ModelConfig,
	defaultModelName string,
	agents []config.AgentConfig,
) *config.Config {
	return &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         defaultModelName,
				MaxTokens:         8192,
				MaxToolIterations: 10,
			},
			List: agents,
		},
		ModelList: modelList,
	}
}

// TestAgentInstance_ModelIDResolvedFromAlias verifies that when a model alias is set,
// the AgentInstance.ModelID is resolved to the raw model ID (without protocol prefix),
// while AgentInstance.Model stays as the user-facing alias.
func TestAgentInstance_ModelIDResolvedFromAlias(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-model-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfgWithModelList(tmpDir, []config.ModelConfig{
		{ModelName: "MyGPT", Model: "openai/gpt-4o", APIKey: "sk-test"},
	}, "MyGPT", nil)

	registry := NewAgentRegistry(cfg, &mockRegistryProvider{})
	agent, ok := registry.GetAgent("main")
	if !ok || agent == nil {
		t.Fatal("expected main agent")
	}

	if agent.Model != "MyGPT" {
		t.Errorf("agent.Model = %q, want %q", agent.Model, "MyGPT")
	}
	if agent.ModelID != "gpt-4o" {
		t.Errorf("agent.ModelID = %q, want %q", agent.ModelID, "gpt-4o")
	}
}

// TestAgentInstance_ModelAliasPersistsAfterReload verifies that ReloadAgents does not
// overwrite cfg.Agents.Defaults.ModelName with the raw modelID.
func TestAgentInstance_ModelAliasPersistsAfterReload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-reload-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfgWithModelList(tmpDir, []config.ModelConfig{
		{ModelName: "MyAlias", Model: "anthropic/claude-sonnet-4.6", APIKey: "sk-anth"},
	}, "MyAlias", nil)

	msgBus := bus.NewMessageBus()
	al := NewAgentLoop(cfg, msgBus, &mockRegistryProvider{})

	// Simulate what the web UI does: reload agents after saving config
	al.ReloadAgents()

	// The alias must survive the reload — it must NOT be replaced with the raw modelID
	if cfg.Agents.Defaults.ModelName != "MyAlias" {
		t.Errorf("ModelName after reload = %q, want %q (alias was clobbered)", cfg.Agents.Defaults.ModelName, "MyAlias")
	}
}

// TestAgentInstance_PerAgentModelIDResolution verifies that an agent with a custom model
// gets its ModelID resolved from model_list, not from the defaults.
func TestAgentInstance_PerAgentModelIDResolution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-per-agent-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfgWithModelList(tmpDir, []config.ModelConfig{
		{ModelName: "DefaultModel", Model: "openai/gpt-4o", APIKey: "sk-default"},
		{ModelName: "AgentModel", Model: "anthropic/claude-haiku-3", APIKey: "sk-agent"},
	}, "DefaultModel", []config.AgentConfig{
		{ID: "main", Default: true},
		{
			ID:    "specialized",
			Model: &config.AgentModelConfig{Primary: "AgentModel"},
		},
	})

	registry := NewAgentRegistry(cfg, &mockRegistryProvider{})

	main, ok := registry.GetAgent("main")
	if !ok {
		t.Fatal("expected main agent")
	}
	if main.Model != "DefaultModel" {
		t.Errorf("main.Model = %q, want %q", main.Model, "DefaultModel")
	}
	if main.ModelID != "gpt-4o" {
		t.Errorf("main.ModelID = %q, want %q", main.ModelID, "gpt-4o")
	}

	specialized, ok := registry.GetAgent("specialized")
	if !ok {
		t.Fatal("expected specialized agent")
	}
	if specialized.Model != "AgentModel" {
		t.Errorf("specialized.Model = %q, want %q", specialized.Model, "AgentModel")
	}
	if specialized.ModelID != "claude-haiku-3" {
		t.Errorf("specialized.ModelID = %q, want %q", specialized.ModelID, "claude-haiku-3")
	}
}

// TestAgentInstance_PerAgentProviderIsolation verifies that an agent with a custom model
// gets its own provider instance rather than the shared default provider.
func TestAgentInstance_PerAgentProviderIsolation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-provider-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	defaultProvider := &mockRegistryProvider{}

	cfg := testCfgWithModelList(tmpDir, []config.ModelConfig{
		{ModelName: "DefaultModel", Model: "openai/gpt-4o", APIKey: "sk-default"},
		// Agent model uses oauth so CreateProviderFromConfig will try to load auth creds.
		// Use a second openai key-based model to avoid auth store dependency in tests.
		{ModelName: "AgentModel", Model: "openai/gpt-4o-mini", APIKey: "sk-agent-key"},
	}, "DefaultModel", []config.AgentConfig{
		{ID: "main", Default: true},
		{
			ID:    "custom",
			Model: &config.AgentModelConfig{Primary: "AgentModel"},
		},
	})

	registry := NewAgentRegistry(cfg, defaultProvider)

	main, ok := registry.GetAgent("main")
	if !ok {
		t.Fatal("expected main agent")
	}
	custom, ok := registry.GetAgent("custom")
	if !ok {
		t.Fatal("expected custom agent")
	}

	// The custom agent must have its own provider (not the shared default).
	if custom.Provider == main.Provider {
		t.Error("custom agent should have its own provider, not the shared default provider")
	}
}

// TestSwitchCommand_ValidModel verifies /switch model to <name> accepts a valid alias.
func TestSwitchCommand_ValidModel(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-switch-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfgWithModelList(tmpDir, []config.ModelConfig{
		{ModelName: "ModelA", Model: "openai/gpt-4o", APIKey: "sk-a"},
		{ModelName: "ModelB", Model: "openai/gpt-4o-mini", APIKey: "sk-b"},
	}, "ModelA", nil)

	msgBus := bus.NewMessageBus()
	al := NewAgentLoop(cfg, msgBus, &mockRegistryProvider{})

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}
	if defaultAgent.Model != "ModelA" {
		t.Errorf("initial model = %q, want %q", defaultAgent.Model, "ModelA")
	}

	// Execute the /switch command via handleCommand
	inMsg := newTestInboundMessage("/switch model to ModelB")
	resp, handled := al.handleCommand(
		nil,
		inMsg,
	) //nolint:staticcheck // context is unused in handleCommand for commands
	if !handled {
		t.Fatal("expected /switch to be handled")
	}
	if defaultAgent.Model != "ModelB" {
		t.Errorf("model after switch = %q, want %q", defaultAgent.Model, "ModelB")
	}
	if defaultAgent.ModelID != "gpt-4o-mini" {
		t.Errorf("modelID after switch = %q, want %q", defaultAgent.ModelID, "gpt-4o-mini")
	}
	_ = resp
}

// TestSwitchCommand_InvalidModel verifies /switch model to <unknown> is rejected.
func TestSwitchCommand_InvalidModel(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-switch-invalid-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfgWithModelList(tmpDir, []config.ModelConfig{
		{ModelName: "OnlyModel", Model: "openai/gpt-4o", APIKey: "sk-x"},
	}, "OnlyModel", nil)

	msgBus := bus.NewMessageBus()
	al := NewAgentLoop(cfg, msgBus, &mockRegistryProvider{})

	defaultAgent := al.registry.GetDefaultAgent()
	originalModel := defaultAgent.Model

	inMsg := newTestInboundMessage("/switch model to nonexistent-model")
	resp, handled := al.handleCommand(nil, inMsg) //nolint:staticcheck // context unused in command handling
	if !handled {
		t.Fatal("expected /switch to be handled")
	}
	// Model must NOT change on invalid input
	if defaultAgent.Model != originalModel {
		t.Errorf("model changed to %q after invalid switch, want %q", defaultAgent.Model, originalModel)
	}
	// Response should contain an error indication
	if resp == "" {
		t.Error("expected non-empty error response for invalid model switch")
	}
}

// TestTwoAgentsDifferentProviders verifies two agents can be configured with
// different model aliases pointing to different providers.
func TestTwoAgentsDifferentProviders(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-two-providers-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := testCfgWithModelList(tmpDir, []config.ModelConfig{
		{ModelName: "OpenAIModel", Model: "openai/gpt-4o", APIKey: "sk-openai"},
		{ModelName: "AnthropicModel", Model: "openai/gpt-4o-mini", APIKey: "sk-anthropic-compat"},
	}, "OpenAIModel", []config.AgentConfig{
		{ID: "main", Default: true},
		{
			ID:    "agent-b",
			Model: &config.AgentModelConfig{Primary: "AnthropicModel"},
		},
	})

	registry := NewAgentRegistry(cfg, &mockRegistryProvider{})

	main, ok := registry.GetAgent("main")
	if !ok {
		t.Fatal("expected main agent")
	}
	agentB, ok := registry.GetAgent("agent-b")
	if !ok {
		t.Fatal("expected agent-b")
	}

	// Each agent should have resolved its own alias
	if main.Model != "OpenAIModel" {
		t.Errorf("main.Model = %q, want OpenAIModel", main.Model)
	}
	if main.ModelID != "gpt-4o" {
		t.Errorf("main.ModelID = %q, want gpt-4o", main.ModelID)
	}
	if agentB.Model != "AnthropicModel" {
		t.Errorf("agentB.Model = %q, want AnthropicModel", agentB.Model)
	}
	if agentB.ModelID != "gpt-4o-mini" {
		t.Errorf("agentB.ModelID = %q, want gpt-4o-mini", agentB.ModelID)
	}

	// The two agents should have different providers (each built from their own model config)
	if main.Provider == agentB.Provider {
		t.Error("agents with different model configs should have different provider instances")
	}
}

// newTestInboundMessage creates a minimal InboundMessage for command handling tests.
func newTestInboundMessage(content string) bus.InboundMessage {
	return bus.InboundMessage{
		Content:  content,
		Channel:  "cli",
		ChatID:   "test",
		SenderID: "test-user",
	}
}
