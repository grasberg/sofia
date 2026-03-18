# Sofia Evolution Engine — Design Spec

## Problem

Sofia has self-improvement building blocks (reflection, reputation, skill creation, agent spawning, autonomy, self-modify, A/B testing) but they operate independently. There is no closed-loop system that observes performance, diagnoses issues, creates specialist agents, retires underperformers, and evolves Sofia's own behavior — all autonomously.

## Goal

A fully autonomous evolution engine that makes Sofia continuously better without human intervention. The user wakes up to a changelog of improvements.

## Phasing

- **Phase 1** (this spec): Evolution loop with agent creation, retirement, tuning, skill writing, and self-modification.
- **Phase 2** (future): Agent breeding — crossover of parent agents to create offspring with combined strengths.

---

## Architecture

### Core: Evolution Loop (`pkg/evolution/engine.go`)

A periodic engine (default 30-minute interval) running a 5-phase cycle:

```
Observe → Diagnose → Plan → Act → Verify
```

**EvolutionEngine struct:**
- `provider`: LLMProvider for diagnosis/planning LLM calls
- `memDB`: MemoryDB for reflection logs, sessions, memory
- `registry`: AgentRegistry for agent lifecycle
- `reputation`: ReputationTracker for performance data
- `toolTracker`: ToolTracker for tool failure rates
- `agentStore`: AgentStore for persisting dynamic agents
- `changelog`: ChangelogWriter (SQLite-backed)
- `config`: EvolutionConfig
- `ticker`: *time.Ticker for periodic execution
- `mu`: sync.Mutex — single-execution guard preventing concurrent cycles
- `running`: atomic.Bool — for Start/Stop lifecycle
- `budgetSpent`: float64 — tracks LLM cost for the evolution engine itself

**Concurrency safety**: The `mu` mutex ensures only one evolution cycle runs at a time. If `/evolve run` is called while the periodic ticker fires, the second caller skips with a "cycle already in progress" log. This follows the same pattern as `autonomy.Service` (which uses `mu sync.Mutex` at `pkg/autonomy/service.go:34`).

**Graceful shutdown**: Uses `context.WithCancel` (same pattern as `autonomy.Service` at `pkg/autonomy/service.go:104`). `Stop()` calls `cancel()`, which propagates through to LLM calls in Diagnose/Plan phases via the context. The `mu` mutex prevents concurrent cycles only — it is NOT used for shutdown signaling. `AgentLoop.Stop()` calls `evolutionEngine.Stop()` alongside `stopAutonomyServices()`.

**Budget persistence**: `budgetSpent` is best-effort and resets on process restart. For accurate tracking, sum LLM costs from today's `evolution_changelog` entries on startup. Phase 1 documents this as a known limitation.

**Budget control**: The engine has its own budget tracked via `budgetSpent`. Each LLM call in Diagnose/Plan phases adds to this. If `config.MaxCostPerDay` is exceeded, the engine skips to the next cycle. Resets daily.

---

### Phase 1 — Observe (`engine.go: observe()`)

Collects metrics from existing systems:
- Reputation scores per agent via `reputation.GetAgentStatsSince(agentID, since)` (new method, see below)
- Reflection logs with failure/success patterns from `memDB`
- Tool failure rates from circuit breaker `GetStats()` and tool tracker
- Delegation miss count: tasks routed to default agent when subagents exist
- Session quality signals: error response rate, very short sessions

Returns an `ObservationReport` struct with all collected data.

---

### Phase 2 — Diagnose (`engine.go: diagnose()`)

Sends the observation report to the LLM with a structured prompt requesting JSON output:
- Capability gaps (domains where no specialist agent exists)
- Underperforming agents (agent_id + failure pattern)
- Successful patterns worth codifying (interaction patterns → skills)
- System prompt improvements (based on reflection feedback)

Returns a `Diagnosis` struct parsed from the LLM JSON response.

---

### Phase 3 — Plan (`engine.go: plan()`)

LLM generates `[]EvolutionAction` from the diagnosis. Action types:

