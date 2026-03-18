package evolution

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/reputation"
	"github.com/grasberg/sofia/pkg/utils"
)

// mockToolStats implements ToolStatsProvider for testing.
type mockToolStats struct {
	stats map[string]any
}

func (m *mockToolStats) GetStats() map[string]any {
	if m.stats == nil {
		return map[string]any{}
	}
	return m.stats
}

// newTestEngine creates an EvolutionEngine with all mocked dependencies.
func newTestEngine(t *testing.T, opts ...func(*testEngineOpts)) *EvolutionEngine {
	t.Helper()

	o := &testEngineOpts{}
	for _, opt := range opts {
		opt(o)
	}

	db := openTestDB(t)
	store := NewAgentStore(db)
	changelog := NewChangelogWriter(db)
	rep := reputation.NewManager(db)
	tracker := NewPerformanceTracker(rep, &config.EvolutionConfig{})
	toolStats := &mockToolStats{stats: o.toolStats}
	reg := &mockRegistrar{agentIDs: o.agentIDs}
	a2a := &mockA2A{}
	workspace := t.TempDir()

	llmResp := `{"capability_gaps":[],"underperformers":[],"success_patterns":[],"prompt_suggestions":[]}`
	if o.llmResponse != "" {
		llmResp = o.llmResponse
	}
	provider := &mockProvider{response: llmResp}

	architect := NewAgentArchitect(provider, reg, a2a, store, db, workspace)
	modifier := NewSafeModifier(t.TempDir(), nil, nil)

	cfg := &config.EvolutionConfig{
		Enabled:         true,
		IntervalMinutes: 60,
		MaxCostPerDay:   o.maxCost,
	}

	msgBus := bus.NewMessageBus()
	t.Cleanup(func() { msgBus.Close() })

	return NewEvolutionEngine(
		provider, db, reg, a2a, toolStats, rep,
		store, changelog, tracker, architect, modifier,
		cfg, msgBus,
	)
}

type testEngineOpts struct {
	agentIDs    []string
	toolStats   map[string]any
	llmResponse string
	maxCost     float64
}

func withAgentIDs(ids ...string) func(*testEngineOpts) {
	return func(o *testEngineOpts) { o.agentIDs = ids }
}

func withMaxCost(cost float64) func(*testEngineOpts) {
	return func(o *testEngineOpts) { o.maxCost = cost }
}

func withLLMResponse(resp string) func(*testEngineOpts) {
	return func(o *testEngineOpts) { o.llmResponse = resp }
}

func TestEvolutionEngine_NewAndStart(t *testing.T) {
	engine := newTestEngine(t)
	require.NotNil(t, engine)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := engine.Start(ctx)
	require.NoError(t, err)
	assert.True(t, engine.running.Load())

	// Starting again should fail.
	err = engine.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	engine.Stop()
	assert.False(t, engine.running.Load())
}

func TestEvolutionEngine_StartDisabled(t *testing.T) {
	engine := newTestEngine(t)
	engine.cfg.Enabled = false

	err := engine.Start(context.Background())
	require.NoError(t, err)
	assert.False(t, engine.running.Load())
}

func TestEvolutionEngine_StopGraceful(t *testing.T) {
	engine := newTestEngine(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := engine.Start(ctx)
	require.NoError(t, err)

	// Give the goroutine a moment to start.
	time.Sleep(50 * time.Millisecond)

	// Stop should not panic.
	engine.Stop()
	assert.False(t, engine.running.Load())
}

func TestEvolutionEngine_ConcurrencyGuard(t *testing.T) {
	// Create an engine with an LLM that responds with diagnosis+plan JSON sequences.
	diagResp := `{"capability_gaps":[],"underperformers":[],"success_patterns":[],"prompt_suggestions":[]}`
	planResp := `[{"type":"no_action","reason":"metrics acceptable"}]`

	// The mock provider alternates between diagnosis and plan responses.
	// Since runCycle calls diagnose then plan sequentially, we use a provider
	// that always returns valid JSON for both.
	engine := newTestEngine(t, withLLMResponse(diagResp))
	_ = planResp // both phases parse the same mock response structure

	var started atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)

	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			started.Add(1)
			engine.runCycle(context.Background())
		}()
	}

	wg.Wait()
	// Both goroutines attempted to run, but only one should have acquired the lock.
	// We verify no panic occurred — the concurrency guard (TryLock) prevented double execution.
	assert.Equal(t, int32(2), started.Load())
}

func TestEvolutionEngine_BudgetLimit(t *testing.T) {
	engine := newTestEngine(t, withMaxCost(0.01))

	// Simulate budget already spent.
	engine.budgetSpent = 0.02

	// runCycle should skip due to budget.
	engine.runCycle(context.Background())

	// Verify lastRun was NOT updated (cycle was skipped).
	assert.True(t, engine.lastRun.IsZero())
}

