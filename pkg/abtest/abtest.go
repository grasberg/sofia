package abtest

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
)

// VariantConfig holds the parameter overrides for a single variant.
type VariantConfig struct {
	Model        string   `json:"model,omitempty"`
	Temperature  *float64 `json:"temperature,omitempty"`
	MaxTokens    int      `json:"max_tokens,omitempty"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
	PromptPrefix string   `json:"prompt_prefix,omitempty"`
	PromptSuffix string   `json:"prompt_suffix,omitempty"`
}

// Variant is a single approach in an experiment.
type Variant struct {
	ID           int64         `json:"id"`
	ExperimentID int64         `json:"experiment_id"`
	Name         string        `json:"name"`
	Config       VariantConfig `json:"config"`
}

// Trial records one execution of a variant.
type Trial struct {
	ID           int64     `json:"id"`
	ExperimentID int64     `json:"experiment_id"`
	VariantID    int64     `json:"variant_id"`
	VariantName  string    `json:"variant_name"`
	Prompt       string    `json:"prompt"`
	Response     string    `json:"response"`
	Score        *float64  `json:"score"`
	LatencyMs    int64     `json:"latency_ms"`
	TokensIn     int       `json:"tokens_in"`
	TokensOut    int       `json:"tokens_out"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Experiment groups variants and their trials.
type Experiment struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	Winner      string     `json:"winner,omitempty"`
	Variants    []Variant  `json:"variants"`
	CreatedAt   time.Time  `json:"created_at"`
	ConcludedAt *time.Time `json:"concluded_at,omitempty"`
}

// VariantStats holds aggregate statistics for a single variant.
type VariantStats struct {
	VariantName  string  `json:"variant_name"`
	TrialCount   int     `json:"trial_count"`
	ScoredCount  int     `json:"scored_count"`
	AvgScore     float64 `json:"avg_score"`
	MinScore     float64 `json:"min_score"`
	MaxScore     float64 `json:"max_score"`
	StdDevScore  float64 `json:"std_dev_score"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	AvgTokensIn  float64 `json:"avg_tokens_in"`
	AvgTokensOut float64 `json:"avg_tokens_out"`
	ErrorCount   int     `json:"error_count"`
	ErrorRate    float64 `json:"error_rate"`
}

// Analysis is the complete analysis of an experiment.
type Analysis struct {
	ExperimentName string         `json:"experiment_name"`
	Status         string         `json:"status"`
	TotalTrials    int            `json:"total_trials"`
	Stats          []VariantStats `json:"stats"`
	Recommendation string         `json:"recommendation"`
}

// Manager coordinates A/B test experiments.
type Manager struct {
	db *memory.MemoryDB
}

// NewManager creates a new A/B test manager.
func NewManager(db *memory.MemoryDB) *Manager {
	return &Manager{db: db}
}

// CreateExperiment creates a new experiment with the given variants.
func (m *Manager) CreateExperiment(
	name, description string, variants map[string]VariantConfig,
) (*Experiment, error) {
	if len(variants) < 2 {
		return nil, fmt.Errorf("experiment requires at least 2 variants")
	}

	res, err := m.db.Exec(
		`INSERT INTO ab_experiments (name, description) VALUES (?, ?)`,
		name, description,
	)
	if err != nil {
		return nil, fmt.Errorf("create experiment: %w", err)
	}
	expID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get experiment id: %w", err)
	}

	exp := &Experiment{
		ID:          expID,
		Name:        name,
		Description: description,
		Status:      "active",
		CreatedAt:   time.Now(),
	}

	for vName, vCfg := range variants {
		cfgJSON, mErr := json.Marshal(vCfg)
		if mErr != nil {
			return nil, fmt.Errorf("marshal variant config: %w", mErr)
		}
		vRes, vErr := m.db.Exec(
			`INSERT INTO ab_variants (experiment_id, name, config)
			 VALUES (?, ?, ?)`,
			expID, vName, string(cfgJSON),
		)
		if vErr != nil {
			return nil, fmt.Errorf("create variant %q: %w", vName, vErr)
		}
		vID, _ := vRes.LastInsertId() //nolint:errcheck
		exp.Variants = append(exp.Variants, Variant{
			ID: vID, ExperimentID: expID, Name: vName, Config: vCfg,
		})
	}

	sort.Slice(exp.Variants, func(i, j int) bool {
		return exp.Variants[i].Name < exp.Variants[j].Name
	})

	return exp, nil
}

// GetExperiment loads an experiment by name.
func (m *Manager) GetExperiment(name string) (*Experiment, error) {
	row := m.db.QueryRow(
		`SELECT id, name, description, status, winner, created_at,
		        concluded_at
		 FROM ab_experiments WHERE name = ?`, name,
	)

	var exp Experiment
	var concludedAt *string
	err := row.Scan(
		&exp.ID, &exp.Name, &exp.Description, &exp.Status,
		&exp.Winner, &exp.CreatedAt, &concludedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("experiment %q not found", name)
	}
	if concludedAt != nil {
		for _, layout := range []string{
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05Z",
			time.RFC3339,
		} {
			if t, pErr := time.Parse(
				layout, *concludedAt,
			); pErr == nil {
				exp.ConcludedAt = &t
				break
			}
		}
	}

	exp.Variants, err = m.getVariants(exp.ID)
	if err != nil {
		return nil, err
	}

	return &exp, nil
}

// ListExperiments returns all experiments.
func (m *Manager) ListExperiments() ([]Experiment, error) {
	rows, err := m.db.Query(
		`SELECT id, name, description, status, winner, created_at
		 FROM ab_experiments ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	var experiments []Experiment
	for rows.Next() {
		var exp Experiment
		if err := rows.Scan(
			&exp.ID, &exp.Name, &exp.Description, &exp.Status,
			&exp.Winner, &exp.CreatedAt,
		); err != nil {
			return nil, err
		}
		experiments = append(experiments, exp)
	}
	return experiments, rows.Err()
}