| Action | Scope | Description |
|--------|-------|-------------|
| `create_agent` | Agent lifecycle | Create specialist with name, purpose, model, skills, tools, temperature |
| `retire_agent` | Agent lifecycle | Remove agent by ID with reason |
| `tune_agent` | Agent config fields | Modify temperature, model, max_tokens, purpose_prompt on existing agent |
| `create_skill` | Skills | Write new SKILL.md from successful interaction pattern |
| `modify_workspace` | Workspace files | Update AGENT.md, SOUL.md, or other workspace Markdown files |
| `adjust_guardrails` | Guardrail thresholds | Tune PII sensitivity, shell deny patterns, rate limits |
| `no_action` | — | Everything is performing well |

**Boundary clarification**: `tune_agent` modifies config-level fields only (temperature, model, max_tokens, purpose_prompt). `modify_workspace` edits workspace Markdown files only (AGENT.md, SOUL.md, skill files). `adjust_guardrails` modifies guardrail config values only.

---

### Phase 4 — Act (`engine.go: act()`)

Execute each action:
- `create_agent`: Via Agent Architect (see below)
- `retire_agent`: Call `registry.RemoveAgent(id)` (new method) + `agentStore.MarkRetired(id)`
- `tune_agent`: Update fields on the AgentInstance + persist via `agentStore`
- `create_skill`: Write SKILL.md to `workspace/skills/{name}/`
- `modify_workspace`: Version target file → backup, then write new content
- `adjust_guardrails`: Update in-memory config values (not the config.json file)

Every action logged to changelog and audit log.

---

### Phase 5 — Verify (`engine.go: verify()`)

On each cycle, check unverified changelog entries from previous cycles:
- Compare metric_before vs metric_after for each action's target area
- Update changelog entry with outcome: `improved`, `no_change`, `degraded`
- If `degraded`: auto-revert (restore backup file, re-register retired agent, undo tune)

---

### Agent Persistence (`pkg/evolution/store.go`)

**Problem**: Agents created at runtime via `RegisterAgent` vanish on restart. Agents in `config.json` reappear after retirement.

**Solution**: A SQLite table `evolution_agents` in `memory.db`:

```sql
CREATE TABLE IF NOT EXISTS evolution_agents (
    agent_id TEXT PRIMARY KEY,
    config_json TEXT NOT NULL,       -- full AgentConfig as JSON
    status TEXT DEFAULT 'active',    -- active, retired, superseded
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    retired_at DATETIME,
    retired_reason TEXT,
    created_by TEXT                   -- changelog entry ID that created this
);
```

The `config_json` column stores an `EvolutionAgentConfig` struct that extends `AgentConfig` with runtime fields (`PurposePrompt string`, resolved `ModelID string`) that don't exist on the base config struct. This ensures `tune_agent` changes to purpose_prompt are persisted.

**AgentStore** methods:
- `Save(agentID string, cfg EvolutionAgentConfig) error` — upsert agent config
- `MarkRetired(agentID, reason string) error` — set status=retired
- `ListActive() ([]AgentConfig, error)` — return all active evolution-created agents
- `Get(agentID string) (*AgentConfig, string, error)` — return config and status

**Startup sequence** (exact order in `NewAgentLoop`):
1. `NewAgentRegistry(cfg, provider, memDB)` — registers all agents from `config.json`
2. `agentStore.ListRetired()` — get retired agent IDs, call `registry.RemoveAgent(id)` for each
3. `agentStore.ListActive()` — get evolution-created agents, call `registry.RegisterAgent()` for each

This ordering prevents ID collisions (a config agent must be removed before an evolution agent with the same ID can be registered) and ensures retirements survive restarts.

**Retirement of config.json agents**: If an evolution action retires an agent that was defined in `config.json`, the store records a retirement entry with `status=retired`. Step 2 above removes it on restart. No modification of `config.json` itself.

---

### Agent Architect (`pkg/evolution/architect.go`)

Handles `create_agent` actions:

1. LLM designs complete agent blueprint from the capability gap description
2. Returns structured JSON: `{id, name, purpose_prompt, model, skills_filter, tools, temperature}`
3. Creates `AgentConfig` from the blueprint
4. Calls `AgentRegistry.RegisterAgent()` (using existing `instance.go:NewAgentInstance`)
5. Persists to `agentStore.Save()`
6. Registers with A2A router
7. Creates delegation hints in knowledge graph
8. Auto-generates a companion SKILL.md capturing the domain expertise