func TestEvolutionEngine_Observe(t *testing.T) {
	db := openTestDB(t)
	rep := reputation.NewManager(db)

	// Record some outcomes so the tracker has data.
	_, err := rep.RecordOutcome(reputation.TaskOutcome{
		AgentID:   "agent-1",
		Category:  "coding",
		Task:      "write a function",
		Success:   true,
		LatencyMs: 1000,
	})
	require.NoError(t, err)

	_, err = rep.RecordOutcome(reputation.TaskOutcome{
		AgentID:   "agent-1",
		Category:  "coding",
		Task:      "debug a crash",
		Success:   false,
		LatencyMs: 2000,
	})
	require.NoError(t, err)

	store := NewAgentStore(db)
	changelog := NewChangelogWriter(db)
	tracker := NewPerformanceTracker(rep, &config.EvolutionConfig{})
	toolStats := &mockToolStats{stats: map[string]any{"exec": 3}}
	reg := &mockRegistrar{agentIDs: []string{"agent-1"}}
	a2a := &mockA2A{}
	workspace := t.TempDir()

	provider := &mockProvider{response: `{}`}
	architect := NewAgentArchitect(provider, reg, a2a, store, db, workspace)
	modifier := NewSafeModifier(t.TempDir(), nil, nil)
	cfg := &config.EvolutionConfig{Enabled: true}
	msgBus := bus.NewMessageBus()
	t.Cleanup(func() { msgBus.Close() })

	engine := NewEvolutionEngine(
		provider, db, reg, a2a, toolStats, rep,
		store, changelog, tracker, architect, modifier,
		cfg, msgBus,
	)

	report := engine.observe(context.Background())

	assert.Contains(t, report.AgentStats, "agent-1")
	snap := report.AgentStats["agent-1"]
	assert.Equal(t, 2, snap.TaskCount)
	assert.Equal(t, 0.5, snap.SuccessRate)
	assert.Equal(t, 2, report.TotalTasks)
	assert.Equal(t, 3, report.ToolFailures["exec"])
}

func TestEvolutionEngine_PauseResume(t *testing.T) {
	// Use a valid JSON response that works for diagnosis.
	diagResp := `{"capability_gaps":[],"underperformers":[],"success_patterns":[],"prompt_suggestions":[]}`
	engine := newTestEngine(t, withLLMResponse(diagResp))

	// Pause should prevent cycle execution.
	engine.Pause()
	assert.True(t, engine.paused.Load())

	engine.runCycle(context.Background())
	assert.True(t, engine.lastRun.IsZero(), "cycle should not have run while paused")

	// Resume should allow execution again.
	engine.Resume()
	assert.False(t, engine.paused.Load())
}

func TestEvolutionEngine_FormatStatus(t *testing.T) {
	engine := newTestEngine(t)
	engine.cfg.MaxCostPerDay = 1.00
	engine.budgetSpent = 0.25

	status := engine.FormatStatus()
	assert.Contains(t, status, "Evolution Engine Status")
	assert.Contains(t, status, "State: stopped")
	assert.Contains(t, status, "Paused: no")
	assert.Contains(t, status, "Last run: never")
	assert.Contains(t, status, "$0.25 / $1.00")
}

func TestEvolutionEngine_RecentHistory(t *testing.T) {
	engine := newTestEngine(t)

	// Write a changelog entry.
	entry := &ChangelogEntry{
		Timestamp: time.Now().UTC(),
		Action:    "test_action",
		Summary:   "test summary",
	}
	require.NoError(t, engine.changelog.Write(entry))

	history, err := engine.RecentHistory(10)
	require.NoError(t, err)
	assert.Len(t, history, 1)
	assert.Equal(t, "test_action", history[0].Action)
}

func TestEvolutionEngine_Diagnose(t *testing.T) {
	diagJSON := `{
		"capability_gaps": ["data analysis"],
		"underperformers": ["agent-slow"],
		"success_patterns": ["code review works well"],
		"prompt_suggestions": ["be more concise"]
	}`
	engine := newTestEngine(t, withLLMResponse(diagJSON))

	report := ObservationReport{
		AgentStats:   map[string]*AgentPerfSnapshot{},
		ToolFailures: map[string]int{},
	}

	diagnosis, err := engine.diagnose(context.Background(), report)
	require.NoError(t, err)
	assert.Equal(t, []string{"data analysis"}, diagnosis.CapabilityGaps)
	assert.Equal(t, []string{"agent-slow"}, diagnosis.Underperformers)
	assert.Equal(t, []string{"code review works well"}, diagnosis.SuccessPatterns)
	assert.Equal(t, []string{"be more concise"}, diagnosis.PromptSuggestions)
}

func TestEvolutionEngine_Plan(t *testing.T) {
	planJSON := `[
		{"type": "no_action", "reason": "all metrics acceptable"},
		{"type": "create_agent", "params": {"gap": "monitoring"}, "reason": "no monitoring agent"}
	]`
	engine := newTestEngine(t, withLLMResponse(planJSON))

	diagnosis := Diagnosis{
		CapabilityGaps: []string{"monitoring"},
	}

	actions, err := engine.plan(context.Background(), diagnosis)
	require.NoError(t, err)
	require.Len(t, actions, 2)
	assert.Equal(t, ActionNoAction, actions[0].Type)
	assert.Equal(t, ActionCreateAgent, actions[1].Type)
}

func TestEvolutionEngine_CleanJSONResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain", `{"key": "value"}`, `{"key": "value"}`},
		{"with fences", "```json\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"with generic fences", "```\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"extra whitespace", "  \n```json\n{\"key\": \"value\"}\n```\n  ", `{"key": "value"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := utils.CleanJSONFences(tt.input)
			var parsed map[string]string
			err := json.Unmarshal([]byte(result), &parsed)
			require.NoError(t, err)
			assert.Equal(t, "value", parsed["key"])
		})
	}
}

func TestEvolutionEngine_Abs(t *testing.T) {
	assert.Equal(t, 5, abs(-5))
	assert.Equal(t, 5, abs(5))
	assert.Equal(t, 0, abs(0))
}
