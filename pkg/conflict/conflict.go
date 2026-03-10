package conflict

import (
	"fmt"
	"math"
	"sort"
	"strings"
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

// Detect analyzes a set of outputs for conflicts.
// It compares outputs pairwise using content similarity and contradiction heuristics.
func Detect(outputs []Output) DetectResult {
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
					Type:        "divergence",
					Description: fmt.Sprintf("Agents %s and %s produced significantly different outputs", a.AgentID, b.AgentID),
					Outputs:     []Output{a, b},
					Severity:    divergenceSeverity(sim),
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
		return Resolution{Strategy: "majority_vote", Reason: "no groups formed"}
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
		Strategy: "majority_vote",
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
		Strategy: "priority",
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
		Strategy: "merge",
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

	strategy := "longest"
	if shortest {
		strategy = "shortest"
	}

	winner := sorted[0]
	return Resolution{
		Strategy: strategy,
		Winner:   &winner,
		Reason:   fmt.Sprintf("selected %s output (%d chars) from agent %s", strategy, len(winner.Content), winner.AgentID),
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
		Strategy: "all",
		Merged:   sb.String(),
		Reason:   fmt.Sprintf("returned all %d outputs without resolution", len(outputs)),
	}
}

// --- Helpers ---

// contentSimilarity returns a 0.0-1.0 similarity score between two strings
// using Jaccard similarity on word sets.
func contentSimilarity(a, b string) float64 {
	return wordSimilarity(strings.ToLower(a), strings.ToLower(b))
}

func wordSimilarity(a, b string) float64 {
	wordsA := wordSet(a)
	wordsB := wordSet(b)
	if len(wordsA) == 0 && len(wordsB) == 0 {
		return 1.0
	}
	intersection := 0
	for w := range wordsA {
		if wordsB[w] {
			intersection++
		}
	}
	union := len(wordsA) + len(wordsB) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

func wordSet(s string) map[string]bool {
	words := strings.Fields(s)
	set := make(map[string]bool, len(words))
	for _, w := range words {
		// Strip punctuation
		w = strings.Trim(w, ".,;:!?\"'()[]{}")
		if w != "" {
			set[w] = true
		}
	}
	return set
}

// detectContradiction checks if two outputs contain direct contradictions.
func detectContradiction(a, b Output) *Conflict {
	contentA := strings.ToLower(strings.TrimSpace(a.Content))
	contentB := strings.ToLower(strings.TrimSpace(b.Content))

	// Short answer contradiction: yes vs no
	isAffirmA := isAffirmative(contentA)
	isNegA := isNegative(contentA)
	isAffirmB := isAffirmative(contentB)
	isNegB := isNegative(contentB)

	if (isAffirmA && isNegB) || (isNegA && isAffirmB) {
		return &Conflict{
			Type:        "contradiction",
			Description: fmt.Sprintf("Agents %s and %s gave opposing answers", a.AgentID, b.AgentID),
			Outputs:     []Output{a, b},
			Severity:    "high",
		}
	}

	// Numeric contradiction: different numbers for same context
	numsA := extractNumbers(contentA)
	numsB := extractNumbers(contentB)
	if len(numsA) > 0 && len(numsB) > 0 {
		// If the texts are about similar topics but have different key numbers
		if contentSimilarity(a.Content, b.Content) > 0.3 {
			for _, na := range numsA {
				for _, nb := range numsB {
					if na != nb && math.Abs(na-nb) > 0.01 {
						return &Conflict{
							Type: "contradiction",
							Description: fmt.Sprintf(
								"Agents %s and %s report different values (%.2f vs %.2f)",
								a.AgentID, b.AgentID, na, nb,
							),
							Outputs:  []Output{a, b},
							Severity: "medium",
						}
					}
				}
			}
		}
	}

	return nil
}

func isAffirmative(s string) bool {
	prefixes := []string{"yes", "true", "correct", "confirmed", "affirmative"}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func isNegative(s string) bool {
	prefixes := []string{"no", "false", "incorrect", "denied", "negative"}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func extractNumbers(s string) []float64 {
	var nums []float64
	words := strings.Fields(s)
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()[]{}")
		var n float64
		if _, err := fmt.Sscanf(w, "%f", &n); err == nil {
			nums = append(nums, n)
		}
	}
	return nums
}

func splitSentences(s string) []string {
	var sentences []string
	var current strings.Builder
	for _, r := range s {
		current.WriteRune(r)
		if r == '.' || r == '!' || r == '?' || r == '\n' {
			sentence := strings.TrimSpace(current.String())
			if sentence != "" {
				sentences = append(sentences, sentence)
			}
			current.Reset()
		}
	}
	if remaining := strings.TrimSpace(current.String()); remaining != "" {
		sentences = append(sentences, remaining)
	}
	return sentences
}

func divergenceSeverity(similarity float64) string {
	if similarity < 0.1 {
		return "high"
	}
	if similarity < 0.2 {
		return "medium"
	}
	return "low"
}

// groupBySimilarity groups outputs by content similarity using a threshold.
func groupBySimilarity(outputs []Output, threshold float64) [][]Output {
	assigned := make([]bool, len(outputs))
	var groups [][]Output

	for i := range outputs {
		if assigned[i] {
			continue
		}
		group := []Output{outputs[i]}
		assigned[i] = true
		for j := i + 1; j < len(outputs); j++ {
			if assigned[j] {
				continue
			}
			if contentSimilarity(outputs[i].Content, outputs[j].Content) >= threshold {
				group = append(group, outputs[j])
				assigned[j] = true
			}
		}
		groups = append(groups, group)
	}
	return groups
}

// pickBest selects the best output from a group by score, then priority.
func pickBest(outputs []Output) *Output {
	if len(outputs) == 0 {
		return nil
	}
	best := outputs[0]
	for _, o := range outputs[1:] {
		if o.Score > best.Score || (o.Score == best.Score && o.Priority > best.Priority) {
			best = o
		}
	}
	return &best
}

// Format returns a human-readable summary of a DetectResult.
func (dr DetectResult) Format() string {
	if !dr.HasConflicts {
		return fmt.Sprintf("No conflicts detected (agreement: %.0f%%)", dr.Agreement*100)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Detected %d conflict(s) (agreement: %.0f%%):\n", len(dr.Conflicts), dr.Agreement*100)
	for i, c := range dr.Conflicts {
		fmt.Fprintf(&sb, "  %d. [%s/%s] %s\n", i+1, c.Type, c.Severity, c.Description)
		for _, o := range c.Outputs {
			preview := o.Content
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			fmt.Fprintf(&sb, "     - %s: %s\n", o.AgentID, preview)
		}
	}
	return sb.String()
}

// Format returns a human-readable summary of a Resolution.
func (r Resolution) Format() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Resolution (strategy: %s): %s\n", r.Strategy, r.Reason)
	if r.Winner != nil {
		preview := r.Winner.Content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		fmt.Fprintf(&sb, "  Winner [%s]: %s\n", r.Winner.AgentID, preview)
	}
	if r.Merged != "" {
		preview := r.Merged
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		fmt.Fprintf(&sb, "  Merged: %s\n", preview)
	}
	if len(r.Rejected) > 0 {
		ids := make([]string, len(r.Rejected))
		for i, o := range r.Rejected {
			ids[i] = o.AgentID
		}
		fmt.Fprintf(&sb, "  Rejected: %s\n", strings.Join(ids, ", "))
	}
	return sb.String()
}
