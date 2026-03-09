package agent

import (
	"fmt"
	"math"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
)

// PruneOptions configures the strategic forgetting process.
type PruneOptions struct {
	// MaxAge is the maximum time since last access before a node becomes prunable.
	// Default: 90 days.
	MaxAge time.Duration

	// MinAccessCount is the minimum number of reads for a node to survive.
	// Nodes with fewer accesses than this are candidates for pruning.
	// Default: 2.
	MinAccessCount int

	// ScoreThreshold is the minimum survival score. Nodes below this are pruned.
	// Score = access_count * recency_factor, where recency_factor decays with time.
	// Default: 0.1.
	ScoreThreshold float64

	// HalfLife is the number of days after which recency_factor = 0.5.
	// Default: 30.
	HalfLife float64

	// DryRun previews what would be pruned without actually deleting.
	DryRun bool
}

// DefaultPruneOptions returns sensible defaults for pruning.
func DefaultPruneOptions() PruneOptions {
	return PruneOptions{
		MaxAge:         90 * 24 * time.Hour,
		MinAccessCount: 2,
		ScoreThreshold: 0.1,
		HalfLife:       30,
		DryRun:         false,
	}
}

// PruneReport describes what the pruner did (or would do in dry-run mode).
type PruneReport struct {
	Pruned   int
	Survived int
	DryRun   bool
	Details  []string
}

// MemoryPruner handles strategic forgetting of outdated memories.
type MemoryPruner struct {
	db      *memory.MemoryDB
	agentID string
}

// NewMemoryPruner creates a new pruner.
func NewMemoryPruner(db *memory.MemoryDB, agentID string) *MemoryPruner {
	return &MemoryPruner{
		db:      db,
		agentID: agentID,
	}
}

// Prune removes nodes that score below the threshold.
// Score = access_count * recency_factor
// recency_factor = 1.0 / (1.0 + days_since_last_access / half_life)
func (mp *MemoryPruner) Prune(opts PruneOptions) (PruneReport, error) {
	if mp.db == nil {
		return PruneReport{}, nil
	}

	// Fill defaults
	if opts.MaxAge == 0 {
		opts.MaxAge = 90 * 24 * time.Hour
	}
	if opts.MinAccessCount == 0 {
		opts.MinAccessCount = 2
	}
	if opts.ScoreThreshold == 0 {
		opts.ScoreThreshold = 0.1
	}
	if opts.HalfLife == 0 {
		opts.HalfLife = 30
	}

	report := PruneReport{DryRun: opts.DryRun}

	// Get candidates: low access count and old
	candidates, err := mp.db.GetStaleNodes(mp.agentID, opts.MaxAge, opts.MinAccessCount)
	if err != nil {
		return report, fmt.Errorf("prune: get stale nodes: %w", err)
	}

	now := time.Now().UTC()
	var toPrune []int64

	for _, node := range candidates {
		// Calculate decay score
		score := mp.calculateScore(node, now, opts.HalfLife)

		if score < opts.ScoreThreshold {
			toPrune = append(toPrune, node.ID)
			detail := fmt.Sprintf("Prune [%s] %s (score=%.3f, accesses=%d)",
				node.Label, node.Name, score, node.AccessCount)
			report.Details = append(report.Details, detail)
		} else {
			report.Survived++
		}
	}

	report.Pruned = len(toPrune)

	if !opts.DryRun && len(toPrune) > 0 {
		// Record stats before deleting
		for _, id := range toPrune {
			nodeIDCopy := id
			_ = mp.db.RecordStat(mp.agentID, "prune", &nodeIDCopy, "Strategic forgetting")
		}

		if err := mp.db.DeleteNodes(toPrune); err != nil {
			return report, fmt.Errorf("prune: delete nodes: %w", err)
		}

		logger.InfoCF("memory", "Strategic forgetting completed",
			map[string]any{
				"pruned":   report.Pruned,
				"survived": report.Survived,
			})
	}

	return report, nil
}

// calculateScore computes the survival score for a node.
func (mp *MemoryPruner) calculateScore(node memory.SemanticNode, now time.Time, halfLife float64) float64 {
	var daysSinceAccess float64
	if node.LastAccessed != nil {
		daysSinceAccess = now.Sub(*node.LastAccessed).Hours() / 24
	} else {
		// Never accessed — use creation date
		daysSinceAccess = now.Sub(node.CreatedAt).Hours() / 24
	}

	recencyFactor := 1.0 / (1.0 + daysSinceAccess/halfLife)
	score := float64(node.AccessCount) * recencyFactor

	// Minimum score is ~0 for zero-access ancient nodes
	return math.Max(0, score)
}
