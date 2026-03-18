# Evolution Engine Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an autonomous self-improvement engine that creates specialist agents, retires underperformers, writes skills, and evolves Sofia's behavior — all without human intervention.

**Architecture:** A periodic 5-phase loop (Observe->Diagnose->Plan->Act->Verify) in `pkg/evolution/` that wraps existing reputation, reflection, skill, and agent-creation systems into a closed feedback loop. Uses SQLite for changelog and agent persistence. Follows the `autonomy.Service` lifecycle pattern (context cancellation, mutex guard, ticker loop). Uses interfaces to avoid circular imports between `pkg/evolution/` and `pkg/agent/`.

**Tech Stack:** Go, SQLite (modernc.org/sqlite via existing MemoryDB), existing LLMProvider interface, existing AgentRegistry/ReputationManager

**Spec:** `docs/superpowers/specs/2026-03-18-evolution-engine-design.md`

**Import Cycle Prevention:** `pkg/agent/loop.go` imports `pkg/evolution/`. To avoid a circular dependency, `pkg/evolution/` must NOT import `pkg/agent/`. Instead, Task 8 defines interfaces (`AgentRegistrar`, `A2ARegistrar`, `ToolStatsProvider`) in `pkg/evolution/` that `pkg/agent/` types satisfy. The engine receives these interfaces, not concrete types.

---

## Chunk 1: Foundation (Config, DB Migration, Registry, Reputation)

### Task 1: Add EvolutionConfig to config

**Files:**
- Modify: `pkg/config/config.go`

- [ ] **Step 1: Add EvolutionConfig struct and field**

Add after the `AutonomyConfig` struct (around line 401):

```go
type EvolutionConfig struct {
	Enabled              bool     `json:"enabled"              env:"SOFIA_EVOLUTION_ENABLED"`
	IntervalMinutes      int      `json:"interval_minutes"     env:"SOFIA_EVOLUTION_INTERVAL"`
	MaxCostPerDay        float64  `json:"max_cost_per_day"     env:"SOFIA_EVOLUTION_MAX_COST"`
	DailySummary         bool     `json:"daily_summary"        env:"SOFIA_EVOLUTION_DAILY_SUMMARY"`
	DailySummaryTime     string   `json:"daily_summary_time"   env:"SOFIA_EVOLUTION_SUMMARY_TIME"`
	DailySummaryChannel  string   `json:"daily_summary_channel" env:"SOFIA_EVOLUTION_SUMMARY_CHANNEL"`
	DailySummaryChatID   string   `json:"daily_summary_chat_id" env:"SOFIA_EVOLUTION_SUMMARY_CHAT_ID"`
	RetirementThreshold  float64  `json:"retirement_threshold"`
	RetirementMinTasks   int      `json:"retirement_min_tasks"`
	RetirementInactiveDays int    `json:"retirement_inactive_days"`
	SelfModifyEnabled    bool     `json:"self_modify_enabled"`
	ImmutableFiles       []string `json:"immutable_files,omitempty"`
	MaxAgents            int      `json:"max_agents"`
}
```

Add `Evolution EvolutionConfig` field to the `Config` struct (around line 50-71, near `Autonomy AutonomyConfig`).

- [ ] **Step 2: Verify build**

Run: `CGO_ENABLED=0 go build -tags stdjson ./pkg/config/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add pkg/config/config.go
git commit -m "feat(evolution): add EvolutionConfig to config"
```

---

### Task 2: Add RemoveAgent to AgentRegistry

**Files:**
- Modify: `pkg/agent/registry.go`
- Test: `pkg/agent/registry_test.go`

- [ ] **Step 1: Write failing test**

