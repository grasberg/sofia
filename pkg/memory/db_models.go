package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/grasberg/sofia/pkg/config"
)

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// upsertModelTx inserts or replaces a model row inside an existing transaction.
// Only used for user-authored (non-catalog) rows today, so user_configured is
// hard-coded to 1 — the caller is SyncModels reacting to a Save from the UI.
func upsertModelTx(tx *sql.Tx, mc config.ModelConfig, isCatalog int) error {
	apiKeysJSON, _ := json.Marshal(mc.APIKeys)
	capJSON, _ := json.Marshal(mc.Capabilities)
	_, err := tx.Exec(`
		INSERT OR REPLACE INTO models
			(model_name, display_name, provider, model, api_base, api_key,
			 api_keys, pool_strategy, proxy,
			 auth_method, connect_mode, workspace, rpm, max_tokens, max_tokens_field,
			 request_timeout, request_delay, context_window,
			 cost_per_1k_input, cost_per_1k_output, capabilities, is_catalog,
			 user_configured, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, datetime('now'))`,
		mc.ModelName, mc.DisplayName, mc.Provider, mc.Model, mc.APIBase, mc.APIKey,
		string(apiKeysJSON), mc.PoolStrategy, mc.Proxy,
		mc.AuthMethod, mc.ConnectMode, mc.Workspace,
		mc.RPM, mc.MaxTokens, mc.MaxTokensField,
		mc.RequestTimeout, mc.RequestDelay, mc.ContextWindow,
		mc.CostPer1KInput, mc.CostPer1KOutput, string(capJSON),
		isCatalog,
	)
	return err
}

// scanModel reads a single row into a ModelConfig.
func scanModel(rows *sql.Rows) (config.ModelConfig, error) {
	var mc config.ModelConfig
	var apiKeysJSON, capJSON string
	if err := rows.Scan(
		&mc.ModelName, &mc.DisplayName, &mc.Provider,
		&mc.Model, &mc.APIBase, &mc.APIKey,
		&apiKeysJSON, &mc.PoolStrategy, &mc.Proxy,
		&mc.AuthMethod, &mc.ConnectMode, &mc.Workspace,
		&mc.RPM, &mc.MaxTokens, &mc.MaxTokensField,
		&mc.RequestTimeout, &mc.RequestDelay, &mc.ContextWindow,
		&mc.CostPer1KInput, &mc.CostPer1KOutput, &capJSON,
	); err != nil {
		return mc, err
	}
	if apiKeysJSON != "" && apiKeysJSON != "null" && apiKeysJSON != "[]" {
		_ = json.Unmarshal([]byte(apiKeysJSON), &mc.APIKeys)
	}
	if capJSON != "" && capJSON != "null" && capJSON != "[]" {
		_ = json.Unmarshal([]byte(capJSON), &mc.Capabilities)
	}
	return mc, nil
}

