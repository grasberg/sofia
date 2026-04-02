package abtest

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
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

// RunTrial executes a prompt against all variants in an experiment
// and records the results.
func (m *Manager) RunTrial(
	ctx context.Context,
	experimentName, prompt string,
	provider providers.LLMProvider,
	defaultModel string,
) ([]Trial, error) {
	exp, err := m.GetExperiment(experimentName)
	if err != nil {
		return nil, err
	}
	if exp.Status != "active" {
		return nil, fmt.Errorf(
			"experiment %q is %s, not active",
			experimentName, exp.Status,
		)
	}

	var trials []Trial
	for _, v := range exp.Variants {
		trial := m.runSingleTrial(
			ctx, exp.ID, v, prompt, provider, defaultModel,
		)
		trials = append(trials, trial)
	}

	return trials, nil
}

func (m *Manager) runSingleTrial(
	ctx context.Context,
	experimentID int64,
	v Variant,
	prompt string,
	provider providers.LLMProvider,
	defaultModel string,
) Trial {
	// Build messages with variant overrides.
	var messages []providers.Message
	if v.Config.SystemPrompt != "" {
		messages = append(messages, providers.Message{
			Role:    "system",
			Content: v.Config.SystemPrompt,
		})
	}

	userContent := prompt
	if v.Config.PromptPrefix != "" {
		userContent = v.Config.PromptPrefix + "\n" + userContent
	}
	if v.Config.PromptSuffix != "" {
		userContent = userContent + "\n" + v.Config.PromptSuffix
	}
	messages = append(messages, providers.Message{
		Role:    "user",
		Content: userContent,
	})

	// Select model.
	model := defaultModel
	if v.Config.Model != "" {
		model = v.Config.Model
	}

	// Build options.
	opts := map[string]any{}
	if v.Config.Temperature != nil {
		opts["temperature"] = *v.Config.Temperature
	}
	if v.Config.MaxTokens > 0 {
		opts["max_tokens"] = v.Config.MaxTokens
	}

	start := time.Now()
	resp, err := provider.Chat(ctx, messages, nil, model, opts)
	latencyMs := time.Since(start).Milliseconds()

	trial := Trial{
		ExperimentID: experimentID,
		VariantID:    v.ID,
		VariantName:  v.Name,
		Prompt:       prompt,
		LatencyMs:    latencyMs,
		CreatedAt:    time.Now(),
	}

	if err != nil {
		trial.Error = err.Error()
	} else {
		trial.Response = resp.Content
		if resp.Usage != nil {
			trial.TokensIn = resp.Usage.PromptTokens
			trial.TokensOut = resp.Usage.CompletionTokens
		}
	}

	// Persist the trial.
	res, dbErr := m.db.Exec(
		`INSERT INTO ab_trials
		 (experiment_id, variant_id, prompt, response, latency_ms,
		  tokens_in, tokens_out, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		experimentID, v.ID, prompt, trial.Response, trial.LatencyMs,
		trial.TokensIn, trial.TokensOut, trial.Error,
	)
	if dbErr == nil {
		trial.ID, _ = res.LastInsertId() //nolint:errcheck
	}

	return trial
}

// ScoreTrial assigns a score (0.0-1.0) to a trial.
func (m *Manager) ScoreTrial(trialID int64, score float64) error {
	if score < 0 || score > 1 {
		return fmt.Errorf("score must be between 0.0 and 1.0")
	}
	_, err := m.db.Exec(
		`UPDATE ab_trials SET score = ? WHERE id = ?`, score, trialID,
	)
	return err
}

// Analyze produces aggregate statistics for an experiment.
func (m *Manager) Analyze(experimentName string) (*Analysis, error) {
	exp, err := m.GetExperiment(experimentName)
	if err != nil {
		return nil, err
	}

	analysis := &Analysis{
		ExperimentName: exp.Name,
		Status:         exp.Status,
	}

	for _, v := range exp.Variants {
		trials, tErr := m.getTrials(exp.ID, v.ID)
		if tErr != nil {
			return nil, tErr
		}

		stats := computeStats(v.Name, trials)
		analysis.TotalTrials += stats.TrialCount
		analysis.Stats = append(analysis.Stats, stats)
	}

	analysis.Recommendation = recommend(analysis.Stats)
	return analysis, nil
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

func computeStats(variantName string, trials []Trial) VariantStats {
	s := VariantStats{VariantName: variantName, TrialCount: len(trials)}
	if len(trials) == 0 {
		return s
	}

	var (
		totalLatency int64
		totalIn      int
		totalOut     int
		scores       []float64
	)

	for _, t := range trials {
		totalLatency += t.LatencyMs
		totalIn += t.TokensIn
		totalOut += t.TokensOut
		if t.Error != "" {
			s.ErrorCount++
		}
		if t.Score != nil {
			scores = append(scores, *t.Score)
		}
	}

	n := float64(len(trials))
	s.AvgLatencyMs = float64(totalLatency) / n
	s.AvgTokensIn = float64(totalIn) / n
	s.AvgTokensOut = float64(totalOut) / n
	s.ErrorRate = float64(s.ErrorCount) / n

	if len(scores) > 0 {
		s.ScoredCount = len(scores)
		sum := 0.0
		s.MinScore = scores[0]
		s.MaxScore = scores[0]
		for _, sc := range scores {
			sum += sc
			if sc < s.MinScore {
				s.MinScore = sc
			}
			if sc > s.MaxScore {
				s.MaxScore = sc
			}
		}
		s.AvgScore = sum / float64(len(scores))

		if len(scores) > 1 {
			variance := 0.0
			for _, sc := range scores {
				diff := sc - s.AvgScore
				variance += diff * diff
			}
			s.StdDevScore = math.Sqrt(
				variance / float64(len(scores)),
			)
		}
	}

	return s
}

func recommend(stats []VariantStats) string {
	if len(stats) == 0 {
		return "No data available."
	}

	hasScoredData := false
	for _, s := range stats {
		if s.ScoredCount > 0 {
			hasScoredData = true
			break
		}
	}

	if !hasScoredData {
		best := stats[0]
		for _, s := range stats[1:] {
			if s.ErrorRate < best.ErrorRate {
				best = s
			} else if s.ErrorRate == best.ErrorRate &&
				s.AvgLatencyMs < best.AvgLatencyMs {
				best = s
			}
		}
		return fmt.Sprintf(
			"No scores yet. Based on error rate and latency, "+
				"%q looks best (%.0fms avg, %.0f%% errors). "+
				"Score trials for a quality-based recommendation.",
			best.VariantName, best.AvgLatencyMs, best.ErrorRate*100,
		)
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].AvgScore > stats[j].AvgScore
	})
	best := stats[0]

	minTrials := 5
	if best.ScoredCount < minTrials {
		return fmt.Sprintf(
			"%q leads with avg score %.2f (%d trials scored). "+
				"Run at least %d more trials for "+
				"statistical confidence.",
			best.VariantName, best.AvgScore, best.ScoredCount,
			minTrials-best.ScoredCount,
		)
	}

	if len(stats) > 1 {
		second := stats[1]
		gap := best.AvgScore - second.AvgScore
		if gap < 0.05 {
			return fmt.Sprintf(
				"Results are close: %q (%.2f) vs %q (%.2f). "+
					"Run more trials to establish significance.",
				best.VariantName, best.AvgScore,
				second.VariantName, second.AvgScore,
			)
		}
	}

	return fmt.Sprintf(
		"%q is the clear winner with avg score %.2f "+
			"(%.0fms avg latency, %d trials). "+
			"Consider concluding the experiment.",
		best.VariantName, best.AvgScore,
		best.AvgLatencyMs, best.ScoredCount,
	)
}