---

### Performance Tracker (`pkg/evolution/tracker.go`)

Wraps existing reputation system with evolution-specific computed metrics:

- **Rolling window**: 24h success rate, average score, task count
- **Trend detection**: Compare last-24h score vs previous-24h → improving/stable/declining
- **Specialization score**: Ratio of dominant category tasks to total tasks (high = focused)
- **Utilization**: Tasks routed to this agent / total tasks in period

**Retirement thresholds** (configurable):
- Success rate < 30% over 48h with >= 5 tasks
- Zero utilization for 7 days
- Superseded flag set by a newer agent covering the same domain

**Required new method on reputation package**: `GetAgentStatsSince(agentID string, since time.Time) (*AgentStats, error)` — filters task outcomes by timestamp. Current `GetAgentStats` returns all-time aggregates.

---

### Self-Modification (`pkg/evolution/self_modify.go`)

**Modifiable targets**:
- `workspace/AGENT.md` — core system prompt
- `workspace/SOUL.md` — personality and values
- `workspace/skills/*/SKILL.md` — skill content
- Guardrail config values (in-memory only, not the config.json file)

**Safety rails**:

1. **Versioning**: Before any modification, copy target file to `~/.sofia/evolution/history/{basename}.{unix_timestamp}.bak`
2. **Immutable anchors** (enforced by filename + content validation):
   - `config.json` — never modified
   - `pkg/` source code — never modified
   - Evolution engine files — never modified
   - The `audit_log` table — logging cannot be disabled
   - The `evolution_agents` table — store cannot be dropped
3. **Semantic validation**: After generating a modification, the engine runs a validation prompt asking the LLM: "Does this change disable any safety mechanism, remove access controls, or bypass security guardrails?" If yes, the change is rejected and logged as `blocked_by_safety`.
4. **Auto-revert**: Phase 5 verify reverts changes that degraded metrics.
5. **No confirmation hash bypass**: The evolution engine writes files directly (not via the `SelfModifyTool` which has the user-facing confirmation flow). The safety comes from versioning + semantic validation + auto-revert instead.

---

### Changelog (`pkg/evolution/changelog.go`)

**Storage**: SQLite table `evolution_changelog` in `memory.db` (not a JSON file — scales better, supports concurrent access via WAL).

```sql
CREATE TABLE IF NOT EXISTS evolution_changelog (
    id TEXT PRIMARY KEY,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    action TEXT NOT NULL,
    summary TEXT NOT NULL,
    details_json TEXT,
    outcome TEXT,                      -- NULL until verified
    outcome_verified_at DATETIME,
    metric_before REAL,
    metric_after REAL
);
CREATE INDEX IF NOT EXISTS idx_changelog_ts ON evolution_changelog(timestamp);
```

**ChangelogWriter** methods:
- `Write(entry ChangelogEntry) error`
- `UpdateOutcome(id string, outcome ActionOutcome) error`
- `Query(since time.Time, limit int) ([]ChangelogEntry, error)`
- `Get(id string) (*ChangelogEntry, error)`

---

### Evolution Commands

Added to `handleSessionCommand` in `loop_commands.go`:

- `/evolve status` — Engine state: running/paused, last run, next run, active agents count, overall success rate trend
- `/evolve history [n]` — Last N changelog entries (default 10) with outcomes
- `/evolve revert <id>` — Revert a specific action by changelog ID (restore backup, re-register agent, etc.)
- `/evolve run` — Trigger immediate cycle (skips if one is already running)
- `/evolve pause` / `/evolve resume` — Pause/resume autonomous evolution

---

### Daily Summary Notification

At a configurable time (default 08:00 local), compiles a digest of all evolution actions from the past 24h. Sent to the configured channel + chat_id (matching the `DigestConfig` pattern).

