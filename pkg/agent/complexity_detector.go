package agent

import "strings"

// ComplexityLevel represents the detected complexity of a user message.
type ComplexityLevel int

const (
	ComplexitySimple     ComplexityLevel = iota // Simple question or lookup
	ComplexityModerate                          // Single-tool task
	ComplexityComplex                           // Multi-step task
	ComplexityAutonomous                        // Goal-driven workflow
)

// ModelTier indicates which model quality tier to use.
type ModelTier int

const (
	ModelTierPrimary  ModelTier = iota // Use the agent's default model
	ModelTierFallback                  // Use first fallback (cheaper/faster)
)

// ComplexityResult holds the detected complexity and recommended limits.
type ComplexityResult struct {
	Level         ComplexityLevel
	MaxIterations int
	ModelTier     ModelTier
}

// DetectComplexity analyzes a user message and returns the recommended complexity level.
// Uses lightweight heuristics (no LLM call) to avoid adding cost.
func DetectComplexity(message string) ComplexityResult {
	msg := strings.ToLower(message)
	wordCount := len(strings.Fields(message))
	newlineCount := strings.Count(message, "\n")
	hasCodeBlock := strings.Contains(message, "```")
	hasURL := strings.Contains(msg, "http://") || strings.Contains(msg, "https://")

	// Autonomous indicators — long, goal-oriented requests
	if containsAny(msg, autonomousKeywords) && wordCount > 20 {
		return ComplexityResult{Level: ComplexityAutonomous, MaxIterations: 30, ModelTier: ModelTierPrimary}
	}

	// Complex indicators — multi-step, multi-file tasks
	if containsAny(msg, complexKeywords) || hasCodeBlock {
		return ComplexityResult{Level: ComplexityComplex, MaxIterations: 20, ModelTier: ModelTierPrimary}
	}

	// Simple indicators — short questions, no code, no URLs
	if wordCount < 15 && newlineCount < 2 && !hasCodeBlock && !hasURL && containsAny(msg, simpleKeywords) {
		return ComplexityResult{Level: ComplexitySimple, MaxIterations: 5, ModelTier: ModelTierFallback}
	}

	return ComplexityResult{Level: ComplexityModerate, MaxIterations: 10, ModelTier: ModelTierPrimary}
}

var autonomousKeywords = []string{
	"goal:", "objective:", "build me a", "create a full",
	"develop a complete", "implement the entire", "end to end",
	"autonomous", "work on this until",
}

var complexKeywords = []string{
	"step by step", "multiple files", "refactor", "migrate",
	"across the codebase", "all instances", "redesign",
	"integrate with", "set up the", "configure and deploy",
}

var simpleKeywords = []string{
	"what is", "what's", "how do", "how does", "explain",
	"when did", "where is", "who is", "why does", "define",
	"show me", "list the",
}

func containsAny(s string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
