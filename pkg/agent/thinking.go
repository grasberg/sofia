package agent

// ThinkingLevel controls LLM reasoning depth per session.
type ThinkingLevel string

const (
	ThinkingOff     ThinkingLevel = "off"
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
)

var thinkingBudgets = map[ThinkingLevel]int{
	ThinkingOff:     0,
	ThinkingMinimal: 1024,
	ThinkingLow:     4096,
	ThinkingMedium:  10000,
	ThinkingHigh:    32000,
}

// IsValidThinkingLevel returns true if level is a recognized thinking level.
func IsValidThinkingLevel(level ThinkingLevel) bool {
	_, ok := thinkingBudgets[level]
	return ok
}

// ThinkingBudgetTokens returns the token budget for a thinking level.
func ThinkingBudgetTokens(level ThinkingLevel) int {
	return thinkingBudgets[level]
}
