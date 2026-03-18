package agent

import "strings"

// Suggestion represents a quick-reply action.
type Suggestion struct {
	Label   string `json:"label"`
	Command string `json:"command"` // what gets sent if the user picks this
}

// GenerateSuggestions returns context-aware follow-up suggestions based on the last agent response.
func GenerateSuggestions(response string, hasToolCalls bool) []Suggestion {
	var suggestions []Suggestion
	lower := strings.ToLower(response)

	// After code/file operations, suggest review and test
	if hasToolCalls {
		if strings.Contains(lower, "created") || strings.Contains(lower, "wrote") ||
			strings.Contains(lower, "written") {
			suggestions = append(suggestions,
				Suggestion{Label: "Review changes", Command: "Show me what you changed"},
				Suggestion{Label: "Run tests", Command: "Run the tests"},
			)
		}
		if strings.Contains(lower, "error") || strings.Contains(lower, "failed") {
			suggestions = append(suggestions,
				Suggestion{Label: "Debug this", Command: "Help me debug this error"},
				Suggestion{Label: "Try again", Command: "Try a different approach"},
			)
		}
	}

	// After explanations, suggest deeper dive
	if strings.Contains(lower, "here's how") || strings.Contains(lower, "this works by") ||
		strings.Contains(lower, "explained") {
		suggestions = append(suggestions,
			Suggestion{Label: "Explain more", Command: "Can you explain that in more detail?"},
			Suggestion{Label: "Show example", Command: "Show me a concrete example"},
		)
	}

	// After research/analysis
	if strings.Contains(lower, "found") || strings.Contains(lower, "results") ||
		strings.Contains(lower, "searched") {
		suggestions = append(suggestions,
			Suggestion{Label: "Summarize", Command: "Summarize the key findings"},
			Suggestion{Label: "Save this", Command: "Save this to a file"},
		)
	}

	// After task completion
	if strings.Contains(lower, "done") || strings.Contains(lower, "complete") ||
		strings.Contains(lower, "finished") {
		suggestions = append(suggestions,
			Suggestion{Label: "What's next?", Command: "What should we work on next?"},
		)
	}

	// General suggestions when nothing specific matches
	if len(suggestions) == 0 {
		suggestions = append(suggestions,
			Suggestion{Label: "Tell me more", Command: "Can you elaborate?"},
			Suggestion{Label: "Help me with something else", Command: "I need help with something different"},
		)
	}

	// Limit to max 4 suggestions
	if len(suggestions) > 4 {
		suggestions = suggestions[:4]
	}

	return suggestions
}
