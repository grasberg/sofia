package conflict

import (
	"fmt"
	"math"
	"strings"
)

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
		// If the texts are about similar topics but have different key numbers.
		// Use 0.5 word overlap threshold to reduce false positives.
		if contentSimilarity(a.Content, b.Content) > 0.5 {
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
