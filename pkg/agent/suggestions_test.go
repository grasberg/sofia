package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSuggestions_AfterCodeChange(t *testing.T) {
	suggestions := GenerateSuggestions("I've created the new handler file and wrote the tests.", true)

	require.NotEmpty(t, suggestions)
	labels := extractLabels(suggestions)
	assert.Contains(t, labels, "Review changes")
	assert.Contains(t, labels, "Run tests")
}

func TestGenerateSuggestions_AfterError(t *testing.T) {
	suggestions := GenerateSuggestions("The build failed with a compilation error.", true)

	require.NotEmpty(t, suggestions)
	labels := extractLabels(suggestions)
	assert.Contains(t, labels, "Debug this")
	assert.Contains(t, labels, "Try again")
}

func TestGenerateSuggestions_AfterExplanation(t *testing.T) {
	suggestions := GenerateSuggestions("Here's how the routing layer works in this codebase.", false)

	require.NotEmpty(t, suggestions)
	labels := extractLabels(suggestions)
	assert.Contains(t, labels, "Explain more")
	assert.Contains(t, labels, "Show example")
}

func TestGenerateSuggestions_AfterResearch(t *testing.T) {
	suggestions := GenerateSuggestions("I searched the codebase and found several results.", false)

	require.NotEmpty(t, suggestions)
	labels := extractLabels(suggestions)
	assert.Contains(t, labels, "Summarize")
	assert.Contains(t, labels, "Save this")
}

func TestGenerateSuggestions_Default(t *testing.T) {
	suggestions := GenerateSuggestions("Hello, how can I help you today?", false)

	require.Len(t, suggestions, 2)
	labels := extractLabels(suggestions)
	assert.Contains(t, labels, "Tell me more")
	assert.Contains(t, labels, "Help me with something else")
}

func TestGenerateSuggestions_MaxFour(t *testing.T) {
	// This response triggers code-change (created + wrote), error (failed), explanation
	// (here's how), and research (found + results) categories, producing many suggestions.
	response := "I created the file and wrote tests. The build failed. " +
		"Here's how the fix works. I found multiple results."
	suggestions := GenerateSuggestions(response, true)

	assert.LessOrEqual(t, len(suggestions), 4, "suggestions must be capped at 4")
}

// extractLabels returns only the Label field from each Suggestion.
func extractLabels(suggestions []Suggestion) []string {
	labels := make([]string, len(suggestions))
	for i, s := range suggestions {
		labels[i] = s.Label
	}
	return labels
}
