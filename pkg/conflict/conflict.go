package conflict

import (
	"fmt"
)

// Output represents one agent's result for a given task or question.
type Output struct {
	AgentID  string  `json:"agent_id"`
	TaskID   string  `json:"task_id,omitempty"`
	Content  string  `json:"content"`
	Priority int     `json:"priority,omitempty"` // Higher = more authoritative
	Score    float64 `json:"score,omitempty"`    // Quality score from agent scorer
}

// Conflict describes a detected disagreement between agent outputs.
type Conflict struct {
	Type        string   `json:"type"`        // "contradiction", "divergence", "overlap"
	Description string   `json:"description"` // Human-readable explanation
	Outputs     []Output `json:"outputs"`     // The conflicting outputs
	Severity    string   `json:"severity"`    // "low", "medium", "high"
}

// Resolution is the result of resolving one or more conflicts.
type Resolution struct {
	Strategy string   `json:"strategy"` // Which strategy was applied
	Winner   *Output  `json:"winner,omitempty"`
	Merged   string   `json:"merged,omitempty"`
	Reason   string   `json:"reason"`
	Rejected []Output `json:"rejected,omitempty"`
}

// Strategy defines how conflicts should be resolved.
type Strategy string

const (
	StrategyMajorityVote Strategy = "majority_vote"
	StrategyPriority     Strategy = "priority"
	StrategyMerge        Strategy = "merge"
	StrategyShortest     Strategy = "shortest"
	StrategyLongest      Strategy = "longest"
	StrategyAll          Strategy = "all" // Return all outputs, no resolution
)

// DetectResult holds all detected conflicts from a set of outputs.
type DetectResult struct {
	HasConflicts bool       `json:"has_conflicts"`
	Conflicts    []Conflict `json:"conflicts"`
	Agreement    float64    `json:"agreement"` // 0.0 to 1.0
}

// DefaultMaxOutputs is the maximum number of outputs to compare in conflict detection.
// If more outputs are provided, only the first DefaultMaxOutputs are considered.
const DefaultMaxOutputs = 20

// DetectWithLimit analyzes outputs for conflicts, capping at maxOutputs.
// Use maxOutputs <= 0 to apply the DefaultMaxOutputs limit.
func DetectWithLimit(outputs []Output, maxOutputs int) DetectResult {
	if maxOutputs <= 0 {
		maxOutputs = DefaultMaxOutputs
	}
	if len(outputs) > maxOutputs {
		outputs = outputs[:maxOutputs]
	}
	return detectOutputs(outputs)
}

// Detect analyzes a set of outputs for conflicts.
// It compares outputs pairwise using content similarity and contradiction heuristics.
// Outputs are capped at DefaultMaxOutputs to bound computation.
func Detect(outputs []Output) DetectResult {
	if len(outputs) > DefaultMaxOutputs {
		outputs = outputs[:DefaultMaxOutputs]
	}
	return detectOutputs(outputs)
}

// detectOutputs is the internal implementation for conflict detection.
func detectOutputs(outputs []Output) DetectResult {
	if len(outputs) <= 1 {
		return DetectResult{Agreement: 1.0}
	}

	var conflicts []Conflict
	totalPairs := 0
	agreePairs := 0

	for i := 0; i < len(outputs); i++ {
		for j := i + 1; j < len(outputs); j++ {
			totalPairs++
			a, b := outputs[i], outputs[j]

			sim := contentSimilarity(a.Content, b.Content)

			// High similarity = agreement
			if sim > 0.7 {
				agreePairs++
				continue
			}

			// Check for direct contradictions
			if c := detectContradiction(a, b); c != nil {
				conflicts = append(conflicts, *c)
				continue
			}

			// Low similarity = divergence
			if sim < 0.3 {
				conflicts = append(conflicts, Conflict{
					Type: "divergence",
					Description: fmt.Sprintf(
						"Agents %s and %s produced significantly different outputs",
						a.AgentID,
						b.AgentID,
					),
					Outputs:  []Output{a, b},
					Severity: divergenceSeverity(sim),
				})
			} else {
				// Moderate similarity — partial overlap
				agreePairs++ // treat as soft agreement
			}
		}
	}

	agreement := 1.0
	if totalPairs > 0 {
		agreement = float64(agreePairs) / float64(totalPairs)
	}

	return DetectResult{
		HasConflicts: len(conflicts) > 0,
		Conflicts:    conflicts,
		Agreement:    agreement,
	}
}
