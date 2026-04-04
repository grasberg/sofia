package evolution

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pt "github.com/grasberg/sofia/pkg/providers/protocoltypes"
)

// mockProvider implements providers.LLMProvider for testing.
type mockProvider struct {
	response string
	err      error
}

func (m *mockProvider) Chat(
	_ context.Context,
	_ []pt.Message,
	_ []pt.ToolDefinition,
	_ string,
	_ map[string]any,
) (*pt.LLMResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &pt.LLMResponse{Content: m.response}, nil
}

func (m *mockProvider) GetDefaultModel() string { return "mock-model" }

// mockRegistrar implements AgentRegistrar for testing.
type mockRegistrar struct {
	removedIDs []string
	agentIDs   []string
}

func (m *mockRegistrar) RemoveAgent(agentID string) error {
	m.removedIDs = append(m.removedIDs, agentID)
	return nil
}

func (m *mockRegistrar) ListAgentIDs() []string {
	return m.agentIDs
}

// mockA2A implements A2ARegistrar for testing.
type mockA2A struct {
	registeredIDs []string
}

func (m *mockA2A) Register(agentID string) {
	m.registeredIDs = append(m.registeredIDs, agentID)
}

func TestAgentArchitect_DesignAgent(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)

	llm := &mockProvider{
		response: `{
			"id": "code-reviewer",
			"name": "Code Reviewer",
			"purpose_prompt": "You are a specialist code review agent.",
			"model": "gpt-4o",
			"skills_filter": ["git", "code-review"],
			"temperature": 0.3
		}`,
	}

	architect := NewAgentArchitect(llm, "test-model", &mockRegistrar{}, &mockA2A{}, store, db, t.TempDir())

	cfg, err := architect.DesignAgent(context.Background(), "code review and quality assurance")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "code-reviewer", cfg.ID)
	assert.Equal(t, "Code Reviewer", cfg.Name)
	assert.Equal(t, "You are a specialist code review agent.", cfg.PurposePrompt)
	assert.Equal(t, "gpt-4o", cfg.ModelID)
	assert.Equal(t, []string{"git", "code-review"}, cfg.Skills)
}

func TestAgentArchitect_DesignAgent_MalformedJSON(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)

	llm := &mockProvider{
		response: `this is not json at all`,
	}

	architect := NewAgentArchitect(llm, "test-model", &mockRegistrar{}, &mockA2A{}, store, db, t.TempDir())

	cfg, err := architect.DesignAgent(context.Background(), "something")
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "failed to parse LLM response")
}

func TestAgentArchitect_DesignAgent_EmptyID(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)

	llm := &mockProvider{
		response: `{"id": "", "name": "No ID Agent", "purpose_prompt": "test", "model": "gpt-4o"}`,
	}

	architect := NewAgentArchitect(llm, "test-model", &mockRegistrar{}, &mockA2A{}, store, db, t.TempDir())

	cfg, err := architect.DesignAgent(context.Background(), "something")
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "empty id")
}

func TestAgentArchitect_DesignAgent_InvalidSlug(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)

	llm := &mockProvider{
		response: `{"id": "Bad Slug!", "name": "Agent", "purpose_prompt": "test", "model": "gpt-4o"}`,
	}

	architect := NewAgentArchitect(llm, "test-model", &mockRegistrar{}, &mockA2A{}, store, db, t.TempDir())

	cfg, err := architect.DesignAgent(context.Background(), "something")
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "not a valid slug")
}

func TestAgentArchitect_DesignAgent_MarkdownFences(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)

	// LLM wraps JSON in markdown code fences.
	llm := &mockProvider{
		response: "```json\n" + `{
			"id": "data-analyst",
			"name": "Data Analyst",
			"purpose_prompt": "Analyze data sets.",
			"model": "gpt-4o",
			"skills_filter": [],
			"temperature": 0.5
		}` + "\n```",
	}

	architect := NewAgentArchitect(llm, "test-model", &mockRegistrar{}, &mockA2A{}, store, db, t.TempDir())

	cfg, err := architect.DesignAgent(context.Background(), "data analysis")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "data-analyst", cfg.ID)
}

func TestAgentArchitect_CreateAgent(t *testing.T) {
	db := openTestDB(t)
	store := NewAgentStore(db)
	reg := &mockRegistrar{}
	a2a := &mockA2A{}
	workspace := t.TempDir()

	architect := NewAgentArchitect(&mockProvider{}, "test-model", reg, a2a, store, db, workspace)

	cfg := EvolutionAgentConfig{}
	cfg.ID = "test-agent"
	cfg.Name = "Test Agent"
	cfg.PurposePrompt = "You help with tests."
	cfg.ModelID = "gpt-4o"

	err := architect.CreateAgent(context.Background(), cfg)
	require.NoError(t, err)

	// Verify store persistence.
	got, status, err := store.Get("test-agent")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "active", status)
	assert.Equal(t, "Test Agent", got.Name)

	// Verify A2A registration.
	assert.Equal(t, []string{"test-agent"}, a2a.registeredIDs)

	// Verify registrar was NOT called (no RegisterAgent — loop.go handles that).
	assert.Empty(t, reg.removedIDs)

	// Verify skill file was written.
	skillPath := filepath.Join(workspace, "skills", "test-agent", "SKILL.md")
	data, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "name: Test Agent")
	assert.Contains(t, content, "You help with tests.")
}
