package evolution

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/memory"
)

// EvolutionAgentConfig extends config.AgentConfig with runtime fields.
type EvolutionAgentConfig struct {
	config.AgentConfig
	PurposePrompt string `json:"purpose_prompt,omitempty"`
	ModelID       string `json:"model_id,omitempty"`
}

// AgentStore persists dynamically created agents in the evolution_agents SQLite table.
type AgentStore struct {
	db *memory.MemoryDB
}

// NewAgentStore creates a new AgentStore backed by the given MemoryDB.
func NewAgentStore(db *memory.MemoryDB) *AgentStore {
	return &AgentStore{db: db}
}

// Save upserts an agent configuration into the evolution_agents table.
func (s *AgentStore) Save(agentID string, cfg EvolutionAgentConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("evolution: marshal agent config: %w", err)
	}
	_, err = s.db.Exec(
		`INSERT OR REPLACE INTO evolution_agents (id, agent_id, config_json, status) VALUES (?, ?, ?, 'active')`,
		agentID, agentID, string(data),
	)
	if err != nil {
		return fmt.Errorf("evolution: save agent %s: %w", agentID, err)
	}
	return nil
}

// Get returns the stored config and current status for the given agent ID.
// Returns nil config when the agent is not found.
func (s *AgentStore) Get(agentID string) (*EvolutionAgentConfig, string, error) {
	row := s.db.QueryRow(
		`SELECT config_json, status FROM evolution_agents WHERE agent_id = ?`,
		agentID,
	)
	var configJSON, status string
	if err := row.Scan(&configJSON, &status); err != nil {
		if err == sql.ErrNoRows {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("evolution: get agent %s: %w", agentID, err)
	}
	var cfg EvolutionAgentConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return nil, "", fmt.Errorf("evolution: unmarshal agent config: %w", err)
	}
	return &cfg, status, nil
}

// ListActive returns all agent configs with status='active'.
func (s *AgentStore) ListActive() ([]EvolutionAgentConfig, error) {
	rows, err := s.db.Query(
		`SELECT config_json FROM evolution_agents WHERE status = 'active' ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("evolution: list active agents: %w", err)
	}
	defer rows.Close()

	var result []EvolutionAgentConfig
	for rows.Next() {
		var configJSON string
		if err := rows.Scan(&configJSON); err != nil {
			return nil, fmt.Errorf("evolution: scan active agent: %w", err)
		}
		var cfg EvolutionAgentConfig
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			return nil, fmt.Errorf("evolution: unmarshal active agent: %w", err)
		}
		result = append(result, cfg)
	}
	return result, rows.Err()
}

// ListRetired returns the agent IDs of all retired agents.
func (s *AgentStore) ListRetired() ([]string, error) {
	rows, err := s.db.Query(
		`SELECT agent_id FROM evolution_agents WHERE status = 'retired' ORDER BY retired_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("evolution: list retired agents: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("evolution: scan retired agent: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MarkRetired sets an agent's status to 'retired' with a timestamp and reason.
func (s *AgentStore) MarkRetired(agentID, reason string) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	result, err := s.db.Exec(
		`UPDATE evolution_agents SET status = 'retired', retired_at = ?, reason = ? WHERE agent_id = ?`,
		now, reason, agentID,
	)
	if err != nil {
		return fmt.Errorf("evolution: mark retired %s: %w", agentID, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("evolution: agent %s not found", agentID)
	}
	return nil
}