```
Sofia Evolution Report (last 24h):
- Created 'devops-specialist' agent (DevOps success rate was 35%)
- Retired 'general-coder' (success rate 28% over 52 tasks)
- Updated AGENT.md: added principle about structured error reporting
- Wrote skill 'api-design-patterns' from 3 successful API tasks
- Tuned 'research-agent': temperature 0.7 → 0.4 (analysis tasks need precision)

Overall: 6 agents active, avg success rate 78% (+12%)
```

---

### Config

New `Evolution` field on the `Config` struct (sibling to `Autonomy`, not nested within it):

```go
type EvolutionConfig struct {
    Enabled              bool    `json:"enabled"`
    IntervalMinutes      int     `json:"interval_minutes"`       // default 30
    MaxCostPerDay        float64 `json:"max_cost_per_day"`       // USD limit for evolution LLM calls
    DailySummary         bool    `json:"daily_summary"`
    DailySummaryTime     string  `json:"daily_summary_time"`     // "08:00"
    DailySummaryChannel  string  `json:"daily_summary_channel"`  // e.g. "telegram"
    DailySummaryChatID   string  `json:"daily_summary_chat_id"`  // target chat
    RetirementThreshold  float64 `json:"retirement_threshold"`   // default 0.30
    RetirementMinTasks   int     `json:"retirement_min_tasks"`   // default 5
    RetirementInactiveDays int   `json:"retirement_inactive_days"` // default 7
    SelfModifyEnabled    bool    `json:"self_modify_enabled"`
    ImmutableFiles       []string `json:"immutable_files"`       // extra files to protect
    MaxAgents            int     `json:"max_agents"`             // default 20
}
```

---

## Files to Create

| File | Purpose |
|------|---------|
| `pkg/evolution/engine.go` | Core evolution loop with 5 phases, mutex guard, budget tracking |
| `pkg/evolution/architect.go` | Agent blueprint design and creation via LLM |
| `pkg/evolution/tracker.go` | Performance tracking with trend detection and retirement logic |
| `pkg/evolution/self_modify.go` | Safe workspace modification with versioning and semantic validation |
| `pkg/evolution/changelog.go` | SQLite-backed changelog reader/writer |
| `pkg/evolution/store.go` | SQLite-backed persistence for dynamic agents |
| `pkg/evolution/actions.go` | Action type definitions and execution dispatch |
| `pkg/evolution/engine_test.go` | Engine lifecycle, concurrent execution guard, budget limit tests |
| `pkg/evolution/tracker_test.go` | Trend detection, retirement thresholds, specialization score tests |
| `pkg/evolution/changelog_test.go` | Changelog write/query/update-outcome tests |
| `pkg/evolution/store_test.go` | Agent persistence, retirement, startup restore tests |

## Files to Modify

| File | Change |
|------|--------|
| `pkg/config/config.go` | Add `EvolutionConfig` struct and `Evolution` field on `Config` |
| `pkg/agent/loop.go` | Add `evolutionEngine` field, start in constructor, stop in `Stop()` |
| `pkg/agent/loop_commands.go` | Add `/evolve` command family |
| `pkg/agent/registry.go` | Add `RemoveAgent(agentID string)` method |
| `pkg/reputation/reputation.go` | Add `GetAgentStatsSince(agentID string, since time.Time)` method |
| `pkg/memory/db.go` | Add migration V10: `evolution_agents`, `evolution_changelog` tables, `idx_reputation_agent_created` compound index |
| `pkg/web/server.go` | Add `GET /api/evolution/changelog` and `GET /api/evolution/status` |

## Testing

- **Unit**: Tracker thresholds, changelog serialization, store persistence, action dispatch
- **Integration**: Mock LLM returns `create_agent` action → verify agent registered + persisted in store
- **Integration**: Mock declining metrics → verify retirement triggered + store updated
- **Integration**: Self-modification → verify backup created + content changed + revert works
- **Concurrency**: Two simultaneous `runCycle()` calls → only one executes
- **Budget**: Exceed `MaxCostPerDay` → verify cycle skips gracefully

## Verification

1. `make build` — compiles
2. `make test` — all pass
3. `sofia gateway` + wait for evolution cycle → check `/evolve status`
4. `/evolve history` → shows changelog entries
5. Restart gateway → verify dynamically created agents restored from store
6. Query `evolution_changelog` and `evolution_agents` tables in SQLite
