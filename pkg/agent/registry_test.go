package agent

import (
	"context"
	"testing"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
)

type mockRegistryProvider struct{}

func (m *mockRegistryProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: "mock", FinishReason: "stop"}, nil
}

func (m *mockRegistryProvider) GetDefaultModel() string {
	return "mock-model"
}

func testCfg(agents []config.AgentConfig) *config.Config {
	return &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         "/tmp/sofia-test-registry",
				Model:             "gpt-4",
				MaxTokens:         8192,
				MaxToolIterations: 10,
			},
			List: agents,
		},
	}
}

func testMemDB(t *testing.T) *memory.MemoryDB {
	t.Helper()
	db, err := memory.Open(":memory:")
	if err != nil {
		t.Fatalf("open test memdb: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestNewAgentRegistry_ImplicitMain(t *testing.T) {
	cfg := testCfg(nil)
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	// Main agent must always exist.
	agent, ok := registry.GetAgent("main")
	if !ok || agent == nil {
		t.Fatal("expected to find 'main' agent")
	}
	if agent.ID != "main" {
		t.Errorf("agent.ID = %q, want 'main'", agent.ID)
	}

	// When no user agents are configured, templates are auto-seeded.
	ids := registry.ListAgentIDs()
	if len(ids) < 2 {
		t.Errorf("expected auto-seeded agents from templates, got %d agents", len(ids))
	}
}

func TestNewAgentRegistry_ExplicitAgents(t *testing.T) {
	cfg := testCfg([]config.AgentConfig{
		{ID: "sales", Default: true, Name: "Sales Bot"},
		{ID: "support", Name: "Support Bot"},
	})
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	ids := registry.ListAgentIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 agents, got %d: %v", len(ids), ids)
	}

	sales, ok := registry.GetAgent("sales")
	if !ok || sales == nil {
		t.Fatal("expected to find 'sales' agent")
	}
	if sales.Name != "Sales Bot" {
		t.Errorf("sales.Name = %q, want 'Sales Bot'", sales.Name)
	}

	support, ok := registry.GetAgent("support")
	if !ok || support == nil {
		t.Fatal("expected to find 'support' agent")
	}
}

func TestAgentRegistry_GetAgent_Normalize(t *testing.T) {
	cfg := testCfg([]config.AgentConfig{
		{ID: "my-agent", Default: true},
	})
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	agent, ok := registry.GetAgent("My-Agent")
	if !ok || agent == nil {
		t.Fatal("expected to find agent with normalized ID")
	}
	if agent.ID != "my-agent" {
		t.Errorf("agent.ID = %q, want 'my-agent'", agent.ID)
	}
}

func TestAgentRegistry_GetDefaultAgent(t *testing.T) {
	cfg := testCfg([]config.AgentConfig{
		{ID: "alpha"},
		{ID: "beta", Default: true},
	})
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	// GetDefaultAgent first checks for "main", then returns any
	agent := registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected a default agent")
	}
}

func TestAgentRegistry_CanSpawnSubagent(t *testing.T) {
	cfg := testCfg([]config.AgentConfig{
		{
			ID:      "parent",
			Default: true,
			Subagents: &config.SubagentsConfig{
				AllowAgents: []string{"child1", "child2"},
			},
		},
		{ID: "child1"},
		{ID: "child2"},
		{ID: "restricted"},
	})
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	if !registry.CanSpawnSubagent("parent", "child1") {
		t.Error("expected parent to be allowed to spawn child1")
	}
	if !registry.CanSpawnSubagent("parent", "child2") {
		t.Error("expected parent to be allowed to spawn child2")
	}
	if registry.CanSpawnSubagent("parent", "restricted") {
		t.Error("expected parent to NOT be allowed to spawn restricted")
	}
	if registry.CanSpawnSubagent("child1", "child2") {
		t.Error("expected child1 to NOT be allowed to spawn (no subagents config)")
	}
}

func TestAgentRegistry_CanSpawnSubagent_Wildcard(t *testing.T) {
	cfg := testCfg([]config.AgentConfig{
		{
			ID:      "admin",
			Default: true,
			Subagents: &config.SubagentsConfig{
				AllowAgents: []string{"*"},
			},
		},
		{ID: "any-agent"},
	})
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	if !registry.CanSpawnSubagent("admin", "any-agent") {
		t.Error("expected wildcard to allow spawning any agent")
	}
	if !registry.CanSpawnSubagent("admin", "nonexistent") {
		t.Error("expected wildcard to allow spawning even nonexistent agents")
	}
}

func TestAgentInstance_Model(t *testing.T) {
	model := &config.AgentModelConfig{Primary: "claude-opus"}
	cfg := testCfg([]config.AgentConfig{
		{ID: "custom", Default: true, Model: model},
	})
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	agent, _ := registry.GetAgent("custom")
	if agent.Model != "claude-opus" {
		t.Errorf("agent.Model = %q, want 'claude-opus'", agent.Model)
	}
}

func TestAgentInstance_FallbackInheritance(t *testing.T) {
	cfg := testCfg([]config.AgentConfig{
		{ID: "inherit", Default: true},
	})
	cfg.Agents.Defaults.ModelFallbacks = []string{"openai/gpt-4o-mini", "anthropic/haiku"}
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	agent, _ := registry.GetAgent("inherit")
	if len(agent.Fallbacks) != 2 {
		t.Errorf("expected 2 fallbacks inherited from defaults, got %d", len(agent.Fallbacks))
	}
}

