package memory

import (
	"database/sql"
)

// ---------------------------------------------------------------------------
// Schema migrations v2–v14
// ---------------------------------------------------------------------------

func (m *MemoryDB) applyV2tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS semantic_nodes (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id      TEXT    NOT NULL DEFAULT '',
    label         TEXT    NOT NULL DEFAULT '',
    name          TEXT    NOT NULL DEFAULT '',
    properties    TEXT    NOT NULL DEFAULT '{}',
    access_count  INTEGER NOT NULL DEFAULT 0,
    last_accessed DATETIME,
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_semantic_nodes_key ON semantic_nodes(agent_id, label, name);
CREATE INDEX IF NOT EXISTS idx_semantic_nodes_agent ON semantic_nodes(agent_id);

CREATE TABLE IF NOT EXISTS semantic_edges (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id    TEXT    NOT NULL DEFAULT '',
    source_id   INTEGER NOT NULL REFERENCES semantic_nodes(id) ON DELETE CASCADE,
    target_id   INTEGER NOT NULL REFERENCES semantic_nodes(id) ON DELETE CASCADE,
    relation    TEXT    NOT NULL DEFAULT '',
    weight      REAL    NOT NULL DEFAULT 1.0,
    properties  TEXT    NOT NULL DEFAULT '{}',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_semantic_edges_key ON semantic_edges(agent_id, source_id, target_id, relation);
CREATE INDEX IF NOT EXISTS idx_semantic_edges_source ON semantic_edges(source_id);
CREATE INDEX IF NOT EXISTS idx_semantic_edges_target ON semantic_edges(target_id);

CREATE TABLE IF NOT EXISTS memory_stats (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id    TEXT    NOT NULL DEFAULT '',
    event_type  TEXT    NOT NULL DEFAULT '',
    node_id     INTEGER,
    details     TEXT    NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_memory_stats_agent ON memory_stats(agent_id, event_type);
`
	_, err := tx.Exec(ddl)
	return err
}

func (m *MemoryDB) applyV3tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS reflections (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id     TEXT NOT NULL DEFAULT '',
    session_key  TEXT NOT NULL DEFAULT '',
    task_summary TEXT NOT NULL DEFAULT '',
    what_worked  TEXT NOT NULL DEFAULT '',
    what_failed  TEXT NOT NULL DEFAULT '',
    lessons      TEXT NOT NULL DEFAULT '',
    score        REAL NOT NULL DEFAULT 0.0,
    tool_count   INTEGER NOT NULL DEFAULT 0,
    error_count  INTEGER NOT NULL DEFAULT 0,
    duration_ms  INTEGER NOT NULL DEFAULT 0,
    created_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_reflections_agent ON reflections(agent_id, created_at);
`
	_, err := tx.Exec(ddl)
	return err
}

func (m *MemoryDB) applyV4tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS plan_templates (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT    NOT NULL UNIQUE,
    goal         TEXT    NOT NULL DEFAULT '',
    steps        TEXT    NOT NULL DEFAULT '[]',
    tags         TEXT    NOT NULL DEFAULT '',
    use_count    INTEGER NOT NULL DEFAULT 0,
    success_rate REAL    NOT NULL DEFAULT 0.0,
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_plan_templates_name ON plan_templates(name);
`
	_, err := tx.Exec(ddl)
	return err
}

func (m *MemoryDB) applyV5tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS checkpoints (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_key TEXT    NOT NULL REFERENCES sessions(key) ON DELETE CASCADE,
    agent_id    TEXT    NOT NULL DEFAULT '',
    name        TEXT    NOT NULL DEFAULT '',
    iteration   INTEGER NOT NULL DEFAULT 0,
    msg_count   INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_checkpoints_session ON checkpoints(session_key, created_at);
`
	_, err := tx.Exec(ddl)
	return err
}

func (m *MemoryDB) applyV6tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS ab_experiments (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL UNIQUE,
    description   TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'active',
    winner        TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    concluded_at  DATETIME
);

CREATE TABLE IF NOT EXISTS ab_variants (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    experiment_id  INTEGER NOT NULL
        REFERENCES ab_experiments(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    config         TEXT NOT NULL DEFAULT '{}',
    UNIQUE(experiment_id, name)
);

CREATE TABLE IF NOT EXISTS ab_trials (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    experiment_id  INTEGER NOT NULL
        REFERENCES ab_experiments(id) ON DELETE CASCADE,
    variant_id     INTEGER NOT NULL
        REFERENCES ab_variants(id) ON DELETE CASCADE,
    prompt         TEXT NOT NULL,
    response       TEXT NOT NULL DEFAULT '',
    score          REAL,
    latency_ms     INTEGER NOT NULL DEFAULT 0,
    tokens_in      INTEGER NOT NULL DEFAULT 0,
    tokens_out     INTEGER NOT NULL DEFAULT 0,
    error          TEXT NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_ab_trials_experiment
    ON ab_trials(experiment_id);
CREATE INDEX IF NOT EXISTS idx_ab_trials_variant
    ON ab_trials(variant_id);
`
	_, err := tx.Exec(ddl)
	return err
}

func (m *MemoryDB) applyV7tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS dynamic_tools (
    name        TEXT PRIMARY KEY,
    definition  TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
`
	_, err := tx.Exec(ddl)
	return err
}

func (m *MemoryDB) applyV8tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS agent_reputation (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id   TEXT    NOT NULL,
    category   TEXT    NOT NULL DEFAULT 'general',
    task       TEXT    NOT NULL,
    success    INTEGER NOT NULL DEFAULT 0,
    score      REAL,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    tokens_in  INTEGER NOT NULL DEFAULT 0,
    tokens_out INTEGER NOT NULL DEFAULT 0,
    error      TEXT    NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_reputation_agent
    ON agent_reputation(agent_id);
CREATE INDEX IF NOT EXISTS idx_reputation_agent_category
    ON agent_reputation(agent_id, category);
CREATE INDEX IF NOT EXISTS idx_reputation_created
    ON agent_reputation(created_at DESC);
`
	_, err := tx.Exec(ddl)
	return err
}

// applyV9 adds the tool_name column to messages (ALTER TABLE, outside transaction).
func (m *MemoryDB) applyV9() error {
	if m.columnExists("messages", "tool_name") {
		return nil
	}
	_, err := m.db.Exec(`ALTER TABLE messages ADD COLUMN tool_name TEXT NOT NULL DEFAULT ''`)
	return err
}

func (m *MemoryDB) applyV10tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS evolution_agents (
    id          TEXT PRIMARY KEY,
    agent_id    TEXT NOT NULL,
    parent_id   TEXT NOT NULL DEFAULT '',
    reason      TEXT NOT NULL DEFAULT '',
    config_json TEXT NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    retired_at  DATETIME
);
CREATE INDEX IF NOT EXISTS idx_evolution_agents_status ON evolution_agents(status);

CREATE TABLE IF NOT EXISTS evolution_changelog (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id   TEXT NOT NULL DEFAULT '',
    action     TEXT NOT NULL,
    detail     TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_evolution_changelog_agent ON evolution_changelog(agent_id, created_at);

CREATE INDEX IF NOT EXISTS idx_reputation_agent_created ON agent_reputation(agent_id, created_at);
`
	_, err := tx.Exec(ddl)
	return err
}

func (m *MemoryDB) applyV11tx(tx *sql.Tx) error {
	const ddl = `ALTER TABLE semantic_nodes ADD COLUMN quality_score REAL NOT NULL DEFAULT 0.5`
	_, err := tx.Exec(ddl)
	return err
}

func (m *MemoryDB) applyV12() error {
	if m.columnExists("checkpoints", "summary") {
		return nil
	}
	_, err := m.db.Exec(`ALTER TABLE checkpoints ADD COLUMN summary TEXT NOT NULL DEFAULT ''`)
	return err
}

func (m *MemoryDB) applyV13tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS execution_traces (
    id          TEXT PRIMARY KEY,
    trace_id    TEXT NOT NULL,
    parent_id   TEXT NOT NULL DEFAULT '',
    kind        TEXT NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    agent_id    TEXT NOT NULL DEFAULT '',
    session_key TEXT NOT NULL DEFAULT '',
    start_time  DATETIME NOT NULL,
    end_time    DATETIME,
    status      TEXT NOT NULL DEFAULT 'running',
    attributes  TEXT NOT NULL DEFAULT '{}',
    scores      TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_traces_trace_id ON execution_traces(trace_id);
CREATE INDEX IF NOT EXISTS idx_traces_agent ON execution_traces(agent_id, start_time);
CREATE INDEX IF NOT EXISTS idx_traces_kind ON execution_traces(kind, start_time);
`
	_, err := tx.Exec(ddl)
	return err
}

func (m *MemoryDB) applyV14tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS goal_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    goal_id    INTEGER NOT NULL,
    agent_id   TEXT    NOT NULL DEFAULT '',
    step       TEXT    NOT NULL DEFAULT '',
    result     TEXT    NOT NULL DEFAULT '',
    success    INTEGER NOT NULL DEFAULT 1,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_goal_log_goal ON goal_log(goal_id, created_at);
`
	_, err := tx.Exec(ddl)
	return err
}

func (m *MemoryDB) applyV15tx(tx *sql.Tx) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS models (
    model_name        TEXT PRIMARY KEY,
    model             TEXT    NOT NULL DEFAULT '',
    api_base          TEXT    NOT NULL DEFAULT '',
    api_key           TEXT    NOT NULL DEFAULT '',
    api_keys          TEXT    NOT NULL DEFAULT '[]',
    pool_strategy     TEXT    NOT NULL DEFAULT '',
    proxy             TEXT    NOT NULL DEFAULT '',
    auth_method       TEXT    NOT NULL DEFAULT '',
    connect_mode      TEXT    NOT NULL DEFAULT '',
    workspace         TEXT    NOT NULL DEFAULT '',
    rpm               INTEGER NOT NULL DEFAULT 0,
    max_tokens        INTEGER NOT NULL DEFAULT 0,
    max_tokens_field  TEXT    NOT NULL DEFAULT '',
    request_timeout   INTEGER NOT NULL DEFAULT 0,
    request_delay     INTEGER NOT NULL DEFAULT 0,
    context_window    INTEGER NOT NULL DEFAULT 0,
    cost_per_1k_input  REAL   NOT NULL DEFAULT 0,
    cost_per_1k_output REAL   NOT NULL DEFAULT 0,
    capabilities      TEXT    NOT NULL DEFAULT '[]',
    is_catalog        INTEGER NOT NULL DEFAULT 0,
    created_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at        DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_models_catalog ON models(is_catalog);
`
	_, err := tx.Exec(ddl)
	return err
}

// v16: Add provider/display_name columns and clear old catalog data so it can
// be re-seeded with the new fields.  Models the user already configured (those
// with an API key) are kept.
func (m *MemoryDB) applyV16tx(tx *sql.Tx) error {
	stmts := []string{
		`ALTER TABLE models ADD COLUMN provider     TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE models ADD COLUMN display_name TEXT NOT NULL DEFAULT ''`,
		`DELETE FROM models WHERE api_key = ''`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

// v17: Fix NVIDIA-hosted model identifiers that were stored without the
// "nvidia/" protocol prefix.  The original catalog shipped NVIDIA entries with
// Model values like "meta/llama-3.1-8b-instruct", which the provider factory
// parsed as protocol="meta" (unknown) and refused to build a provider for —
// users saw "No model is configured" despite a valid API key.  Any row whose
// api_base points at integrate.api.nvidia.com gets its model prefixed with
// "nvidia/" so CreateProviderFromConfig routes it through the NVIDIA branch.
func (m *MemoryDB) applyV17tx(tx *sql.Tx) error {
	_, err := tx.Exec(`
		UPDATE models
		SET model = 'nvidia/' || model,
		    updated_at = datetime('now')
		WHERE api_base LIKE '%integrate.api.nvidia.com%'
		  AND model != ''
		  AND model NOT LIKE 'nvidia/%'`)
	return err
}

// v18: Add an explicit user_configured flag so "the AI Models settings page
// only shows what the user actually configured" stops lumping seeded OAuth
// catalog entries in with real user configuration.  Before this, the catalog
// shipped 10 rows (7 OpenAI ChatGPT OAuth + 3 Qwen OAuth Free) with
// auth_method pre-set, and ListConfiguredModels treated auth_method != ''
// as "configured" — those rows appeared on the settings page after every
// fresh install even though the user had done nothing.
//
// Backfill keeps existing user state intact:
//   - user-authored (is_catalog = 0) rows: always user_configured = 1
//   - catalog rows with a stored api_key or api_keys pool: user_configured = 1
//   - catalog rows with ONLY auth_method set (the false positives above):
//     user_configured = 0 — the user re-enables them explicitly via the UI.
func (m *MemoryDB) applyV18tx(tx *sql.Tx) error {
	stmts := []string{
		`ALTER TABLE models ADD COLUMN user_configured INTEGER NOT NULL DEFAULT 0`,
		`UPDATE models SET user_configured = 1
		   WHERE is_catalog = 0
		      OR api_key != ''
		      OR (api_keys != '' AND api_keys != '[]' AND api_keys != 'null')`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
