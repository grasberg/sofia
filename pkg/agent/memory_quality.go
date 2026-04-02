package agent

import (
	"encoding/json"
	"math"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
)

// MemoryQualityScorer computes a 0.0-1.0 quality score for semantic nodes
// based on usage frequency, recency, connectedness, and property richness.
type MemoryQualityScorer struct{}

// Score computes the quality score for a node given its edge count.
func (mqs *MemoryQualityScorer) Score(node memory.SemanticNode, edgeCount int) float64 {
	score := 0.0

	// Factor 1: Access frequency (0-0.3)
	accessScore := math.Min(float64(node.AccessCount)/20.0, 1.0) * 0.3
	score += accessScore

	// Factor 2: Recency (0-0.3)
	if node.LastAccessed != nil {
		daysSince := time.Since(*node.LastAccessed).Hours() / 24
		recencyScore := math.Max(0, 1.0-daysSince/90.0) * 0.3
		score += recencyScore
	}

	// Factor 3: Connectedness — edge count (0-0.2)
	connectScore := math.Min(float64(edgeCount)/5.0, 1.0) * 0.2
	score += connectScore

	// Factor 4: Properties richness (0-0.2)
	propCount := countJSONKeys(node.Properties)
	propScore := math.Min(float64(propCount)/3.0, 1.0) * 0.2
	score += propScore

	return score
}

// ShouldPrune returns true if the node is low-quality and stale enough to remove.
func (mqs *MemoryQualityScorer) ShouldPrune(node memory.SemanticNode, edgeCount int, maxAgeDays int) bool {
	score := mqs.Score(node, edgeCount)
	if score >= 0.2 {
		return false
	}
	if node.AccessCount >= 2 {
		return false
	}
	if node.LastAccessed != nil {
		daysSince := time.Since(*node.LastAccessed).Hours() / 24
		return daysSince > float64(maxAgeDays)
	}
	// No last_accessed — check created_at
	daysSinceCreated := time.Since(node.CreatedAt).Hours() / 24
	return daysSinceCreated > float64(maxAgeDays)
}

func countJSONKeys(jsonStr string) int {
	if jsonStr == "" || jsonStr == "{}" {
		return 0
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return 0
	}
	return len(m)
}
