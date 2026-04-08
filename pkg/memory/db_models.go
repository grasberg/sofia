package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/grasberg/sofia/pkg/config"
)

// SeedCatalogModels inserts built-in catalog entries that don't already exist.
// Existing rows are never overwritten, so user API keys are preserved.
func (m *MemoryDB) SeedCatalogModels(models []config.ModelConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, mc := range models {
		apiKeysJSON, _ := json.Marshal(mc.APIKeys)
		capJSON, _ := json.Marshal(mc.Capabilities)
		_, err := m.db.Exec(`
			INSERT OR IGNORE INTO models
				(model_name, model, api_base, api_key, api_keys, pool_strategy, proxy,
				 auth_method, connect_mode, workspace, rpm, max_tokens, max_tokens_field,
				 request_timeout, request_delay, context_window,
				 cost_per_1k_input, cost_per_1k_output, capabilities, is_catalog)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
			mc.ModelName, mc.Model, mc.APIBase, mc.APIKey,
			string(apiKeysJSON), mc.PoolStrategy, mc.Proxy,
			mc.AuthMethod, mc.ConnectMode, mc.Workspace,
			mc.RPM, mc.MaxTokens, mc.MaxTokensField,
			mc.RequestTimeout, mc.RequestDelay, mc.ContextWindow,
			mc.CostPer1KInput, mc.CostPer1KOutput, string(capJSON),
		)
		if err != nil {
			return fmt.Errorf("memory: seed catalog model %q: %w", mc.ModelName, err)
		}
	}
	return nil
}

// upsertModelTx inserts or replaces a model row inside an existing transaction.
func upsertModelTx(tx *sql.Tx, mc config.ModelConfig, isCatalog int) error {
	apiKeysJSON, _ := json.Marshal(mc.APIKeys)
	capJSON, _ := json.Marshal(mc.Capabilities)
	_, err := tx.Exec(`
		INSERT OR REPLACE INTO models
			(model_name, model, api_base, api_key, api_keys, pool_strategy, proxy,
			 auth_method, connect_mode, workspace, rpm, max_tokens, max_tokens_field,
			 request_timeout, request_delay, context_window,
			 cost_per_1k_input, cost_per_1k_output, capabilities, is_catalog,
			 updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		mc.ModelName, mc.Model, mc.APIBase, mc.APIKey,
		string(apiKeysJSON), mc.PoolStrategy, mc.Proxy,
		mc.AuthMethod, mc.ConnectMode, mc.Workspace,
		mc.RPM, mc.MaxTokens, mc.MaxTokensField,
		mc.RequestTimeout, mc.RequestDelay, mc.ContextWindow,
		mc.CostPer1KInput, mc.CostPer1KOutput, string(capJSON),
		isCatalog,
	)
	return err
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

// ListModels returns all model entries ordered by model_name.
func (m *MemoryDB) ListModels() ([]config.ModelConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`
		SELECT model_name, model, api_base, api_key, api_keys, pool_strategy, proxy,
		       auth_method, connect_mode, workspace, rpm, max_tokens, max_tokens_field,
		       request_timeout, request_delay, context_window,
		       cost_per_1k_input, cost_per_1k_output, capabilities
		FROM models
		ORDER BY model_name`)
	if err != nil {
		return nil, fmt.Errorf("memory: list models: %w", err)
	}
	defer rows.Close()

	var models []config.ModelConfig
	for rows.Next() {
		var mc config.ModelConfig
		var apiKeysJSON, capJSON string
		if err := rows.Scan(
			&mc.ModelName, &mc.Model, &mc.APIBase, &mc.APIKey,
			&apiKeysJSON, &mc.PoolStrategy, &mc.Proxy,
			&mc.AuthMethod, &mc.ConnectMode, &mc.Workspace,
			&mc.RPM, &mc.MaxTokens, &mc.MaxTokensField,
			&mc.RequestTimeout, &mc.RequestDelay, &mc.ContextWindow,
			&mc.CostPer1KInput, &mc.CostPer1KOutput, &capJSON,
		); err != nil {
			return nil, fmt.Errorf("memory: scan model: %w", err)
		}
		if apiKeysJSON != "" && apiKeysJSON != "null" && apiKeysJSON != "[]" {
			_ = json.Unmarshal([]byte(apiKeysJSON), &mc.APIKeys)
		}
		if capJSON != "" && capJSON != "null" && capJSON != "[]" {
			_ = json.Unmarshal([]byte(capJSON), &mc.Capabilities)
		}
		models = append(models, mc)
	}
	return models, rows.Err()
}

// LoadModelsIntoConfig populates cfg.ModelList from the database.
func (m *MemoryDB) LoadModelsIntoConfig(cfg *config.Config) error {
	models, err := m.ListModels()
	if err != nil {
		return err
	}
	cfg.ModelList = models
	return nil
}

// SyncModels synchronises the models table with the given user-submitted list.
//
// Catalog models in the list have their api_key (and other user-adjustable fields)
// updated. Non-catalog models not in the list are deleted. New non-catalog models
// are inserted. Catalog models absent from the list keep their DB row (with any
// existing api_key cleared to empty).
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

	// Delete user-custom (non-catalog) models not present in the incoming list.
	rows, err := tx.Query(`SELECT model_name FROM models WHERE is_catalog = 0`)
	if err != nil {
		return fmt.Errorf("memory: sync models: query non-catalog: %w", err)
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

	// Upsert all incoming models.
	for _, mc := range incoming {
		isCatalog := 0
		if catalogNames[mc.ModelName] {
			isCatalog = 1
		}
		if err := upsertModelTx(tx, mc, isCatalog); err != nil {
			return fmt.Errorf("memory: sync models: upsert %q: %w", mc.ModelName, err)
		}
	}

	return tx.Commit()
}

// InitModels seeds the catalog and loads models into cfg.
//
// On first run (empty table) it also migrates any entries already in
// cfg.ModelList (e.g. from a legacy config.json that still has model_list).
// After loading from DB, any cfg.ModelList entries that are NOT in the DB
// (e.g. test-only models set directly on the config) are preserved in
// cfg.ModelList so that GetModelConfig continues to find them.
func (m *MemoryDB) InitModels(cfg *config.Config) error {
	catalog := config.DefaultModelList()

	// Check whether the table is empty (first run after migration).
	var count int
	_ = m.db.QueryRow(`SELECT COUNT(*) FROM models`).Scan(&count)

	if count == 0 {
		// Migrate any entries from the in-memory list (legacy config.json).
		if len(cfg.ModelList) > 0 {
			if err := m.SeedCatalogModels(cfg.ModelList); err != nil {
				return err
			}
		}
		// Seed catalog entries not yet in DB.
		if err := m.SeedCatalogModels(catalog); err != nil {
			return err
		}
	} else {
		// DB already has entries — add new catalog models introduced in updates.
		if err := m.SeedCatalogModels(catalog); err != nil {
			return err
		}
	}

	// Load from DB.
	dbModels, err := m.ListModels()
	if err != nil {
		return err
	}

	// Preserve any cfg.ModelList entries absent from the DB (e.g. test configs
	// that set ModelList directly without going through DB persistence).
	dbNames := make(map[string]bool, len(dbModels))
	for _, mc := range dbModels {
		dbNames[mc.ModelName] = true
	}
	result := dbModels
	for _, mc := range cfg.ModelList {
		if !dbNames[mc.ModelName] {
			result = append(result, mc)
		}
	}
	cfg.ModelList = result
	return nil
}