// ConcludeExperiment marks an experiment as concluded with a winner.
func (m *Manager) ConcludeExperiment(name, winner string) error {
	_, err := m.db.Exec(
		`UPDATE ab_experiments
		 SET status = 'concluded', winner = ?,
		     concluded_at = datetime('now')
		 WHERE name = ? AND status = 'active'`,
		winner, name,
	)
	return err
}

// DeleteExperiment removes an experiment and all related data (variants and trials).
func (m *Manager) DeleteExperiment(name string) error {
	// Look up the experiment ID first.
	row := m.db.QueryRow(`SELECT id FROM ab_experiments WHERE name = ?`, name)
	var expID int64
	if err := row.Scan(&expID); err != nil {
		return fmt.Errorf("experiment %q not found", name)
	}

	// Delete child rows explicitly, then the experiment itself.
	// The MemoryDB has foreign_keys ON with ON DELETE CASCADE, but we delete
	// explicitly to be safe regardless of PRAGMA state.
	if _, err := m.db.Exec(`DELETE FROM ab_trials WHERE experiment_id = ?`, expID); err != nil {
		return fmt.Errorf("delete trials: %w", err)
	}
	if _, err := m.db.Exec(`DELETE FROM ab_variants WHERE experiment_id = ?`, expID); err != nil {
		return fmt.Errorf("delete variants: %w", err)
	}
	if _, err := m.db.Exec(`DELETE FROM ab_experiments WHERE id = ?`, expID); err != nil {
		return fmt.Errorf("delete experiment: %w", err)
	}
	return nil
}

func (m *Manager) getVariants(experimentID int64) ([]Variant, error) {
	rows, err := m.db.Query(
		`SELECT id, experiment_id, name, config
		 FROM ab_variants WHERE experiment_id = ? ORDER BY name`,
		experimentID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	var variants []Variant
	for rows.Next() {
		var v Variant
		var cfgJSON string
		if err := rows.Scan(
			&v.ID, &v.ExperimentID, &v.Name, &cfgJSON,
		); err != nil {
			return nil, err
		}
		if uErr := json.Unmarshal(
			[]byte(cfgJSON), &v.Config,
		); uErr != nil {
			return nil, fmt.Errorf(
				"unmarshal variant config: %w", uErr,
			)
		}
		variants = append(variants, v)
	}
	return variants, rows.Err()
}

func (m *Manager) getTrials(
	experimentID, variantID int64,
) ([]Trial, error) {
	rows, err := m.db.Query(
		`SELECT id, experiment_id, variant_id, prompt, response,
		        score, latency_ms, tokens_in, tokens_out, error,
		        created_at
		 FROM ab_trials
		 WHERE experiment_id = ? AND variant_id = ?
		 ORDER BY created_at`,
		experimentID, variantID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck

	var trials []Trial
	for rows.Next() {
		var t Trial
		if err := rows.Scan(
			&t.ID, &t.ExperimentID, &t.VariantID, &t.Prompt,
			&t.Response, &t.Score, &t.LatencyMs, &t.TokensIn,
			&t.TokensOut, &t.Error, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		trials = append(trials, t)
	}
	return trials, rows.Err()
}