func TestAgentInstance_FallbackExplicitEmpty(t *testing.T) {
	model := &config.AgentModelConfig{
		Primary:   "gpt-4",
		Fallbacks: []string{}, // explicitly empty = disable
	}
	cfg := testCfg([]config.AgentConfig{
		{ID: "no-fallback", Default: true, Model: model},
	})
	cfg.Agents.Defaults.ModelFallbacks = []string{"should-not-inherit"}
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	agent, _ := registry.GetAgent("no-fallback")
	if len(agent.Fallbacks) != 0 {
		t.Errorf("expected 0 fallbacks (explicit empty), got %d: %v", len(agent.Fallbacks), agent.Fallbacks)
	}
}

func TestTemplateDisplayName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"backend-specialist", "Backend Specialist"},
		{"researcher", "Researcher"},
		{"code-review-expert", "Code Review Expert"},
		{"", ""},
	}
	for _, tt := range tests {
		got := templateDisplayName(tt.input)
		if got != tt.want {
			t.Errorf("templateDisplayName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestHasUserSubagents(t *testing.T) {
	tests := []struct {
		name   string
		agents []config.AgentConfig
		want   bool
	}{
		{"nil list", nil, false},
		{"empty list", []config.AgentConfig{}, false},
		{"only main", []config.AgentConfig{{ID: "main", Default: true}}, false},
		{"main plus subagent", []config.AgentConfig{
			{ID: "main", Default: true},
			{ID: "sales"},
		}, true},
		{"only non-main default", []config.AgentConfig{
			{ID: "custom", Default: true},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasUserSubagents(tt.agents)
			if got != tt.want {
				t.Errorf("hasUserSubagents() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgentRegistry_RegisterAgent(t *testing.T) {
	cfg := testCfg([]config.AgentConfig{
		{ID: "existing", Default: true},
	})
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	newInstance := &AgentInstance{
		ID:   "dynamic",
		Name: "Dynamic Agent",
	}

	err := registry.RegisterAgent(newInstance)
	if err != nil {
		t.Fatalf("RegisterAgent returned error: %v", err)
	}

	agent, ok := registry.GetAgent("dynamic")
	if !ok || agent == nil {
		t.Fatal("expected 'dynamic' agent to exist after registration")
	}
	if agent.Name != "Dynamic Agent" {
		t.Errorf("agent.Name = %q, want 'Dynamic Agent'", agent.Name)
	}
}

func TestAgentRegistry_RegisterAgent_Duplicate(t *testing.T) {
	cfg := testCfg([]config.AgentConfig{
		{ID: "existing", Default: true},
	})
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	err := registry.RegisterAgent(&AgentInstance{ID: "existing"})
	if err == nil {
		t.Fatal("expected error when registering duplicate agent")
	}
}

func TestAgentRegistry_ListAgents(t *testing.T) {
	cfg := testCfg([]config.AgentConfig{
		{ID: "alpha", Default: true},
		{ID: "beta"},
	})
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	agents := registry.ListAgents()
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}

	ids := make(map[string]bool)
	for _, a := range agents {
		ids[a.ID] = true
	}
	if !ids["alpha"] {
		t.Error("expected 'alpha' in agent list")
	}
	if !ids["beta"] {
		t.Error("expected 'beta' in agent list")
	}
}

func TestAgentRegistry_GetDefaultAgent_NoMain(t *testing.T) {
	// When no "main" agent exists, GetDefaultAgent returns any agent.
	cfg := testCfg([]config.AgentConfig{
		{ID: "only-one", Default: true},
	})
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	// Remove "main" if it was auto-created, to test the fallback path.
	_ = registry.RemoveAgent("main")

	agent := registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected GetDefaultAgent to return a non-nil agent")
	}
}

func TestAgentRegistry_CanSpawnSubagent_NonexistentParent(t *testing.T) {
	cfg := testCfg([]config.AgentConfig{
		{ID: "child", Default: true},
	})
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	if registry.CanSpawnSubagent("nonexistent", "child") {
		t.Error("expected false when parent doesn't exist")
	}
}

func TestAgentRegistry_RemoveAgent(t *testing.T) {
	cfg := testCfg([]config.AgentConfig{
		{ID: "ephemeral", Default: true, Name: "Ephemeral Agent"},
	})
	registry := NewAgentRegistry(cfg, &mockRegistryProvider{}, testMemDB(t))

	// Verify agent exists before removal.
	_, ok := registry.GetAgent("ephemeral")
	if !ok {
		t.Fatal("expected 'ephemeral' agent to exist before removal")
	}

	// Remove the agent.
	err := registry.RemoveAgent("ephemeral")
	if err != nil {
		t.Fatalf("RemoveAgent returned unexpected error: %v", err)
	}

	// Verify agent no longer exists.
	_, ok = registry.GetAgent("ephemeral")
	if ok {
		t.Error("expected GetAgent to return false after removal")
	}

	// Removing an unknown agent should return an error.
	err = registry.RemoveAgent("nonexistent")
	if err == nil {
		t.Error("expected error when removing unknown agent")
	}
}