const modelColumns = `model_name, display_name, provider, model, api_base, api_key,
	api_keys, pool_strategy, proxy,
	auth_method, connect_mode, workspace, rpm, max_tokens, max_tokens_field,
	request_timeout, request_delay, context_window,
	cost_per_1k_input, cost_per_1k_output, capabilities`

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// SeedCatalogModels inserts catalog entries that don't already exist, and
// refreshes provider-level fields (model id, api_base, display_name, etc.)
// on existing catalog rows so corrections to the default catalog (e.g. a
// vendor renaming a model) propagate to existing installs.  User-set fields
// (api_key, api_keys, rpm, …) are preserved.
//
// max_tokens_field is backfill-only: new catalog values are copied in when
// the existing row still holds the empty default, but non-empty values are
// preserved so overrides made from the Settings UI (which writes the same
// column via SyncModels) survive a restart.
//
// The whole sweep runs in one transaction — the catalog has ~80 rows, which
// would otherwise be ~160 round-trips with separate fsyncs on every startup.
func (m *MemoryDB) SeedCatalogModels(models []config.ModelConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("memory: seed catalog models: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, mc := range models {
		apiKeysJSON, _ := json.Marshal(mc.APIKeys)
		capJSON, _ := json.Marshal(mc.Capabilities)
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO models
				(model_name, display_name, provider, model, api_base, api_key,
				 api_keys, pool_strategy, proxy,
				 auth_method, connect_mode, workspace, rpm, max_tokens, max_tokens_field,
				 request_timeout, request_delay, context_window,
				 cost_per_1k_input, cost_per_1k_output, capabilities, is_catalog, user_configured)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 0)`,
			mc.ModelName, mc.DisplayName, mc.Provider, mc.Model, mc.APIBase, mc.APIKey,
			string(apiKeysJSON), mc.PoolStrategy, mc.Proxy,
			mc.AuthMethod, mc.ConnectMode, mc.Workspace,
			mc.RPM, mc.MaxTokens, mc.MaxTokensField,
			mc.RequestTimeout, mc.RequestDelay, mc.ContextWindow,
			mc.CostPer1KInput, mc.CostPer1KOutput, string(capJSON),
		); err != nil {
			return fmt.Errorf("memory: seed catalog model %q: %w", mc.ModelName, err)
		}

		if _, err := tx.Exec(`
			UPDATE models SET
				display_name = ?, provider = ?, model = ?, api_base = ?,
				auth_method = ?, context_window = ?,
				cost_per_1k_input = ?, cost_per_1k_output = ?, capabilities = ?,
				max_tokens_field = CASE
					WHEN max_tokens_field = '' AND ? != '' THEN ?
					ELSE max_tokens_field
				END,
				updated_at = datetime('now')
			WHERE model_name = ? AND is_catalog = 1`,
			mc.DisplayName, mc.Provider, mc.Model, mc.APIBase,
			mc.AuthMethod, mc.ContextWindow,
			mc.CostPer1KInput, mc.CostPer1KOutput, string(capJSON),
			mc.MaxTokensField, mc.MaxTokensField,
			mc.ModelName,
		); err != nil {
			return fmt.Errorf("memory: refresh catalog model %q: %w", mc.ModelName, err)
		}
	}

	return tx.Commit()
}

// SetModelAPIKey updates the api_key for a single model.
func (m *MemoryDB) SetModelAPIKey(modelName, apiKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		`UPDATE models SET api_key = ?, updated_at = datetime('now') WHERE model_name = ?`,
		apiKey, modelName,
	)
	return err
}

// ListModels returns all model entries ordered by provider then model_name.
func (m *MemoryDB) ListModels() ([]config.ModelConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`SELECT ` + modelColumns + ` FROM models ORDER BY provider, model_name`)
	if err != nil {
		return nil, fmt.Errorf("memory: list models: %w", err)
	}
	defer rows.Close()

	var models []config.ModelConfig
	for rows.Next() {
		mc, err := scanModel(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: scan model: %w", err)
		}
		models = append(models, mc)
	}
	return models, rows.Err()
}

// ListConfiguredModels returns only models the user has explicitly enabled.
// Enablement is tracked by the user_configured flag, which SyncModels sets
// whenever the user saves a model via the Settings UI (or CLI); it's reset
// when the model is removed from the user's list.
//
// The flag exists because catalog seeding populates ~120 entries, including
// OAuth variants whose auth_method is baked in from DefaultModelList. Before
// user_configured, the OAuth-only rows passed the "configured" filter the
// moment Sofia started, cluttering the Models page with entries the user
// never touched.  Now OAuth catalog entries only appear once the user opts
// in (adds them via the UI); `sofia auth login` alone no longer auto-enables
// anything.
func (m *MemoryDB) ListConfiguredModels() ([]config.ModelConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`SELECT ` + modelColumns + ` FROM models
		WHERE user_configured = 1
		ORDER BY provider, model_name`)
	if err != nil {
		return nil, fmt.Errorf("memory: list configured models: %w", err)
	}
	defer rows.Close()

	var models []config.ModelConfig
	for rows.Next() {
		mc, err := scanModel(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: scan model: %w", err)
		}
		models = append(models, mc)
	}
	return models, rows.Err()
}

// LoadModelsIntoConfig populates cfg.ModelList with only the user-configured
// models (those with an API key or local providers like Ollama).
// The full catalog is served separately via the /api/models endpoint.
func (m *MemoryDB) LoadModelsIntoConfig(cfg *config.Config) error {
	models, err := m.ListConfiguredModels()
	if err != nil {
		return err
	}
	cfg.ModelList = models
	return nil
}

// SyncModels synchronises user-configured models with the database.
//
// Only non-catalog models are affected: those not in the incoming list are
// deleted, and all incoming entries are upserted. Catalog entries are never
// deleted by this call — their API keys are updated if present in incoming.
func (m *MemoryDB) SyncModels(incoming []config.ModelConfig, catalogNames map[string]bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("memory: sync models: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Build set of incoming model names.
	incomingSet := make(map[string]bool, len(incoming))
	for _, mc := range incoming {
		incomingSet[mc.ModelName] = true
	}

	// Delete non-catalog models not present in the incoming list.
	rows, err := tx.Query(`SELECT model_name FROM models WHERE is_catalog = 0`)
	if err != nil {
		return fmt.Errorf("memory: sync models: query: %w", err)
	}
	var toDelete []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		if !incomingSet[name] {
			toDelete = append(toDelete, name)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, name := range toDelete {
		if _, err := tx.Exec(`DELETE FROM models WHERE model_name = ?`, name); err != nil {
			return fmt.Errorf("memory: sync models: delete %q: %w", name, err)
		}
	}

	// Catalog rows are permanent, but when a configured catalog model is
	// removed from the user's list, clear its user-editable fields and flip
	// user_configured back to 0 so it drops out of ListConfiguredModels.
	// We key off user_configured (not api_key) so OAuth catalog entries —
	// which the user enables without ever supplying an api_key — get
	// unconfigured too when they're removed from the list.
	catRows, err := tx.Query(`SELECT model_name FROM models WHERE is_catalog = 1 AND user_configured = 1`)
	if err != nil {
		return fmt.Errorf("memory: sync models: query catalog: %w", err)
	}
	var toUnconfigure []string
	for catRows.Next() {
		var name string
		if err := catRows.Scan(&name); err != nil {
			catRows.Close()
			return err
		}
		if !incomingSet[name] {
			toUnconfigure = append(toUnconfigure, name)
		}
	}
	catRows.Close()
	if err := catRows.Err(); err != nil {
		return err
	}

	for _, name := range toUnconfigure {
		if _, err := tx.Exec(`
			UPDATE models SET
				api_key = '', api_keys = '[]', pool_strategy = '', proxy = '',
				rpm = 0, max_tokens = 0, max_tokens_field = '',
				request_timeout = 0, request_delay = 0, workspace = '',
				connect_mode = '', user_configured = 0,
				updated_at = datetime('now')
			WHERE model_name = ?`, name); err != nil {
			return fmt.Errorf("memory: sync models: unconfigure catalog %q: %w", name, err)
		}
	}

	// Upsert incoming models.  For catalog entries only update user-editable
	// fields (api_key etc.); for non-catalog do a full upsert.
	for _, mc := range incoming {
		// Check if this is a catalog entry.
		var isCatalog int
		_ = tx.QueryRow(`SELECT is_catalog FROM models WHERE model_name = ?`, mc.ModelName).Scan(&isCatalog)
		if isCatalog == 1 {
			// Update only the API key and user-adjustable fields.  The row
			// appears in the incoming list because the user clicked Save on
			// it, so flip user_configured to 1 — even OAuth entries that
			// leave api_key empty must now be recognised as configured.
			apiKeysJSON, _ := json.Marshal(mc.APIKeys)
			_, err := tx.Exec(`
				UPDATE models SET
					api_key = ?, api_keys = ?, pool_strategy = ?, proxy = ?,
					rpm = ?, max_tokens = ?, max_tokens_field = ?,
					request_timeout = ?, request_delay = ?, workspace = ?,
					connect_mode = ?, user_configured = 1,
					updated_at = datetime('now')
				WHERE model_name = ?`,
				mc.APIKey, string(apiKeysJSON), mc.PoolStrategy, mc.Proxy,
				mc.RPM, mc.MaxTokens, mc.MaxTokensField,
				mc.RequestTimeout, mc.RequestDelay, mc.Workspace,
				mc.ConnectMode,
				mc.ModelName,
			)
			if err != nil {
				return fmt.Errorf("memory: sync models: update catalog %q: %w", mc.ModelName, err)
			}
		} else {
			if err := upsertModelTx(tx, mc, 0); err != nil {
				return fmt.Errorf("memory: sync models: upsert %q: %w", mc.ModelName, err)
			}
		}
	}

	return tx.Commit()
}

// InitModels seeds the catalog and loads models into cfg.
//
// On each startup, new catalog entries from DefaultModelList() are inserted
// (existing rows are never overwritten, preserving user-set API keys).
// Only models with an API key (or local providers) are loaded into
// cfg.ModelList for use by the agent.  In-memory-only entries (e.g. test
// configs) are preserved.
func (m *MemoryDB) InitModels(cfg *config.Config) error {
	// Seed catalog — INSERT OR IGNORE keeps existing rows intact.
	catalog := config.DefaultModelList()
	if err := m.SeedCatalogModels(catalog); err != nil {
		return err
	}

	// Load only configured models (those with API keys) into cfg.
	configured, err := m.ListConfiguredModels()
	if err != nil {
		return err
	}

	// Preserve any cfg.ModelList entries absent from the DB (e.g. test configs
	// that set ModelList directly without going through DB persistence).
	dbNames := make(map[string]bool, len(configured))
	for _, mc := range configured {
		dbNames[mc.ModelName] = true
	}
	result := configured
	for _, mc := range cfg.ModelList {
		if !dbNames[mc.ModelName] {
			result = append(result, mc)
		}
	}
	cfg.ModelList = result
	return nil
}
