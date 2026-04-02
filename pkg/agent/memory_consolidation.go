package agent

import (
	"fmt"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
)

// ConsolidationReport describes what the consolidator did.
type ConsolidationReport struct {
	MergedNodes      int
	ResolvedConflict int
	PrunedNodes      int
	Details          []string
}

// MemoryConsolidator merges new memories with existing ones and resolves conflicts.
type MemoryConsolidator struct {
	db       *memory.MemoryDB
	agentID  string
	semantic *SemanticMemory
}

// NewMemoryConsolidator creates a new consolidator.
func NewMemoryConsolidator(db *memory.MemoryDB, agentID string) *MemoryConsolidator {
	return &MemoryConsolidator{
		db:       db,
		agentID:  agentID,
		semantic: NewSemanticMemory(db, agentID),
	}
}

// Consolidate runs the full consolidation process:
// 1. Merge duplicate nodes (same label + similar name)
// 2. Resolve conflicting edges (same source+target, different relations — keep strongest)
func (mc *MemoryConsolidator) Consolidate() (ConsolidationReport, error) {
	if mc.db == nil {
		return ConsolidationReport{}, nil
	}

	report := ConsolidationReport{}

	// Step 1: Merge duplicate nodes
	duplicates, err := mc.db.FindDuplicateNodes(mc.agentID)
	if err != nil {
		return report, fmt.Errorf("consolidation: find duplicates: %w", err)
	}

	for _, cluster := range duplicates {
		if len(cluster) < 2 {
			continue
		}

		// The primary node is the one with the highest access count
		primaryIdx := 0
		for i, n := range cluster {
			if n.AccessCount > cluster[primaryIdx].AccessCount {
				primaryIdx = i
			}
		}
		primary := cluster[primaryIdx]

		// Collect secondary IDs
		secondaryIDs := make([]int64, 0, len(cluster)-1)
		secondaryNames := make([]string, 0, len(cluster)-1)
		for i, n := range cluster {
			if i != primaryIdx {
				secondaryIDs = append(secondaryIDs, n.ID)
				secondaryNames = append(secondaryNames, n.Name)
			}
		}

		// Merge
		if err := mc.db.MergeNodes(primary.ID, secondaryIDs); err != nil {
			logger.WarnCF("memory", "Consolidation merge failed",
				map[string]any{"primary": primary.Name, "error": err.Error()})
			continue
		}

		report.MergedNodes += len(secondaryIDs)
		detail := fmt.Sprintf("Merged %v into %q (label=%s)", secondaryNames, primary.Name, primary.Label)
		report.Details = append(report.Details, detail)

		// Record stat
		_ = mc.db.RecordStat(mc.agentID, "consolidation", &primary.ID, detail)
	}

	// Step 2: Resolve conflicting edges
	conflicts, err := mc.db.GetConflictingEdges(mc.agentID)
	if err != nil {
		return report, fmt.Errorf("consolidation: find conflicts: %w", err)
	}

	for _, edgeGroup := range conflicts {
		if len(edgeGroup) < 2 {
			continue
		}
		// Keep the highest-weight edge, delete the rest
		// Edges are already sorted by weight DESC
		for i := 1; i < len(edgeGroup); i++ {
			if err := mc.db.DeleteEdge(edgeGroup[i].ID); err != nil {
				logger.WarnCF("memory", "Consolidation edge delete failed",
					map[string]any{"edge_id": edgeGroup[i].ID, "error": err.Error()})
				continue
			}
			report.ResolvedConflict++
			detail := fmt.Sprintf("Removed weaker edge %q (w=%.2f) between %s→%s, kept %q (w=%.2f)",
				edgeGroup[i].Relation, edgeGroup[i].Weight,
				edgeGroup[i].SourceName, edgeGroup[i].TargetName,
				edgeGroup[0].Relation, edgeGroup[0].Weight)
			report.Details = append(report.Details, detail)
		}
	}

	// Step 3: Quality-based pruning — remove stale, low-value nodes
	report.PrunedNodes = mc.pruneStaleNodes(30)

	if report.MergedNodes > 0 || report.ResolvedConflict > 0 || report.PrunedNodes > 0 {
		logger.InfoCF("memory", "Memory consolidation completed",
			map[string]any{
				"merged_nodes":       report.MergedNodes,
				"resolved_conflicts": report.ResolvedConflict,
				"pruned_nodes":       report.PrunedNodes,
			})
	}

	return report, nil
}

// pruneStaleNodes removes low-quality nodes that haven't been accessed in maxAgeDays.
func (mc *MemoryConsolidator) pruneStaleNodes(maxAgeDays int) int {
	if mc.db == nil || maxAgeDays <= 0 {
		return 0
	}

	maxAge := time.Duration(maxAgeDays) * 24 * time.Hour
	staleNodes, err := mc.db.GetStaleNodes(mc.agentID, maxAge, 2)
	if err != nil {
		logger.WarnCF("memory", "Failed to get stale nodes for pruning",
			map[string]any{"error": err.Error()})
		return 0
	}

	scorer := &MemoryQualityScorer{}
	pruned := 0

	for _, node := range staleNodes {
		edges, _ := mc.db.GetEdges(mc.agentID, node.ID)
		if scorer.ShouldPrune(node, len(edges), maxAgeDays) {
			if err := mc.db.DeleteNode(node.ID); err != nil {
				continue
			}
			pruned++
		} else {
			// Update the quality score for surviving nodes
			score := scorer.Score(node, len(edges))
			_ = mc.db.UpdateNodeQuality(node.ID, score)
		}
	}

	return pruned
}
