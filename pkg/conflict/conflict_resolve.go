package conflict

import (
	"fmt"
	"sort"
	"strings"
)

// Resolve applies the given strategy to resolve conflicts in a set of outputs.
func Resolve(outputs []Output, strategy Strategy) Resolution {
	if len(outputs) == 0 {
		return Resolution{Strategy: string(strategy), Reason: "no outputs to resolve"}
	}
	if len(outputs) == 1 {
		return Resolution{
			Strategy: string(strategy),
			Winner:   &outputs[0],
			Reason:   "single output, no conflict",
		}
	}

	switch strategy {
	case StrategyMajorityVote:
		return resolveMajorityVote(outputs)
	case StrategyPriority:
		return resolvePriority(outputs)
	case StrategyMerge:
		return resolveMerge(outputs)
	case StrategyShortest:
		return resolveByLength(outputs, true)
	case StrategyLongest:
		return resolveByLength(outputs, false)
	case StrategyAll:
		return resolveAll(outputs)
	default:
		return resolveMajorityVote(outputs)
	}
}

// resolveMajorityVote groups similar outputs and picks the largest group.
func resolveMajorityVote(outputs []Output) Resolution {
	// Group outputs by similarity
	groups := groupBySimilarity(outputs, 0.5)

	if len(groups) == 0 {
		return Resolution{Strategy: string(StrategyMajorityVote), Reason: "no groups formed"}
	}

	// Find the largest group
	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i]) > len(groups[j])
	})

	winners := groups[0]
	var rejected []Output
	for _, g := range groups[1:] {
		rejected = append(rejected, g...)
	}

	// Pick the highest-scored or first output from the winning group
	best := pickBest(winners)

	return Resolution{
		Strategy: string(StrategyMajorityVote),
		Winner:   best,
		Reason:   fmt.Sprintf("%d/%d agents agreed", len(winners), len(outputs)),
		Rejected: rejected,
	}
}

// resolvePriority picks the output with the highest priority.
func resolvePriority(outputs []Output) Resolution {
	sorted := make([]Output, len(outputs))
	copy(sorted, outputs)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority > sorted[j].Priority
		}
		return sorted[i].Score > sorted[j].Score
	})

	winner := sorted[0]
	return Resolution{
		Strategy: string(StrategyPriority),
		Winner:   &winner,
		Reason:   fmt.Sprintf("agent %s has highest priority (%d)", winner.AgentID, winner.Priority),
		Rejected: sorted[1:],
	}
}

// resolveMerge combines non-redundant content from all outputs.
func resolveMerge(outputs []Output) Resolution {
	seen := make(map[string]bool)
	var parts []string

	for _, out := range outputs {
		sentences := splitSentences(out.Content)
		for _, s := range sentences {
			normalized := strings.ToLower(strings.TrimSpace(s))
			if normalized == "" || seen[normalized] {
				continue
			}
			// Check if a similar sentence was already added
			duplicate := false
			for existing := range seen {
				if wordSimilarity(normalized, existing) > 0.8 {
					duplicate = true
					break
				}
			}
			if !duplicate {
				seen[normalized] = true
				parts = append(parts, strings.TrimSpace(s))
			}
		}
	}

	merged := strings.Join(parts, " ")
	return Resolution{
		Strategy: string(StrategyMerge),
		Merged:   merged,
		Reason:   fmt.Sprintf("merged unique content from %d outputs", len(outputs)),
	}
}

// resolveByLength picks the shortest or longest output.
func resolveByLength(outputs []Output, shortest bool) Resolution {
	sorted := make([]Output, len(outputs))
	copy(sorted, outputs)
	sort.Slice(sorted, func(i, j int) bool {
		if shortest {
			return len(sorted[i].Content) < len(sorted[j].Content)
		}
		return len(sorted[i].Content) > len(sorted[j].Content)
	})

	strategy := StrategyLongest
	if shortest {
		strategy = StrategyShortest
	}

	winner := sorted[0]
	return Resolution{
		Strategy: string(strategy),
		Winner:   &winner,
		Reason: fmt.Sprintf(
			"selected %s output (%d chars) from agent %s",
			strategy,
			len(winner.Content),
			winner.AgentID,
		),
		Rejected: sorted[1:],
	}
}

// resolveAll returns all outputs without choosing.
func resolveAll(outputs []Output) Resolution {
	var sb strings.Builder
	for i, out := range outputs {
		if i > 0 {
			sb.WriteString("\n---\n")
		}
		fmt.Fprintf(&sb, "[Agent %s]: %s", out.AgentID, out.Content)
	}
	return Resolution{
		Strategy: string(StrategyAll),
		Merged:   sb.String(),
		Reason:   fmt.Sprintf("returned all %d outputs without resolution", len(outputs)),
	}
}