```go
func TestAgentRegistry_RemoveAgent(t *testing.T) {
	// Create registry, register a test agent, then remove it
	// Verify GetAgent returns false after removal
	// Verify removing non-existent agent returns error
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `CGO_ENABLED=0 go test -tags stdjson ./pkg/agent/... -run TestAgentRegistry_RemoveAgent -v`
Expected: FAIL -- `RemoveAgent` undefined

- [ ] **Step 3: Implement RemoveAgent**

Add to `registry.go` after `RegisterAgent` (line ~152):

```go
func (r *AgentRegistry) RemoveAgent(agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := routing.NormalizeAgentID(agentID)
	if _, exists := r.agents[id]; !exists {
		return fmt.Errorf("agent %q not found", id)
	}
	delete(r.agents, id)
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `CGO_ENABLED=0 go test -tags stdjson ./pkg/agent/... -run TestAgentRegistry_RemoveAgent -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/agent/registry.go pkg/agent/registry_test.go
git commit -m "feat(evolution): add RemoveAgent to AgentRegistry"
```

---

### Task 3: Add GetAgentStatsSince to reputation

**Files:**
- Modify: `pkg/reputation/reputation.go`
- Test: `pkg/reputation/reputation_test.go` (or create if absent)

- [ ] **Step 1: Write failing test**

```go
func TestGetAgentStatsSince(t *testing.T) {
	// Insert outcomes at different timestamps using RecordOutcome
	// Query with since=1h ago -- only recent outcomes included
	// Verify counts match expected
}
```

- [ ] **Step 2: Run test -- FAIL**

- [ ] **Step 3: Implement GetAgentStatsSince**

Same SQL as `GetAgentStats` (line 101) but with `WHERE agent_id = ? AND created_at >= ?`:

```go
func (m *Manager) GetAgentStatsSince(agentID string, since time.Time) (*AgentStats, error) {
	row := m.db.QueryRow(`
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as successes,
			AVG(latency_ms) as avg_latency,
			AVG(tokens_out) as avg_tokens_out
		FROM agent_reputation
		WHERE agent_id = ? AND created_at >= ?`, agentID, since.UTC().Format("2006-01-02 15:04:05"))
	// Same scan/calculation as GetAgentStats
}
```

- [ ] **Step 4: Run test -- PASS**

- [ ] **Step 5: Commit**

```bash
git add pkg/reputation/reputation.go pkg/reputation/reputation_test.go
git commit -m "feat(evolution): add time-windowed GetAgentStatsSince"
```

---

### Task 4: Add DB migration V10 for evolution tables

**Files:**
- Modify: `pkg/memory/db.go`

- [ ] **Step 1: Increment schemaVersion from 9 to 10**

Change `const schemaVersion = 9` to `const schemaVersion = 10` at line 17.

- [ ] **Step 2: Add applyV10 method**

```go
func (m *MemoryDB) applyV10() error {
	ddl := `
	CREATE TABLE IF NOT EXISTS evolution_agents (
		agent_id TEXT PRIMARY KEY,
		config_json TEXT NOT NULL,
		status TEXT DEFAULT 'active',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		retired_at DATETIME,
		retired_reason TEXT,
		created_by TEXT
	);
	CREATE TABLE IF NOT EXISTS evolution_changelog (
		id TEXT PRIMARY KEY,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		action TEXT NOT NULL,
		summary TEXT NOT NULL,
		details_json TEXT,
		outcome TEXT,
		outcome_verified_at DATETIME,
		metric_before REAL,
		metric_after REAL
	);
	CREATE INDEX IF NOT EXISTS idx_changelog_ts ON evolution_changelog(timestamp);
	CREATE INDEX IF NOT EXISTS idx_reputation_agent_created ON agent_reputation(agent_id, created_at);
	`
	_, err := m.db.Exec(ddl)
	return err
}
```

- [ ] **Step 3: Wire applyV10 into migrate()**

The migration uses `if current < N` guards (NOT a switch). Add after the existing `if current < 9` block (around line 165):

```go
if current < 10 {
	if err = m.applyV10(); err != nil {
		return err
	}
}
```

- [ ] **Step 4: Verify build and all existing tests pass**

Run: `CGO_ENABLED=0 go build -tags stdjson ./...` and `CGO_ENABLED=0 go test -tags stdjson ./pkg/memory/...`

- [ ] **Step 5: Commit**

```bash
git add pkg/memory/db.go
git commit -m "feat(evolution): add migration V10 for evolution tables"
```

---

### Task 4b: Store ToolTracker on AgentLoop

**Files:**
- Modify: `pkg/agent/loop.go`

The `toolTracker` is currently created in `NewAgentLoop` (around line 147) and passed to `registerSharedTools` but NOT stored as a field. The evolution engine needs it.

- [ ] **Step 1: Add `toolTracker *tools.ToolTracker` field to AgentLoop struct**

Add after `dashboardHub` field (around line 62).

- [ ] **Step 2: Assign in constructor**

In the `al := &AgentLoop{` block, add: `toolTracker: toolTracker,` (the variable already exists from line ~147).

- [ ] **Step 3: Verify build**

Run: `CGO_ENABLED=0 go build -tags stdjson ./...`

- [ ] **Step 4: Commit**

```bash
git add pkg/agent/loop.go
git commit -m "feat(evolution): store ToolTracker on AgentLoop for evolution access"
```

---

## Chunk 2: Persistence Layer (AgentStore, Changelog)

### Task 5: Implement AgentStore

**Files:**
- Create: `pkg/evolution/store.go`
- Create: `pkg/evolution/store_test.go`

- [ ] **Step 1: Write tests**

Tests for: `Save`, `Get`, `ListActive`, `MarkRetired`, `ListRetired` (returns `[]string` of agent IDs, not full configs), round-trip persistence. Use `:memory:` SQLite via `memory.Open(":memory:")`.

- [ ] **Step 2: Run tests -- FAIL**

- [ ] **Step 3: Implement AgentStore**

```go
// EvolutionAgentConfig extends config.AgentConfig with runtime fields.
type EvolutionAgentConfig struct {
	config.AgentConfig
	PurposePrompt string `json:"purpose_prompt,omitempty"`
	ModelID       string `json:"model_id,omitempty"`
}
```

Store serializes `EvolutionAgentConfig` to JSON for the `config_json` column.

Methods:
- `NewAgentStore(db *memory.MemoryDB) *AgentStore`
- `Save(agentID string, cfg EvolutionAgentConfig) error` -- upsert
- `Get(agentID string) (*EvolutionAgentConfig, string, error)` -- return config + status
- `ListActive() ([]EvolutionAgentConfig, error)` -- all where status='active'
- `ListRetired() ([]string, error)` -- just agent IDs where status='retired'
- `MarkRetired(agentID, reason string) error` -- set status, retired_at, reason

- [ ] **Step 4: Run tests -- PASS**

- [ ] **Step 5: Commit**

```bash
git add pkg/evolution/store.go pkg/evolution/store_test.go
git commit -m "feat(evolution): implement AgentStore for dynamic agent persistence"
```

---

### Task 6: Implement ChangelogWriter

**Files:**
- Create: `pkg/evolution/changelog.go`
- Create: `pkg/evolution/changelog_test.go`

- [ ] **Step 1: Write tests**

Tests for: `Write`, `Query` (with time filter + limit), `Get` by ID, `UpdateOutcome`, `QueryUnverified` (entries with NULL outcome).

- [ ] **Step 2: Run tests -- FAIL**

- [ ] **Step 3: Implement ChangelogWriter**

```go
type ChangelogEntry struct {
	ID           string         `json:"id"`
	Timestamp    time.Time      `json:"timestamp"`
	Action       string         `json:"action"`
	Summary      string         `json:"summary"`
	Details      map[string]any `json:"details,omitempty"`
	Outcome      string         `json:"outcome,omitempty"`
	VerifiedAt   *time.Time     `json:"verified_at,omitempty"`
	MetricBefore float64        `json:"metric_before,omitempty"`
	MetricAfter  float64        `json:"metric_after,omitempty"`
}

type ActionOutcome struct {
	Result       string  `json:"result"` // improved, no_change, degraded, reverted
	MetricBefore float64 `json:"metric_before"`
	MetricAfter  float64 `json:"metric_after"`
}
```

Methods: `NewChangelogWriter(db)`, `Write`, `UpdateOutcome`, `Query(since, limit)`, `Get(id)`, `QueryUnverified(limit)`.

- [ ] **Step 4: Run tests -- PASS**

- [ ] **Step 5: Commit**

```bash
git add pkg/evolution/changelog.go pkg/evolution/changelog_test.go
git commit -m "feat(evolution): implement SQLite-backed ChangelogWriter"
```

---

## Chunk 3: Performance Tracker

### Task 7: Implement Performance Tracker

**Files:**
- Create: `pkg/evolution/tracker.go`
- Create: `pkg/evolution/tracker_test.go`

- [ ] **Step 1: Write tests**

Tests for:
- `GetAgentPerformance` -- returns rolling 24h stats
- `DetectTrend` -- improving/stable/declining based on two 24h windows
- `ShouldRetire` -- threshold checks (success rate, utilization, min tasks)
- `GetSpecializationScore` -- ratio of dominant category to total

- [ ] **Step 2: Run tests -- FAIL**

- [ ] **Step 3: Implement tracker**

```go
type AgentPerformance struct {
	AgentID             string  `json:"agent_id"`
	SuccessRate24h      float64 `json:"success_rate_24h"`
	AvgScore24h         float64 `json:"avg_score_24h"`
	TaskCount24h        int     `json:"task_count_24h"`
	Trend               string  `json:"trend"` // improving, stable, declining
	SpecializationScore float64 `json:"specialization"`
	Utilization         float64 `json:"utilization"`
}

type PerformanceTracker struct {
	reputation *reputation.Manager
	cfg        *config.EvolutionConfig
}
```

`ShouldRetire(agentID)` checks: success rate < threshold over 48h with >= min tasks, OR zero utilization for inactive days.

`DetectTrend`: compare `GetAgentStatsSince(id, 24h_ago)` vs `GetAgentStatsSince(id, 48h_ago)`. If current > previous + 0.05 -> improving. If current < previous - 0.05 -> declining. Otherwise stable.

- [ ] **Step 4: Run tests -- PASS**

- [ ] **Step 5: Commit**

```bash
git add pkg/evolution/tracker.go pkg/evolution/tracker_test.go
git commit -m "feat(evolution): implement PerformanceTracker with trend detection"
```

---

## Chunk 4: Action Types, Interfaces, and Self-Modification

### Task 8: Define action types and dependency interfaces

**Files:**
- Create: `pkg/evolution/actions.go`
- Create: `pkg/evolution/interfaces.go`

- [ ] **Step 1: Define action types in `actions.go`**

```go
package evolution

type ActionType string

const (
	ActionCreateAgent      ActionType = "create_agent"
	ActionRetireAgent      ActionType = "retire_agent"
	ActionTuneAgent        ActionType = "tune_agent"
	ActionCreateSkill      ActionType = "create_skill"
	ActionModifyWorkspace  ActionType = "modify_workspace"
	ActionAdjustGuardrails ActionType = "adjust_guardrails"
	ActionNoAction         ActionType = "no_action"
)

type EvolutionAction struct {
	Type    ActionType     `json:"type"`
	AgentID string         `json:"agent_id,omitempty"`
	Params  map[string]any `json:"params"`
	Reason  string         `json:"reason"`
}

type ObservationReport struct {
	AgentStats      map[string]*AgentPerfSnapshot `json:"agent_stats"`
	ToolFailures    map[string]int                `json:"tool_failures"`
	DelegationMisses int                          `json:"delegation_misses"`
	TotalTasks       int                          `json:"total_tasks"`
	ErrorRate        float64                      `json:"error_rate"`
}

type AgentPerfSnapshot struct {
	AgentID     string  `json:"agent_id"`
	SuccessRate float64 `json:"success_rate"`
	TaskCount   int     `json:"task_count"`
	AvgScore    float64 `json:"avg_score"`
	Trend       string  `json:"trend"`
}

type Diagnosis struct {
	CapabilityGaps    []string          `json:"capability_gaps"`
	Underperformers   []string          `json:"underperformers"`
	SuccessPatterns   []string          `json:"success_patterns"`
	PromptSuggestions []string          `json:"prompt_suggestions"`
}
```

- [ ] **Step 2: Define interfaces in `interfaces.go`**

These interfaces prevent import cycles. `pkg/agent/` types satisfy them without `pkg/evolution/` importing `pkg/agent/`.

```go
package evolution

// AgentRegistrar manages agent registration (satisfied by agent.AgentRegistry).
type AgentRegistrar interface {
	RegisterAgent(instance any) error
	RemoveAgent(agentID string) error
	ListAgentIDs() []string
	GetDefaultAgent() any
}

// A2ARegistrar handles inter-agent routing (satisfied by agent.A2ARouter).
type A2ARegistrar interface {
	Register(agentID string)
}

// ToolStatsProvider gives tool execution statistics (satisfied by tools.ToolTracker).
type ToolStatsProvider interface {
	GetStats() map[string]any
}
```

Note: The `any` return types avoid importing agent-specific types. The evolution engine uses them for inspection only, not for calling methods on the returned objects. Where the engine needs to create agents, it calls through the `AgentRegistrar` with an `any` parameter that the registry type-asserts internally.

- [ ] **Step 3: Verify build**

Run: `CGO_ENABLED=0 go build -tags stdjson ./pkg/evolution/...`

- [ ] **Step 4: Commit**

```bash
git add pkg/evolution/actions.go pkg/evolution/interfaces.go
git commit -m "feat(evolution): define action types and dependency interfaces"
```

---

### Task 9: Implement self-modification with safety

**Files:**
- Create: `pkg/evolution/self_modify.go`
- Create: `pkg/evolution/self_modify_test.go`

- [ ] **Step 1: Write tests**

Tests for:
- `VersionFile` -- backup created at `~/.sofia/evolution/history/{basename}.{timestamp}.bak`
- `IsImmutable` -- config.json, pkg/ paths, evolution engine files blocked
- `ModifyFile` -- versions first, then writes new content
- `RevertFile` -- restores content from backup
- `ListBackups` -- finds all backups for a file sorted by timestamp
- `ValidateSafety` -- rejects content that contains `DROP TABLE audit_log` or similar

- [ ] **Step 2: Run tests -- FAIL**

- [ ] **Step 3: Implement SafeModifier**

```go
type SafeModifier struct {
	historyDir     string
	immutablePaths []string
	provider       providers.LLMProvider // for semantic validation
}

func NewSafeModifier(historyDir string, extraImmutable []string, provider providers.LLMProvider) *SafeModifier
func (sm *SafeModifier) IsImmutable(path string) bool
func (sm *SafeModifier) VersionFile(path string) (backupPath string, err error)
func (sm *SafeModifier) ModifyFile(ctx context.Context, path, newContent string) error  // versions + validates + writes
func (sm *SafeModifier) RevertFile(path, backupPath string) error
func (sm *SafeModifier) ListBackups(path string) ([]string, error)
```

**Semantic validation** (called inside `ModifyFile`): Send a prompt to the LLM asking "Does this change disable any safety mechanism, remove access controls, or bypass security guardrails?" Parse the yes/no response. If yes, reject with `blocked_by_safety` error. If LLM call fails (timeout, etc.), proceed cautiously with a warning log.

Default immutable paths: `config.json`, `config.yaml`, `.env`, any path starting with `pkg/`, any path containing `evolution/`.

- [ ] **Step 4: Run tests -- PASS**

- [ ] **Step 5: Commit**

```bash
git add pkg/evolution/self_modify.go pkg/evolution/self_modify_test.go
git commit -m "feat(evolution): implement SafeModifier with versioning and semantic validation"
```

---

## Chunk 5: Agent Architect

### Task 10: Implement Agent Architect

**Files:**
- Create: `pkg/evolution/architect.go`
- Create: `pkg/evolution/architect_test.go`

- [ ] **Step 1: Write tests**

Tests for:
- `DesignAgent` -- given a gap description + mock LLM returning canned JSON, returns valid `EvolutionAgentConfig`
- `DesignAgent` -- malformed LLM JSON returns error
- `CreateAgent` -- mock registrar + store, verify both called

- [ ] **Step 2: Run tests -- FAIL**

- [ ] **Step 3: Implement AgentArchitect**

Uses interfaces from `interfaces.go` to avoid importing `pkg/agent/`:

```go
type AgentArchitect struct {
	provider   providers.LLMProvider
	registrar  AgentRegistrar   // interface, not concrete type
	a2a        A2ARegistrar     // interface
	store      *AgentStore
	memDB      *memory.MemoryDB
	workspace  string
}
```

`DesignAgent(ctx, gapDescription string) (*EvolutionAgentConfig, error)`:
- Sends structured prompt to LLM requesting JSON blueprint:
  ```
  Design a specialist AI agent for the following capability gap:
  {gapDescription}

  Return JSON: {"id": "slug-name", "name": "Human Name", "purpose_prompt": "...",
  "model": "model-name", "skills_filter": [...], "temperature": 0.X}
  ```
- Parses response into `EvolutionAgentConfig`

`CreateAgent(ctx, cfg EvolutionAgentConfig) error`:
- Calls `registrar.RegisterAgent()` (the `AgentRegistry` accepts this via type assertion or by wrapping the config into an `AgentInstance` in `loop.go` before calling)
- Calls `store.Save()`
- Calls `a2a.Register(cfg.ID)`
- Writes `SKILL.md` to `workspace/skills/{id}/SKILL.md`
- Adds delegation hints to knowledge graph via `memDB.AddNode`

- [ ] **Step 4: Run tests -- PASS**

- [ ] **Step 5: Commit**

```bash
git add pkg/evolution/architect.go pkg/evolution/architect_test.go
git commit -m "feat(evolution): implement AgentArchitect for autonomous agent creation"
```

---

## Chunk 6: Evolution Engine Core

### Task 11: Implement the Evolution Engine

**Files:**
- Create: `pkg/evolution/engine.go`
- Create: `pkg/evolution/engine_test.go`

- [ ] **Step 1: Write tests**

Tests for:
- `NewEvolutionEngine` -- constructor wires all dependencies
- `Start/Stop` lifecycle -- starts ticker, stops on context cancel (not mutex block)
- `runCycle` concurrency guard -- two simultaneous calls via goroutines, only one runs (verify with atomic counter)
- `observe` -- with mock reputation data, returns populated `ObservationReport`
- Budget limit -- set `MaxCostPerDay=0.001`, mock LLM that "costs" 0.01, verify cycle skips

- [ ] **Step 2: Run tests -- FAIL**

- [ ] **Step 3: Implement EvolutionEngine**

```go
type EvolutionEngine struct {
	provider    providers.LLMProvider
	memDB       *memory.MemoryDB
	registrar   AgentRegistrar
	a2a         A2ARegistrar
	toolStats   ToolStatsProvider
	reputation  *reputation.Manager
	store       *AgentStore
	changelog   *ChangelogWriter
	tracker     *PerformanceTracker
	architect   *AgentArchitect
	modifier    *SafeModifier
	cfg         *config.EvolutionConfig
	bus         *bus.MessageBus      // for daily summary notifications

	mu          sync.Mutex           // single-execution guard
	cancelFunc  context.CancelFunc   // for graceful shutdown (NOT mu)
	running     atomic.Bool
	budgetSpent float64
	lastRun     time.Time
	paused      atomic.Bool
}
```

Follow `autonomy.Service` lifecycle (context.WithCancel pattern):
- `Start(ctx)` -- mutex-guarded, context.WithCancel, goroutine `runLoop`
- `Stop()` -- calls `cancelFunc()`, logs shutdown
- `runLoop(ctx, interval)` -- 2min initial delay, ticker, select on ctx.Done
- `runCycle(ctx)` -- `mu.TryLock()` or check-and-skip, calls 5 phases
- `observe(ctx)` -- collects from reputation.GetAgentStatsSince, toolStats.GetStats, tracker
- `diagnose(ctx, report)` -- LLM call, parse JSON into Diagnosis
- `plan(ctx, diagnosis)` -- LLM call, parse JSON into []EvolutionAction
- `act(ctx, actions)` -- dispatch: create->architect, retire->store+registrar, tune->store, skill->write file, modify->SafeModifier, guardrails->in-memory config
- `verify(ctx)` -- query unverified changelog, compare metrics, update outcomes, revert if degraded
- `RunNow(ctx)` -- public method for `/evolve run`, calls `runCycle` directly

**Diagnose prompt template:**
```
You are an AI system analyst. Analyze these performance metrics and identify issues.

Agent Performance (last 24h):
{JSON of ObservationReport}

Respond in JSON: {"capability_gaps": [...], "underperformers": [...],
"success_patterns": [...], "prompt_suggestions": [...]}
```

**Plan prompt template:**
```
You are an AI system architect. Based on this diagnosis, propose evolution actions.

Diagnosis:
{JSON of Diagnosis}

Available agents: {list of agent IDs}
Available action types: create_agent, retire_agent, tune_agent, create_skill, modify_workspace, adjust_guardrails, no_action

Respond as JSON array: [{"type": "...", "agent_id": "...", "params": {...}, "reason": "..."}]
Be conservative. Only propose actions with clear evidence from the metrics.
```

- [ ] **Step 4: Run tests -- PASS**

- [ ] **Step 5: Commit**

```bash
git add pkg/evolution/engine.go pkg/evolution/engine_test.go
git commit -m "feat(evolution): implement 5-phase EvolutionEngine core"
```

---

## Chunk 7: Integration (AgentLoop, Commands, Web)

### Task 12: Wire EvolutionEngine into AgentLoop

**Files:**
- Modify: `pkg/agent/loop.go`
- Test: `pkg/agent/loop_test.go` (add smoke test)

- [ ] **Step 1: Add field, import, and initialization**

Add import: `"github.com/grasberg/sofia/pkg/evolution"`

Add `evolutionEngine *evolution.EvolutionEngine` field to `AgentLoop` struct.

In `NewAgentLoop` constructor, after existing initialization (after `branchManager` init, around line 186):

```go
// Evolution: restore dynamic agents from store
agentStore := evolution.NewAgentStore(memDB)
retiredIDs, _ := agentStore.ListRetired()
for _, id := range retiredIDs {
	_ = registry.RemoveAgent(id)
}
activeAgents, _ := agentStore.ListActive()
for _, aCfg := range activeAgents {
	// Create AgentInstance from EvolutionAgentConfig and register
	inst, err := NewAgentInstance(/* build from aCfg */)
	if err == nil {
		_ = registry.RegisterAgent(inst)
	}
}
```

Create the evolution engine (pass interfaces, not concrete types):

```go
if cfg.Evolution.Enabled {
	repMgr := reputation.NewManager(memDB)
	changelogWriter := evolution.NewChangelogWriter(memDB)
	perfTracker := evolution.NewPerformanceTracker(repMgr, &cfg.Evolution)
	safeModifier := evolution.NewSafeModifier(
		filepath.Join(filepath.Dir(memDBPath), "evolution", "history"),
		cfg.Evolution.ImmutableFiles,
		provider,
	)
	architect := evolution.NewAgentArchitect(provider, registry, a2aRouter, agentStore, memDB, workspacePath)
	al.evolutionEngine = evolution.NewEvolutionEngine(
		provider, memDB, registry, a2aRouter, al.toolTracker,
		repMgr, agentStore, changelogWriter, perfTracker, architect,
		safeModifier, &cfg.Evolution, msgBus,
	)
}
```

In `Run()` method (around line 221), after starting the main loop, start the engine:

```go
if al.evolutionEngine != nil {
	if err := al.evolutionEngine.Start(ctx); err != nil {
		logger.WarnCF("agent", "Failed to start evolution engine", map[string]any{"error": err.Error()})
	}
}
```

In `Stop()` method, add before or alongside `stopAutonomyServices()`:

```go
if al.evolutionEngine != nil {
	al.evolutionEngine.Stop()
}
```

- [ ] **Step 2: Write smoke test**

Test that `NewAgentLoop` with `Evolution.Enabled = true` doesn't panic, and `Stop()` shuts down cleanly.

- [ ] **Step 3: Verify full build**

Run: `CGO_ENABLED=0 go build -tags stdjson ./...`

- [ ] **Step 4: Run all agent tests**

Run: `CGO_ENABLED=0 go test -tags stdjson ./pkg/agent/...`

- [ ] **Step 5: Commit**

```bash
git add pkg/agent/loop.go pkg/agent/loop_test.go
git commit -m "feat(evolution): wire EvolutionEngine into AgentLoop lifecycle"
```

---

### Task 13: Add /evolve commands

**Files:**
- Modify: `pkg/agent/loop_commands.go`
- Test: `pkg/agent/loop_commands_test.go` (or add to existing)

- [ ] **Step 1: Write test for command dispatch**

```go
func TestHandleEvolveCommand_Disabled(t *testing.T) {
	// al.evolutionEngine == nil -> returns "not enabled" message
}
func TestHandleEvolveCommand_Status(t *testing.T) {
	// Mock engine with GetStatus() -> returns formatted status
}
```

- [ ] **Step 2: Add /evolve case to handleSessionCommand**

In the switch block (around line 135, after the `/search` case), add:

```go
case "/evolve":
	return al.handleEvolveCommand(args, sessionKey)
```

- [ ] **Step 3: Implement handleEvolveCommand**

```go
func (al *AgentLoop) handleEvolveCommand(args []string, _ string) (string, bool) {
	if al.evolutionEngine == nil {
		return "Evolution engine is not enabled. Set evolution.enabled=true in config.", true
	}
	if len(args) == 0 {
		return "Usage: /evolve status|history|run|pause|resume|revert <id>", true
	}
	switch args[0] {
	case "status":
		// Query engine state: lastRun, paused, active agent count, overall trend
		return al.evolutionEngine.FormatStatus(), true
	case "history":
		n := 10
		if len(args) > 1 { /* parse args[1] as int */ }
		entries, _ := al.evolutionEngine.RecentHistory(n)
		// Format entries as readable text
		return formatted, true
	case "run":
		go al.evolutionEngine.RunNow(context.Background())
		return "Evolution cycle triggered.", true
	case "pause":
		al.evolutionEngine.Pause()
		return "Evolution paused.", true
	case "resume":
		al.evolutionEngine.Resume()
		return "Evolution resumed.", true
	case "revert":
		if len(args) < 2 { return "Usage: /evolve revert <id>", true }
		if err := al.evolutionEngine.Revert(args[1]); err != nil {
			return fmt.Sprintf("Revert failed: %v", err), true
		}
		return fmt.Sprintf("Reverted action %s.", args[1]), true
	default:
		return "Unknown. Usage: /evolve status|history|run|pause|resume|revert <id>", true
	}
}
```

- [ ] **Step 4: Run tests**

Run: `CGO_ENABLED=0 go test -tags stdjson ./pkg/agent/... -run TestHandleEvolve -v`

- [ ] **Step 5: Verify full build**

Run: `CGO_ENABLED=0 go build -tags stdjson ./...`

- [ ] **Step 6: Commit**

```bash
git add pkg/agent/loop_commands.go pkg/agent/loop_commands_test.go
git commit -m "feat(evolution): add /evolve command family"
```

---

### Task 14: Add web API endpoints

**Files:**
- Modify: `pkg/web/server.go`
- Test: `pkg/web/server_test.go` (add endpoint tests)

- [ ] **Step 1: Write test**

```go
func TestEvolutionEndpoints(t *testing.T) {
	// httptest.NewServer, GET /api/evolution/status -> 200
	// GET /api/evolution/changelog?limit=5 -> 200 with JSON array
}
```

- [ ] **Step 2: Add routes and handlers**

```go
mux.HandleFunc("GET /api/evolution/status", s.handleEvolutionStatus)
mux.HandleFunc("GET /api/evolution/changelog", s.handleEvolutionChangelog)
```

`handleEvolutionStatus` -- call `evolutionEngine.FormatStatus()` or return JSON state.
`handleEvolutionChangelog` -- parse `?limit=` and `?since=` params, query changelog, return JSON.

- [ ] **Step 3: Verify build and tests**

Run: `CGO_ENABLED=0 go build -tags stdjson ./...` and `CGO_ENABLED=0 go test -tags stdjson ./pkg/web/...`

- [ ] **Step 4: Commit**

```bash
git add pkg/web/server.go pkg/web/server_test.go
git commit -m "feat(evolution): add web API for evolution status and changelog"
```

---

### Task 15: Add daily summary notification

**Files:**
- Modify: `pkg/evolution/engine.go`
- Test: `pkg/evolution/engine_test.go`

- [ ] **Step 1: Write test**

Test in `engine_test.go`:

```go
func TestDailySummary_Format(t *testing.T) {
	// Insert changelog entries, call formatDailySummary()
	// Verify output contains "Created", "Retired", "Overall" sections
}
```

- [ ] **Step 2: Add daily summary logic to runLoop**

In `runLoop`, alongside the ticker select, track `lastSummaryDate`. On each tick, check if `time.Now().Format("15:04")` matches `cfg.DailySummaryTime` and `lastSummaryDate != today`. If so, compile changelog entries from last 24h, format into readable message, publish via `bus.PublishOutbound()` to `cfg.DailySummaryChannel` + `cfg.DailySummaryChatID`.

- [ ] **Step 3: Run test -- PASS**

- [ ] **Step 4: Commit**

```bash
git add pkg/evolution/engine.go pkg/evolution/engine_test.go
git commit -m "feat(evolution): add daily summary notification"
```

---

### Task 16: Final integration test and build verification

- [ ] **Step 1: Run full build**

```bash
CGO_ENABLED=0 go build -tags stdjson ./...
```

- [ ] **Step 2: Run full test suite**

```bash
CGO_ENABLED=0 go test -tags stdjson ./...
```

- [ ] **Step 3: Run linter**

```bash
golangci-lint run
```

Fix any new warnings.

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat: complete Evolution Engine — autonomous self-improvement for Sofia"
```
